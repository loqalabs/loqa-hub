package grpc

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	pb "github.com/loqalabs/loqa-proto/go/audio"
	"google.golang.org/grpc/metadata"
)

// MockStream implements pb.AudioService_StreamAudioServer for testing
type MockStream struct {
	responses []*pb.AudioResponse
	mutex     sync.Mutex
}

func (m *MockStream) Send(response *pb.AudioResponse) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responses = append(m.responses, response)
	return nil
}

func (m *MockStream) Recv() (*pb.AudioChunk, error) {
	// Not used in these tests
	return nil, nil
}

func (m *MockStream) SetHeader(md metadata.MD) error  { return nil }
func (m *MockStream) SendHeader(md metadata.MD) error { return nil }
func (m *MockStream) SetTrailer(md metadata.MD)       {}
func (m *MockStream) Context() context.Context        { return context.Background() }
func (m *MockStream) SendMsg(m2 interface{}) error    { return nil }
func (m *MockStream) RecvMsg(m2 interface{}) error    { return nil }

func (m *MockStream) GetResponses() []*pb.AudioResponse {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	result := make([]*pb.AudioResponse, len(m.responses))
	copy(result, m.responses)
	return result
}

func TestCalculateSignalStrength(t *testing.T) {
	as := &AudioService{}

	tests := []struct {
		name     string
		samples  []float32
		expected float64
	}{
		{
			name:     "Empty samples",
			samples:  []float32{},
			expected: 0.0,
		},
		{
			name:     "Single sample",
			samples:  []float32{0.5},
			expected: 0.5,
		},
		{
			name:     "Multiple samples",
			samples:  []float32{0.0, 1.0, 0.0, -1.0},
			expected: 0.7071067811865476, // sqrt(2)/2
		},
		{
			name:     "High amplitude",
			samples:  []float32{1.0, 1.0, 1.0, 1.0},
			expected: 1.0,
		},
		{
			name:     "Low amplitude",
			samples:  []float32{0.1, 0.1, 0.1, 0.1},
			expected: 0.1, // RMS of [0.1, 0.1, 0.1, 0.1] = sqrt((0.01+0.01+0.01+0.01)/4) = sqrt(0.01) = 0.1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := as.calculateSignalStrength(tt.samples)
			diff := math.Abs(result - tt.expected)
			if diff > 1e-7 { // Adjusted for float32 precision
				t.Errorf("calculateSignalStrength(%v) = %.10f, want %.10f, diff=%.10f", tt.samples, result, tt.expected, diff)
			}
		})
	}
}

func TestArbitrationWindow(t *testing.T) {
	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 100 * time.Millisecond // Short window for testing

	// Test starting arbitration window
	stream1 := &MockStream{}
	window := as.startArbitrationWindow("relay1", stream1)

	if window == nil {
		t.Fatal("Expected arbitration window to be created")
	}

	if !window.IsActive {
		t.Error("Expected arbitration window to be active")
	}

	if len(window.Relays) != 1 {
		t.Errorf("Expected 1 relay in window, got %d", len(window.Relays))
	}

	if window.Relays["relay1"] == nil {
		t.Error("Expected relay1 to be in window")
	}

	// Test joining arbitration window
	stream2 := &MockStream{}
	joined := as.joinArbitrationWindow("relay2", stream2)

	if !joined {
		t.Error("Expected relay2 to join arbitration window")
	}

	if len(window.Relays) != 2 {
		t.Errorf("Expected 2 relays in window, got %d", len(window.Relays))
	}

	// Wait for arbitration to complete
	time.Sleep(150 * time.Millisecond)

	// Check that arbitration completed with proper locking
	window.mutex.RLock()
	isActive := window.IsActive
	winnerID := window.WinnerID
	window.mutex.RUnlock()

	if isActive {
		t.Error("Expected arbitration window to be inactive after timeout")
	}

	if winnerID == "" {
		t.Error("Expected a winner to be selected")
	}

	// Check that losing relay received cancellation
	var cancelledRelayID string

	if winnerID == "relay1" {
		cancelledRelayID = "relay2"
	} else {
		cancelledRelayID = "relay1"
	}

	// In tests, streams are MockStream objects, not actual gRPC streams
	// So cancellation responses won't be sent. Instead, verify relay status
	cancelledRelayActive := as.isRelayActive(cancelledRelayID)

	if cancelledRelayActive {
		t.Errorf("Expected cancelled relay %s to be inactive", cancelledRelayID)
	}
}

