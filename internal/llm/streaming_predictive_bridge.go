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

	"github.com/loqalabs/loqa-hub/internal/skills"
)

// StreamingPredictiveBridge integrates streaming LLM with predictive response architecture
type StreamingPredictiveBridge struct {
	// Core components
	streamingParser     *StreamingCommandParser
	predictiveEngine    *PredictiveResponseEngine
	statusManager       *StatusManager
	commandClassifier   *CommandClassifier
	executionPipeline   *AsyncExecutionPipeline

	// Configuration
	enablePredictive    bool
	streamingEnabled    bool
	confidenceThreshold float64
	fallbackToStreaming bool

	// State management
	mu              sync.RWMutex
	activeSessions  map[string]*BridgeSession
	metrics         *BridgeMetrics
}

// BridgeSession represents a unified streaming + predictive session
type BridgeSession struct {
	SessionID         string
	UserTranscript    string
	Classification    *CommandClassification
	PredictiveResponse *PredictiveResponse
	StreamingContext  string // Context ID for streaming parser
	StatusUpdates     chan StatusUpdate

	// Timing
	StartTime         time.Time
	FirstTokenTime    time.Time
	PredictionTime    time.Time
	ExecutionStartTime time.Time

	// State
	StreamingActive   bool
	PredictionActive  bool
	Completed         bool

	// Cancellation
	CancelFunc        context.CancelFunc
}

// BridgeMetrics tracks performance of the integrated system
type BridgeMetrics struct {
	mu                      sync.RWMutex
	TotalSessions           int64         `json:"total_sessions"`
	StreamingOnlySessions   int64         `json:"streaming_only_sessions"`
	PredictiveOnlySessions  int64         `json:"predictive_only_sessions"`
	HybridSessions          int64         `json:"hybrid_sessions"`
	AverageResponseTime     time.Duration `json:"average_response_time"`
	AveragePredictionTime   time.Duration `json:"average_prediction_time"`
	SuccessfulPredictions   int64         `json:"successful_predictions"`
	FailedPredictions       int64         `json:"failed_predictions"`
	FallbackToStreaming     int64         `json:"fallback_to_streaming"`
	UserInterruptions       int64         `json:"user_interruptions"`
}

// NewStreamingPredictiveBridge creates a new integrated bridge
func NewStreamingPredictiveBridge(
	streamingParser *StreamingCommandParser,
	predictiveEngine *PredictiveResponseEngine,
	statusManager *StatusManager,
	commandClassifier *CommandClassifier,
	executionPipeline *AsyncExecutionPipeline,
) *StreamingPredictiveBridge {
	return &StreamingPredictiveBridge{
		streamingParser:     streamingParser,
		predictiveEngine:    predictiveEngine,
		statusManager:       statusManager,
		commandClassifier:   commandClassifier,
		executionPipeline:   executionPipeline,

		enablePredictive:    true,
		streamingEnabled:    true,
		confidenceThreshold: 0.8,
		fallbackToStreaming: true,

		activeSessions: make(map[string]*BridgeSession),
		metrics:        &BridgeMetrics{},
	}
}

// ProcessVoiceCommand handles a voice command with hybrid streaming + predictive approach
func (spb *StreamingPredictiveBridge) ProcessVoiceCommand(ctx context.Context, transcript string) (*BridgeSession, error) {
	sessionID := generateSessionID()
	startTime := time.Now()

	log.Printf("🎯 Processing voice command with hybrid approach: %s", transcript)

	// Create session context
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &BridgeSession{
		SessionID:      sessionID,
		UserTranscript: transcript,
		StartTime:      startTime,
		StatusUpdates:  make(chan StatusUpdate, 20),
		CancelFunc:     cancel,
	}

	// Store session
	spb.mu.Lock()
	spb.activeSessions[sessionID] = session
	spb.mu.Unlock()

	// Phase 1: Quick classification to determine approach
	classification, err := spb.quickClassifyCommand(sessionCtx, transcript)
	if err != nil {
		log.Printf("⚠️ Quick classification failed, falling back to streaming: %v", err)
		return spb.fallbackToStreamingOnly(sessionCtx, session)
	}

	session.Classification = classification
	session.PredictionTime = time.Now()

	// Phase 2: Determine processing strategy
	strategy := spb.determineProcessingStrategy(classification)

	switch strategy {
	case ProcessingHybrid:
		return spb.processHybrid(sessionCtx, session)
	case ProcessingPredictiveOnly:
		return spb.processPredictiveOnly(sessionCtx, session)
	case ProcessingStreamingOnly:
		return spb.processStreamingOnly(sessionCtx, session)
	default:
		return spb.fallbackToStreamingOnly(sessionCtx, session)
	}
}

