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
	"strings"
	"sync"
	"time"
)

// StatusManager handles intelligent status updates and error recovery
type StatusManager struct {
	// TTS integration for audio responses
	ttsClient TextToSpeech

	// Status tracking
	mu            sync.RWMutex
	activeUpdates map[string]*StatusContext
	updateHistory map[string][]*StatusUpdate
	errorPatterns map[string]*ErrorPattern

	// Configuration
	maxHistorySize     int
	errorCooldown      time.Duration
	updateTimeout      time.Duration
	enableAudioUpdates bool

	// Metrics
	metrics *StatusMetrics
}

// StatusContext tracks the context for status updates
type StatusContext struct {
	ExecutionID    string
	UpdateStrategy UpdateStrategy
	DeviceID       string
	LastUpdate     time.Time
	ErrorCount     int
	SuccessCount   int
	UpdateChan     chan StatusUpdate
	CancelFunc     context.CancelFunc
	AudioEnabled   bool
	Priority       UpdatePriority
}

// UpdatePriority determines the importance of status updates
type UpdatePriority string

const (
	PriorityLow      UpdatePriority = "low"      // Routine operations
	PriorityNormal   UpdatePriority = "normal"   // Standard operations
	PriorityHigh     UpdatePriority = "high"     // Important operations
	PriorityCritical UpdatePriority = "critical" // Security/safety operations
)

// ErrorPattern tracks recurring error patterns for better recovery
type ErrorPattern struct {
	DeviceID        string
	ErrorType       string
	OccurrenceCount int
	LastOccurrence  time.Time
	RecoveryActions []RecoveryAction
	Resolved        bool
}

// RecoveryAction defines automated recovery strategies
type RecoveryAction struct {
	Type        RecoveryType  `json:"type"`
	Description string        `json:"description"`
	Delay       time.Duration `json:"delay"`
	MaxRetries  int           `json:"max_retries"`
	Success     bool          `json:"success"`
}

// RecoveryType defines types of automated recovery
type RecoveryType string

const (
	RecoveryRetry    RecoveryType = "retry"    // Simple retry operation
	RecoveryReset    RecoveryType = "reset"    // Reset device connection
	RecoveryFallback RecoveryType = "fallback" // Use alternative method
	RecoverySkip     RecoveryType = "skip"     // Skip and notify
	RecoveryEscalate RecoveryType = "escalate" // Escalate to user attention
)

// StatusMetrics tracks status update performance
type StatusMetrics struct {
	mu                   sync.RWMutex
	TotalUpdates         int64         `json:"total_updates"`
	SuccessfulUpdates    int64         `json:"successful_updates"`
	FailedUpdates        int64         `json:"failed_updates"`
	AudioUpdates         int64         `json:"audio_updates"`
	SilentUpdates        int64         `json:"silent_updates"`
	ErrorRecoveries      int64         `json:"error_recoveries"`
	AverageUpdateLatency time.Duration `json:"average_update_latency"`
	ErrorPatternDetected int64         `json:"error_patterns_detected"`
	RecoverySuccessRate  float64       `json:"recovery_success_rate"`
}

// NewStatusManager creates a new intelligent status manager
func NewStatusManager(ttsClient TextToSpeech) *StatusManager {
	return &StatusManager{
		ttsClient:     ttsClient,
		activeUpdates: make(map[string]*StatusContext),
		updateHistory: make(map[string][]*StatusUpdate),
		errorPatterns: make(map[string]*ErrorPattern),

		maxHistorySize:     50, // Keep 50 updates per execution
		errorCooldown:      5 * time.Minute,
		updateTimeout:      10 * time.Second,
		enableAudioUpdates: true,

		metrics: &StatusMetrics{},
	}
}

// RegisterExecution sets up status tracking for a new execution
func (sm *StatusManager) RegisterExecution(executionID string, strategy UpdateStrategy, deviceID string, updateChan chan StatusUpdate) {
	_, cancel := context.WithCancel(context.Background())

	statusCtx := &StatusContext{
		ExecutionID:    executionID,
		UpdateStrategy: strategy,
		DeviceID:       deviceID,
		LastUpdate:     time.Now(),
		UpdateChan:     updateChan,
		CancelFunc:     cancel,
		AudioEnabled:   sm.enableAudioUpdates,
		Priority:       sm.determinePriority(strategy),
	}

	sm.mu.Lock()
	sm.activeUpdates[executionID] = statusCtx
	sm.updateHistory[executionID] = make([]*StatusUpdate, 0)
	sm.mu.Unlock()

	log.Printf("📊 Registered status tracking for execution %s with strategy %s", executionID, strategy)
}

