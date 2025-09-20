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

package intent

import (
	"context"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

func TestNewCascadeProcessor(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	ollamaURL := "http://localhost:11434"
	model := "llama3.2:3b"

	cp := NewCascadeProcessor(ollamaURL, model)

	// Verify configuration
	if cp.llmProcessor.ollamaURL != ollamaURL {
		t.Errorf("LLM ollamaURL = %s, want %s", cp.llmProcessor.ollamaURL, ollamaURL)
	}
	if cp.llmProcessor.model != model {
		t.Errorf("LLM model = %s, want %s", cp.llmProcessor.model, model)
	}
	if cp.reflexThreshold != 0.8 {
		t.Errorf("reflexThreshold = %f, want 0.8", cp.reflexThreshold)
	}
	if cp.llmThreshold != 0.7 {
		t.Errorf("llmThreshold = %f, want 0.7", cp.llmThreshold)
	}
	if cp.enableCloud != false {
		t.Errorf("enableCloud = %t, want false", cp.enableCloud)
	}
	if cp.cascadeTimeout != 15*time.Second {
		t.Errorf("cascadeTimeout = %v, want 15s", cp.cascadeTimeout)
	}

	// Verify processors are initialized
	if cp.reflexProcessor.patterns == nil {
		t.Error("Reflex processor patterns should be initialized")
	}
	if cp.llmProcessor.timeout != 5*time.Second {
		t.Errorf("LLM timeout = %v, want 5s", cp.llmProcessor.timeout)
	}
	if cp.cloudProcessor.enabled != false {
		t.Errorf("Cloud processor enabled = %t, want false", cp.cloudProcessor.enabled)
	}
}

func TestNewReflexProcessor(t *testing.T) {
	rp := NewReflexProcessor()

	// Verify patterns are initialized
	if rp.patterns == nil {
		t.Error("Patterns map should be initialized")
	}

	// Check that main intent types have patterns
	expectedIntents := []IntentType{
		IntentLightControl,
		IntentMediaControl,
		IntentVolumeControl,
		IntentTimeQuery,
		IntentWeatherQuery,
	}

	for _, intentType := range expectedIntents {
		patterns, exists := rp.patterns[intentType]
		if !exists {
			t.Errorf("Intent type %s should have patterns", intentType)
		}
		if len(patterns) == 0 {
			t.Errorf("Intent type %s should have at least one pattern", intentType)
		}
	}
}

func TestReflexProcessor_LightControl(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	rp := NewReflexProcessor()

	tests := []struct {
		name           string
		text           string
		expectedType   IntentType
		expectedAction string
		shouldMatch    bool
	}{
		{
			name:           "Turn on lights",
			text:           "turn on the lights",
			expectedType:   IntentLightControl,
			expectedAction: "on",
			shouldMatch:    true,
		},
		{
			name:           "Turn off lights",
			text:           "turn off lights",
			expectedType:   IntentLightControl,
			expectedAction: "off",
			shouldMatch:    true,
		},
		{
			name:           "Dim lights",
			text:           "dim the lights",
			expectedType:   IntentLightControl,
			expectedAction: "dim",
			shouldMatch:    true,
		},
		{
			name:           "Lights on",
			text:           "lights on",
			expectedType:   IntentLightControl,
			expectedAction: "on",
			shouldMatch:    true,
		},
		{
			name:        "No match",
			text:        "hello world",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := rp.ProcessIntent(context.Background(), tt.text)

			if tt.shouldMatch {
				if err != nil {
					t.Errorf("ProcessIntent() error = %v, want nil", err)
				}
				if intent == nil {
					t.Fatal("Intent should not be nil for matching text")
				}
				if intent.Type != tt.expectedType {
					t.Errorf("Intent type = %v, want %v", intent.Type, tt.expectedType)
				}
				if intent.Confidence != 0.95 {
					t.Errorf("Confidence = %f, want 0.95", intent.Confidence)
				}
				if intent.Source != ProcessorReflex {
					t.Errorf("Source = %v, want %v", intent.Source, ProcessorReflex)
				}
				if intent.RawText != tt.text {
					t.Errorf("RawText = %s, want %s", intent.RawText, tt.text)
				}

				// Check action entity
				if tt.expectedAction != "" {
					action, exists := intent.Entities["action"]
					if !exists {
						t.Error("Action entity should exist")
					} else if action != tt.expectedAction {
						t.Errorf("Action = %s, want %s", action, tt.expectedAction)
					}
				}
			} else {
				if err == nil {
					t.Error("ProcessIntent() should return error for non-matching text")
				}
				if intent != nil {
					t.Error("Intent should be nil for non-matching text")
				}
			}
		})
	}
}

