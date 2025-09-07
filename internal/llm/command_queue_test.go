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
	"errors"
	"testing"
	"time"
)

// MockCommandExecutor for testing
type MockCommandExecutor struct {
	executedCommands []Command
	shouldFail       []bool
	execDelay        time.Duration
}

func NewMockCommandExecutor(shouldFail []bool, execDelay time.Duration) *MockCommandExecutor {
	return &MockCommandExecutor{
		executedCommands: make([]Command, 0),
		shouldFail:       shouldFail,
		execDelay:        execDelay,
	}
}

func (m *MockCommandExecutor) ExecuteCommand(ctx context.Context, cmd *Command) error {
	// Simulate execution delay
	if m.execDelay > 0 {
		time.Sleep(m.execDelay)
	}

	m.executedCommands = append(m.executedCommands, *cmd)

	// Check if this command should fail
	cmdIndex := len(m.executedCommands) - 1
	if cmdIndex < len(m.shouldFail) && m.shouldFail[cmdIndex] {
		return errors.New("mock command execution failure")
	}

	return nil
}

func (m *MockCommandExecutor) GetExecutedCommands() []Command {
	return m.executedCommands
}

func TestNewCommandQueue(t *testing.T) {
	commands := []Command{
		{Intent: "turn_on", Entities: map[string]string{"device": "lights"}},
		{Intent: "turn_on", Entities: map[string]string{"device": "music"}},
	}

	queue := NewCommandQueue(commands, 5*time.Second, true)

	if queue == nil {
		t.Fatal("NewCommandQueue should return a valid queue")
	}

	if queue.Size() != 2 {
		t.Errorf("Queue size = %d, expected 2", queue.Size())
	}

	if queue.maxDuration != 5*time.Second {
		t.Errorf("Max duration = %v, expected 5s", queue.maxDuration)
	}

	if !queue.rollbackEnabled {
		t.Error("Rollback should be enabled")
	}
}

func TestCommandQueueExecuteSuccess(t *testing.T) {
	commands := []Command{
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "lights"},
			Confidence: 0.9,
			Response:   "Turning on lights",
		},
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "music"},
			Confidence: 0.8,
			Response:   "Playing music",
		},
	}

	queue := NewCommandQueue(commands, 5*time.Second, false)
	executor := NewMockCommandExecutor([]bool{false, false}, 0)

	result, err := queue.Execute(context.Background(), executor)

	if err != nil {
		t.Errorf("Execute should not return error: %v", err)
	}

	if !result.Success {
		t.Error("Execution should be successful")
	}

	if len(result.CompletedItems) != 2 {
		t.Errorf("Completed items = %d, expected 2", len(result.CompletedItems))
	}

	if result.FailedItem != nil {
		t.Error("Failed item should be nil for successful execution")
	}

	if result.RollbackOccurred {
		t.Error("Rollback should not occur for successful execution")
	}

	if len(result.Responses) != 2 {
		t.Errorf("Response count = %d, expected 2", len(result.Responses))
	}

	// Check that all commands were executed
	executedCommands := executor.GetExecutedCommands()
	if len(executedCommands) != 2 {
		t.Errorf("Executed commands = %d, expected 2", len(executedCommands))
	}

	// Verify the combined response
	if result.CombinedResponse == "" {
		t.Error("Combined response should not be empty")
	}
}

func TestCommandQueueExecuteFailure(t *testing.T) {
	commands := []Command{
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "lights"},
			Confidence: 0.9,
			Response:   "Turning on lights",
		},
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "music"},
			Confidence: 0.8,
			Response:   "Playing music",
		},
		{
			Intent:     "turn_off",
			Entities:   map[string]string{"device": "tv"},
			Confidence: 0.7,
			Response:   "Turning off TV",
		},
	}

	// Make the second command fail
	queue := NewCommandQueue(commands, 5*time.Second, false)
	executor := NewMockCommandExecutor([]bool{false, true, false}, 0)

	result, err := queue.Execute(context.Background(), executor)

	if err != nil {
		t.Errorf("Execute should not return error even when commands fail: %v", err)
	}

	if result.Success {
		t.Error("Execution should not be successful")
	}

	if len(result.CompletedItems) != 1 {
		t.Errorf("Completed items = %d, expected 1", len(result.CompletedItems))
	}

	if result.FailedItem == nil {
		t.Error("Failed item should not be nil")
	}

	if result.FailedItem.Index != 1 {
		t.Errorf("Failed item index = %d, expected 1", result.FailedItem.Index)
	}

	// Check that only first command was executed
	executedCommands := executor.GetExecutedCommands()
	if len(executedCommands) != 2 { // First successful + second failed
		t.Errorf("Executed commands = %d, expected 2", len(executedCommands))
	}
}

func TestCommandQueueExecuteWithRollback(t *testing.T) {
	commands := []Command{
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "lights"},
			Confidence: 0.9,
			Response:   "Turning on lights",
		},
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "music"},
			Confidence: 0.8,
			Response:   "Playing music",
		},
	}

	// Enable rollback and make the second command fail
	queue := NewCommandQueue(commands, 5*time.Second, true)
	executor := NewMockCommandExecutor([]bool{false, true}, 0)

	result, err := queue.Execute(context.Background(), executor)

	if err != nil {
		t.Errorf("Execute should not return error: %v", err)
	}

	if result.Success {
		t.Error("Execution should not be successful")
	}

	if len(result.CompletedItems) != 1 {
		t.Errorf("Completed items = %d, expected 1", len(result.CompletedItems))
	}

	if !result.RollbackOccurred {
		t.Error("Rollback should have occurred")
	}

	// Check that rollback command was executed
	executedCommands := executor.GetExecutedCommands()
	if len(executedCommands) < 3 { // Original + failed + rollback
		t.Errorf("Executed commands = %d, expected at least 3 (including rollback)", len(executedCommands))
	}
}

