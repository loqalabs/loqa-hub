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

package arbitration

import (
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/transport"
)

func TestNewArbitrator(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Verify default configuration
	if arbitrator.confidenceWeight != 0.5 {
		t.Errorf("confidenceWeight = %f, want 0.5", arbitrator.confidenceWeight)
	}
	if arbitrator.snrWeight != 0.3 {
		t.Errorf("snrWeight = %f, want 0.3", arbitrator.snrWeight)
	}
	if arbitrator.proximityWeight != 0.2 {
		t.Errorf("proximityWeight = %f, want 0.2", arbitrator.proximityWeight)
	}
	if arbitrator.minConfidence != 0.3 {
		t.Errorf("minConfidence = %f, want 0.3", arbitrator.minConfidence)
	}
	if arbitrator.maxWindowExtension != 200*time.Millisecond {
		t.Errorf("maxWindowExtension = %v, want 200ms", arbitrator.maxWindowExtension)
	}

	// Verify initial state
	if arbitrator.activeWindow != nil {
		t.Error("activeWindow should be nil initially")
	}
	if arbitrator.pendingDetections != nil {
		t.Error("pendingDetections should be nil initially")
	}
	if arbitrator.stats.ConfidenceDistribution == nil {
		t.Error("ConfidenceDistribution should be initialized")
	}
}

func TestProcessWakeWordDetection_SinglePuck(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Set up callback to capture result
	var result *ArbitrationResult
	var wg sync.WaitGroup
	wg.Add(1)

	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		result = r
		wg.Done()
	})

	// Create a detection
	detection := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.85,
		Timestamp:  time.Now(),
		AudioLevel: 0.8,
		NoiseLevel: 0.2,
	}

	// Process detection
	err := arbitrator.ProcessWakeWordDetection(detection)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection() error = %v", err)
	}

	// Wait for arbitration to complete (window expires)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(ArbitrationWindowDuration + 100*time.Millisecond):
		t.Fatal("Arbitration did not complete within expected time")
	}

	// Verify result
	if result == nil {
		t.Fatal("No arbitration result received")
	}
	if result.WinnerPuckID != "puck-001" {
		t.Errorf("WinnerPuckID = %s, want puck-001", result.WinnerPuckID)
	}
	if len(result.AllDetections) != 1 {
		t.Errorf("AllDetections length = %d, want 1", len(result.AllDetections))
	}
	if result.DecisionReason != "Single puck detection" {
		t.Errorf("DecisionReason = %s, want 'Single puck detection'", result.DecisionReason)
	}
}

func TestProcessWakeWordDetection_MultiPuck(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Set up callback to capture result
	var result *ArbitrationResult
	var wg sync.WaitGroup
	wg.Add(1)

	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		result = r
		wg.Done()
	})

	// Create detections from multiple pucks
	detection1 := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.75,
		Timestamp:  time.Now(),
		AudioLevel: 0.6,
		NoiseLevel: 0.3,
	}

	detection2 := WakeWordDetection{
		PuckID:     "puck-002",
		SessionID:  "session-123",
		Confidence: 0.85, // Higher confidence
		Timestamp:  time.Now(),
		AudioLevel: 0.9,
		NoiseLevel: 0.1, // Better SNR
	}

	// Process first detection
	err := arbitrator.ProcessWakeWordDetection(detection1)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(1) error = %v", err)
	}

	// Process second detection within window
	time.Sleep(50 * time.Millisecond)
	err = arbitrator.ProcessWakeWordDetection(detection2)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(2) error = %v", err)
	}

	// Wait for arbitration to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(ArbitrationWindowDuration + 200*time.Millisecond):
		t.Fatal("Arbitration did not complete within expected time")
	}

	// Verify result - puck-002 should win due to higher confidence and better SNR
	if result == nil {
		t.Fatal("No arbitration result received")
	}
	if result.WinnerPuckID != "puck-002" {
		t.Errorf("WinnerPuckID = %s, want puck-002", result.WinnerPuckID)
	}
	if len(result.AllDetections) != 2 {
		t.Errorf("AllDetections length = %d, want 2", len(result.AllDetections))
	}
	if result.WinnerScore <= 0 {
		t.Errorf("WinnerScore = %f, want > 0", result.WinnerScore)
	}
}