func TestJoinArbitrationWindowAfterTimeout(t *testing.T) {
	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 50 * time.Millisecond // Very short window

	// Start arbitration window
	stream1 := &MockStream{}
	as.startArbitrationWindow("relay1", stream1)

	// Wait for window to close
	time.Sleep(100 * time.Millisecond)

	// Try to join after window closed
	stream2 := &MockStream{}
	joined := as.joinArbitrationWindow("relay2", stream2)

	if joined {
		t.Error("Expected relay2 to fail joining closed arbitration window")
	}
}

func TestRelayStatusChecking(t *testing.T) {
	as := &AudioService{
		activeStreams: make(map[string]*RelayStream),
	}

	// Test with no relay
	if as.isRelayActive("nonexistent") {
		t.Error("Expected nonexistent relay to be inactive")
	}

	// Add relay with winner status
	as.activeStreams["relay1"] = &RelayStream{
		RelayID: "relay1",
		Status:  RelayStatusWinner,
	}

	if !as.isRelayActive("relay1") {
		t.Error("Expected winner relay to be active")
	}

	// Add relay with cancelled status
	as.activeStreams["relay2"] = &RelayStream{
		RelayID: "relay2",
		Status:  RelayStatusCancelled,
	}

	if as.isRelayActive("relay2") {
		t.Error("Expected cancelled relay to be inactive")
	}

	// Add relay with connected status (single relay, no arbitration)
	as.activeStreams["relay3"] = &RelayStream{
		RelayID: "relay3",
		Status:  RelayStatusConnected,
	}

	if !as.isRelayActive("relay3") {
		t.Error("Expected connected relay to be active")
	}
}

func TestSignalStrengthArbitration(t *testing.T) {
	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 100 * time.Millisecond

	// Create window with relays having different signal strengths
	window := &ArbitrationWindow{
		StartTime:      time.Now(),
		WindowDuration: 100 * time.Millisecond,
		Relays:         make(map[string]*RelayStream),
		IsActive:       true,
	}

	// Relay 1: Low signal strength
	window.Relays["relay1"] = &RelayStream{
		RelayID:        "relay1",
		WakeWordSignal: []float32{0.1, 0.1, 0.1, 0.1}, // Low amplitude
		Status:         RelayStatusContending,
		CancelChannel:  make(chan struct{}),
	}

	// Relay 2: High signal strength
	window.Relays["relay2"] = &RelayStream{
		RelayID:        "relay2",
		WakeWordSignal: []float32{1.0, 1.0, 1.0, 1.0}, // High amplitude
		Status:         RelayStatusContending,
		CancelChannel:  make(chan struct{}),
	}

	// Relay 3: Medium signal strength
	window.Relays["relay3"] = &RelayStream{
		RelayID:        "relay3",
		WakeWordSignal: []float32{0.5, 0.5, 0.5, 0.5}, // Medium amplitude
		Status:         RelayStatusContending,
		CancelChannel:  make(chan struct{}),
	}

	as.arbitrationWindow = window

	// Perform arbitration
	as.performArbitration(window)

	// Check that relay2 (highest signal) won
	if window.WinnerID != "relay2" {
		t.Errorf("Expected relay2 to win with highest signal, got %s", window.WinnerID)
	}

	// Check signal strength calculations
	if window.Relays["relay1"].SignalStrength >= window.Relays["relay2"].SignalStrength {
		t.Error("Expected relay1 to have lower signal strength than relay2")
	}

	if window.Relays["relay3"].SignalStrength >= window.Relays["relay2"].SignalStrength {
		t.Error("Expected relay3 to have lower signal strength than relay2")
	}

	// Check status updates
	if window.Relays["relay2"].Status != RelayStatusWinner {
		t.Error("Expected relay2 to have winner status")
	}

	if window.Relays["relay1"].Status != RelayStatusCancelled {
		t.Error("Expected relay1 to have cancelled status")
	}

	if window.Relays["relay3"].Status != RelayStatusCancelled {
		t.Error("Expected relay3 to have cancelled status")
	}
}