// ProcessingStrategy defines how to handle the command
type ProcessingStrategy string

const (
	ProcessingHybrid         ProcessingStrategy = "hybrid"          // Streaming + predictive
	ProcessingPredictiveOnly ProcessingStrategy = "predictive_only" // High confidence commands
	ProcessingStreamingOnly  ProcessingStrategy = "streaming_only"  // Complex queries
)

// quickClassifyCommand performs rapid classification for strategy determination
func (spb *StreamingPredictiveBridge) quickClassifyCommand(ctx context.Context, transcript string) (*CommandClassification, error) {
	// Use command classifier for quick analysis
	classification, err := spb.commandClassifier.ClassifyCommand(ctx, transcript)
	if err != nil {
		return nil, err
	}

	log.Printf("📊 Quick classification: intent=%s, confidence=%.2f, type=%s",
		classification.Intent, classification.Confidence, classification.ResponseType)

	return classification, nil
}

// determineProcessingStrategy chooses the best approach based on classification
func (spb *StreamingPredictiveBridge) determineProcessingStrategy(classification *CommandClassification) ProcessingStrategy {
	// High confidence + predictive type = predictive only
	if classification.Confidence >= 0.95 && classification.ResponseType == PredictiveOptimistic {
		return ProcessingPredictiveOnly
	}

	// Query or complex operations = streaming only
	if strings.Contains(strings.ToLower(classification.Intent), "what") ||
	   strings.Contains(strings.ToLower(classification.Intent), "how") ||
	   strings.Contains(strings.ToLower(classification.Intent), "explain") {
		return ProcessingStreamingOnly
	}

	// Medium confidence commands = hybrid approach
	if classification.Confidence >= spb.confidenceThreshold {
		return ProcessingHybrid
	}

	// Low confidence = streaming only for safety
	return ProcessingStreamingOnly
}

// processHybrid handles commands with both streaming and predictive responses
func (spb *StreamingPredictiveBridge) processHybrid(ctx context.Context, session *BridgeSession) (*BridgeSession, error) {
	log.Printf("🔄 Processing with hybrid approach: %s", session.SessionID)

	session.StreamingActive = true
	session.PredictionActive = true

	// Register status tracking
	deviceID := extractDeviceID(session.Classification.Entities)
	spb.statusManager.RegisterExecution(
		session.SessionID,
		session.Classification.UpdateStrategy,
		deviceID,
		session.StatusUpdates,
	)

	// Start predictive response immediately
	predictiveResponse, err := spb.predictiveEngine.ProcessCommand(ctx, session.UserTranscript)
	if err != nil {
		log.Printf("⚠️ Predictive response failed, falling back to streaming: %v", err)
		return spb.fallbackToStreamingOnly(ctx, session)
	}

	session.PredictiveResponse = predictiveResponse

	// Start streaming response for visual feedback
	go spb.startStreamingForVisualFeedback(ctx, session)

	// Monitor both channels
	go spb.monitorHybridExecution(ctx, session)

	spb.updateMetrics(ProcessingHybrid)
	return session, nil
}

// processPredictiveOnly handles high-confidence commands with immediate execution
func (spb *StreamingPredictiveBridge) processPredictiveOnly(ctx context.Context, session *BridgeSession) (*BridgeSession, error) {
	log.Printf("⚡ Processing with predictive-only approach: %s", session.SessionID)

	session.PredictionActive = true

	// Register status tracking
	deviceID := extractDeviceID(session.Classification.Entities)
	spb.statusManager.RegisterExecution(
		session.SessionID,
		session.Classification.UpdateStrategy,
		deviceID,
		session.StatusUpdates,
	)

	// Execute prediction immediately
	predictiveResponse, err := spb.predictiveEngine.ProcessCommand(ctx, session.UserTranscript)
	if err != nil {
		log.Printf("⚠️ Predictive response failed, falling back to streaming: %v", err)
		return spb.fallbackToStreamingOnly(ctx, session)
	}

	session.PredictiveResponse = predictiveResponse

	// Monitor execution
	go spb.monitorPredictiveExecution(ctx, session)

	spb.updateMetrics(ProcessingPredictiveOnly)
	return session, nil
}

