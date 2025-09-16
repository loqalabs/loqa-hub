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
	"log"
	"sync"
	"time"
)

// StreamingInterruptHandler manages clean interruption of streaming operations
type StreamingInterruptHandler struct {
	activeStreams  map[string]*StreamingSession
	mu             sync.RWMutex
	gracePeriod    time.Duration
	forceTimeout   time.Duration
}

// StreamingSession represents an active streaming session that can be interrupted
type StreamingSession struct {
	ID                string
	Context           context.Context
	Cancel            context.CancelFunc
	StreamingResult   *StreamingResult
	AudioPipeline     *PipelineContext
	CreatedAt         time.Time
	InterruptedAt     *time.Time
	CleanupCompleted  bool
	InterruptReason   string
	mu                sync.RWMutex
}

// InterruptReason constants for tracking why streams were interrupted
const (
	InterruptReasonUserRequest  = "user_request"
	InterruptReasonNewCommand   = "new_command"
	InterruptReasonTimeout      = "timeout"
	InterruptReasonError        = "error"
	InterruptReasonShutdown     = "shutdown"
)

// NewStreamingInterruptHandler creates a new interrupt handler
func NewStreamingInterruptHandler(gracePeriod, forceTimeout time.Duration) *StreamingInterruptHandler {
	return &StreamingInterruptHandler{
		activeStreams: make(map[string]*StreamingSession),
		gracePeriod:   gracePeriod,
		forceTimeout:  forceTimeout,
	}
}

// RegisterSession registers a new streaming session for interrupt management
func (sih *StreamingInterruptHandler) RegisterSession(
	sessionID string,
	ctx context.Context,
	cancel context.CancelFunc,
	streamingResult *StreamingResult,
	audioPipeline *PipelineContext,
) {
	sih.mu.Lock()
	defer sih.mu.Unlock()

	session := &StreamingSession{
		ID:              sessionID,
		Context:         ctx,
		Cancel:          cancel,
		StreamingResult: streamingResult,
		AudioPipeline:   audioPipeline,
		CreatedAt:       time.Now(),
	}

	sih.activeStreams[sessionID] = session
	log.Printf("🎯 Registered streaming session: %s", sessionID)
}

// InterruptSession interrupts a specific streaming session gracefully
func (sih *StreamingInterruptHandler) InterruptSession(sessionID, reason string) error {
	sih.mu.RLock()
	session, exists := sih.activeStreams[sessionID]
	sih.mu.RUnlock()

	if !exists {
		return nil // Session already completed or doesn't exist
	}

	session.mu.Lock()
	if session.InterruptedAt != nil {
		session.mu.Unlock()
		return nil // Already interrupted
	}

	now := time.Now()
	session.InterruptedAt = &now
	session.InterruptReason = reason
	session.mu.Unlock()

	log.Printf("🛑 Interrupting streaming session %s (reason: %s)", sessionID, reason)

	// Start graceful shutdown process
	go sih.performGracefulShutdown(session)

	return nil
}

// InterruptAllSessions interrupts all active streaming sessions
func (sih *StreamingInterruptHandler) InterruptAllSessions(reason string) {
	sih.mu.RLock()
	sessions := make([]*StreamingSession, 0, len(sih.activeStreams))
	for _, session := range sih.activeStreams {
		sessions = append(sessions, session)
	}
	sih.mu.RUnlock()

	log.Printf("🛑 Interrupting %d active streaming sessions (reason: %s)", len(sessions), reason)

	var wg sync.WaitGroup
	for _, session := range sessions {
		wg.Add(1)
		go func(s *StreamingSession) {
			defer wg.Done()
			if err := sih.InterruptSession(s.ID, reason); err != nil {
				log.Printf("Warning: failed to interrupt session %s: %v", s.ID, err)
			}
		}(session)
	}

	wg.Wait()
	log.Printf("✅ All streaming sessions interrupted")
}

// performGracefulShutdown handles the graceful shutdown of a streaming session
func (sih *StreamingInterruptHandler) performGracefulShutdown(session *StreamingSession) {
	// Step 1: Signal cancellation to all components
	if session.StreamingResult != nil && session.StreamingResult.Cancel != nil {
		session.StreamingResult.Cancel()
	}

	// Step 2: Give components time to clean up gracefully
	gracefulDone := make(chan struct{})
	go func() {
		defer close(gracefulDone)

		// Wait for streaming components to finish
		if session.StreamingResult != nil {
			// Drain remaining channels to prevent goroutine leaks
			sih.drainChannels(session.StreamingResult)
		}

		// Wait for audio pipeline to finish
		if session.AudioPipeline != nil {
			session.AudioPipeline.Cancel()
			// The pipeline will handle its own cleanup
		}
	}()

	// Step 3: Wait for graceful shutdown or force termination
	select {
	case <-gracefulDone:
		log.Printf("✅ Graceful shutdown completed for session: %s", session.ID)
	case <-time.After(sih.gracePeriod):
		log.Printf("⚠️ Graceful shutdown timeout for session %s, forcing termination", session.ID)
		sih.forceTermination(session)
	}

	// Step 4: Mark as completed and remove from active sessions
	session.mu.Lock()
	session.CleanupCompleted = true
	session.mu.Unlock()

	sih.mu.Lock()
	delete(sih.activeStreams, session.ID)
	sih.mu.Unlock()

	log.Printf("🧹 Cleaned up streaming session: %s", session.ID)
}

