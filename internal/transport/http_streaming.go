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

package transport

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// StreamingTransport handles HTTP/1.1 chunked transfer with binary frames
type StreamingTransport struct {
	sessions map[string]*StreamSession
	mutex    sync.RWMutex

	// Handlers for different frame types
	frameHandlers map[FrameType]FrameHandler

	// Configuration
	maxSessions       int
	sessionTimeout    time.Duration
	heartbeatInterval time.Duration

	// Resource monitoring (learned from puck testing)
	resourceMonitor *ResourceMonitor

	// Performance monitoring (enhanced from puck testing lessons)
	performanceMonitor *PerformanceMonitor

	// Context for graceful shutdown of background goroutines
	ctx    context.Context
	cancel context.CancelFunc

	// Metrics
	totalConnections  uint64
	droppedFrames     uint64
	invalidFrames     uint64
}

// StreamSession represents an active streaming session with a puck
type StreamSession struct {
	ID           string
	PuckID       string
	StartTime    time.Time
	LastActivity time.Time
	Context      context.Context
	Cancel       context.CancelFunc

	// Channels for communication
	IncomingFrames chan *Frame
	OutgoingFrames chan *Frame

	// Session state
	mutex     sync.RWMutex
	connected bool
	sequence  uint32
}

// FrameHandler processes incoming frames of a specific type
type FrameHandler func(session *StreamSession, frame *Frame) error

