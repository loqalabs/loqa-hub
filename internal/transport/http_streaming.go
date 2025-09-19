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
	maxSessions     int
	sessionTimeout  time.Duration
	heartbeatInterval time.Duration
}

// StreamSession represents an active streaming session with a puck
type StreamSession struct {
	ID            string
	PuckID        string
	StartTime     time.Time
	LastActivity  time.Time
	Context       context.Context
	Cancel        context.CancelFunc

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
	st := &StreamingTransport{
		sessions:          make(map[string]*StreamSession),
		frameHandlers:     make(map[FrameType]FrameHandler),
		maxSessions:       100, // ESP32 compatibility - conservative limit
		sessionTimeout:    30 * time.Second,
		heartbeatInterval: 5 * time.Second,
	}

	// Start background cleanup goroutine
	go st.cleanupSessions()

	return st
}

// HandleStream processes HTTP/1.1 streaming connections
func (st *StreamingTransport) HandleStream(w http.ResponseWriter, r *http.Request) {
	// Validate request method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract puck ID from headers
	puckID := r.Header.Get("X-Puck-ID")
	if puckID == "" {
		http.Error(w, "X-Puck-ID header required", http.StatusBadRequest)
		return
	}

	// Create session
	session, err := st.createSession(puckID, r.Context())
	if err != nil {
		logging.Sugar.Errorw("Failed to create session", "puck_id", puckID, "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

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
	defer body.Close()
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
			frame, err := DeserializeFrame(headerBuf)
			if err != nil {
				logging.Sugar.Errorw("Invalid frame header", "session_id", session.ID, "error", err)
				return
			}

			// Read remaining data if present
			if len(frame.Data) > 0 {
				frameData := make([]byte, len(frame.Data))
				if _, err := io.ReadFull(body, frameData); err != nil {
					logging.Sugar.Errorw("Failed to read frame data", "session_id", session.ID, "error", err)
					return
				}
				frame.Data = frameData
			}

			// Update session activity
			session.updateActivity()

			// Process frame
			if handler, exists := st.frameHandlers[frame.Type]; exists {
				if err := handler(session, frame); err != nil {
					logging.Sugar.Errorw("Frame handler error", "session_id", session.ID, "frame_type", frame.Type, "error", err)
				}
			} else {
				logging.Sugar.Warnw("No handler for frame type", "session_id", session.ID, "frame_type", frame.Type)
			}

			// Forward to processing pipeline
			select {
			case session.IncomingFrames <- frame:
			case <-session.Context.Done():
				return
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
			heartbeat := NewFrame(FrameTypeHeartbeat, session.getSessionIDHash(), session.nextSequence(), uint64(time.Now().UnixMicro()), nil)

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

// createSession creates a new streaming session
func (st *StreamingTransport) createSession(puckID string, ctx context.Context) (*StreamSession, error) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	// Check session limit
	if len(st.sessions) >= st.maxSessions {
		return nil, fmt.Errorf("maximum sessions reached: %d", st.maxSessions)
	}

	// Generate session ID
	sessionID := fmt.Sprintf("%s-%d", puckID, time.Now().UnixNano())

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
	return session, nil
}

// removeSession removes a session and cleans up resources
func (st *StreamingTransport) removeSession(sessionID string) {
	st.mutex.Lock()
	defer st.mutex.Unlock()

	if session, exists := st.sessions[sessionID]; exists {
		session.Cancel()
		close(session.OutgoingFrames)
		delete(st.sessions, sessionID)
	}
}

// cleanupSessions removes inactive sessions
func (st *StreamingTransport) cleanupSessions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		st.mutex.Lock()
		now := time.Now()

		for sessionID, session := range st.sessions {
			if now.Sub(session.LastActivity) > st.sessionTimeout {
				logging.Sugar.Infow("Cleaning up inactive session", "session_id", sessionID)
				session.Cancel()
				close(session.OutgoingFrames)
				delete(st.sessions, sessionID)
			}
		}

		st.mutex.Unlock()
	}
}

// RegisterFrameHandler registers a handler for a specific frame type
func (st *StreamingTransport) RegisterFrameHandler(frameType FrameType, handler FrameHandler) {
	st.frameHandlers[frameType] = handler
}

// SendFrame sends a frame to a specific session
func (st *StreamingTransport) SendFrame(sessionID string, frame *Frame) error {
	st.mutex.RLock()
	session, exists := st.sessions[sessionID]
	st.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	select {
	case session.OutgoingFrames <- frame:
		return nil
	default:
		return fmt.Errorf("session outgoing buffer full")
	}
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