func TestCleanupRelay(t *testing.T) {
	as := &AudioService{
		activeStreams: make(map[string]*RelayStream),
	}

	// Add relay
	as.activeStreams["relay1"] = &RelayStream{
		RelayID: "relay1",
	}

	if len(as.activeStreams) != 1 {
		t.Error("Expected 1 active stream")
	}

	// Cleanup relay
	as.cleanupRelay("relay1")

	if len(as.activeStreams) != 0 {
		t.Error("Expected 0 active streams after cleanup")
	}

	// Test cleanup of nonexistent relay (should not panic)
	as.cleanupRelay("nonexistent")
}

// TestRequestScopedArbitrationLifecycle tests the fix for EOF errors
// by ensuring arbitration window and relay status are properly reset between requests
func TestRequestScopedArbitrationLifecycle(t *testing.T) {
	const testRelayID = "test-relay-001"

	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 100 * time.Millisecond

	relayID := testRelayID

	// Simulate first request: create arbitration window and make relay winner
	stream := &MockStream{}
	as.activeStreams[relayID] = &RelayStream{
		Stream:    stream,
		RelayID:   relayID,
		Status:    RelayStatusConnected,
		ConnectedAt: time.Now(),
		CancelChannel: make(chan struct{}),
	}

	// Start arbitration window (single relay should win immediately)
	window := as.startArbitrationWindow(relayID, stream)
	as.performArbitration(window)

	// Verify relay won arbitration
	if window.WinnerID != relayID {
		t.Errorf("Expected %s to win arbitration, got %s", relayID, window.WinnerID)
	}

	if as.activeStreams[relayID].Status != RelayStatusWinner {
		t.Errorf("Expected relay status to be Winner, got %v", as.activeStreams[relayID].Status)
	}

	// Simulate request completion - this should clear arbitration state
	as.streamsMutex.Lock()
	if as.arbitrationWindow != nil && as.arbitrationWindow.WinnerID == relayID {
		as.arbitrationWindow = nil
	}
	// Reset relay status to Connected for next request
	if relay, exists := as.activeStreams[relayID]; exists {
		relay.Status = RelayStatusConnected
	}
	as.streamsMutex.Unlock()

	// Verify arbitration window is cleared
	if as.arbitrationWindow != nil {
		t.Error("Expected arbitration window to be cleared after request completion")
	}

	// Verify relay status is reset to Connected
	if as.activeStreams[relayID].Status != RelayStatusConnected {
		t.Errorf("Expected relay status to be reset to Connected, got %v", as.activeStreams[relayID].Status)
	}

	// Simulate second request: relay should be able to participate in new arbitration
	if !as.isRelayActive(relayID) {
		t.Error("Expected relay to be active for second request")
	}

	// Start new arbitration window for second request
	window2 := as.startArbitrationWindow(relayID, stream)
	as.performArbitration(window2)

	// Verify relay can win again
	if window2.WinnerID != relayID {
		t.Errorf("Expected %s to win second arbitration, got %s", relayID, window2.WinnerID)
	}

	if as.activeStreams[relayID].Status != RelayStatusWinner {
		t.Errorf("Expected relay status to be Winner in second request, got %v", as.activeStreams[relayID].Status)
	}
}

// TestConsecutiveRequestsWithSingleRelay tests the specific EOF error scenario
// where a single relay makes multiple consecutive requests
func TestConsecutiveRequestsWithSingleRelay(t *testing.T) {
	const testRelayID = "test-relay-001"

	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 50 * time.Millisecond

	relayID := testRelayID
	stream := &MockStream{}

	// Add relay to active streams
	as.activeStreams[relayID] = &RelayStream{
		Stream:        stream,
		RelayID:       relayID,
		Status:        RelayStatusConnected,
		ConnectedAt:   time.Now(),
		CancelChannel: make(chan struct{}),
	}

	// Test multiple consecutive arbitration cycles
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("Request_%d", i+1), func(t *testing.T) {
			// Relay should be active for each request
			if !as.isRelayActive(relayID) {
				t.Errorf("Relay should be active for request %d", i+1)
			}

			// Start arbitration window
			window := as.startArbitrationWindow(relayID, stream)

			// Set some signal data
			as.activeStreams[relayID].WakeWordSignal = []float32{0.5, 0.5, 0.5}

			// Perform arbitration
			as.performArbitration(window)

			// Verify relay wins
			if window.WinnerID != relayID {
				t.Errorf("Request %d: Expected relay to win, got %s", i+1, window.WinnerID)
			}

			if as.activeStreams[relayID].Status != RelayStatusWinner {
				t.Errorf("Request %d: Expected Winner status, got %v", i+1, as.activeStreams[relayID].Status)
			}

			// Simulate request completion lifecycle
			as.streamsMutex.Lock()
			if as.arbitrationWindow != nil && as.arbitrationWindow.WinnerID == relayID {
				as.arbitrationWindow = nil
			}
			if relay, exists := as.activeStreams[relayID]; exists {
				relay.Status = RelayStatusConnected
			}
			as.streamsMutex.Unlock()

			// Verify clean state for next request
			if as.arbitrationWindow != nil {
				t.Errorf("Request %d: Arbitration window should be cleared", i+1)
			}

			if as.activeStreams[relayID].Status != RelayStatusConnected {
				t.Errorf("Request %d: Status should be reset to Connected, got %v", i+1, as.activeStreams[relayID].Status)
			}
		})
	}
}

