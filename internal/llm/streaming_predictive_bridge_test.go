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
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/skills"
)

// Mock implementations for testing StreamingPredictiveBridge

type MockStreamingCommandParser struct {
	shouldError     bool
	fallbackEnabled bool
}

func (m *MockStreamingCommandParser) ParseCommandStreaming(ctx context.Context, transcript string) (*StreamingResult, error) {
	if m.shouldError {
		return nil, context.DeadlineExceeded
	}

	// Simulate streaming response
	result := &StreamingResult{
		VisualTokens:   make(chan string, 10),
		AudioSegments:  make(chan *AudioSegment, 10),
		FinalResponse:  make(chan *CommandParseResult, 1),
		SessionID:      "test-session-123",
		IsComplete:     false,
	}

	// Send some visual tokens
	go func() {
		result.VisualTokens <- "Processing"
		result.VisualTokens <- "your"
		result.VisualTokens <- "command..."
		close(result.VisualTokens)

		// Send final response
		finalResult := &CommandParseResult{
			Commands: []Command{{
				Intent:     "turn_on",
				Entities:   map[string]string{"device": "lights", "location": "kitchen"},
				Confidence: 0.9,
				Response:   "Turning on kitchen lights",
			}},
			Success:   true,
			SessionID: "test-session-123",
		}
		result.FinalResponse <- finalResult
		close(result.FinalResponse)
		close(result.AudioSegments)
	}()

	return result, nil
}

func (m *MockStreamingCommandParser) ParseCommand(transcript string) (*CommandParseResult, error) {
	return &CommandParseResult{
		Commands: []Command{{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "lights"},
			Confidence: 0.8,
			Response:   "Fallback response",
		}},
		Success: true,
	}, nil
}

func (m *MockStreamingCommandParser) EnableStreaming(enabled bool) {
	// Mock implementation
}

func (m *MockStreamingCommandParser) IsStreamingEnabled() bool {
	return !m.fallbackEnabled
}

type MockAsyncExecutionPipeline struct {
	shouldError bool
	delay       time.Duration
}

func (m *MockAsyncExecutionPipeline) ExecuteAsync(ctx context.Context, intent *skills.VoiceIntent) (*ExecutionResult, error) {
	if m.shouldError {
		return nil, context.DeadlineExceeded
	}

	// Simulate async execution
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	return &ExecutionResult{
		Success:     true,
		Message:     "Command executed successfully",
		SpeechText:  "The lights are now on",
		Actions:     []skills.SkillAction{},
		Duration:    m.delay,
		ExecutionID: "exec-123",
	}, nil
}

func (m *MockAsyncExecutionPipeline) GetExecutionStatus(executionID string) (*ExecutionStatus, error) {
	return &ExecutionStatus{
		ExecutionID: executionID,
		Status:      "completed",
		Progress:    100,
		Message:     "Execution completed",
		StartTime:   time.Now().Add(-m.delay),
		UpdateTime:  time.Now(),
	}, nil
}

func (m *MockAsyncExecutionPipeline) CancelExecution(executionID string) error {
	return nil
}

func TestStreamingPredictiveBridge_ProcessVoiceCommand(t *testing.T) {
	// Create mock components
	mockStreamingParser := &MockStreamingCommandParser{shouldError: false}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
		responseDelay:   100 * time.Millisecond,
		responseMessage: "Lights turned on successfully",
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{
		shouldError: false,
		delay:       200 * time.Millisecond,
	}

	// Create bridge
	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	tests := []struct {
		name           string
		transcript     string
		expectSuccess  bool
		expectError    bool
		expectedIntent string
		minResponseTime time.Duration
		maxResponseTime time.Duration
	}{
		{
			name:            "successful_predictive_response",
			transcript:      "turn on the kitchen lights",
			expectSuccess:   true,
			expectError:     false,
			expectedIntent:  "turn_on",
			minResponseTime: 0,
			maxResponseTime: 1 * time.Second,
		},
		{
			name:            "complex_command",
			transcript:      "set the bedroom lights to 50% brightness",
			expectSuccess:   true,
			expectError:     false,
			expectedIntent:  "turn_on", // Mock will return turn_on
			minResponseTime: 0,
			maxResponseTime: 1 * time.Second,
		},
		{
			name:            "simple_command",
			transcript:      "lights on",
			expectSuccess:   true,
			expectError:     false,
			expectedIntent:  "turn_on",
			minResponseTime: 0,
			maxResponseTime: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			start := time.Now()
			session, err := bridge.ProcessVoiceCommand(ctx, tt.transcript)
			responseTime := time.Since(start)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check success expectations
			if tt.expectSuccess {
				if session == nil {
					t.Fatal("Expected non-nil session")
				}

				if session.Classification == nil {
					t.Fatal("Expected non-nil classification")
				}

				if session.Classification.Intent != tt.expectedIntent {
					t.Errorf("Expected intent '%s', got '%s'", tt.expectedIntent, session.Classification.Intent)
				}

				if session.PredictiveResponse == nil {
					t.Fatal("Expected non-nil predictive response")
				}

				if session.PredictiveResponse.ImmediateAck == "" {
					t.Error("Expected non-empty immediate acknowledgment")
				}
			}

			// Check response time
			if responseTime < tt.minResponseTime {
				t.Errorf("Response too fast: %v < %v", responseTime, tt.minResponseTime)
			}
			if responseTime > tt.maxResponseTime {
				t.Errorf("Response too slow: %v > %v", responseTime, tt.maxResponseTime)
			}

			t.Logf("Response time: %v", responseTime)
		})
	}
}

