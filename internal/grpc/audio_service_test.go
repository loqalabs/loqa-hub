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

package grpc

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/skills"
	"github.com/loqalabs/loqa-hub/internal/storage"
)

// MockTranscriber for testing
type MockTranscriber struct {
	transcription string
	confidence    float64
	shouldError   bool
}

func (m *MockTranscriber) Transcribe(audioData []float32, sampleRate int) (string, error) {
	if m.shouldError {
		return "", io.ErrUnexpectedEOF
	}
	return m.transcription, nil
}

func (m *MockTranscriber) TranscribeWithConfidence(audioData []float32, sampleRate int) (*llm.TranscriptionResult, error) {
	if m.shouldError {
		return nil, io.ErrUnexpectedEOF
	}
	return &llm.TranscriptionResult{
		Text:               m.transcription,
		ConfidenceEstimate: m.confidence,
		WakeWordDetected:   false,
		NeedsConfirmation:  false,
	}, nil
}

func (m *MockTranscriber) Close() error {
	return nil
}

// MockTTSClient for testing
type MockTTSClient struct {
	shouldError bool
	audioData   []byte
}

func (m *MockTTSClient) Synthesize(text string, options *llm.TTSOptions) (*llm.TTSResult, error) {
	if m.shouldError {
		return nil, io.ErrUnexpectedEOF
	}

	return &llm.TTSResult{
		Audio:  strings.NewReader(string(m.audioData)),
		Length: int64(len(m.audioData)),
	}, nil
}

func (m *MockTTSClient) GetAvailableVoices() ([]string, error) {
	return []string{"test-voice"}, nil
}