func TestReflexProcessor_MediaControl(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	rp := NewReflexProcessor()

	tests := []struct {
		name           string
		text           string
		expectedAction string
		expectedTarget string
		shouldMatch    bool
	}{
		{
			name:           "Play music",
			text:           "play music",
			expectedAction: "play",
			shouldMatch:    true,
		},
		{
			name:           "Pause music",
			text:           "pause music",
			expectedAction: "pause",
			shouldMatch:    true,
		},
		{
			name:           "Next song",
			text:           "next song",
			expectedAction: "next",
			shouldMatch:    true,
		},
		{
			name:           "Play specific song",
			text:           "play bohemian rhapsody",
			expectedAction: "bohemian rhapsody", // This is captured in matches[1] by the pattern (?i)play\s+(.+)
			expectedTarget: "",                  // No target captured for this pattern
			shouldMatch:    true,
		},
		{
			name:        "No match",
			text:        "unrelated text here",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := rp.ProcessIntent(context.Background(), tt.text)

			if tt.shouldMatch {
				if err != nil {
					t.Errorf("ProcessIntent() error = %v, want nil", err)
				}
				if intent == nil {
					t.Fatal("Intent should not be nil for matching text")
				}
				if intent.Type != IntentMediaControl {
					t.Errorf("Intent type = %v, want %v", intent.Type, IntentMediaControl)
				}

				// Check action entity
				if tt.expectedAction != "" {
					action, exists := intent.Entities["action"]
					if !exists {
						t.Error("Action entity should exist")
					} else if action != tt.expectedAction {
						t.Errorf("Action = %s, want %s", action, tt.expectedAction)
					}
				}

				// Check target entity
				if tt.expectedTarget != "" {
					target, exists := intent.Entities["target"]
					if !exists {
						t.Error("Target entity should exist")
					} else if target != tt.expectedTarget {
						t.Errorf("Target = %s, want %s", target, tt.expectedTarget)
					}
				}
			} else {
				if err == nil {
					t.Error("ProcessIntent() should return error for non-matching text")
				}
			}
		})
	}
}

func TestReflexProcessor_VolumeControl(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	rp := NewReflexProcessor()

	tests := []struct {
		name           string
		text           string
		expectedAction string
		shouldMatch    bool
	}{
		{
			name:           "Volume up",
			text:           "volume up",
			expectedAction: "up",
			shouldMatch:    true,
		},
		{
			name:           "Volume down",
			text:           "volume down",
			expectedAction: "down",
			shouldMatch:    true,
		},
		{
			name:           "Increase volume",
			text:           "increase volume",
			expectedAction: "increase",
			shouldMatch:    true,
		},
		{
			name:           "Mute",
			text:           "mute",
			expectedAction: "mute",
			shouldMatch:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := rp.ProcessIntent(context.Background(), tt.text)

			if tt.shouldMatch {
				if err != nil {
					t.Errorf("ProcessIntent() error = %v, want nil", err)
				}
				if intent == nil {
					t.Fatal("Intent should not be nil for matching text")
				}
				if intent.Type != IntentVolumeControl {
					t.Errorf("Intent type = %v, want %v", intent.Type, IntentVolumeControl)
				}

				// Check action entity
				action, exists := intent.Entities["action"]
				if !exists {
					t.Error("Action entity should exist")
				} else if action != tt.expectedAction {
					t.Errorf("Action = %s, want %s", action, tt.expectedAction)
				}
			} else {
				if err == nil {
					t.Error("ProcessIntent() should return error for non-matching text")
				}
			}
		})
	}
}