func TestProcessWakeWordDetection_BelowMinConfidence(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Set up callback - should not be called
	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		t.Error("Arbitration callback should not be called for low confidence detection")
	})

	// Create detection below minimum confidence
	detection := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.2, // Below 0.3 minimum
		Timestamp:  time.Now(),
		AudioLevel: 0.5,
		NoiseLevel: 0.3,
	}

	// Process detection
	err := arbitrator.ProcessWakeWordDetection(detection)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection() error = %v", err)
	}

	// Wait to ensure no arbitration starts
	time.Sleep(100 * time.Millisecond)

	// Verify no active window
	arbitrator.mutex.RLock()
	if arbitrator.activeWindow != nil {
		t.Error("activeWindow should be nil for low confidence detection")
	}
	arbitrator.mutex.RUnlock()
}

func TestProcessWakeWordDetection_DuplicateFromSamePuck(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Set up callback to capture result
	var result *ArbitrationResult
	var wg sync.WaitGroup
	wg.Add(1)

	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		result = r
		wg.Done()
	})

	// Create initial detection
	detection1 := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.6,
		Timestamp:  time.Now(),
		AudioLevel: 0.5,
		NoiseLevel: 0.3,
	}

	// Create higher confidence detection from same puck
	detection2 := WakeWordDetection{
		PuckID:     "puck-001", // Same puck
		SessionID:  "session-123",
		Confidence: 0.8, // Higher confidence
		Timestamp:  time.Now(),
		AudioLevel: 0.7,
		NoiseLevel: 0.2,
	}

	// Process first detection
	err := arbitrator.ProcessWakeWordDetection(detection1)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(1) error = %v", err)
	}

	// Process second detection (should replace first)
	time.Sleep(50 * time.Millisecond)
	err = arbitrator.ProcessWakeWordDetection(detection2)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(2) error = %v", err)
	}

	// Wait for arbitration to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(ArbitrationWindowDuration + 200*time.Millisecond):
		t.Fatal("Arbitration did not complete within expected time")
	}

	// Verify only one detection and it's the higher confidence one
	if result == nil {
		t.Fatal("No arbitration result received")
	}
	if len(result.AllDetections) != 1 {
		t.Errorf("AllDetections length = %d, want 1", len(result.AllDetections))
	}
	if result.AllDetections[0].Confidence != 0.8 {
		t.Errorf("Detection confidence = %f, want 0.8", result.AllDetections[0].Confidence)
	}
}

func TestCalculateScore(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	tests := []struct {
		name      string
		detection WakeWordDetection
		wantMin   float64 // Minimum expected score
		wantMax   float64 // Maximum expected score
	}{
		{
			name: "High confidence, good SNR",
			detection: WakeWordDetection{
				PuckID:     "puck-001",
				Confidence: 0.9,
				AudioLevel: 0.8,
				NoiseLevel: 0.1, // Good SNR = 8.0
			},
			wantMin: 0.6,
			wantMax: 1.0,
		},
		{
			name: "Medium confidence, average SNR",
			detection: WakeWordDetection{
				PuckID:     "puck-002",
				Confidence: 0.6,
				AudioLevel: 0.5,
				NoiseLevel: 0.25, // SNR = 2.0
			},
			wantMin: 0.3,
			wantMax: 0.7,
		},
		{
			name: "Low confidence, poor SNR",
			detection: WakeWordDetection{
				PuckID:     "puck-003",
				Confidence: 0.4,
				AudioLevel: 0.3,
				NoiseLevel: 0.5, // Poor SNR = 0.6
			},
			wantMin: 0.1,
			wantMax: 0.4,
		},
		{
			name: "With proximity hint",
			detection: WakeWordDetection{
				PuckID:       "puck-004",
				Confidence:   0.7,
				AudioLevel:   0.6,
				NoiseLevel:   0.2,
				DistanceHint: 1.0, // Close proximity
			},
			wantMin: 0.4,
			wantMax: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := arbitrator.calculateScore(tt.detection)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculateScore() = %f, want between %f and %f", score, tt.wantMin, tt.wantMax)
			}

			// Score should always be positive
			if score < 0 {
				t.Errorf("calculateScore() = %f, want >= 0", score)
			}
		})
	}
}