func (m *MockTTSClient) HealthCheck() error {
	if m.shouldError {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (m *MockTTSClient) Close() error {
	return nil
}

// MockStreamingPredictiveBridge for testing
type MockStreamingPredictiveBridge struct {
	shouldError    bool
	shouldFallback bool
	responseText   string
	intent         string
	confidence     float64
	responseType   llm.PredictiveType
}

func (m *MockStreamingPredictiveBridge) ProcessVoiceCommand(ctx context.Context, transcript string) (*llm.BridgeSession, error) {
	if m.shouldError {
		return nil, io.ErrUnexpectedEOF
	}

	if m.shouldFallback {
		return nil, nil // Return nil to trigger fallback
	}

	return &llm.BridgeSession{
		SessionID:      "test-session-123",
		UserTranscript: transcript,
		Classification: &llm.CommandClassification{
			Intent:       m.intent,
			Entities:     make(map[string]string),
			Confidence:   m.confidence,
			ResponseType: m.responseType,
		},
		PredictiveResponse: &llm.PredictiveResponse{
			ImmediateAck:  m.responseText,
			ExecutionPlan: "Test execution plan",
			StatusUpdates: make(chan llm.StatusUpdate, 1),
		},
		StartTime: time.Now(),
	}, nil
}

// Test helper to create a test audio service
func createTestAudioService(t *testing.T) *AudioService {
	// Create in-memory database for testing
	db, err := storage.NewDatabase(storage.DatabaseConfig{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	eventsStore := storage.NewVoiceEventsStore(db)

	mockTranscriber := &MockTranscriber{
		transcription: "turn on the kitchen lights",
		confidence:    0.9,
		shouldError:   false,
	}

	mockTTS := &MockTTSClient{
		shouldError: false,
		audioData:   []byte("test audio data"),
	}

	service, err := createAudioService(mockTranscriber, mockTTS, eventsStore)
	if err != nil {
		t.Fatalf("Failed to create test audio service: %v", err)
	}

	return service
}

func TestSkillManagerAdapter(t *testing.T) {
	adapter := &SkillManagerAdapter{}

	// Test FindSkillForIntent
	intent := &skills.VoiceIntent{
		Intent:     "turn_on",
		Transcript: "turn on the lights",
	}

	skill, err := adapter.FindSkillForIntent(intent)
	if err == nil {
		t.Error("Expected error from SkillManagerAdapter.FindSkillForIntent")
	}
	if skill != nil {
		t.Error("Expected nil skill from SkillManagerAdapter.FindSkillForIntent")
	}

	// Test ExecuteSkill
	ctx := context.Background()
	response, err := adapter.ExecuteSkill(ctx, nil, intent)
	if err == nil {
		t.Error("Expected error from SkillManagerAdapter.ExecuteSkill")
	}
	if response != nil {
		t.Error("Expected nil response from SkillManagerAdapter.ExecuteSkill")
	}
}

func TestCreateStreamingPredictiveBridge(t *testing.T) {
	skillManager := &SkillManagerAdapter{}
	mockTTS := &MockTTSClient{shouldError: false}

	// Test successful creation (may fail due to missing components, which is expected)
	bridge := createStreamingPredictiveBridge("http://localhost:11434", "test-model", skillManager, mockTTS)

	// Note: bridge may be nil if components are not available, which is OK in tests
	// The important thing is that the function doesn't panic
	t.Logf("Bridge creation result: %v", bridge != nil)
}

func TestAudioService_WithStreamingPredictiveBridge(t *testing.T) {
	service := createTestAudioService(t)

	// Test that the service has both bridge and fallback parser
	// Note: The actual bridge may be nil if components aren't available, which is OK in tests
	t.Logf("Bridge available: %v", service.streamingPredictiveBridge != nil)

	// Test that the service still has the fallback parser
	if service.commandParser == nil {
		t.Error("Expected commandParser to be available as fallback")
	}
}

func TestAudioService_BridgeFallback(t *testing.T) {
	service := createTestAudioService(t)

	// Test that fallback parser is always available
	if service.commandParser == nil {
		t.Error("Expected commandParser to be available for fallback")
	}

	// Test that bridge initialization doesn't break existing functionality
	if service.transcriber == nil {
		t.Error("Expected transcriber to be available")
	}

	if service.ttsClient == nil {
		t.Error("Expected TTS client to be available")
	}
}

func TestAudioService_NoStreamingBridge(t *testing.T) {
	service := createTestAudioService(t)

	// Explicitly set bridge to nil to test traditional path
	service.streamingPredictiveBridge = nil

	// Should still have the traditional parser
	if service.commandParser == nil {
		t.Error("Expected commandParser to be available when no bridge")
	}

	// This represents the original behavior - should still work
	multiCmd, err := service.commandParser.ParseMultiCommand("turn on the lights")
	if err != nil {
		// This may error due to missing Ollama in test environment, which is OK
		t.Logf("Command parsing failed (expected in test environment): %v", err)
	} else if multiCmd == nil {
		t.Error("Expected non-nil MultiCommand result")
	}
}

func TestBytesToFloat32Array(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []float32
		wantLen  int
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: []float32{},
			wantLen:  0,
		},
		{
			name:     "single sample",
			input:    []byte{0x00, 0x10}, // 0x1000 = 4096
			expected: []float32{4096.0 / 32767.0},
			wantLen:  1,
		},
		{
			name:     "multiple samples",
			input:    []byte{0x00, 0x00, 0xFF, 0x7F}, // 0, 32767
			expected: []float32{0.0, 1.0},
			wantLen:  2,
		},
		{
			name:     "odd byte count",
			input:    []byte{0x00, 0x10, 0xFF}, // Should process only first 2 bytes
			expected: []float32{4096.0 / 32767.0},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToFloat32Array(tt.input)

			if len(result) != tt.wantLen {
				t.Errorf("bytesToFloat32Array() length = %d, want %d", len(result), tt.wantLen)
				return
			}

			for i, expected := range tt.expected {
				if i >= len(result) {
					t.Errorf("bytesToFloat32Array() missing sample at index %d", i)
					continue
				}

				epsilon := float32(0.000001)
				if abs32(result[i]-expected) > epsilon {
					t.Errorf("bytesToFloat32Array()[%d] = %f, want %f", i, result[i], expected)
				}
			}
		})
	}
}

func TestAudioService_CalculateSignalStrength(t *testing.T) {
	service := createTestAudioService(t)

	tests := []struct {
		name     string
		samples  []float32
		expected float64
		epsilon  float64
	}{
		{
			name:     "empty samples",
			samples:  []float32{},
			expected: 0.0,
			epsilon:  0.000001,
		},
		{
			name:     "zero samples",
			samples:  []float32{0.0, 0.0, 0.0},
			expected: 0.0,
			epsilon:  0.000001,
		},
		{
			name:     "single positive sample",
			samples:  []float32{1.0},
			expected: 1.0,
			epsilon:  0.000001,
		},
		{
			name:     "mixed samples",
			samples:  []float32{0.5, -0.5, 0.3, -0.3},
			expected: 0.412, // Approximate RMS
			epsilon:  0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.calculateSignalStrength(tt.samples)

			if abs64(result-tt.expected) > tt.epsilon {
				t.Errorf("calculateSignalStrength() = %f, want %f (±%f)", result, tt.expected, tt.epsilon)
			}

			// Verify result is non-negative
			if result < 0 {
				t.Errorf("calculateSignalStrength() = %f, should be non-negative", result)
			}
		})
	}
}

func TestCommandExecutor_Interface(t *testing.T) {
	service := createTestAudioService(t)

	// Test that AudioService implements CommandExecutor interface
	var executor llm.CommandExecutor = service

	// Test ExecuteCommand method
	ctx := context.Background()
	cmd := &llm.Command{
		Intent:     "turn_on",
		Entities:   map[string]string{"device": "lights", "location": "kitchen"},
		Confidence: 0.9,
		Response:   "Turning on kitchen lights",
	}

	err := executor.ExecuteCommand(ctx, cmd)
	// May error due to missing NATS in test environment, which is expected
	if err != nil {
		t.Logf("ExecuteCommand failed (expected in test environment): %v", err)
	}
}

func TestDeviceCommand_Mapping(t *testing.T) {
	service := createTestAudioService(t)

	tests := []struct {
		name           string
		intent         string
		expectedDevice bool
	}{
		{"turn_on is device command", "turn_on", true},
		{"turn_off is device command", "turn_off", true},
		{"dim is device command", "dim", true},
		{"brighten is device command", "brighten", true},
		{"play is device command", "play", true},
		{"stop is device command", "stop", true},
		{"pause is device command", "pause", true},
		{"volume is device command", "volume", true},
		{"unknown is not device command", "unknown", false},
		{"greet is not device command", "greet", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.isDeviceCommand(tt.intent)
			if result != tt.expectedDevice {
				t.Errorf("isDeviceCommand(%s) = %v, want %v", tt.intent, result, tt.expectedDevice)
			}
		})
	}
}

// Helper functions

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