// NewStreamingTransport creates a new HTTP/1.1 streaming transport
func NewStreamingTransport() *StreamingTransport {
	ctx, cancel := context.WithCancel(context.Background())
	st := &StreamingTransport{
		sessions:           make(map[string]*StreamSession),
		frameHandlers:      make(map[FrameType]FrameHandler),
		maxSessions:        100, // ESP32 compatibility - conservative limit
		sessionTimeout:     30 * time.Second,
		heartbeatInterval:  5 * time.Second,
		resourceMonitor:    NewResourceMonitor(),    // Add resource monitoring
		performanceMonitor: NewPerformanceMonitor(), // Add performance monitoring
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start background cleanup goroutine
	go st.cleanupSessions()

	// Start resource monitoring goroutine
	go st.monitorResources()

	// Start performance monitoring goroutine
	go st.monitorPerformance()

	return st
}

// HandleStream processes HTTP/1.1 streaming connections
func (st *StreamingTransport) HandleStream(w http.ResponseWriter, r *http.Request) {
	// Validate request method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract puck ID from headers with validation
	puckID := r.Header.Get("X-Puck-ID")
	if puckID == "" {
		http.Error(w, "X-Puck-ID header required", http.StatusBadRequest)
		return
	}

	// Validate puck ID format (learned from puck testing)
	if err := validatePuckID(puckID); err != nil {
		logging.Sugar.Warnw("Invalid puck ID in stream request", "puck_id", puckID, "error", err)
		http.Error(w, "Invalid X-Puck-ID format", http.StatusBadRequest)
		return
	}

	// Create session with performance tracking
	sessionStartTime := time.Now()
	session, err := st.createSession(puckID, r.Context())
	if err != nil {
		logging.Sugar.Errorw("Failed to create session", "puck_id", puckID, "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}
	st.performanceMonitor.RecordSessionCreated(time.Since(sessionStartTime))

	logging.Sugar.Infow("New streaming session", "session_id", session.ID, "puck_id", puckID)

	// Set streaming response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Session-ID", session.ID)

	// Enable flushing for real-time streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Start bidirectional streaming
	go st.handleIncomingStream(session, r.Body)
	st.handleOutgoingStream(session, w, flusher)

	// Cleanup on disconnect
	st.removeSession(session.ID)
	logging.Sugar.Infow("Session ended", "session_id", session.ID, "puck_id", puckID)
}

// handleIncomingStream processes incoming frames from the puck
func (st *StreamingTransport) handleIncomingStream(session *StreamSession, body io.ReadCloser) {
	defer func() { _ = body.Close() }()
	defer close(session.IncomingFrames)

	for {
		select {
		case <-session.Context.Done():
			return
		default:
			// Read frame header first to determine total frame size
			headerBuf := make([]byte, HeaderSize)
			if _, err := io.ReadFull(body, headerBuf); err != nil {
				if err != io.EOF {
					logging.Sugar.Errorw("Failed to read frame header", "session_id", session.ID, "error", err)
				}
				return
			}

			// Parse header to get data length
			header, err := parseFrameHeader(headerBuf)
			if err != nil {
				logging.Sugar.Errorw("Invalid frame header", "session_id", session.ID, "error", err)
				return
			}

			// Read remaining data if present
			var frameData []byte
			if header.Length > 0 {
				frameData = make([]byte, header.Length)
				if _, err := io.ReadFull(body, frameData); err != nil {
					logging.Sugar.Errorw("Failed to read frame data", "session_id", session.ID, "error", err)
					return
				}
			}

			// Combine header and data for complete frame deserialization
			completeFrame := make([]byte, HeaderSize+int(header.Length))
			copy(completeFrame, headerBuf)
			if len(frameData) > 0 {
				copy(completeFrame[HeaderSize:], frameData)
			}

			// Performance tracking: start timing frame processing
			frameProcessStart := time.Now()

			// Deserialize complete frame
			frame, err := DeserializeFrame(completeFrame)
			if err != nil {
				st.invalidFrames++
				st.performanceMonitor.RecordDeserializationError()
				logging.Sugar.Errorw("Failed to deserialize frame", "session_id", session.ID, "error", err)
				return
			}

			// Apply comprehensive frame validation (learned from puck testing)
			if err := ValidateFrameForHubProcessing(frame); err != nil {
				st.invalidFrames++
				st.performanceMonitor.RecordValidationError()
				logging.Sugar.Warnw("Invalid frame received",
					"session_id", session.ID,
					"error", err,
					"frame_type", frame.Type,
					"frame_size", len(completeFrame))
				return
			}

			// Update session activity
			session.updateActivity()

			// Process frame with error tracking
			if handler, exists := st.frameHandlers[frame.Type]; exists {
				if err := handler(session, frame); err != nil {
					st.performanceMonitor.RecordHandlerError()
					logging.Sugar.Errorw("Frame handler error", "session_id", session.ID, "frame_type", frame.Type, "error", err)
					// Track handler errors but don't terminate session
				}
			} else {
				logging.Sugar.Warnw("No handler for frame type", "session_id", session.ID, "frame_type", frame.Type)
			}

			// Forward to processing pipeline with buffer monitoring
			select {
			case session.IncomingFrames <- frame:
				// Successfully queued - record processing time
				st.performanceMonitor.RecordFrameProcessing(time.Since(frameProcessStart))
			case <-session.Context.Done():
				return
			default:
				// Buffer full - track dropped frames (learned from puck testing)
				st.droppedFrames++
				st.performanceMonitor.RecordBufferOverflow()
				logging.Sugar.Warnw("Incoming frame buffer full, dropping frame",
					"session_id", session.ID,
					"frame_type", frame.Type)
				// Continue processing despite dropped frame
			}
		}
	}
}

// handleOutgoingStream sends frames to the puck
func (st *StreamingTransport) handleOutgoingStream(session *StreamSession, w http.ResponseWriter, flusher http.Flusher) {
	heartbeatTicker := time.NewTicker(st.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-session.Context.Done():
			return

		case frame := <-session.OutgoingFrames:
			if frame == nil {
				return // Channel closed
			}

			data, err := frame.Serialize()
			if err != nil {
				logging.Sugar.Errorw("Failed to serialize frame", "session_id", session.ID, "error", err)
				continue
			}

			if _, err := w.Write(data); err != nil {
				logging.Sugar.Errorw("Failed to write frame", "session_id", session.ID, "error", err)
				return
			}

			flusher.Flush()

		case <-heartbeatTicker.C:
			// Send heartbeat frame
			timestamp := time.Now().UnixMicro()
			if timestamp < 0 {
				timestamp = 0
			}
			heartbeat := NewFrame(FrameTypeHeartbeat, session.getSessionIDHash(), session.nextSequence(), uint64(timestamp), nil) //nolint:gosec // G115: Safe conversion, timestamp is validated above

			data, err := heartbeat.Serialize()
			if err != nil {
				logging.Sugar.Errorw("Failed to serialize heartbeat", "session_id", session.ID, "error", err)
				continue
			}

			if _, err := w.Write(data); err != nil {
				logging.Sugar.Errorw("Failed to write heartbeat", "session_id", session.ID, "error", err)
				return
			}

			flusher.Flush()
		}
	}
}

// createSession creates a new streaming session with enhanced validation
// Based on lessons learned from puck testing - validates inputs more thoroughly
func (st *StreamingTransport) createSession(puckID string, ctx context.Context) (*StreamSession, error) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	// Enhanced puck ID validation (learned from puck testing)
	if err := validatePuckID(puckID); err != nil {
		return nil, fmt.Errorf("invalid puck ID: %w", err)
	}

	// Check session limit
	if len(st.sessions) >= st.maxSessions {
		return nil, fmt.Errorf("maximum sessions reached: %d", st.maxSessions)
	}

	// Generate session ID with randomness to prevent collisions
	now := time.Now()
	//nolint:gosec // Using math/rand for non-cryptographic randomness in connection management
	random := rand.Int63n(1000000) // Random number 0-999999
	sessionID := fmt.Sprintf("%s-%d-%d-%d", puckID, now.Unix(), now.Nanosecond(), random)

	// Create session context with timeout
	sessionCtx, cancel := context.WithTimeout(ctx, st.sessionTimeout)

	session := &StreamSession{
		ID:             sessionID,
		PuckID:         puckID,
		StartTime:      time.Now(),
		LastActivity:   time.Now(),
		Context:        sessionCtx,
		Cancel:         cancel,
		IncomingFrames: make(chan *Frame, 100), // Buffer for ESP32 compatibility
		OutgoingFrames: make(chan *Frame, 100),
		connected:      true,
		sequence:       0,
	}

	st.sessions[sessionID] = session

	// Track connection metrics
	st.totalConnections++

	return session, nil
}

// validatePuckID validates puck ID format and content
// Based on lessons learned from puck testing
func validatePuckID(puckID string) error {
	if puckID == "" {
		return fmt.Errorf("puck ID cannot be empty")
	}

	// Length validation
	if len(puckID) > 64 {
		return fmt.Errorf("puck ID too long: %d characters (max 64)", len(puckID))
	}

	// Character validation - only alphanumeric, hyphens, underscores
	for _, char := range puckID {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' && char != '_' {
			return fmt.Errorf("puck ID contains invalid character: %c", char)
		}
	}

	// Prevent dangerous patterns
	if puckID == "." || puckID == ".." {
		return fmt.Errorf("puck ID cannot be '.' or '..'")
	}

	return nil
}