func TestNormalizeScore(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{"Below minimum", 0.0, 0.5, 10.0, 0.0},
		{"At minimum", 0.5, 0.5, 10.0, 0.0},
		{"Middle value", 5.25, 0.5, 10.0, 0.5},
		{"At maximum", 10.0, 0.5, 10.0, 1.0},
		{"Above maximum", 15.0, 0.5, 10.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeScore(tt.value, tt.min, tt.max)
			if result != tt.expected {
				t.Errorf("normalizeScore(%f, %f, %f) = %f, want %f",
					tt.value, tt.min, tt.max, result, tt.expected)
			}
		})
	}
}

func TestGetDecisionReason(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	tests := []struct {
		name     string
		scored   []ScoredDetection
		winner   ScoredDetection
		expected string
	}{
		{
			name: "Single puck",
			scored: []ScoredDetection{
				{
					Detection: WakeWordDetection{PuckID: "puck-001", Confidence: 0.8},
					Score:     0.7,
				},
			},
			winner: ScoredDetection{
				Detection: WakeWordDetection{PuckID: "puck-001", Confidence: 0.8},
				Score:     0.7,
			},
			expected: "Single puck detection",
		},
		{
			name: "High confidence winner",
			scored: []ScoredDetection{
				{
					Detection: WakeWordDetection{PuckID: "puck-001", Confidence: 0.95},
					Score:     0.8,
				},
				{
					Detection: WakeWordDetection{PuckID: "puck-002", Confidence: 0.7},
					Score:     0.6,
				},
			},
			winner: ScoredDetection{
				Detection: WakeWordDetection{PuckID: "puck-001", Confidence: 0.95},
				Score:     0.8,
			},
			expected: "High confidence detection",
		},
		{
			name: "Superior audio quality",
			scored: []ScoredDetection{
				{
					Detection: WakeWordDetection{
						PuckID:     "puck-001",
						Confidence: 0.7,
						AudioLevel: 0.9,
						NoiseLevel: 0.1, // SNR = 9.0
					},
					Score: 0.75,
				},
				{
					Detection: WakeWordDetection{
						PuckID:     "puck-002",
						Confidence: 0.8,
						AudioLevel: 0.4,
						NoiseLevel: 0.2, // SNR = 2.0 (much lower)
					},
					Score: 0.7,
				},
			},
			winner: ScoredDetection{
				Detection: WakeWordDetection{
					PuckID:     "puck-001",
					Confidence: 0.7,
					AudioLevel: 0.9,
					NoiseLevel: 0.1,
				},
				Score: 0.75,
			},
			expected: "Superior audio quality",
		},
		{
			name: "Best overall score",
			scored: []ScoredDetection{
				{
					Detection: WakeWordDetection{
						PuckID:     "puck-001",
						Confidence: 0.75,
						AudioLevel: 0.6,
						NoiseLevel: 0.3,
					},
					Score: 0.65,
				},
				{
					Detection: WakeWordDetection{
						PuckID:     "puck-002",
						Confidence: 0.7,
						AudioLevel: 0.5,
						NoiseLevel: 0.25,
					},
					Score: 0.6,
				},
			},
			winner: ScoredDetection{
				Detection: WakeWordDetection{
					PuckID:     "puck-001",
					Confidence: 0.75,
					AudioLevel: 0.6,
					NoiseLevel: 0.3,
				},
				Score: 0.65,
			},
			expected: "Best overall score",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := arbitrator.getDecisionReason(tt.scored, tt.winner)
			if reason != tt.expected {
				t.Errorf("getDecisionReason() = %s, want %s", reason, tt.expected)
			}
		})
	}
}

