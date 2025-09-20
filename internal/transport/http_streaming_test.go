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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

func TestNewStreamingTransport(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Verify initialization
	if st.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	if st.frameHandlers == nil {
		t.Error("frameHandlers map should be initialized")
	}
	if st.maxSessions != 100 {
		t.Errorf("maxSessions = %d, want 100", st.maxSessions)
	}
	if st.sessionTimeout != 30*time.Second {
		t.Errorf("sessionTimeout = %v, want 30s", st.sessionTimeout)
	}
	if st.heartbeatInterval != 5*time.Second {
		t.Errorf("heartbeatInterval = %v, want 5s", st.heartbeatInterval)
	}

	// Verify cleanup goroutine was started (indirect test)
	time.Sleep(10 * time.Millisecond) // Allow goroutine to start
}

func TestHandleStream_InvalidMethod(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	w := httptest.NewRecorder()

	st.HandleStream(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleStream_MissingPuckID(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	req := httptest.NewRequest(http.MethodPost, "/stream", nil)
	w := httptest.NewRecorder()

	st.HandleStream(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "X-Puck-ID header required") {
		t.Error("Response should mention missing X-Puck-ID header")
	}
}

func TestCreateSession(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	ctx := context.Background()
	puckID := "test-puck-001"

	session, err := st.createSession(puckID, ctx)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	// Verify session properties
	if session.PuckID != puckID {
		t.Errorf("PuckID = %s, want %s", session.PuckID, puckID)
	}
	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if session.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
	if session.LastActivity.IsZero() {
		t.Error("LastActivity should be set")
	}
	if session.Context == nil {
		t.Error("Context should be set")
	}
	if session.Cancel == nil {
		t.Error("Cancel function should be set")
	}
	if session.IncomingFrames == nil {
		t.Error("IncomingFrames channel should be initialized")
	}
	if session.OutgoingFrames == nil {
		t.Error("OutgoingFrames channel should be initialized")
	}
	if !session.connected {
		t.Error("Session should be marked as connected")
	}
	if session.sequence != 0 {
		t.Errorf("Initial sequence = %d, want 0", session.sequence)
	}

	// Verify session is registered
	st.mutex.RLock()
	_, exists := st.sessions[session.ID]
	st.mutex.RUnlock()

	if !exists {
		t.Error("Session should be registered in sessions map")
	}
}

func TestCreateSession_MaxSessions(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()
	st.maxSessions = 2 // Set low limit for testing

	ctx := context.Background()

	// Create sessions up to limit
	session1, err := st.createSession("puck-001", ctx)
	if err != nil {
		t.Fatalf("createSession(1) error = %v", err)
	}

	session2, err := st.createSession("puck-002", ctx)
	if err != nil {
		t.Fatalf("createSession(2) error = %v", err)
	}

	// Third session should fail
	_, err = st.createSession("puck-003", ctx)
	if err == nil {
		t.Error("createSession(3) should fail due to max sessions limit")
	}
	if !strings.Contains(err.Error(), "maximum sessions reached") {
		t.Errorf("Error should mention max sessions, got: %v", err)
	}

	// Cleanup
	st.removeSession(session1.ID)
	st.removeSession(session2.ID)
}

func TestRemoveSession(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Create session
	session, err := st.createSession("test-puck", context.Background())
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	// Verify session exists
	st.mutex.RLock()
	_, exists := st.sessions[session.ID]
	st.mutex.RUnlock()

	if !exists {
		t.Error("Session should exist before removal")
	}

	// Remove session
	st.removeSession(session.ID)

	// Verify session is removed
	st.mutex.RLock()
	_, exists = st.sessions[session.ID]
	st.mutex.RUnlock()

	if exists {
		t.Error("Session should be removed")
	}

	// Verify context is cancelled
	select {
	case <-session.Context.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Session context should be cancelled")
	}
}

func TestRegisterFrameHandler(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	var handlerCalled bool
	handler := func(session *StreamSession, frame *Frame) error {
		handlerCalled = true
		return nil
	}

	// Register handler
	st.RegisterFrameHandler(FrameTypeAudioData, handler)

	// Verify handler is registered
	if registeredHandler, exists := st.frameHandlers[FrameTypeAudioData]; !exists {
		t.Error("Handler should be registered")
	} else {
		// Test the handler
		session := &StreamSession{}
		frame := &Frame{Type: FrameTypeAudioData}
		err := registeredHandler(session, frame)
		if err != nil {
			t.Errorf("Handler error = %v", err)
		}
		if !handlerCalled {
			t.Error("Handler should be called")
		}
	}
}

func TestSendFrame(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Create session
	session, err := st.createSession("test-puck", context.Background())
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	defer st.removeSession(session.ID)

	// Create test frame
	frame := NewFrame(FrameTypeAudioData, 123, 1, uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test timestamp

	// Send frame
	err = st.SendFrame(session.ID, frame)
	if err != nil {
		t.Fatalf("SendFrame() error = %v", err)
	}

	// Verify frame was received
	select {
	case receivedFrame := <-session.OutgoingFrames:
		if receivedFrame != frame {
			t.Error("Received frame should match sent frame")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Frame should be received within timeout")
	}
}

func TestSendFrame_NonexistentSession(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	frame := NewFrame(FrameTypeAudioData, 123, 1, uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test timestamp

	err := st.SendFrame("nonexistent", frame)
	if err == nil {
		t.Error("SendFrame() should fail for nonexistent session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("Error should mention session not found, got: %v", err)
	}
}

func TestSendFrame_FullBuffer(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Create session
	session, err := st.createSession("test-puck", context.Background())
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	defer st.removeSession(session.ID)

	// Fill the outgoing buffer (capacity is 100)
	for i := 0; i < 100; i++ {
		frame := NewFrame(FrameTypeAudioData, 123, uint32(i), uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test
		err := st.SendFrame(session.ID, frame)
		if err != nil {
			t.Fatalf("SendFrame(%d) error = %v", i, err)
		}
	}

	// Next frame should fail due to full buffer
	frame := NewFrame(FrameTypeAudioData, 123, 101, uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test timestamp
	err = st.SendFrame(session.ID, frame)
	if err == nil {
		t.Error("SendFrame() should fail for full buffer")
	}
	if !strings.Contains(err.Error(), "buffer full") {
		t.Errorf("Error should mention buffer full, got: %v", err)
	}
}

func TestSessionHelperMethods(t *testing.T) {
	session := &StreamSession{
		LastActivity: time.Now().Add(-1 * time.Hour),
		sequence:     0,
		ID:           "test-session-123",
	}

	// Test updateActivity
	oldActivity := session.LastActivity
	session.updateActivity()
	if !session.LastActivity.After(oldActivity) {
		t.Error("updateActivity() should update LastActivity to current time")
	}

	// Test nextSequence
	seq1 := session.nextSequence()
	if seq1 != 1 {
		t.Errorf("First nextSequence() = %d, want 1", seq1)
	}

	seq2 := session.nextSequence()
	if seq2 != 2 {
		t.Errorf("Second nextSequence() = %d, want 2", seq2)
	}

	// Test getSessionIDHash
	hash1 := session.getSessionIDHash()
	hash2 := session.getSessionIDHash()
	if hash1 != hash2 {
		t.Error("getSessionIDHash() should return consistent hash")
	}
	if hash1 == 0 {
		t.Error("getSessionIDHash() should not return zero")
	}

	// Test hash uniqueness with different session ID
	session.ID = "different-session-456"
	hash3 := session.getSessionIDHash()
	if hash1 == hash3 {
		t.Error("Different session IDs should produce different hashes")
	}
}

func TestStreamSessionConcurrency(t *testing.T) {
	session := &StreamSession{
		LastActivity: time.Now(),
		sequence:     0,
		mutex:        sync.RWMutex{},
	}

	// Test concurrent access to sequence
	var wg sync.WaitGroup
	sequences := make([]uint32, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sequences[index] = session.nextSequence()
		}(i)
	}

	wg.Wait()

	// Verify all sequences are unique and in expected range
	sequenceMap := make(map[uint32]bool)
	for _, seq := range sequences {
		if seq < 1 || seq > 100 {
			t.Errorf("Sequence %d out of expected range [1, 100]", seq)
		}
		if sequenceMap[seq] {
			t.Errorf("Duplicate sequence %d found", seq)
		}
		sequenceMap[seq] = true
	}

	if len(sequenceMap) != 100 {
		t.Errorf("Expected 100 unique sequences, got %d", len(sequenceMap))
	}
}

// Mock types for testing HTTP streaming
type mockResponseWriter struct {
	*httptest.ResponseRecorder
	flushed bool
	mutex   sync.Mutex
}

func (m *mockResponseWriter) Flush() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.flushed = true
}

func (m *mockResponseWriter) IsFlushed() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.flushed
}

func (m *mockResponseWriter) GetBodyLen() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.Body.Len()
}

func (m *mockResponseWriter) GetBodyBytes() []byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.Body.Bytes()
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.ResponseRecorder.Write(data)
}

type mockReadCloser struct {
	data   []byte
	pos    int
	closed bool
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}

	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestHandleIncomingStream_EOF(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Create session
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &StreamSession{
		ID:             "test-session",
		Context:        ctx,
		IncomingFrames: make(chan *Frame, 10),
	}

	// Create empty body (immediate EOF)
	body := &mockReadCloser{data: []byte{}}

	// Start handling incoming stream in goroutine
	go st.handleIncomingStream(session, body)

	// Wait for the goroutine to finish (should exit due to EOF)
	time.Sleep(50 * time.Millisecond)

	// Verify the incoming frames channel was closed
	select {
	case frame, ok := <-session.IncomingFrames:
		if ok || frame != nil {
			t.Error("IncomingFrames channel should be closed on EOF")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("IncomingFrames channel should be closed within timeout")
	}

	// Stop the session
	cancel()
}

func TestHandleOutgoingStream_Heartbeat(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()
	st.heartbeatInterval = 50 * time.Millisecond // Fast heartbeat for testing

	// Create session
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &StreamSession{
		ID:             "test-session",
		Context:        ctx,
		OutgoingFrames: make(chan *Frame, 10),
		sequence:       0,
	}

	// Create mock response writer
	mockWriter := &mockResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
	}

	// Start handling outgoing stream in goroutine
	go st.handleOutgoingStream(session, mockWriter, mockWriter)

	// Wait for heartbeat to be sent
	time.Sleep(100 * time.Millisecond)

	// Verify data was written (heartbeat frame)
	if mockWriter.GetBodyLen() == 0 {
		t.Error("Heartbeat should be written to response")
	}

	// Verify flusher was called
	if !mockWriter.IsFlushed() {
		t.Error("Response should be flushed after heartbeat")
	}

	// Stop the session
	cancel()
}

func TestHandleOutgoingStream_Frame(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()

	// Create session
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &StreamSession{
		ID:             "test-session",
		Context:        ctx,
		OutgoingFrames: make(chan *Frame, 10),
		sequence:       0,
	}

	// Create mock response writer
	mockWriter := &mockResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
	}

	// Start handling outgoing stream in goroutine
	go st.handleOutgoingStream(session, mockWriter, mockWriter)

	// Send test frame
	testFrame := NewFrame(FrameTypeResponse, 123, 1, uint64(time.Now().UnixMicro()), []byte("response")) //nolint:gosec // G115: Safe conversion for test timestamp
	session.OutgoingFrames <- testFrame

	// Wait for frame to be processed
	time.Sleep(50 * time.Millisecond)

	// Verify frame was written
	if mockWriter.GetBodyLen() == 0 {
		t.Error("Frame should be written to response")
	}

	// Verify the written data is a valid frame
	data := mockWriter.GetBodyBytes()
	if len(data) < HeaderSize {
		t.Error("Written data should contain at least frame header")
	}

	// Verify flusher was called
	if !mockWriter.IsFlushed() {
		t.Error("Response should be flushed after frame")
	}

	// Stop the session
	cancel()
}

func TestCleanupSessions_InactiveSessions(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	st := NewStreamingTransport()
	st.sessionTimeout = 100 * time.Millisecond // Short timeout for testing

	// Create session
	session, err := st.createSession("test-puck", context.Background())
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	// Set last activity to past
	session.mutex.Lock()
	session.LastActivity = time.Now().Add(-200 * time.Millisecond)
	session.mutex.Unlock()

	// Manually trigger cleanup
	st.mutex.Lock()
	now := time.Now()
	for sessionID, session := range st.sessions {
		if now.Sub(session.LastActivity) > st.sessionTimeout {
			session.Cancel()
			close(session.OutgoingFrames)
			delete(st.sessions, sessionID)
		}
	}
	st.mutex.Unlock()

	// Verify session was cleaned up
	st.mutex.RLock()
	_, exists := st.sessions[session.ID]
	st.mutex.RUnlock()

	if exists {
		t.Error("Inactive session should be cleaned up")
	}
}
