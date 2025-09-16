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

	"github.com/loqalabs/loqa-hub/internal/skills"
)

// AsyncExecutionPipeline manages background execution of skills with status tracking
type AsyncExecutionPipeline struct {
	skillManager     SkillManagerInterface // Use interface for easier testing
	maxConcurrency   int
	executionTimeout time.Duration
	retryAttempts    int
	retryDelay       time.Duration

	// Execution tracking
	mu               sync.RWMutex
	activeExecutions map[string]*ExecutionContext
	executionMetrics *ExecutionMetrics

	// Worker pool
	workQueue    chan *ExecutionTask
	workers      sync.WaitGroup
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// SkillManagerInterface defines the interface for skills integration
type SkillManagerInterface interface {
	FindSkillForIntent(intent *skills.VoiceIntent) (skills.SkillPlugin, error)
	ExecuteSkill(ctx context.Context, skill skills.SkillPlugin, intent *skills.VoiceIntent) (*skills.SkillResponse, error)
}

// ExecutionTask represents a skill execution request
type ExecutionTask struct {
	ExecutionID    string
	Intent         *skills.VoiceIntent
	Classification *CommandClassification
	StatusChan     chan StatusUpdate
	Context        context.Context
	CancelFunc     context.CancelFunc
	StartTime      time.Time
	RetryCount     int
}

// ExecutionMetrics tracks pipeline performance
type ExecutionMetrics struct {
	mu                   sync.RWMutex
	TotalExecutions      int64         `json:"total_executions"`
	SuccessfulExecutions int64         `json:"successful_executions"`
	FailedExecutions     int64         `json:"failed_executions"`
	RetriedExecutions    int64         `json:"retried_executions"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	AverageQueueTime     time.Duration `json:"average_queue_time"`
	ConcurrentExecutions int64         `json:"concurrent_executions"`
	QueueDepth           int64         `json:"queue_depth"`
	LastExecutionTime    time.Time     `json:"last_execution_time"`
}

// AsyncExecutionResult represents the outcome of an async execution
type AsyncExecutionResult struct {
	ExecutionID   string                `json:"execution_id"`
	Success       bool                  `json:"success"`
	Response      *skills.SkillResponse `json:"response,omitempty"`
	Error         error                 `json:"error,omitempty"`
	ExecutionTime time.Duration         `json:"execution_time"`
	QueueTime     time.Duration         `json:"queue_time"`
	RetryCount    int                   `json:"retry_count"`
	Timestamp     time.Time             `json:"timestamp"`
}

// NewAsyncExecutionPipeline creates a new async execution pipeline
func NewAsyncExecutionPipeline(skillManager SkillManagerInterface) *AsyncExecutionPipeline {
	pipeline := &AsyncExecutionPipeline{
		skillManager:     skillManager,
		maxConcurrency:   5, // Allow 5 concurrent skill executions
		executionTimeout: 30 * time.Second,
		retryAttempts:    2, // Retry failed executions twice
		retryDelay:       1 * time.Second,

		activeExecutions: make(map[string]*ExecutionContext),
		executionMetrics: &ExecutionMetrics{},

		workQueue: make(chan *ExecutionTask, 50), // Buffer for 50 pending executions
		shutdown:  make(chan struct{}),
	}

	// Start worker pool
	pipeline.startWorkers()

	return pipeline
}

// SubmitExecution queues a skill execution task
func (aep *AsyncExecutionPipeline) SubmitExecution(ctx context.Context, executionID string, intent *skills.VoiceIntent, classification *CommandClassification, statusChan chan StatusUpdate) error {
	// Create execution context
	execCtx, cancelFunc := context.WithTimeout(ctx, aep.executionTimeout)

	task := &ExecutionTask{
		ExecutionID:    executionID,
		Intent:         intent,
		Classification: classification,
		StatusChan:     statusChan,
		Context:        execCtx,
		CancelFunc:     cancelFunc,
		StartTime:      time.Now(),
		RetryCount:     0,
	}

	// Store execution context
	execution := &ExecutionContext{
		ID:             executionID,
		Intent:         intent,
		StartTime:      time.Now(),
		StatusChan:     statusChan,
		CancelFunc:     cancelFunc,
		Classification: classification,
	}

	aep.mu.Lock()
	aep.activeExecutions[executionID] = execution
	aep.executionMetrics.QueueDepth++
	aep.mu.Unlock()

	// Submit to work queue
	select {
	case aep.workQueue <- task:
		log.Printf("⚡ Queued execution %s for intent: %s", executionID, intent.Intent)
		return nil
	case <-ctx.Done():
		// Clean up if context cancelled
		aep.cleanupExecution(executionID)
		return ctx.Err()
	default:
		// Queue is full
		aep.cleanupExecution(executionID)
		return fmt.Errorf("execution queue is full, cannot process request")
	}
}

// startWorkers initializes the worker pool for parallel execution
func (aep *AsyncExecutionPipeline) startWorkers() {
	for i := 0; i < aep.maxConcurrency; i++ {
		aep.workers.Add(1)
		go aep.worker(i)
	}
}

// worker processes execution tasks from the queue
func (aep *AsyncExecutionPipeline) worker(workerID int) {
	defer aep.workers.Done()

	log.Printf("🔧 Starting async execution worker %d", workerID)

	for {
		select {
		case <-aep.shutdown:
			log.Printf("🛑 Shutting down async execution worker %d", workerID)
			return

		case task := <-aep.workQueue:
			if task == nil {
				continue
			}

			// Update metrics
			aep.updateMetricsStart()

			// Process the execution
			result := aep.executeTask(task)

			// Update metrics
			aep.updateMetricsComplete(result)

			// Send status update
			aep.sendExecutionResult(task, result)

			// Cleanup
			aep.cleanupExecution(task.ExecutionID)
		}
	}
}

// executeTask performs the actual skill execution with retry logic
func (aep *AsyncExecutionPipeline) executeTask(task *ExecutionTask) *AsyncExecutionResult {
	queueTime := time.Since(task.StartTime)
	executionStart := time.Now()

	result := &AsyncExecutionResult{
		ExecutionID: task.ExecutionID,
		QueueTime:   queueTime,
		RetryCount:  task.RetryCount,
		Timestamp:   time.Now(),
	}

	// Find appropriate skill
	skill, err := aep.skillManager.FindSkillForIntent(task.Intent)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("no skill found for intent: %w", err)
		result.ExecutionTime = time.Since(executionStart)
		return result
	}

	// Execute skill with timeout protection
	skillResponse, err := aep.executeWithRetry(task, skill)
	result.ExecutionTime = time.Since(executionStart)

	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = skillResponse.Success
		result.Response = skillResponse
		if !skillResponse.Success {
			result.Error = fmt.Errorf("skill execution failed: %s", skillResponse.Error)
		}
	}

	return result
}

// executeWithRetry attempts skill execution with retry logic
func (aep *AsyncExecutionPipeline) executeWithRetry(task *ExecutionTask, skill skills.SkillPlugin) (*skills.SkillResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= aep.retryAttempts; attempt++ {
		if attempt > 0 {
			// Update retry count
			task.RetryCount = attempt
			aep.updateRetryMetrics()

			// Wait before retry
			select {
			case <-time.After(aep.retryDelay * time.Duration(attempt)):
			case <-task.Context.Done():
				return nil, task.Context.Err()
			}

			log.Printf("🔄 Retrying execution %s, attempt %d/%d", task.ExecutionID, attempt+1, aep.retryAttempts+1)
		}

		// Execute skill
		response, err := aep.skillManager.ExecuteSkill(task.Context, skill, task.Intent)
		if err == nil && response.Success {
			return response, nil
		}

		lastErr = err
		if response != nil && !response.Success {
			lastErr = fmt.Errorf("skill execution failed: %s", response.Error)
		}

		// Don't retry context cancellation
		if task.Context.Err() != nil {
			return nil, task.Context.Err()
		}

		// Send progress update for retries
		if attempt < aep.retryAttempts {
			aep.sendProgressUpdate(task, fmt.Sprintf("Retrying operation (attempt %d)", attempt+2))
		}
	}

	return nil, fmt.Errorf("execution failed after %d retries: %w", aep.retryAttempts, lastErr)
}

// sendExecutionResult sends the final status update based on execution result
func (aep *AsyncExecutionPipeline) sendExecutionResult(task *ExecutionTask, result *AsyncExecutionResult) {
	var statusUpdate StatusUpdate

	if result.Success {
		// Success handling based on update strategy
		updateStrategy := task.Classification.UpdateStrategy

		switch updateStrategy {
		case UpdateSilent:
			// No update for successful routine operations
			return
		case UpdateErrorOnly:
			// No update for successful operations
			return
		case UpdateVerbose, UpdateProgress:
			statusUpdate = StatusUpdate{
				Type:        StatusSuccess,
				Message:     aep.generateSuccessMessage(task, result.Response),
				Success:     true,
				ExecutionID: task.ExecutionID,
				Timestamp:   time.Now(),
			}
		}
	} else {
		// Always send error updates
		statusUpdate = StatusUpdate{
			Type:        StatusError,
			Message:     aep.generateErrorMessage(task, result.Error),
			Success:     false,
			ExecutionID: task.ExecutionID,
			Timestamp:   time.Now(),
			Error:       result.Error.Error(),
		}
	}

	// Send status update
	select {
	case task.StatusChan <- statusUpdate:
		log.Printf("📤 Sent status update for execution %s: %s", task.ExecutionID, statusUpdate.Message)
	default:
		log.Printf("⚠️ Failed to send status update for execution %s: channel full or closed", task.ExecutionID)
	}
}

// sendProgressUpdate sends an intermediate progress update
func (aep *AsyncExecutionPipeline) sendProgressUpdate(task *ExecutionTask, message string) {
	statusUpdate := StatusUpdate{
		Type:        StatusProgress,
		Message:     message,
		Success:     false, // Not final result
		ExecutionID: task.ExecutionID,
		Timestamp:   time.Now(),
	}

	select {
	case task.StatusChan <- statusUpdate:
	default:
		// Don't block on progress updates
	}
}

// generateSuccessMessage creates user-friendly success messages
func (aep *AsyncExecutionPipeline) generateSuccessMessage(task *ExecutionTask, response *skills.SkillResponse) string {
	if response.SpeechText != "" {
		return response.SpeechText
	}
	if response.Message != "" {
		return response.Message
	}

	// Fallback to generic success message based on intent
	intent := task.Intent.Intent
	if entities := task.Intent.Entities; len(entities) > 0 {
		if action, ok := entities["action"]; ok {
			if location, ok := entities["location"]; ok {
				return fmt.Sprintf("%s %s completed successfully", action, location)
			}
			return fmt.Sprintf("%s completed successfully", action)
		}
	}

	return fmt.Sprintf("%s operation completed", intent)
}

// generateErrorMessage creates user-friendly error messages
func (aep *AsyncExecutionPipeline) generateErrorMessage(task *ExecutionTask, err error) string {
	// Extract device/location info for personalized error messages
	entities := task.Intent.Entities
	if location, ok := entities["location"]; ok {
		if device, ok := entities["device"]; ok {
			return fmt.Sprintf("Sorry, I couldn't reach the %s %s", location, device)
		}
		return fmt.Sprintf("Sorry, I couldn't reach the %s device", location)
	}

	// Fallback to generic error message
	return "Sorry, I couldn't complete that operation"
}

// updateMetricsStart updates metrics when execution starts
func (aep *AsyncExecutionPipeline) updateMetricsStart() {
	aep.executionMetrics.mu.Lock()
	defer aep.executionMetrics.mu.Unlock()

	aep.executionMetrics.TotalExecutions++
	aep.executionMetrics.ConcurrentExecutions++
	aep.executionMetrics.QueueDepth--
	aep.executionMetrics.LastExecutionTime = time.Now()
}

// updateMetricsComplete updates metrics when execution completes
func (aep *AsyncExecutionPipeline) updateMetricsComplete(result *AsyncExecutionResult) {
	aep.executionMetrics.mu.Lock()
	defer aep.executionMetrics.mu.Unlock()

	aep.executionMetrics.ConcurrentExecutions--

	if result.Success {
		aep.executionMetrics.SuccessfulExecutions++
	} else {
		aep.executionMetrics.FailedExecutions++
	}

	// Update average execution time
	if aep.executionMetrics.TotalExecutions == 1 {
		aep.executionMetrics.AverageExecutionTime = result.ExecutionTime
		aep.executionMetrics.AverageQueueTime = result.QueueTime
	} else {
		aep.executionMetrics.AverageExecutionTime =
			(aep.executionMetrics.AverageExecutionTime + result.ExecutionTime) / 2
		aep.executionMetrics.AverageQueueTime =
			(aep.executionMetrics.AverageQueueTime + result.QueueTime) / 2
	}
}

// updateRetryMetrics updates retry-specific metrics
func (aep *AsyncExecutionPipeline) updateRetryMetrics() {
	aep.executionMetrics.mu.Lock()
	defer aep.executionMetrics.mu.Unlock()
	aep.executionMetrics.RetriedExecutions++
}

// cleanupExecution removes execution from tracking
func (aep *AsyncExecutionPipeline) cleanupExecution(executionID string) {
	aep.mu.Lock()
	defer aep.mu.Unlock()

	if execution, exists := aep.activeExecutions[executionID]; exists {
		execution.CancelFunc()
		delete(aep.activeExecutions, executionID)
	}
}

// GetMetrics returns current execution metrics
func (aep *AsyncExecutionPipeline) GetMetrics() *ExecutionMetrics {
	aep.executionMetrics.mu.RLock()
	defer aep.executionMetrics.mu.RUnlock()

	// Return a copy to prevent external mutation
	return &ExecutionMetrics{
		TotalExecutions:      aep.executionMetrics.TotalExecutions,
		SuccessfulExecutions: aep.executionMetrics.SuccessfulExecutions,
		FailedExecutions:     aep.executionMetrics.FailedExecutions,
		RetriedExecutions:    aep.executionMetrics.RetriedExecutions,
		AverageExecutionTime: aep.executionMetrics.AverageExecutionTime,
		AverageQueueTime:     aep.executionMetrics.AverageQueueTime,
		ConcurrentExecutions: aep.executionMetrics.ConcurrentExecutions,
		QueueDepth:           aep.executionMetrics.QueueDepth,
		LastExecutionTime:    aep.executionMetrics.LastExecutionTime,
	}
}

// GetActiveExecutions returns information about currently running executions
func (aep *AsyncExecutionPipeline) GetActiveExecutions() map[string]*ExecutionContext {
	aep.mu.RLock()
	defer aep.mu.RUnlock()

	executions := make(map[string]*ExecutionContext)
	for k, v := range aep.activeExecutions {
		executions[k] = v
	}
	return executions
}

// Shutdown gracefully stops the execution pipeline
func (aep *AsyncExecutionPipeline) Shutdown(ctx context.Context) error {
	aep.shutdownOnce.Do(func() {
		log.Printf("🛑 Shutting down async execution pipeline...")

		// Signal shutdown to workers
		close(aep.shutdown)

		// Close work queue
		close(aep.workQueue)

		// Cancel all active executions
		aep.mu.Lock()
		for _, execution := range aep.activeExecutions {
			execution.CancelFunc()
		}
		aep.mu.Unlock()

		// Wait for workers to finish with timeout
		done := make(chan struct{})
		go func() {
			aep.workers.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Printf("✅ Async execution pipeline shutdown complete")
		case <-ctx.Done():
			log.Printf("⚠️ Async execution pipeline shutdown timed out")
		}
	})

	return nil
}
