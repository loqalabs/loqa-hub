/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package llm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CommandQueue manages sequential execution of multiple commands
type CommandQueue struct {
	items           []*CommandQueueItem
	mutex           sync.RWMutex
	maxDuration     time.Duration
	rollbackEnabled bool
}

// CommandExecutor defines the interface for executing individual commands
type CommandExecutor interface {
	ExecuteCommand(ctx context.Context, cmd *Command) error
}

// ExecutionResult contains the results of command queue execution
type ExecutionResult struct {
	Success          bool                `json:"success"`
	CompletedItems   []*CommandQueueItem `json:"completed_items"`
	FailedItem       *CommandQueueItem   `json:"failed_item,omitempty"`
	TotalDuration    time.Duration       `json:"total_duration"`
	RollbackOccurred bool                `json:"rollback_occurred"`
	CombinedResponse string              `json:"combined_response"`
	Responses        []string            `json:"responses"`
	Error            error               `json:"-"`
}

// NewCommandQueue creates a new command queue
func NewCommandQueue(commands []Command, maxDuration time.Duration, rollbackEnabled bool) *CommandQueue {
	items := make([]*CommandQueueItem, len(commands))
	for i, cmd := range commands {
		items[i] = &CommandQueueItem{
			Command:   &cmd,
			Index:     i,
			Executed:  false,
			Success:   false,
			StartTime: time.Time{},
			EndTime:   time.Time{},
			Duration:  0,
		}
	}

	return &CommandQueue{
		items:           items,
		maxDuration:     maxDuration,
		rollbackEnabled: rollbackEnabled,
	}
}

// Execute runs all commands in the queue sequentially
func (cq *CommandQueue) Execute(ctx context.Context, executor CommandExecutor) (*ExecutionResult, error) {
	cq.mutex.Lock()
	defer cq.mutex.Unlock()

	startTime := time.Now()
	result := &ExecutionResult{
		Success:          true,
		CompletedItems:   make([]*CommandQueueItem, 0),
		TotalDuration:    0,
		RollbackOccurred: false,
		Responses:        make([]string, 0),
	}

	// Create context with timeout if specified
	execCtx := ctx
	if cq.maxDuration > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, cq.maxDuration)
		defer cancel()
	}

	log.Printf("🔄 Executing command queue with %d commands", len(cq.items))

	for i, item := range cq.items {
		// Check context cancellation
		select {
		case <-execCtx.Done():
			result.Success = false
			result.Error = fmt.Errorf("command queue execution timeout or cancelled at command %d", i)
			log.Printf("❌ Command queue execution cancelled at command %d", i)
			return result, result.Error
		default:
		}

		// Record start time
		item.StartTime = time.Now()

		log.Printf("🎯 Executing command %d/%d: %s", i+1, len(cq.items), item.Command.Intent)

		// Execute the command
		err := executor.ExecuteCommand(execCtx, item.Command)

		// Record end time and duration
		item.EndTime = time.Now()
		item.Duration = item.EndTime.Sub(item.StartTime)
		item.Executed = true

		if err != nil {
			item.Success = false
			item.Error = err
			result.Success = false
			result.FailedItem = item

			log.Printf("❌ Command %d failed: %v", i+1, err)

			// If rollback is enabled, rollback previously executed commands
			if cq.rollbackEnabled && len(result.CompletedItems) > 0 {
				log.Printf("🔄 Rolling back %d previously executed commands", len(result.CompletedItems))
				rollbackErr := cq.rollbackCommands(execCtx, executor, result.CompletedItems)
				if rollbackErr != nil {
					log.Printf("❌ Rollback failed: %v", rollbackErr)
				} else {
					result.RollbackOccurred = true
					log.Printf("✅ Rollback completed successfully")
				}
			}

			break
		}

		// Command executed successfully
		item.Success = true
		result.CompletedItems = append(result.CompletedItems, item)
		result.Responses = append(result.Responses, item.Command.Response)

		log.Printf("✅ Command %d completed successfully in %v", i+1, item.Duration)

		// Check if we're approaching the performance target
		if item.Duration > 200*time.Millisecond {
			log.Printf("⚠️ Command %d took longer than 200ms: %v", i+1, item.Duration)
		}
	}

	result.TotalDuration = time.Since(startTime)
	result.CombinedResponse = cq.combinedResponse(result.Responses)

	if result.Success {
		log.Printf("✅ Command queue completed successfully in %v", result.TotalDuration)
	} else {
		log.Printf("❌ Command queue failed after %v", result.TotalDuration)
	}

	return result, nil
}

// rollbackCommands attempts to rollback previously executed commands
func (cq *CommandQueue) rollbackCommands(ctx context.Context, executor CommandExecutor, completedItems []*CommandQueueItem) error {
	// Attempt rollback in reverse order
	for i := len(completedItems) - 1; i >= 0; i-- {
		item := completedItems[i]
		rollbackCmd := cq.createRollbackCommand(item.Command)
		if rollbackCmd != nil {
			log.Printf("🔄 Rolling back command: %s", item.Command.Intent)
			err := executor.ExecuteCommand(ctx, rollbackCmd)
			if err != nil {
				log.Printf("❌ Failed to rollback command %d: %v", item.Index, err)
				// Continue with other rollbacks even if one fails
			}
		}
	}
	return nil
}

// createRollbackCommand creates a rollback command for a given command
func (cq *CommandQueue) createRollbackCommand(cmd *Command) *Command {
	// Create opposite commands where possible
	switch cmd.Intent {
	case "turn_on":
		return &Command{
			Intent:     "turn_off",
			Entities:   cmd.Entities,
			Confidence: cmd.Confidence,
			Response:   fmt.Sprintf("Rolling back: turning off %s", cmd.Entities["device"]),
		}
	case "turn_off":
		return &Command{
			Intent:     "turn_on",
			Entities:   cmd.Entities,
			Confidence: cmd.Confidence,
			Response:   fmt.Sprintf("Rolling back: turning on %s", cmd.Entities["device"]),
		}
	default:
		// No rollback available for this command type
		log.Printf("ℹ️ No rollback available for command: %s", cmd.Intent)
		return nil
	}
}

// combinedResponse creates a single response from multiple command responses
func (cq *CommandQueue) combinedResponse(responses []string) string {
	if len(responses) == 0 {
		return "No commands were executed."
	}

	if len(responses) == 1 {
		return responses[0]
	}

	// For multiple responses, create a combined message
	return fmt.Sprintf("I've completed %d commands for you.", len(responses))
}

// GetStatus returns the current status of the queue
func (cq *CommandQueue) GetStatus() []CommandQueueItem {
	cq.mutex.RLock()
	defer cq.mutex.RUnlock()

	status := make([]CommandQueueItem, len(cq.items))
	for i, item := range cq.items {
		status[i] = *item
	}
	return status
}

// Size returns the number of commands in the queue
func (cq *CommandQueue) Size() int {
	cq.mutex.RLock()
	defer cq.mutex.RUnlock()
	return len(cq.items)
}

// IsEmpty returns true if the queue has no commands
func (cq *CommandQueue) IsEmpty() bool {
	return cq.Size() == 0
}