func TestReflexProcessor_TimeQuery(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	rp := NewReflexProcessor()

	tests := []string{
		"what time is it",
		"current time",
		"tell me the time",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			intent, err := rp.ProcessIntent(context.Background(), text)

			if err != nil {
				t.Errorf("ProcessIntent() error = %v, want nil", err)
			}
			if intent == nil {
				t.Fatal("Intent should not be nil for time query")
			}
			if intent.Type != IntentTimeQuery {
				t.Errorf("Intent type = %v, want %v", intent.Type, IntentTimeQuery)
			}
		})
	}
}

func TestReflexProcessor_WeatherQuery(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	rp := NewReflexProcessor()

	tests := []string{
		"what's the weather",
		"weather forecast",
		"how's the weather",
		"is it raining",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			intent, err := rp.ProcessIntent(context.Background(), text)

			if err != nil {
				t.Errorf("ProcessIntent() error = %v, want nil", err)
			}
			if intent == nil {
				t.Fatal("Intent should not be nil for weather query")
			}
			if intent.Type != IntentWeatherQuery {
				t.Errorf("Intent type = %v, want %v", intent.Type, IntentWeatherQuery)
			}
		})
	}
}

func TestReflexProcessor_CanHandle(t *testing.T) {
	rp := NewReflexProcessor()

	tests := []struct {
		text         string
		shouldHandle bool
	}{
		{"turn on lights", true},
		{"play music", true},
		{"what time is it", true},
		{"volume up", true},
		{"random conversation", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			canHandle := rp.CanHandle(tt.text)
			if canHandle != tt.shouldHandle {
				t.Errorf("CanHandle(%s) = %t, want %t", tt.text, canHandle, tt.shouldHandle)
			}
		})
	}
}

func TestReflexProcessor_GetConfidenceThreshold(t *testing.T) {
	rp := NewReflexProcessor()
	threshold := rp.GetConfidenceThreshold()
	if threshold != 0.8 {
		t.Errorf("GetConfidenceThreshold() = %f, want 0.8", threshold)
	}
}

func TestLLMProcessor_ProcessIntent(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	lp := &LLMProcessor{
		ollamaURL: "http://localhost:11434",
		model:     "llama3.2:3b",
		timeout:   5 * time.Second,
	}

	// Test the current simplified implementation
	intent, err := lp.ProcessIntent(context.Background(), "tell me a joke")
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if intent.Type != IntentConversation {
		t.Errorf("Intent type = %v, want %v", intent.Type, IntentConversation)
	}
	if intent.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5", intent.Confidence)
	}
	if intent.Source != ProcessorLLM {
		t.Errorf("Source = %v, want %v", intent.Source, ProcessorLLM)
	}
}

func TestLLMProcessor_CanHandle(t *testing.T) {
	lp := &LLMProcessor{}

	// LLM processor should handle any text
	if !lp.CanHandle("any text") {
		t.Error("LLM processor should handle any text")
	}
	if !lp.CanHandle("") {
		t.Error("LLM processor should handle empty text")
	}
}

func TestLLMProcessor_GetConfidenceThreshold(t *testing.T) {
	lp := &LLMProcessor{}
	threshold := lp.GetConfidenceThreshold()
	if threshold != 0.7 {
		t.Errorf("GetConfidenceThreshold() = %f, want 0.7", threshold)
	}
}

func TestCloudProcessor_ProcessIntent_Disabled(t *testing.T) {
	cp := &CloudProcessor{enabled: false}

	intent, err := cp.ProcessIntent(context.Background(), "test")
	if err == nil {
		t.Error("ProcessIntent() should return error when disabled")
	}
	if intent != nil {
		t.Error("Intent should be nil when processor is disabled")
	}
}

func TestCloudProcessor_CanHandle(t *testing.T) {
	// Test disabled
	cp := &CloudProcessor{enabled: false}
	if cp.CanHandle("test") {
		t.Error("Disabled cloud processor should not handle any text")
	}

	// Test enabled
	cp.enabled = true
	if !cp.CanHandle("test") {
		t.Error("Enabled cloud processor should handle any text")
	}
}

