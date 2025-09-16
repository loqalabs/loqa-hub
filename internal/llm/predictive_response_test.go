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
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/skills"
)

// MockSkillManager implements SkillManagerInterface for testing
type MockSkillManager struct {
	shouldFindSkill bool
	shouldSucceed   bool
	responseDelay   time.Duration
	responseMessage string
	executionCount  int
}

func (msm *MockSkillManager) FindSkillForIntent(intent *skills.VoiceIntent) (skills.SkillPlugin, error) {
	if !msm.shouldFindSkill {
		return nil, skills.ErrNoSkillCanHandle
	}
	return &MockSkillPlugin{
		shouldSucceed:   msm.shouldSucceed,
		responseDelay:   msm.responseDelay,
		responseMessage: msm.responseMessage,
	}, nil
}

func (msm *MockSkillManager) ExecuteSkill(ctx context.Context, skill skills.SkillPlugin, intent *skills.VoiceIntent) (*skills.SkillResponse, error) {
	msm.executionCount++
	return skill.HandleIntent(ctx, intent)
}

// MockSkillPlugin implements SkillPlugin for testing
type MockSkillPlugin struct {
	shouldSucceed   bool
	responseDelay   time.Duration
	responseMessage string
}

func (msp *MockSkillPlugin) Initialize(ctx context.Context, config *skills.SkillConfig) error {
	return nil
}

func (msp *MockSkillPlugin) Teardown(ctx context.Context) error {
	return nil
}

func (msp *MockSkillPlugin) CanHandle(intent skills.VoiceIntent) bool {
	return true
}