// ProcessStatusUpdate handles incoming status updates with intelligent routing
func (sm *StatusManager) ProcessStatusUpdate(update StatusUpdate) error {
	startTime := time.Now()

	sm.mu.RLock()
	statusCtx, exists := sm.activeUpdates[update.ExecutionID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active status context for execution %s", update.ExecutionID)
	}

	// Apply intelligent filtering based on update strategy
	if !sm.shouldSendUpdate(statusCtx, &update) {
		sm.updateMetrics(true, time.Since(startTime), true) // Silent update
		return nil
	}

	// Process error patterns and recovery
	if update.Type == StatusError {
		sm.handleErrorPattern(statusCtx, &update)
	}

	// Enhance update message with context
	sm.enhanceUpdateMessage(statusCtx, &update)

	// Send update through appropriate channels
	err := sm.sendUpdate(statusCtx, &update)

	// Record in history
	sm.recordUpdate(update.ExecutionID, &update)

	// Update metrics
	sm.updateMetrics(err == nil, time.Since(startTime), false)

	return err
}

// shouldSendUpdate determines if an update should be sent based on strategy and context
func (sm *StatusManager) shouldSendUpdate(statusCtx *StatusContext, update *StatusUpdate) bool {
	strategy := statusCtx.UpdateStrategy

	switch strategy {
	case UpdateSilent:
		// Only send critical errors
		return update.Type == StatusError && statusCtx.Priority == PriorityCritical

	case UpdateErrorOnly:
		// Send errors and critical successes
		return update.Type == StatusError ||
			(update.Type == StatusSuccess && statusCtx.Priority == PriorityCritical)

	case UpdateVerbose:
		// Send all updates
		return true

	case UpdateProgress:
		// Send progress, errors, and final results
		return update.Type == StatusProgress ||
			update.Type == StatusError ||
			update.Type == StatusSuccess

	default:
		// Default to error-only
		return update.Type == StatusError
	}
}

// handleErrorPattern detects and handles recurring error patterns
func (sm *StatusManager) handleErrorPattern(statusCtx *StatusContext, update *StatusUpdate) {
	deviceID := statusCtx.DeviceID
	errorType := sm.categorizeError(update.Error)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	patternKey := fmt.Sprintf("%s_%s", deviceID, errorType)
	pattern, exists := sm.errorPatterns[patternKey]

	if !exists {
		pattern = &ErrorPattern{
			DeviceID:        deviceID,
			ErrorType:       errorType,
			OccurrenceCount: 1,
			LastOccurrence:  time.Now(),
			RecoveryActions: []RecoveryAction{},
		}
		sm.errorPatterns[patternKey] = pattern
	} else {
		pattern.OccurrenceCount++
		pattern.LastOccurrence = time.Now()
	}

	// Trigger recovery if pattern is detected
	if pattern.OccurrenceCount >= 3 && !pattern.Resolved {
		go sm.attemptErrorRecovery(statusCtx, pattern, update)
		sm.metrics.mu.Lock()
		sm.metrics.ErrorPatternDetected++
		sm.metrics.mu.Unlock()
	}
}

// categorizeError classifies errors into categories for pattern detection
func (sm *StatusManager) categorizeError(errorMsg string) string {
	if errorMsg == "" {
		return "unknown"
	}

	errorLower := strings.ToLower(errorMsg)

	switch {
	case strings.Contains(errorLower, "timeout") || strings.Contains(errorLower, "no response"):
		return "timeout"
	case strings.Contains(errorLower, "connection") || strings.Contains(errorLower, "network"):
		return "connection"
	case strings.Contains(errorLower, "permission") || strings.Contains(errorLower, "unauthorized"):
		return "permission"
	case strings.Contains(errorLower, "not found") || strings.Contains(errorLower, "unavailable"):
		return "unavailable"
	case strings.Contains(errorLower, "invalid") || strings.Contains(errorLower, "bad request"):
		return "invalid_request"
	default:
		return "generic"
	}
}