func TestCloudProcessor_GetConfidenceThreshold(t *testing.T) {
	cp := &CloudProcessor{}
	threshold := cp.GetConfidenceThreshold()
	if threshold != 0.6 {
		t.Errorf("GetConfidenceThreshold() = %f, want 0.6", threshold)
	}
}

func TestCascadeProcessor_ProcessIntent_ReflexSuccess(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")

	// Test with text that should be caught by reflex processor
	intent, err := cp.ProcessIntent(context.Background(), "turn on lights")
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if intent.Type != IntentLightControl {
		t.Errorf("Intent type = %v, want %v", intent.Type, IntentLightControl)
	}
	if intent.Source != ProcessorReflex {
		t.Errorf("Source = %v, want %v", intent.Source, ProcessorReflex)
	}
	if intent.Confidence < 0.8 {
		t.Errorf("Confidence = %f, want >= 0.8", intent.Confidence)
	}
	if intent.ProcessingTime <= 0 {
		t.Error("ProcessingTime should be set")
	}
}

func TestCascadeProcessor_ProcessIntent_LLMFallback(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")

	// Lower the LLM threshold so the 0.5 confidence from LLM processor passes
	cp.llmThreshold = 0.3

	// Test with text that won't match reflex patterns but will go to LLM
	intent, err := cp.ProcessIntent(context.Background(), "tell me a joke")
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if intent.Source != ProcessorLLM {
		t.Errorf("Source = %v, want %v", intent.Source, ProcessorLLM)
	}
	if intent.ProcessingTime <= 0 {
		t.Error("ProcessingTime should be set")
	}
}

func TestCascadeProcessor_ProcessIntent_UnknownFallback(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")

	// Set high thresholds so both reflex and LLM fail
	cp.reflexThreshold = 0.99
	cp.llmThreshold = 0.99

	// Test with text that should fall through to unknown
	intent, err := cp.ProcessIntent(context.Background(), "some unrecognizable text")
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if intent.Type != IntentUnknown {
		t.Errorf("Intent type = %v, want %v", intent.Type, IntentUnknown)
	}
	if intent.Confidence != 0.0 {
		t.Errorf("Confidence = %f, want 0.0", intent.Confidence)
	}
	if intent.Source != ProcessorReflex {
		t.Errorf("Source = %v, want %v", intent.Source, ProcessorReflex)
	}
	if intent.ProcessingTime <= 0 {
		t.Error("ProcessingTime should be set")
	}
}

func TestCascadeProcessor_SetCloudEnabled(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")

	// Initially disabled
	if cp.enableCloud {
		t.Error("Cloud should be disabled initially")
	}
	if cp.cloudProcessor.enabled {
		t.Error("Cloud processor should be disabled initially")
	}

	// Enable cloud
	cp.SetCloudEnabled(true)
	if !cp.enableCloud {
		t.Error("enableCloud should be true after SetCloudEnabled(true)")
	}
	if !cp.cloudProcessor.enabled {
		t.Error("cloudProcessor.enabled should be true after SetCloudEnabled(true)")
	}

	// Disable cloud
	cp.SetCloudEnabled(false)
	if cp.enableCloud {
		t.Error("enableCloud should be false after SetCloudEnabled(false)")
	}
	if cp.cloudProcessor.enabled {
		t.Error("cloudProcessor.enabled should be false after SetCloudEnabled(false)")
	}
}

func TestCascadeProcessor_ProcessIntent_Timeout(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")
	cp.cascadeTimeout = 1 * time.Millisecond // Very short timeout

	// Create context that will timeout
	ctx := context.Background()
	intent, err := cp.ProcessIntent(ctx, "turn on lights")

	// Should still work because reflex processing is fast
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if intent.Type != IntentLightControl {
		t.Errorf("Intent type = %v, want %v", intent.Type, IntentLightControl)
	}
}

func TestCascadeProcessor_ProcessIntent_ContextCancellation(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cp := NewCascadeProcessor("http://localhost:11434", "llama3.2:3b")

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should still work for reflex patterns since they're immediate
	intent, err := cp.ProcessIntent(ctx, "turn on lights")
	if err != nil {
		t.Errorf("ProcessIntent() error = %v, want nil", err)
	}
	if intent == nil {
		t.Fatal("Intent should not be nil")
	}
}