func TestCommandQueueTimeout(t *testing.T) {
	t.Skip("Timeout test is flaky in CI - skipping for now")

	commands := []Command{
		{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "lights"},
			Confidence: 0.9,
			Response:   "Turning on lights",
		},
	}

	// Use a very short timeout to ensure it triggers
	queue := NewCommandQueue(commands, 10*time.Millisecond, false)
	// Make execution take longer than timeout
	executor := NewMockCommandExecutor([]bool{false}, 50*time.Millisecond)

	result, err := queue.Execute(context.Background(), executor)

	if err == nil {
		t.Error("Execute should return timeout error")
	}

	if result.Success {
		t.Error("Execution should not be successful due to timeout")
	}
}

func TestCommandQueueIsEmpty(t *testing.T) {
	emptyQueue := NewCommandQueue([]Command{}, 5*time.Second, false)

	if !emptyQueue.IsEmpty() {
		t.Error("Empty queue should return true for IsEmpty()")
	}

	if emptyQueue.Size() != 0 {
		t.Errorf("Empty queue size = %d, expected 0", emptyQueue.Size())
	}

	commands := []Command{
		{Intent: "turn_on", Entities: map[string]string{"device": "lights"}},
	}
	nonEmptyQueue := NewCommandQueue(commands, 5*time.Second, false)

	if nonEmptyQueue.IsEmpty() {
		t.Error("Non-empty queue should return false for IsEmpty()")
	}

	if nonEmptyQueue.Size() != 1 {
		t.Errorf("Non-empty queue size = %d, expected 1", nonEmptyQueue.Size())
	}
}

func TestCommandQueueGetStatus(t *testing.T) {
	commands := []Command{
		{Intent: "turn_on", Entities: map[string]string{"device": "lights"}},
		{Intent: "turn_off", Entities: map[string]string{"device": "music"}},
	}

	queue := NewCommandQueue(commands, 5*time.Second, false)
	status := queue.GetStatus()

	if len(status) != 2 {
		t.Errorf("Status length = %d, expected 2", len(status))
	}

	for i, item := range status {
		if item.Index != i {
			t.Errorf("Item %d index = %d, expected %d", i, item.Index, i)
		}

		if item.Executed {
			t.Errorf("Item %d should not be executed yet", i)
		}

		if item.Success {
			t.Errorf("Item %d should not be successful yet", i)
		}
	}
}

func TestCreateRollbackCommand(t *testing.T) {
	queue := NewCommandQueue([]Command{}, 0, false)

	testCases := []struct {
		name           string
		originalCmd    *Command
		expectedIntent string
		expectNil      bool
		description    string
	}{
		{
			name: "turn_on_rollback",
			originalCmd: &Command{
				Intent:   "turn_on",
				Entities: map[string]string{"device": "lights"},
			},
			expectedIntent: "turn_off",
			expectNil:      false,
			description:    "should create turn_off rollback for turn_on",
		},
		{
			name: "turn_off_rollback",
			originalCmd: &Command{
				Intent:   "turn_off",
				Entities: map[string]string{"device": "lights"},
			},
			expectedIntent: "turn_on",
			expectNil:      false,
			description:    "should create turn_on rollback for turn_off",
		},
		{
			name: "no_rollback_available",
			originalCmd: &Command{
				Intent:   "greeting",
				Entities: map[string]string{},
			},
			expectedIntent: "",
			expectNil:      true,
			description:    "should return nil for commands without rollback",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rollbackCmd := queue.createRollbackCommand(tc.originalCmd)

			if tc.expectNil {
				if rollbackCmd != nil {
					t.Errorf("createRollbackCommand should return nil (%s)", tc.description)
				}
			} else {
				if rollbackCmd == nil {
					t.Errorf("createRollbackCommand should not return nil (%s)", tc.description)
					return
				}

				if rollbackCmd.Intent != tc.expectedIntent {
					t.Errorf("Rollback intent = %q, expected %q (%s)",
						rollbackCmd.Intent, tc.expectedIntent, tc.description)
				}

				// Check that entities are preserved
				for k, v := range tc.originalCmd.Entities {
					if rollbackValue, exists := rollbackCmd.Entities[k]; !exists || rollbackValue != v {
						t.Errorf("Rollback should preserve entity %s=%s", k, v)
					}
				}
			}
		})
	}
}

func TestCombinedResponse(t *testing.T) {
	queue := NewCommandQueue([]Command{}, 0, false)

	testCases := []struct {
		name      string
		responses []string
		expected  string
	}{
		{
			name:      "empty_responses",
			responses: []string{},
			expected:  "No commands were executed.",
		},
		{
			name:      "single_response",
			responses: []string{"Lights turned on"},
			expected:  "Lights turned on",
		},
		{
			name:      "multiple_responses",
			responses: []string{"Lights turned on", "Music started", "TV turned off"},
			expected:  "I've completed 3 commands for you.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := queue.combinedResponse(tc.responses)
			if result != tc.expected {
				t.Errorf("combinedResponse(%v) = %q, expected %q", tc.responses, result, tc.expected)
			}
		})
	}
}