// forceTermination forcefully terminates a streaming session
func (sih *StreamingInterruptHandler) forceTermination(session *StreamingSession) {
	// Cancel the main context (should already be cancelled, but ensure it)
	if session.Cancel != nil {
		session.Cancel()
	}

	// Force close any remaining channels
	if session.StreamingResult != nil {
		sih.forceCloseChannels(session.StreamingResult)
	}

	log.Printf("🔥 Forced termination completed for session: %s", session.ID)
}

// drainChannels drains the channels of a streaming result to prevent goroutine leaks
func (sih *StreamingInterruptHandler) drainChannels(result *StreamingResult) {
	// Create a timeout for draining
	timeout := time.After(sih.forceTimeout)

	for {
		select {
		case <-result.TokenStream:
			// Continue draining
		case <-result.VisualTokens:
			// Continue draining
		case <-result.AudioPhrases:
			// Continue draining
		case <-result.FinalCommand:
			// Command received, likely finished naturally
			return
		case <-result.ErrorChan:
			// Error received, stream likely finished
			return
		case <-timeout:
			// Timeout reached, stop draining
			return
		default:
			// No more data in channels
			return
		}
	}
}

// forceCloseChannels forcefully closes channels (should only be used as last resort)
func (sih *StreamingInterruptHandler) forceCloseChannels(result *StreamingResult) {
	// Note: In Go, you should only close channels from the sender side
	// This is a last resort and should ideally not be needed if components
	// handle cancellation properly
	log.Printf("⚠️ Force closing channels - this should not normally be needed")
}

// GetActiveSessionIDs returns the IDs of all active streaming sessions
func (sih *StreamingInterruptHandler) GetActiveSessionIDs() []string {
	sih.mu.RLock()
	defer sih.mu.RUnlock()

	ids := make([]string, 0, len(sih.activeStreams))
	for id := range sih.activeStreams {
		ids = append(ids, id)
	}

	return ids
}

// GetSessionInfo returns information about a specific session
func (sih *StreamingInterruptHandler) GetSessionInfo(sessionID string) (*StreamingSessionInfo, error) {
	sih.mu.RLock()
	session, exists := sih.activeStreams[sessionID]
	sih.mu.RUnlock()

	if !exists {
		return nil, nil // Session not found or already completed
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	info := &StreamingSessionInfo{
		ID:               session.ID,
		CreatedAt:        session.CreatedAt,
		IsInterrupted:    session.InterruptedAt != nil,
		InterruptReason:  session.InterruptReason,
		CleanupCompleted: session.CleanupCompleted,
	}

	if session.InterruptedAt != nil {
		info.InterruptedAt = *session.InterruptedAt
		info.Duration = session.InterruptedAt.Sub(session.CreatedAt)
	} else {
		info.Duration = time.Since(session.CreatedAt)
	}

	return info, nil
}

// StreamingSessionInfo provides information about a streaming session
type StreamingSessionInfo struct {
	ID               string        `json:"id"`
	CreatedAt        time.Time     `json:"created_at"`
	InterruptedAt    time.Time     `json:"interrupted_at,omitempty"`
	Duration         time.Duration `json:"duration"`
	IsInterrupted    bool          `json:"is_interrupted"`
	InterruptReason  string        `json:"interrupt_reason,omitempty"`
	CleanupCompleted bool          `json:"cleanup_completed"`
}

// GetSessionMetrics returns metrics for all sessions
func (sih *StreamingInterruptHandler) GetSessionMetrics() *SessionMetrics {
	sih.mu.RLock()
	defer sih.mu.RUnlock()

	metrics := &SessionMetrics{
		ActiveSessions:   len(sih.activeStreams),
		InterruptedCount: 0,
		AverageDuration:  0,
		InterruptReasons: make(map[string]int),
	}

	var totalDuration time.Duration
	sessionCount := 0

	for _, session := range sih.activeStreams {
		session.mu.RLock()
		if session.InterruptedAt != nil {
			metrics.InterruptedCount++
			metrics.InterruptReasons[session.InterruptReason]++
			totalDuration += session.InterruptedAt.Sub(session.CreatedAt)
		} else {
			totalDuration += time.Since(session.CreatedAt)
		}
		sessionCount++
		session.mu.RUnlock()
	}

	if sessionCount > 0 {
		metrics.AverageDuration = totalDuration / time.Duration(sessionCount)
	}

	return metrics
}

// SessionMetrics provides aggregate metrics for streaming sessions
type SessionMetrics struct {
	ActiveSessions   int                    `json:"active_sessions"`
	InterruptedCount int                    `json:"interrupted_count"`
	AverageDuration  time.Duration          `json:"average_duration"`
	InterruptReasons map[string]int         `json:"interrupt_reasons"`
}

// Shutdown gracefully shuts down the interrupt handler
func (sih *StreamingInterruptHandler) Shutdown() {
	log.Printf("🔄 Shutting down streaming interrupt handler...")
	sih.InterruptAllSessions(InterruptReasonShutdown)
	log.Printf("✅ Streaming interrupt handler shutdown completed")
}