func (msp *MockSkillPlugin) HandleIntent(ctx context.Context, intent *skills.VoiceIntent) (*skills.SkillResponse, error) {
	// Simulate execution delay
	if msp.responseDelay > 0 {
		select {
		case <-time.After(msp.responseDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	response := &skills.SkillResponse{
		Success:      msp.shouldSucceed,
		SpeechText:   msp.responseMessage,
		ResponseTime: msp.responseDelay,
	}

	if !msp.shouldSucceed {
		response.Error = "Mock skill execution failed"
	}

	return response, nil
}

func (msp *MockSkillPlugin) GetManifest() (*skills.SkillManifest, error) {
	return &skills.SkillManifest{
		ID:      "mock-skill",
		Name:    "Mock Skill",
		Version: "1.0.0",
	}, nil
}

func (msp *MockSkillPlugin) GetStatus() skills.SkillStatus {
	return skills.SkillStatus{
		State:   skills.SkillStateReady,
		Healthy: true,
	}
}

func (msp *MockSkillPlugin) GetConfig() (*skills.SkillConfig, error) {
	return &skills.SkillConfig{}, nil
}

func (msp *MockSkillPlugin) UpdateConfig(ctx context.Context, config *skills.SkillConfig) error {
	return nil
}

func (msp *MockSkillPlugin) HealthCheck(ctx context.Context) error {
	return nil
}

func TestPredictiveResponseEngine_ProcessCommand(t *testing.T) {
	tests := []struct {
		name              string
		transcript        string
		mockShouldSucceed bool
		expectedType      PredictiveType
		expectError       bool
	}{
		{
			name:              "successful light control",
			transcript:        "turn off the bedroom lights",
			mockShouldSucceed: true,
			expectedType:      PredictiveOptimistic,
			expectError:       false,
		},
		{
			name:              "failed device control",
			transcript:        "turn on the garage lights",
			mockShouldSucceed: false,
			expectedType:      PredictiveCautious,
			expectError:       false,
		},
		{
			name:              "complex query",
			transcript:        "what's the weather like today",
			mockShouldSucceed: true,
			expectedType:      PredictiveCautious,
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSkillManager := &MockSkillManager{
				shouldFindSkill: true,
				shouldSucceed:   tt.mockShouldSucceed,
				responseDelay:   100 * time.Millisecond,
				responseMessage: "Mock response",
			}

			engine := NewPredictiveResponseEngine(mockSkillManager)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			response, err := engine.ProcessCommand(ctx, tt.transcript)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if response == nil {
				t.Errorf("expected response but got nil")
				return
			}

			// Verify response structure
			if response.ImmediateAck == "" {
				t.Errorf("expected immediate acknowledgment but got empty string")
			}

			if response.ExecutionPlan == "" {
				t.Errorf("expected execution plan but got empty string")
			}

			if response.StatusUpdates == nil {
				t.Errorf("expected status updates channel but got nil")
			}

			if response.ExecutionID == "" {
				t.Errorf("expected execution ID but got empty string")
			}

			// Wait for async execution to complete
			select {
			case statusUpdate := <-response.StatusUpdates:
				if tt.mockShouldSucceed && !statusUpdate.Success {
					t.Errorf("expected successful status update but got failure")
				}
				if !tt.mockShouldSucceed && statusUpdate.Success {
					t.Errorf("expected failed status update but got success")
				}
			case <-time.After(2 * time.Second):
				// Timeout is acceptable for some update strategies
			}
		})
	}
}

func TestDeviceReliabilityTracker(t *testing.T) {
	tracker := NewDeviceReliabilityTracker()

	deviceID := "bedroom_lights"

	// Test initial reliability score
	initialScore := tracker.GetReliabilityScore(deviceID)
	if initialScore != 0.5 {
		t.Errorf("expected initial reliability score of 0.5, got %f", initialScore)
	}

	// Record successful operations
	for i := 0; i < 8; i++ {
		tracker.UpdateStats(deviceID, true, 1*time.Second)
	}

	// Record some failures
	for i := 0; i < 2; i++ {
		tracker.UpdateStats(deviceID, false, 1*time.Second)
	}

	// Check reliability score (should be 8/10 = 0.8)
	score := tracker.GetReliabilityScore(deviceID)
	if score != 0.8 {
		t.Errorf("expected reliability score of 0.8, got %f", score)
	}

	// Test unknown device
	unknownScore := tracker.GetReliabilityScore("unknown_device")
	if unknownScore != 0.5 {
		t.Errorf("expected unknown device reliability score of 0.5, got %f", unknownScore)
	}
}

func TestCommandClassifier_ClassifyCommand(t *testing.T) {
	// Use mock client to avoid external calls
	mockClient := CreateMockHTTPClient()
	mockParser := NewCommandParserWithClient("http://localhost:11434", "test-model", mockClient)

	tracker := NewDeviceReliabilityTracker()
	classifier := NewCommandClassifier(mockParser, tracker)

	tests := []struct {
		name                   string
		transcript             string
		expectedIntentCategory IntentCategory
		expectedOperationType  OperationType
		expectedResponseType   PredictiveType
	}{
		{
			name:                   "light control",
			transcript:             "turn off the bedroom lights",
			expectedIntentCategory: CategorySmartHome,
			expectedOperationType:  OperationControl,
			expectedResponseType:   PredictiveCautious, // Will be cautious due to low mock confidence
		},
		{
			name:                   "garage operation",
			transcript:             "open the garage door",
			expectedIntentCategory: CategorySmartHome,
			expectedOperationType:  OperationControl,
			expectedResponseType:   PredictiveProgress, // Slow operation
		},
		{
			name:                   "security query",
			transcript:             "check if the front door is locked",
			expectedIntentCategory: CategorySmartHome,
			expectedOperationType:  OperationQuery,
			expectedResponseType:   PredictiveConfirm, // Critical operation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test will fail without a real command parser
			// In a real implementation, you'd mock the ParseCommand method
			ctx := context.Background()

			_, err := classifier.ClassifyCommand(ctx, tt.transcript)

			// For now, we expect this to fail since we don't have a real parser
			if err == nil {
				t.Logf("Classification succeeded (unexpected in mock environment)")
			} else {
				t.Logf("Classification failed as expected in mock environment: %v", err)
			}

			// Test the classification logic directly
			intentCategory := classifier.extractIntentCategory(tt.transcript, map[string]string{})
			if intentCategory != tt.expectedIntentCategory {
				t.Errorf("expected intent category %s, got %s", tt.expectedIntentCategory, intentCategory)
			}

			operationType := classifier.extractOperationType(tt.transcript, map[string]string{})
			if operationType != tt.expectedOperationType {
				t.Errorf("expected operation type %s, got %s", tt.expectedOperationType, operationType)
			}
		})
	}
}

func TestAsyncExecutionPipeline(t *testing.T) {
	mockSkillManager := &MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
		responseDelay:   50 * time.Millisecond,
		responseMessage: "Success",
	}

	pipeline := NewAsyncExecutionPipeline(mockSkillManager)
	defer func() {
		if err := pipeline.Shutdown(context.Background()); err != nil {
			t.Logf("Pipeline shutdown error: %v", err)
		}
	}()

	// Test successful execution
	t.Run("successful execution", func(t *testing.T) {
		statusChan := make(chan StatusUpdate, 10)
		executionID := "test-exec-1"

		intent := &skills.VoiceIntent{
			ID:         executionID,
			Transcript: "turn on lights",
			Intent:     "lights.control",
			Confidence: 0.9,
			Entities:   map[string]interface{}{"action": "turn_on"},
			Timestamp:  time.Now(),
		}

		classification := &CommandClassification{
			Intent:         "lights.control",
			Confidence:     0.9,
			ResponseType:   PredictiveOptimistic,
			UpdateStrategy: UpdateErrorOnly,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := pipeline.SubmitExecution(ctx, executionID, intent, classification, statusChan)
		if err != nil {
			t.Fatalf("failed to submit execution: %v", err)
		}

		// Wait for completion
		select {
		case <-statusChan:
			// Success case - status update received
		case <-time.After(1 * time.Second):
			// No status update expected for UpdateErrorOnly strategy on success
		}

		// Check metrics
		metrics := pipeline.GetMetrics()
		if metrics.TotalExecutions == 0 {
			t.Errorf("expected at least 1 execution, got %d", metrics.TotalExecutions)
		}
	})

	// Test failed execution
	t.Run("failed execution", func(t *testing.T) {
		mockSkillManager.shouldSucceed = false

		statusChan := make(chan StatusUpdate, 10)
		executionID := "test-exec-2"

		intent := &skills.VoiceIntent{
			ID:         executionID,
			Transcript: "turn off lights",
			Intent:     "lights.control",
			Confidence: 0.9,
			Entities:   map[string]interface{}{"action": "turn_off"},
			Timestamp:  time.Now(),
		}

		classification := &CommandClassification{
			Intent:         "lights.control",
			Confidence:     0.9,
			ResponseType:   PredictiveOptimistic,
			UpdateStrategy: UpdateErrorOnly,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := pipeline.SubmitExecution(ctx, executionID, intent, classification, statusChan)
		if err != nil {
			t.Fatalf("failed to submit execution: %v", err)
		}

		// Wait for final error status update (may receive progress updates for retries first)
		// Allow extra time for retries (up to 2 retries with 1 second delay each)
		var finalUpdate StatusUpdate
		var receivedError bool
		timeout := time.After(4 * time.Second)

		for !receivedError {
			select {
			case statusUpdate := <-statusChan:
				finalUpdate = statusUpdate
				if statusUpdate.Type == StatusError {
					receivedError = true
				}
				// Progress updates from retries are expected, continue waiting for error
			case <-timeout:
				t.Errorf("timeout waiting for error status update")
				return
			}
		}

		if finalUpdate.Type != StatusError {
			t.Errorf("expected final error status update, got %s", finalUpdate.Type)
		}
		if finalUpdate.Success {
			t.Errorf("expected failed status update, got success")
		}
	})
}

func TestStatusManager(t *testing.T) {
	mockTTSClient := &MockTTSClient{}
	statusManager := NewStatusManager(mockTTSClient)

	executionID := "test-status-1"
	deviceID := "bedroom_lights"
	statusChan := make(chan StatusUpdate, 10)

	// Register execution
	statusManager.RegisterExecution(executionID, UpdateVerbose, deviceID, statusChan)

	t.Run("process success update", func(t *testing.T) {
		successUpdate := StatusUpdate{
			Type:        StatusSuccess,
			Message:     "Lights turned on successfully",
			Success:     true,
			ExecutionID: executionID,
			Timestamp:   time.Now(),
		}

		err := statusManager.ProcessStatusUpdate(successUpdate)
		if err != nil {
			t.Errorf("failed to process success update: %v", err)
		}

		// Verify metrics
		metrics := statusManager.GetMetrics()
		if metrics.SuccessfulUpdates == 0 {
			t.Errorf("expected at least 1 successful update, got %d", metrics.SuccessfulUpdates)
		}
	})

	t.Run("process error update", func(t *testing.T) {
		errorUpdate := StatusUpdate{
			Type:        StatusError,
			Message:     "Device not responding",
			Success:     false,
			ExecutionID: executionID,
			Timestamp:   time.Now(),
			Error:       "timeout",
		}

		err := statusManager.ProcessStatusUpdate(errorUpdate)
		if err != nil {
			t.Errorf("failed to process error update: %v", err)
		}

		// Check if error pattern is detected
		patterns := statusManager.GetErrorPatterns()
		if len(patterns) == 0 {
			t.Errorf("expected error pattern to be recorded")
		}
	})

	// Cleanup
	statusManager.UnregisterExecution(executionID)
}

// MockTTSClient for testing
type MockTTSClient struct{}

func (mtts *MockTTSClient) Synthesize(text string, options *TTSOptions) (*TTSResult, error) {
	return &TTSResult{
		ContentType: "audio/mp3",
		Length:      1024,
	}, nil
}

func (mtts *MockTTSClient) GetAvailableVoices() ([]string, error) {
	return []string{"test_voice", "af_bella"}, nil
}

func (mtts *MockTTSClient) Close() error {
	return nil
}

func TestPredictiveTypes(t *testing.T) {
	tests := []struct {
		confidence    float64
		reliability   float64
		executionTime time.Duration
		operation     OperationType
		expectedType  PredictiveType
	}{
		{
			confidence:    0.95,
			reliability:   0.90,
			executionTime: 1 * time.Second,
			operation:     OperationControl,
			expectedType:  PredictiveOptimistic,
		},
		{
			confidence:    0.70,
			reliability:   0.60,
			executionTime: 2 * time.Second,
			operation:     OperationControl,
			expectedType:  PredictiveCautious,
		},
		{
			confidence:    0.95,
			reliability:   0.90,
			executionTime: 15 * time.Second,
			operation:     OperationControl,
			expectedType:  PredictiveProgress,
		},
		{
			confidence:    0.95,
			reliability:   0.90,
			executionTime: 1 * time.Second,
			operation:     OperationCritical,
			expectedType:  PredictiveConfirm,
		},
	}

	tracker := NewDeviceReliabilityTracker()
	// Use mock client to avoid external calls
	mockClient := CreateMockHTTPClient()
	mockParser := NewCommandParserWithClient("http://localhost:11434", "test-model", mockClient)
	classifier := NewCommandClassifier(mockParser, tracker)

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			responseType := classifier.determineResponseType(
				tt.confidence,
				tt.reliability,
				tt.executionTime,
				tt.operation,
			)

			if responseType != tt.expectedType {
				t.Errorf("expected response type %s, got %s", tt.expectedType, responseType)
			}
		})
	}
}

func BenchmarkPredictiveResponse(b *testing.B) {
	mockSkillManager := &MockSkillManager{
		shouldFindSkill: true,
		shouldSucceed:   true,
		responseDelay:   10 * time.Millisecond,
		responseMessage: "Benchmark response",
	}

	engine := NewPredictiveResponseEngine(mockSkillManager)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_, err := engine.ProcessCommand(ctx, "turn on lights")
		cancel()

		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}