func TestArbitrationStats(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Process multiple arbitrations
	var results []*ArbitrationResult
	var wg sync.WaitGroup

	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		results = append(results, r)
		wg.Done()
	})

	// Single puck arbitration
	wg.Add(1)
	detection1 := WakeWordDetection{
		PuckID:     "puck-001",
		Confidence: 0.8,
		Timestamp:  time.Now(),
		AudioLevel: 0.7,
		NoiseLevel: 0.2,
	}
	err := arbitrator.ProcessWakeWordDetection(detection1)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(1) error = %v", err)
	}

	wg.Wait()

	// Multi-puck arbitration
	wg.Add(1)
	detection2a := WakeWordDetection{
		PuckID:     "puck-002",
		Confidence: 0.7,
		Timestamp:  time.Now(),
		AudioLevel: 0.6,
		NoiseLevel: 0.3,
	}
	detection2b := WakeWordDetection{
		PuckID:     "puck-003",
		Confidence: 0.9,
		Timestamp:  time.Now(),
		AudioLevel: 0.8,
		NoiseLevel: 0.1,
	}

	// Start new arbitration
	time.Sleep(ArbitrationWindowDuration + 100*time.Millisecond)

	err = arbitrator.ProcessWakeWordDetection(detection2a)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(2a) error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	err = arbitrator.ProcessWakeWordDetection(detection2b)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(2b) error = %v", err)
	}

	wg.Wait()

	// Check statistics
	stats := arbitrator.GetStats()

	if stats.TotalArbitrations != 2 {
		t.Errorf("TotalArbitrations = %d, want 2", stats.TotalArbitrations)
	}
	if stats.SinglePuckDecisions != 1 {
		t.Errorf("SinglePuckDecisions = %d, want 1", stats.SinglePuckDecisions)
	}
	if stats.MultiPuckDecisions != 1 {
		t.Errorf("MultiPuckDecisions = %d, want 1", stats.MultiPuckDecisions)
	}
	if stats.AverageWindowTime <= 0 {
		t.Errorf("AverageWindowTime = %v, want > 0", stats.AverageWindowTime)
	}
	if len(stats.ConfidenceDistribution) == 0 {
		t.Error("ConfidenceDistribution should not be empty")
	}
}