// processStreamingOnly handles complex queries with streaming responses
func (spb *StreamingPredictiveBridge) processStreamingOnly(ctx context.Context, session *BridgeSession) (*BridgeSession, error) {
	log.Printf("🌊 Processing with streaming-only approach: %s", session.SessionID)

	session.StreamingActive = true

	// Start streaming response
	streamingCtx, err := spb.streamingParser.StartStreaming(ctx, session.UserTranscript)
	if err != nil {
		return nil, fmt.Errorf("failed to start streaming: %w", err)
	}

	session.StreamingContext = streamingCtx
	session.FirstTokenTime = time.Now()

	// Monitor streaming
	go spb.monitorStreamingExecution(ctx, session)

	spb.updateMetrics(ProcessingStreamingOnly)
	return session, nil
}

// fallbackToStreamingOnly provides fallback when predictive approach fails
func (spb *StreamingPredictiveBridge) fallbackToStreamingOnly(ctx context.Context, session *BridgeSession) (*BridgeSession, error) {
	log.Printf("🔄 Falling back to streaming-only for session: %s", session.SessionID)

	session.StreamingActive = true
	session.PredictionActive = false

	// Start streaming response
	streamingCtx, err := spb.streamingParser.StartStreaming(ctx, session.UserTranscript)
	if err != nil {
		return nil, fmt.Errorf("fallback streaming failed: %w", err)
	}

	session.StreamingContext = streamingCtx
	session.FirstTokenTime = time.Now()

	// Monitor streaming
	go spb.monitorStreamingExecution(ctx, session)

	spb.updateMetricsFallback()
	return session, nil
}

// startStreamingForVisualFeedback provides immediate visual response in hybrid mode
func (spb *StreamingPredictiveBridge) startStreamingForVisualFeedback(ctx context.Context, session *BridgeSession) {
	// Create visual confirmation message
	visualResponse := fmt.Sprintf("Processing: %s", session.PredictiveResponse.ImmediateAck)

	// Send immediate visual feedback
	visualUpdate := StatusUpdate{
		Type:        StatusProgress,
		Message:     visualResponse,
		Success:     false, // Not final result
		ExecutionID: session.SessionID,
		Timestamp:   time.Now(),
	}

	select {
	case session.StatusUpdates <- visualUpdate:
		session.FirstTokenTime = time.Now()
		log.Printf("📱 Sent visual feedback for hybrid session: %s", session.SessionID)
	default:
		log.Printf("⚠️ Failed to send visual feedback for session: %s", session.SessionID)
	}
}

// monitorHybridExecution manages both streaming and predictive execution
func (spb *StreamingPredictiveBridge) monitorHybridExecution(ctx context.Context, session *BridgeSession) {
	defer spb.cleanupSession(session.SessionID)

	// Monitor status updates from predictive execution
	for {
		select {
		case <-ctx.Done():
			return

		case statusUpdate, ok := <-session.PredictiveResponse.StatusUpdates:
			if !ok {
				// Predictive execution completed
				session.PredictionActive = false
				session.Completed = true

				// Send final status through session channel
				finalUpdate := StatusUpdate{
					Type:        statusUpdate.Type,
					Message:     statusUpdate.Message,
					Success:     statusUpdate.Success,
					ExecutionID: session.SessionID,
					Timestamp:   time.Now(),
				}

				select {
				case session.StatusUpdates <- finalUpdate:
				default:
				}

				spb.updateMetricsCompletion(session, statusUpdate.Success)
				return
			}

			// Forward status update
			statusUpdate.ExecutionID = session.SessionID
			spb.statusManager.ProcessStatusUpdate(statusUpdate)

		case <-time.After(30 * time.Second):
			// Timeout
			log.Printf("⏰ Hybrid execution timed out for session: %s", session.SessionID)
			spb.updateMetricsCompletion(session, false)
			return
		}
	}
}

// monitorPredictiveExecution manages predictive-only execution
func (spb *StreamingPredictiveBridge) monitorPredictiveExecution(ctx context.Context, session *BridgeSession) {
	defer spb.cleanupSession(session.SessionID)

	// Monitor status updates
	for {
		select {
		case <-ctx.Done():
			return

		case statusUpdate, ok := <-session.PredictiveResponse.StatusUpdates:
			if !ok {
				session.Completed = true
				spb.updateMetricsCompletion(session, true)
				return
			}

			// Process status update
			statusUpdate.ExecutionID = session.SessionID
			spb.statusManager.ProcessStatusUpdate(statusUpdate)

		case <-time.After(30 * time.Second):
			log.Printf("⏰ Predictive execution timed out for session: %s", session.SessionID)
			spb.updateMetricsCompletion(session, false)
			return
		}
	}
}