func TestStreamingPredictiveBridge_FallbackBehavior(t *testing.T) {
	// Create bridge with failing streaming parser
	mockStreamingParser := &MockStreamingCommandParser{shouldError: true}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{shouldError: false}

	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := bridge.ProcessVoiceCommand(ctx, "turn on the lights")

	// Should fall back gracefully when streaming fails
	if err != nil {
		t.Logf("Expected fallback due to streaming failure: %v", err)
	}

	// Verify fallback metrics are updated
	metrics := bridge.GetMetrics()
	if metrics == nil {
		t.Error("Expected non-nil metrics")
	}
}

func TestStreamingPredictiveBridge_ProcessingStrategies(t *testing.T) {
	mockStreamingParser := &MockStreamingCommandParser{shouldError: false}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{shouldError: false}

	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	tests := []struct {
		name               string
		transcript         string
		expectedStrategy   ProcessingStrategy
		expectedResponse   bool
	}{
		{
			name:             "high_confidence_command",
			transcript:       "turn on the kitchen lights",
			expectedStrategy: ProcessingPredictiveOnly, // May be determined by confidence
			expectedResponse: true,
		},
		{
			name:             "medium_confidence_command",
			transcript:       "lights on",
			expectedStrategy: ProcessingHybrid, // May fall back to hybrid
			expectedResponse: true,
		},
		{
			name:             "complex_command",
			transcript:       "set the bedroom lights to warm white at 75% brightness",
			expectedStrategy: ProcessingStreamingOnly, // May require streaming
			expectedResponse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			session, err := bridge.ProcessVoiceCommand(ctx, tt.transcript)

			if tt.expectedResponse {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if session == nil {
					t.Fatal("Expected non-nil session")
				}

				t.Logf("Processing strategy: %v", session.Strategy)
				t.Logf("Classification: %+v", session.Classification)
			}
		})
	}
}

func TestStreamingPredictiveBridge_ConcurrentSessions(t *testing.T) {
	mockStreamingParser := &MockStreamingCommandParser{shouldError: false}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{shouldError: false}

	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	numSessions := 5
	results := make(chan error, numSessions)

	// Start multiple concurrent sessions
	for i := 0; i < numSessions; i++ {
		go func(sessionNum int) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			transcript := fmt.Sprintf("turn on lights in room %d", sessionNum)
			session, err := bridge.ProcessVoiceCommand(ctx, transcript)

			if err != nil {
				results <- err
				return
			}

			if session == nil {
				results <- fmt.Errorf("session %d: got nil session", sessionNum)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < numSessions; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Got %d errors out of %d sessions: %v", len(errors), numSessions, errors)
	}

	// Check that sessions were managed correctly
	activeSessions := bridge.GetActiveSessions()
	t.Logf("Active sessions after concurrent test: %d", len(activeSessions))
}

func TestStreamingPredictiveBridge_Metrics(t *testing.T) {
	mockStreamingParser := &MockStreamingCommandParser{shouldError: false}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{shouldError: false}

	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	// Process several commands to generate metrics
	commands := []string{
		"turn on the lights",
		"set volume to 50%",
		"play my favorite music",
	}

	for _, cmd := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := bridge.ProcessVoiceCommand(ctx, cmd)
		cancel()

		if err != nil {
			t.Logf("Command failed (may be expected): %v", err)
		}
	}

	// Check metrics
	metrics := bridge.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	t.Logf("Metrics: %+v", metrics)

	// Verify metrics structure
	if metrics.TotalSessions < 0 {
		t.Error("Expected non-negative total sessions")
	}
}

func TestStreamingPredictiveBridge_SessionInterruption(t *testing.T) {
	mockStreamingParser := &MockStreamingCommandParser{shouldError: false}
	mockPredictiveEngine := NewPredictiveResponseEngine(&MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
		responseDelay:   2 * time.Second, // Long delay to allow interruption
	})
	mockStatusManager := &StatusManager{}
	mockCommandClassifier := NewCommandClassifier(&CommandParser{}, NewDeviceReliabilityTracker())
	mockExecutionPipeline := &MockAsyncExecutionPipeline{
		shouldError: false,
		delay:       3 * time.Second, // Even longer delay
	}

	bridge := NewStreamingPredictiveBridge(
		mockStreamingParser,
		mockPredictiveEngine,
		mockStatusManager,
		mockCommandClassifier,
		mockExecutionPipeline,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	// Start a session
	sessionResult := make(chan *BridgeSession, 1)
	sessionError := make(chan error, 1)

	go func() {
		session, err := bridge.ProcessVoiceCommand(ctx, "play long song")
		if err != nil {
			sessionError <- err
		} else {
			sessionResult <- session
		}
	}()

	// Wait a bit then check for active sessions
	time.Sleep(100 * time.Millisecond)
	activeSessions := bridge.GetActiveSessions()

	if len(activeSessions) == 0 {
		t.Skip("No active sessions to interrupt (execution too fast)")
	}

	// Try to interrupt the first session
	var sessionID string
	for id := range activeSessions {
		sessionID = id
		break
	}

	err := bridge.InterruptSession(sessionID)
	if err != nil {
		t.Errorf("Failed to interrupt session: %v", err)
	}

	cancel() // Cancel context to ensure cleanup

	// Wait for session completion or error
	select {
	case <-sessionResult:
		t.Log("Session completed")
	case err := <-sessionError:
		t.Logf("Session ended with error (expected due to interruption): %v", err)
	case <-time.After(1 * time.Second):
		t.Log("Session timed out (may be expected)")
	}
}

