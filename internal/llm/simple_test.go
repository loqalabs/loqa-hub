/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 */

package llm

import (
	"testing"
	"time"
)

func TestPredictiveTypes(t *testing.T) {
	// Test that our types are properly defined
	var responseType PredictiveType = PredictiveOptimistic
	if responseType != "optimistic" {
		t.Errorf("Expected 'optimistic', got %s", responseType)
	}

	var updateStrategy UpdateStrategy = UpdateSilent
	if updateStrategy != "silent" {
		t.Errorf("Expected 'silent', got %s", updateStrategy)
	}

	var intentCategory IntentCategory = CategoryInformation
	if intentCategory != "information" {
		t.Errorf("Expected 'information', got %s", intentCategory)
	}
}

func TestDeviceReliabilityTracker(t *testing.T) {
	tracker := NewDeviceReliabilityTracker()

	deviceID := "test_device"

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
}

func TestIntentCategoryExtraction(t *testing.T) {
	tracker := NewDeviceReliabilityTracker()
	mockParser := &CommandParser{
		ollamaURL: "http://localhost:11434",
		model:     "test-model",
	}
	classifier := NewCommandClassifier(mockParser, tracker)

	tests := []struct {
		intent   string
		expected IntentCategory
	}{
		{"what is the weather today", CategoryWeather},
		{"play some music", CategoryEntertainment},
		{"turn on the lights", CategorySmartHome},
		{"send a message to john", CategoryCommunication},
		{"set a reminder for 3pm", CategoryProductivity},
		{"what is 2+2", CategoryInformation},
		{"tell me the news", CategoryNews},
	}

	for _, tt := range tests {
		t.Run(tt.intent, func(t *testing.T) {
			result := classifier.extractIntentCategory(tt.intent, map[string]string{})
			if result != tt.expected {
				t.Errorf("for intent '%s', expected %s, got %s", tt.intent, tt.expected, result)
			}
		})
	}
}