// monitorStreamingExecution manages streaming-only execution
func (spb *StreamingPredictiveBridge) monitorStreamingExecution(ctx context.Context, session *BridgeSession) {
	defer spb.cleanupSession(session.SessionID)

	// Monitor streaming completion
	// This would integrate with the streaming parser's completion signals

	// For now, simulate streaming completion
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
		session.Completed = true
		spb.updateMetricsCompletion(session, true)
		return
	}
}

// cleanupSession removes session from tracking
func (spb *StreamingPredictiveBridge) cleanupSession(sessionID string) {
	spb.mu.Lock()
	defer spb.mu.Unlock()

	if session, exists := spb.activeSessions[sessionID]; exists {
		session.CancelFunc()
		close(session.StatusUpdates)
		delete(spb.activeSessions, sessionID)
	}

	// Cleanup status manager
	spb.statusManager.UnregisterExecution(sessionID)

	log.Printf("🧹 Cleaned up session: %s", sessionID)
}

// updateMetrics updates metrics for different processing strategies
func (spb *StreamingPredictiveBridge) updateMetrics(strategy ProcessingStrategy) {
	spb.metrics.mu.Lock()
	defer spb.metrics.mu.Unlock()

	spb.metrics.TotalSessions++

	switch strategy {
	case ProcessingHybrid:
		spb.metrics.HybridSessions++
	case ProcessingPredictiveOnly:
		spb.metrics.PredictiveOnlySessions++
	case ProcessingStreamingOnly:
		spb.metrics.StreamingOnlySessions++
	}
}

// updateMetricsFallback updates fallback metrics
func (spb *StreamingPredictiveBridge) updateMetricsFallback() {
	spb.metrics.mu.Lock()
	defer spb.metrics.mu.Unlock()
	spb.metrics.FallbackToStreaming++
}

// updateMetricsCompletion updates completion metrics
func (spb *StreamingPredictiveBridge) updateMetricsCompletion(session *BridgeSession, success bool) {
	spb.metrics.mu.Lock()
	defer spb.metrics.mu.Unlock()

	responseTime := time.Since(session.StartTime)

	// Update average response time
	if spb.metrics.TotalSessions == 1 {
		spb.metrics.AverageResponseTime = responseTime
	} else {
		spb.metrics.AverageResponseTime =
			(spb.metrics.AverageResponseTime + responseTime) / 2
	}

	// Update prediction metrics
	if session.PredictionActive {
		predictionTime := session.PredictionTime.Sub(session.StartTime)

		if spb.metrics.TotalSessions == 1 {
			spb.metrics.AveragePredictionTime = predictionTime
		} else {
			spb.metrics.AveragePredictionTime =
				(spb.metrics.AveragePredictionTime + predictionTime) / 2
		}

		if success {
			spb.metrics.SuccessfulPredictions++
		} else {
			spb.metrics.FailedPredictions++
		}
	}
}

// GetMetrics returns current bridge metrics
func (spb *StreamingPredictiveBridge) GetMetrics() *BridgeMetrics {
	spb.metrics.mu.RLock()
	defer spb.metrics.mu.RUnlock()

	return &BridgeMetrics{
		TotalSessions:           spb.metrics.TotalSessions,
		StreamingOnlySessions:   spb.metrics.StreamingOnlySessions,
		PredictiveOnlySessions:  spb.metrics.PredictiveOnlySessions,
		HybridSessions:          spb.metrics.HybridSessions,
		AverageResponseTime:     spb.metrics.AverageResponseTime,
		AveragePredictionTime:   spb.metrics.AveragePredictionTime,
		SuccessfulPredictions:   spb.metrics.SuccessfulPredictions,
		FailedPredictions:       spb.metrics.FailedPredictions,
		FallbackToStreaming:     spb.metrics.FallbackToStreaming,
		UserInterruptions:       spb.metrics.UserInterruptions,
	}
}

// GetActiveSessions returns information about currently active sessions
func (spb *StreamingPredictiveBridge) GetActiveSessions() map[string]*BridgeSession {
	spb.mu.RLock()
	defer spb.mu.RUnlock()

	sessions := make(map[string]*BridgeSession)
	for k, v := range spb.activeSessions {
		sessions[k] = v
	}
	return sessions
}

// InterruptSession allows graceful interruption of active sessions
func (spb *StreamingPredictiveBridge) InterruptSession(sessionID string) error {
	spb.mu.RLock()
	session, exists := spb.activeSessions[sessionID]
	spb.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Cancel session context
	session.CancelFunc()

	// Update metrics
	spb.metrics.mu.Lock()
	spb.metrics.UserInterruptions++
	spb.metrics.mu.Unlock()

	log.Printf("🛑 Interrupted session: %s", sessionID)
	return nil
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}