func TestSerializeDeserializeWakeWordDetection(t *testing.T) {
	detection := WakeWordDetection{
		PuckID:       "puck-001",
		SessionID:    "session-123",
		Confidence:   0.85,
		Timestamp:    time.Now(),
		AudioLevel:   0.8,
		NoiseLevel:   0.2,
		DistanceHint: 1.5,
	}

	// Test serialization
	data, err := SerializeWakeWordDetection(detection)
	if err != nil {
		t.Fatalf("SerializeWakeWordDetection() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Test deserialization
	deserialized, err := DeserializeWakeWordDetection(data)
	if err != nil {
		t.Fatalf("DeserializeWakeWordDetection() error = %v", err)
	}

	// Verify all fields match
	if deserialized.PuckID != detection.PuckID {
		t.Errorf("PuckID = %s, want %s", deserialized.PuckID, detection.PuckID)
	}
	if deserialized.SessionID != detection.SessionID {
		t.Errorf("SessionID = %s, want %s", deserialized.SessionID, detection.SessionID)
	}
	if deserialized.Confidence != detection.Confidence {
		t.Errorf("Confidence = %f, want %f", deserialized.Confidence, detection.Confidence)
	}
	if deserialized.AudioLevel != detection.AudioLevel {
		t.Errorf("AudioLevel = %f, want %f", deserialized.AudioLevel, detection.AudioLevel)
	}
	if deserialized.NoiseLevel != detection.NoiseLevel {
		t.Errorf("NoiseLevel = %f, want %f", deserialized.NoiseLevel, detection.NoiseLevel)
	}
	if deserialized.DistanceHint != detection.DistanceHint {
		t.Errorf("DistanceHint = %f, want %f", deserialized.DistanceHint, detection.DistanceHint)
	}
}

func TestCreateWakeWordFrame(t *testing.T) {
	detection := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.85,
		Timestamp:  time.Now(),
		AudioLevel: 0.8,
		NoiseLevel: 0.2,
	}

	frame, err := CreateWakeWordFrame(detection, 12345, 67)
	if err != nil {
		t.Fatalf("CreateWakeWordFrame() error = %v", err)
	}

	// Verify frame properties
	if frame.Type != transport.FrameTypeWakeWord {
		t.Errorf("Frame type = %v, want %v", frame.Type, transport.FrameTypeWakeWord)
	}
	if frame.SessionID != 12345 {
		t.Errorf("SessionID = %d, want 12345", frame.SessionID)
	}
	if frame.Sequence != 67 {
		t.Errorf("Sequence = %d, want 67", frame.Sequence)
	}
	if len(frame.Data) == 0 {
		t.Error("Frame data is empty")
	}

	// Verify we can deserialize the detection from frame data
	deserialized, err := DeserializeWakeWordDetection(frame.Data)
	if err != nil {
		t.Fatalf("DeserializeWakeWordDetection() error = %v", err)
	}
	if deserialized.PuckID != detection.PuckID {
		t.Errorf("Deserialized PuckID = %s, want %s", deserialized.PuckID, detection.PuckID)
	}
}

func TestProcessWakeWordDetection_WindowExpiry(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	arbitrator := NewArbitrator()

	// Set up callback to capture result
	var result *ArbitrationResult
	var wg sync.WaitGroup
	var callbackOnce sync.Once
	wg.Add(1)

	arbitrator.SetArbitrationCompleteCallback(func(r *ArbitrationResult) {
		callbackOnce.Do(func() {
			result = r
			wg.Done()
		})
	})

	// Create detection
	detection := WakeWordDetection{
		PuckID:     "puck-001",
		SessionID:  "session-123",
		Confidence: 0.8,
		Timestamp:  time.Now(),
		AudioLevel: 0.7,
		NoiseLevel: 0.2,
	}

	// Process detection
	err := arbitrator.ProcessWakeWordDetection(detection)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection() error = %v", err)
	}

	// Try to add another detection after window expires
	time.Sleep(ArbitrationWindowDuration + arbitrator.maxWindowExtension + 50*time.Millisecond)

	detection2 := WakeWordDetection{
		PuckID:     "puck-002",
		SessionID:  "session-123",
		Confidence: 0.9,
		Timestamp:  time.Now(),
		AudioLevel: 0.8,
		NoiseLevel: 0.1,
	}

	err = arbitrator.ProcessWakeWordDetection(detection2)
	if err != nil {
		t.Fatalf("ProcessWakeWordDetection(2) error = %v", err)
	}

	// Wait for first arbitration to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("First arbitration did not complete")
	}

	// Verify first result only contains first detection
	if result == nil {
		t.Fatal("No arbitration result received")
	}
	if result.WinnerPuckID != "puck-001" {
		t.Errorf("WinnerPuckID = %s, want puck-001", result.WinnerPuckID)
	}
	if len(result.AllDetections) != 1 {
		t.Errorf("AllDetections length = %d, want 1", len(result.AllDetections))
	}

	// Verify second detection started a new window
	arbitrator.mutex.RLock()
	hasActiveWindow := arbitrator.activeWindow != nil
	arbitrator.mutex.RUnlock()

	if !hasActiveWindow {
		t.Error("Second detection should have started a new arbitration window")
	}
}