// attemptErrorRecovery tries automated recovery for recurring errors
func (sm *StatusManager) attemptErrorRecovery(statusCtx *StatusContext, pattern *ErrorPattern, update *StatusUpdate) {
	log.Printf("🔧 Attempting error recovery for pattern %s_%s (occurrence #%d)",
		pattern.DeviceID, pattern.ErrorType, pattern.OccurrenceCount)

	var recoveryAction RecoveryAction

	// Select recovery strategy based on error type
	switch pattern.ErrorType {
	case "timeout":
		recoveryAction = RecoveryAction{
			Type:        RecoveryRetry,
			Description: "Retrying with extended timeout",
			Delay:       3 * time.Second,
			MaxRetries:  2,
		}

	case "connection":
		recoveryAction = RecoveryAction{
			Type:        RecoveryReset,
			Description: "Resetting device connection",
			Delay:       5 * time.Second,
			MaxRetries:  1,
		}

	case "unavailable":
		recoveryAction = RecoveryAction{
			Type:        RecoverySkip,
			Description: "Device temporarily unavailable",
			Delay:       0,
			MaxRetries:  0,
		}

	default:
		recoveryAction = RecoveryAction{
			Type:        RecoveryEscalate,
			Description: "Error requires manual attention",
			Delay:       0,
			MaxRetries:  0,
		}
	}

	// Execute recovery action
	success := sm.executeRecoveryAction(statusCtx, &recoveryAction)
	recoveryAction.Success = success

	// Record recovery attempt
	pattern.RecoveryActions = append(pattern.RecoveryActions, recoveryAction)
	pattern.Resolved = success

	// Update metrics
	sm.metrics.mu.Lock()
	sm.metrics.ErrorRecoveries++
	if success {
		sm.metrics.RecoverySuccessRate =
			(sm.metrics.RecoverySuccessRate + 1.0) / 2.0
	}
	sm.metrics.mu.Unlock()

	// Send recovery status update
	recoveryUpdate := StatusUpdate{
		Type:        StatusProgress,
		ExecutionID: statusCtx.ExecutionID,
		Timestamp:   time.Now(),
	}

	if success {
		recoveryUpdate.Message = fmt.Sprintf("Recovered from %s issue, retrying operation", pattern.ErrorType)
	} else {
		recoveryUpdate.Message = fmt.Sprintf("Could not recover from %s issue automatically", pattern.ErrorType)
	}

	sm.sendUpdate(statusCtx, &recoveryUpdate)
}

// executeRecoveryAction performs the actual recovery operation
func (sm *StatusManager) executeRecoveryAction(statusCtx *StatusContext, action *RecoveryAction) bool {
	switch action.Type {
	case RecoveryRetry:
		// Signal retry to execution pipeline
		// This would integrate with the AsyncExecutionPipeline
		return true // Simplified for now

	case RecoveryReset:
		// Reset device connection
		// This would integrate with device management
		return true // Simplified for now

	case RecoverySkip:
		// Skip operation and notify
		return false

	case RecoveryEscalate:
		// Escalate to user attention
		return false

	default:
		return false
	}
}

// enhanceUpdateMessage adds contextual information to update messages
func (sm *StatusManager) enhanceUpdateMessage(statusCtx *StatusContext, update *StatusUpdate) {
	// Add timing information for slow operations
	if update.Type == StatusProgress {
		elapsed := time.Since(statusCtx.LastUpdate)
		if elapsed > 5*time.Second {
			update.Message = fmt.Sprintf("%s (taking longer than usual)", update.Message)
		}
	}

	// Add recovery context for error messages
	if update.Type == StatusError {
		deviceID := statusCtx.DeviceID
		if sm.hasRecentErrors(deviceID) {
			update.Message = fmt.Sprintf("%s - this device has been having issues", update.Message)
		}
	}

	// Add confidence information for predictions
	if update.Type == StatusSuccess && statusCtx.Priority == PriorityLow {
		update.Message = fmt.Sprintf("%s", update.Message) // Keep success messages clean
	}
}

// sendUpdate delivers the status update through appropriate channels
func (sm *StatusManager) sendUpdate(statusCtx *StatusContext, update *StatusUpdate) error {
	// Send to status channel
	select {
	case statusCtx.UpdateChan <- *update:
	case <-time.After(sm.updateTimeout):
		return fmt.Errorf("timeout sending status update")
	}

	// Send audio update if enabled and appropriate
	if statusCtx.AudioEnabled && sm.shouldSendAudio(statusCtx, update) {
		go sm.sendAudioUpdate(update)
	}

	statusCtx.LastUpdate = time.Now()
	return nil
}