// removeSession removes a session and cleans up resources
func (st *StreamingTransport) removeSession(sessionID string) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if session, exists := st.sessions[sessionID]; exists {
		// Record session duration for performance tracking
		sessionDuration := time.Since(session.StartTime)
		st.performanceMonitor.RecordSessionClosed(sessionDuration)

		session.Cancel()
		close(session.OutgoingFrames)
		delete(st.sessions, sessionID)
	}
}

// cleanupInactiveSessions does a single pass of inactive session cleanup
func (st *StreamingTransport) cleanupInactiveSessions() {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range st.sessions {
		if now.Sub(session.LastActivity) > st.sessionTimeout {
			logging.Sugar.Infow("Cleaning up inactive session", "session_id", sessionID)
			session.Cancel()
			close(session.OutgoingFrames)
			delete(st.sessions, sessionID)
		}
	}
}

// cleanupSessions removes inactive sessions (background loop)
func (st *StreamingTransport) cleanupSessions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			st.cleanupInactiveSessions()
		}
	}
}

// RegisterFrameHandler registers a handler for a specific frame type
func (st *StreamingTransport) RegisterFrameHandler(frameType FrameType, handler FrameHandler) {
	st.frameHandlers[frameType] = handler
}

// getSession safely retrieves a session with validation
func (st *StreamingTransport) getSession(sessionID, puckID string) (*StreamSession, error) {
	st.mutex.RLock()
	defer st.mutex.RUnlock()

	session, exists := st.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Verify puck ID matches if provided
	if puckID != "" && session.PuckID != puckID {
		return nil, fmt.Errorf("puck ID mismatch: expected %s, got %s", session.PuckID, puckID)
	}

	return session, nil
}

// SendFrame sends a frame to a specific session
func (st *StreamingTransport) SendFrame(sessionID string, frame *Frame) error {
	session, err := st.getSession(sessionID, "")
	if err != nil {
		return err
	}

	select {
	case session.OutgoingFrames <- frame:
		return nil
	case <-session.Context.Done():
		return fmt.Errorf("session closed")
	default:
		return fmt.Errorf("session outgoing buffer full")
	}
}

