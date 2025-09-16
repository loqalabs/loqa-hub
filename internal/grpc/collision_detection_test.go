package grpc

import (
	"context"
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
func (m *MockStream) Context() context.Context              { return context.Background() }
func (m *MockStream) SendMsg(m2 interface{}) error          { return nil }
func (m *MockStream) RecvMsg(m2 interface{}) error          { return nil }

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
	as := &AudioService{
		activeStreams:             make(map[string]*RelayStream),
		arbitrationWindowDuration: 100 * time.Millisecond, // Short window for testing
	}

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

	// Check that arbitration completed
	if window.IsActive {
		t.Error("Expected arbitration window to be inactive after timeout")
	}

	if window.WinnerID == "" {
		t.Error("Expected a winner to be selected")
	}

	// Check that losing relay received cancellation
	var cancelledRelay *MockStream
	var cancelledRelayID string

	if window.WinnerID == "relay1" {
		cancelledRelay = stream2
		cancelledRelayID = "relay2"
	} else {
		cancelledRelay = stream1
		cancelledRelayID = "relay1"
	}

	responses := cancelledRelay.GetResponses()
	if len(responses) != 1 {
		t.Errorf("Expected cancelled relay to receive 1 response, got %d", len(responses))
	} else {
		response := responses[0]
		if response.Command != "relay_cancelled" {
			t.Errorf("Expected cancellation command, got %s", response.Command)
		}
		if response.RequestId != cancelledRelayID {
			t.Errorf("Expected request ID %s, got %s", cancelledRelayID, response.RequestId)
		}
		if response.Success {
			t.Error("Expected cancelled response to have Success=false")
		}
	}
}

func TestJoinArbitrationWindowAfterTimeout(t *testing.T) {
	as := &AudioService{
		activeStreams:             make(map[string]*RelayStream),
		arbitrationWindowDuration: 50 * time.Millisecond, // Very short window
	}

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
	as := &AudioService{
		activeStreams:             make(map[string]*RelayStream),
		arbitrationWindowDuration: 100 * time.Millisecond,
	}

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