// shouldSendAudio determines if an update should be spoken
func (sm *StatusManager) shouldSendAudio(statusCtx *StatusContext, update *StatusUpdate) bool {
	// Only send audio for important updates
	switch update.Type {
	case StatusError:
		return true // Always speak errors
	case StatusSuccess:
		return statusCtx.Priority == PriorityCritical // Only critical successes
	case StatusProgress:
		return statusCtx.Priority >= PriorityHigh // High priority progress
	default:
		return false
	}
}

// sendAudioUpdate synthesizes and plays the status update
func (sm *StatusManager) sendAudioUpdate(update *StatusUpdate) {
	if sm.ttsClient == nil {
		return
	}

	// Synthesize audio
	options := &TTSOptions{
		Voice:          "af_bella",
		Speed:          1.1, // Slightly faster for status updates
		ResponseFormat: "mp3",
	}

	_, err := sm.ttsClient.Synthesize(update.Message, options)
	if err != nil {
		log.Printf("⚠️ Failed to synthesize audio update: %v", err)
		return
	}

	sm.metrics.mu.Lock()
	sm.metrics.AudioUpdates++
	sm.metrics.mu.Unlock()
}

// recordUpdate stores the update in history
func (sm *StatusManager) recordUpdate(executionID string, update *StatusUpdate) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	history := sm.updateHistory[executionID]
	history = append(history, update)

	// Trim history if it gets too long
	if len(history) > sm.maxHistorySize {
		history = history[len(history)-sm.maxHistorySize:]
	}

	sm.updateHistory[executionID] = history
}

// hasRecentErrors checks if a device has had recent errors
func (sm *StatusManager) hasRecentErrors(deviceID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, pattern := range sm.errorPatterns {
		if pattern.DeviceID == deviceID &&
			time.Since(pattern.LastOccurrence) < sm.errorCooldown {
			return true
		}
	}
	return false
}

// determinePriority assigns priority based on update strategy
func (sm *StatusManager) determinePriority(strategy UpdateStrategy) UpdatePriority {
	switch strategy {
	case UpdateSilent:
		return PriorityLow
	case UpdateErrorOnly:
		return PriorityNormal
	case UpdateVerbose:
		return PriorityHigh
	case UpdateProgress:
		return PriorityHigh
	default:
		return PriorityNormal
	}
}

// updateMetrics tracks status update performance
func (sm *StatusManager) updateMetrics(success bool, latency time.Duration, silent bool) {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()

	sm.metrics.TotalUpdates++

	if success {
		sm.metrics.SuccessfulUpdates++
	} else {
		sm.metrics.FailedUpdates++
	}

	if silent {
		sm.metrics.SilentUpdates++
	}

	// Update average latency
	if sm.metrics.TotalUpdates == 1 {
		sm.metrics.AverageUpdateLatency = latency
	} else {
		sm.metrics.AverageUpdateLatency =
			(sm.metrics.AverageUpdateLatency + latency) / 2
	}
}

// UnregisterExecution cleans up status tracking for completed execution
func (sm *StatusManager) UnregisterExecution(executionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if statusCtx, exists := sm.activeUpdates[executionID]; exists {
		statusCtx.CancelFunc()
		delete(sm.activeUpdates, executionID)
	}

	// Keep history but limit size
	if history, exists := sm.updateHistory[executionID]; exists && len(history) > 10 {
		sm.updateHistory[executionID] = history[len(history)-10:]
	}

	log.Printf("📊 Unregistered status tracking for execution %s", executionID)
}

// GetMetrics returns current status manager metrics
func (sm *StatusManager) GetMetrics() *StatusMetrics {
	sm.metrics.mu.RLock()
	defer sm.metrics.mu.RUnlock()

	return &StatusMetrics{
		TotalUpdates:         sm.metrics.TotalUpdates,
		SuccessfulUpdates:    sm.metrics.SuccessfulUpdates,
		FailedUpdates:        sm.metrics.FailedUpdates,
		AudioUpdates:         sm.metrics.AudioUpdates,
		SilentUpdates:        sm.metrics.SilentUpdates,
		ErrorRecoveries:      sm.metrics.ErrorRecoveries,
		AverageUpdateLatency: sm.metrics.AverageUpdateLatency,
		ErrorPatternDetected: sm.metrics.ErrorPatternDetected,
		RecoverySuccessRate:  sm.metrics.RecoverySuccessRate,
	}
}

// GetErrorPatterns returns current error patterns for debugging
func (sm *StatusManager) GetErrorPatterns() map[string]*ErrorPattern {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	patterns := make(map[string]*ErrorPattern)
	for k, v := range sm.errorPatterns {
		patterns[k] = v
	}
	return patterns
}