// HandleSend processes individual frame submissions from pucks
func (st *StreamingTransport) HandleSend(w http.ResponseWriter, r *http.Request) {
	// Validate request method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract puck ID from query parameters
	puckID := r.URL.Query().Get("puck_id")
	if puckID == "" {
		http.Error(w, "puck_id parameter required", http.StatusBadRequest)
		return
	}

	// Extract session ID from headers
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "X-Session-ID header required", http.StatusBadRequest)
		return
	}

	// Read frame data
	frameData, err := io.ReadAll(r.Body)
	if err != nil {
		logging.Sugar.Errorw("Failed to read frame data", "puck_id", puckID, "session_id", sessionID, "error", err)
		http.Error(w, "Failed to read frame data", http.StatusBadRequest)
		return
	}

	// Deserialize frame
	frame, err := DeserializeFrame(frameData)
	if err != nil {
		st.invalidFrames++
		logging.Sugar.Errorw("Failed to deserialize frame", "puck_id", puckID, "session_id", sessionID, "error", err)
		http.Error(w, "Invalid frame data", http.StatusBadRequest)
		return
	}

	// Apply comprehensive frame validation (learned from puck testing)
	if err := ValidateFrameForHubProcessing(frame); err != nil {
		st.invalidFrames++
		logging.Sugar.Warnw("Invalid frame received via HandleSend",
			"puck_id", puckID,
			"session_id", sessionID,
			"error", err,
			"frame_type", frame.Type,
			"frame_size", len(frameData))
		http.Error(w, "Frame validation failed", http.StatusBadRequest)
		return
	}

	// Find and validate session
	session, err := st.getSession(sessionID, puckID)
	if err != nil {
		logging.Sugar.Warnw("Session validation failed", "puck_id", puckID, "session_id", sessionID, "error", err)
		if err.Error() == fmt.Sprintf("session not found: %s", sessionID) {
			http.Error(w, "Session not found", http.StatusNotFound)
		} else {
			http.Error(w, "Session validation failed", http.StatusForbidden)
		}
		return
	}

	// Update session activity
	session.updateActivity()

	// Process frame through handlers
	if handler, exists := st.frameHandlers[frame.Type]; exists {
		if err := handler(session, frame); err != nil {
			logging.Sugar.Errorw("Frame handler error", "session_id", sessionID, "frame_type", frame.Type, "error", err)
		}
	} else {
		logging.Sugar.Warnw("No handler for frame type", "session_id", sessionID, "frame_type", frame.Type)
	}

	// Forward to processing pipeline with enhanced buffer monitoring
	select {
	case session.IncomingFrames <- frame:
		// Successfully queued frame
	case <-session.Context.Done():
		http.Error(w, "Session closed", http.StatusGone)
		return
	default:
		// Buffer full - track dropped frames (learned from puck testing)
		st.droppedFrames++
		logging.Sugar.Warnw("Incoming frame buffer full via HandleSend",
			"session_id", sessionID,
			"puck_id", puckID,
			"frame_type", frame.Type)
		http.Error(w, "Frame buffer full", http.StatusServiceUnavailable)
		return
	}

	// Send success response
	w.WriteHeader(http.StatusOK)
}

// Session helper methods
func (s *StreamSession) updateActivity() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.LastActivity = time.Now()
}

func (s *StreamSession) nextSequence() uint32 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sequence++
	return s.sequence
}

func (s *StreamSession) getSessionIDHash() uint32 {
	// Simple hash of session ID for frame session field
	hash := uint32(0)
	for _, b := range []byte(s.ID) {
		hash = hash*31 + uint32(b)
	}
	return hash
}

// Public methods for external access

// GetSessionIDHash returns the session ID hash for frame headers
func (s *StreamSession) GetSessionIDHash() uint32 {
	return s.getSessionIDHash()
}

// NextSequence returns the next sequence number for frame headers
func (s *StreamSession) NextSequence() uint32 {
	return s.nextSequence()
}

// monitorResources monitors system resources and logs warnings
// Based on lessons learned from puck testing
func (st *StreamingTransport) monitorResources() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			st.mutex.RLock()
			activeSessions := len(st.sessions)
			st.mutex.RUnlock()

			st.resourceMonitor.UpdateMetrics(activeSessions)

			warnings := st.resourceMonitor.CheckResourceLeaks()
			if len(warnings) > 0 {
				logging.Sugar.Warnw("Resource warnings detected",
					"warnings", warnings,
					"active_sessions", activeSessions,
					"total_connections", st.totalConnections,
					"dropped_frames", st.droppedFrames,
					"invalid_frames", st.invalidFrames)

				// Force cleanup if we have serious issues
				if !st.resourceMonitor.IsHealthy() {
					logging.Sugar.Warn("Forcing resource cleanup due to warnings")
					st.resourceMonitor.ForceCleanup()
				}
			}
		}
	}
}