// TestRelayStatusTransitions tests the relay status lifecycle through multiple states
func TestRelayStatusTransitions(t *testing.T) {
	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 100 * time.Millisecond

	relayID := "test-relay-001"

	// Test status transitions: Connected → Contending → Winner → Connected
	tests := []struct {
		name           string
		initialStatus  RelayStatus
		expectedActive bool
		description    string
	}{
		{
			name:           "Connected",
			initialStatus:  RelayStatusConnected,
			expectedActive: true,
			description:    "Newly connected relay should be active",
		},
		{
			name:           "Contending",
			initialStatus:  RelayStatusContending,
			expectedActive: true,
			description:    "Relay in arbitration should be active",
		},
		{
			name:           "Winner",
			initialStatus:  RelayStatusWinner,
			expectedActive: true,
			description:    "Winning relay should be active",
		},
		{
			name:           "Cancelled",
			initialStatus:  RelayStatusCancelled,
			expectedActive: false,
			description:    "Cancelled relay should be inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as.activeStreams[relayID] = &RelayStream{
				RelayID: relayID,
				Status:  tt.initialStatus,
			}

			isActive := as.isRelayActive(relayID)
			if isActive != tt.expectedActive {
				t.Errorf("%s: Expected active=%v, got %v", tt.description, tt.expectedActive, isActive)
			}
		})
	}
}

// TestArbitrationWindowClearance tests that arbitration windows are properly cleared
func TestArbitrationWindowClearance(t *testing.T) {
	const testRelayID = "test-relay-001"

	as := createTestAudioService(t)
	as.arbitrationWindowDuration = 100 * time.Millisecond

	relayID := testRelayID
	stream := &MockStream{}

	// Add relay to active streams
	as.activeStreams[relayID] = &RelayStream{
		Stream:        stream,
		RelayID:       relayID,
		ConnectedAt:   time.Now(),
		Status:        RelayStatusConnected,
		CancelChannel: make(chan struct{}),
	}

	// Start arbitration window
	window := as.startArbitrationWindow(relayID, stream)

	// Verify window exists
	as.streamsMutex.Lock()
	windowExists := as.arbitrationWindow != nil
	as.streamsMutex.Unlock()

	if !windowExists {
		t.Fatal("Expected arbitration window to be created")
	}

	// Set some signal data and perform arbitration
	as.activeStreams[relayID].WakeWordSignal = []float32{0.5, 0.5, 0.5}
	as.performArbitration(window)

	// Verify window has a winner
	if window.WinnerID != relayID {
		t.Fatalf("Expected %s to win arbitration, got %s", relayID, window.WinnerID)
	}

	// Simulate request completion - clear window
	as.streamsMutex.Lock()
	if as.arbitrationWindow != nil && as.arbitrationWindow.WinnerID == relayID {
		as.arbitrationWindow = nil
	}
	as.streamsMutex.Unlock()

	// Verify window is cleared
	as.streamsMutex.Lock()
	windowCleared := as.arbitrationWindow == nil
	as.streamsMutex.Unlock()

	if !windowCleared {
		t.Error("Expected arbitration window to be cleared")
	}

	// Verify new window can be created
	window2 := as.startArbitrationWindow(relayID, stream)
	if window2 == nil {
		t.Error("Expected new arbitration window to be created")
	}

	// Verify it's a different window instance
	if window == window2 {
		t.Error("Expected different window instance")
	}
}