// ProcessIncomingFrame processes and validates incoming frames
// Enhanced with lessons learned from puck testing
func (st *StreamingTransport) ProcessIncomingFrame(sessionID, puckID string, frameData []byte) error {
	// Get session with validation
	session, err := st.getSession(sessionID, puckID)
	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}

	// Deserialize frame
	frame, err := DeserializeFrame(frameData)
	if err != nil {
		st.invalidFrames++
		return fmt.Errorf("frame deserialization failed: %w", err)
	}

	// Apply comprehensive frame validation (learned from puck testing)
	if err := ValidateFrameForHubProcessing(frame); err != nil {
		st.invalidFrames++
		logging.Sugar.Warnw("Invalid frame received",
			"session_id", sessionID,
			"puck_id", puckID,
			"error", err,
			"frame_type", frame.Type,
			"frame_size", len(frameData))
		return fmt.Errorf("frame validation failed: %w", err)
	}

	// Update session activity
	session.LastActivity = time.Now()

	// Process frame with registered handler
	if handler, exists := st.frameHandlers[frame.Type]; exists {
		if err := handler(session, frame); err != nil {
			logging.Sugar.Errorw("Frame handler error",
				"session_id", sessionID,
				"puck_id", puckID,
				"frame_type", frame.Type,
				"error", err)
			return fmt.Errorf("frame handler error: %w", err)
		}
	} else {
		logging.Sugar.Warnw("No handler registered for frame type",
			"frame_type", frame.Type,
			"session_id", sessionID)
		return fmt.Errorf("no handler for frame type: 0x%02X", frame.Type)
	}

	return nil
}

// GetHealthStatus returns the health status including resource and performance monitoring
func (st *StreamingTransport) GetHealthStatus() map[string]interface{} {
	st.mutex.RLock()
	totalConnections := st.totalConnections
	droppedFrames := st.droppedFrames
	invalidFrames := st.invalidFrames
	st.mutex.RUnlock()

	// Combine resource and performance monitoring
	healthStatus := st.resourceMonitor.GetHealthStatus()
	performanceStatus := st.performanceMonitor.GetPerformanceStatus()

	// Merge performance metrics into health status
	for key, value := range performanceStatus {
		healthStatus[key] = value
	}

	// Add transport-specific metrics
	healthStatus["total_connections"] = totalConnections
	healthStatus["dropped_frames"] = droppedFrames
	healthStatus["invalid_frames"] = invalidFrames

	return healthStatus
}

// Shutdown gracefully shuts down the streaming transport
// Implements proper cleanup patterns learned from puck testing
func (st *StreamingTransport) Shutdown(ctx context.Context) error {
	logging.Sugar.Info("StreamingTransport: Starting graceful shutdown")

	// Cancel background goroutines first
	st.cancel()

	// Cancel all active sessions
	st.mutex.Lock()
	sessionIDs := make([]string, 0, len(st.sessions))
	for sessionID := range st.sessions {
		sessionIDs = append(sessionIDs, sessionID)
	}
	st.mutex.Unlock()

	// Cancel sessions with timeout
	for _, sessionID := range sessionIDs {
		st.removeSession(sessionID)
	}

	// Give goroutines a brief moment to exit cleanly
	select {
	case <-ctx.Done():
		logging.Sugar.Warn("StreamingTransport: Shutdown timeout reached")
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
		logging.Sugar.Info("StreamingTransport: Graceful shutdown completed")
	}

	// Force final cleanup
	st.resourceMonitor.ForceCleanup()

	return nil
}

// monitorPerformance monitors performance metrics and logs summaries
// Based on lessons learned from puck testing - provides actionable insights
func (st *StreamingTransport) monitorPerformance() {
	ticker := time.NewTicker(60 * time.Second) // Every minute
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			// Log performance summary
			st.performanceMonitor.LogPerformanceSummary()

			// Check for performance issues and log warnings
			recommendations := st.performanceMonitor.GetRecommendations()
			if len(recommendations) > 0 {
				logging.Sugar.Warnw("Performance recommendations available",
					"count", len(recommendations),
					"recommendations", recommendations)
			}

			// Check critical performance thresholds
			metrics := st.performanceMonitor.GetFrameProcessingMetrics()
			if metrics.CurrentThroughput < 10 && metrics.FramesProcessed > 100 {
				logging.Sugar.Errorw("Critical performance issue detected",
					"current_throughput", metrics.CurrentThroughput,
					"frames_processed", metrics.FramesProcessed,
					"buffer_overflows", metrics.BufferOverflows)
			}
		}
	}
}
