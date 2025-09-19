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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/transport"
)

// ArbitrationWindowDuration defines the time window for collecting wake word detections
const ArbitrationWindowDuration = 500 * time.Millisecond

// WakeWordDetection represents a wake word detection from a puck
type WakeWordDetection struct {
	PuckID       string    `json:"puck_id"`
	SessionID    string    `json:"session_id"`
	Confidence   float64   `json:"confidence"`
	Timestamp    time.Time `json:"timestamp"`
	AudioLevel   float64   `json:"audio_level"`
	NoiseLevel   float64   `json:"noise_level"`
	DistanceHint float64   `json:"distance_hint"` // Optional distance estimation
}

// ArbitrationResult represents the outcome of arbitration
type ArbitrationResult struct {
	WinnerPuckID string                `json:"winner_puck_id"`
	WinnerScore  float64               `json:"winner_score"`
	AllDetections []WakeWordDetection  `json:"all_detections"`
	ArbitrationTime time.Duration      `json:"arbitration_time_ms"`
	DecisionReason  string             `json:"decision_reason"`
}

// Arbitrator handles multi-puck wake word arbitration
type Arbitrator struct {
	// Current arbitration session
	mutex              sync.RWMutex
	activeWindow       *ArbitrationWindow
	pendingDetections  []WakeWordDetection
	windowTimer        *time.Timer

	// Configuration
	confidenceWeight   float64 // Weight for confidence score (0.0-1.0)
	snrWeight         float64 // Weight for signal-to-noise ratio
	proximityWeight   float64 // Weight for proximity hints
	minConfidence     float64 // Minimum confidence to participate
	maxWindowExtension time.Duration // Max time to extend window for late arrivals

	// Callbacks
	onArbitrationComplete func(*ArbitrationResult)
	onWindowExpired       func([]WakeWordDetection)

	// Statistics
	stats ArbitrationStats
}

// ArbitrationWindow tracks an active arbitration session
type ArbitrationWindow struct {
	StartTime      time.Time
	ExpectedPucks  map[string]bool // Pucks that should participate
	ReceivedFrom   map[string]bool // Pucks we've received from
	WindowExtended bool            // Whether we've extended the window
}

// ArbitrationStats tracks arbitration performance metrics
type ArbitrationStats struct {
	TotalArbitrations     int64         `json:"total_arbitrations"`
	AverageWindowTime     time.Duration `json:"average_window_time"`
	SinglePuckDecisions   int64         `json:"single_puck_decisions"`
	MultiPuckDecisions    int64         `json:"multi_puck_decisions"`
	WindowExtensions      int64         `json:"window_extensions"`
	ConfidenceDistribution map[string]int64 `json:"confidence_distribution"`
}

// NewArbitrator creates a new arbitration system
func NewArbitrator() *Arbitrator {
	return &Arbitrator{
		confidenceWeight:   0.5,
		snrWeight:         0.3,
		proximityWeight:   0.2,
		minConfidence:     0.3, // Require 30% confidence minimum
		maxWindowExtension: 200 * time.Millisecond,
		stats: ArbitrationStats{
			ConfidenceDistribution: make(map[string]int64),
		},
	}
}

// ProcessWakeWordDetection handles incoming wake word detections
func (a *Arbitrator) ProcessWakeWordDetection(detection WakeWordDetection) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// Validate detection
	if detection.Confidence < a.minConfidence {
		logging.Sugar.Debugw("Wake word detection below minimum confidence",
			"puck_id", detection.PuckID,
			"confidence", detection.Confidence,
			"min_confidence", a.minConfidence)
		return nil
	}

	logging.Sugar.Infow("Wake word detection received",
		"puck_id", detection.PuckID,
		"confidence", detection.Confidence,
		"audio_level", detection.AudioLevel,
		"noise_level", detection.NoiseLevel)

	// Check if we're in an active arbitration window
	if a.activeWindow == nil {
		// Start new arbitration window
		a.startArbitrationWindow(detection)
	} else {
		// Add to existing window if within time limit
		elapsed := time.Since(a.activeWindow.StartTime)
		if elapsed <= ArbitrationWindowDuration+a.maxWindowExtension {
			a.addDetectionToWindow(detection)
		} else {
			// Window expired, start new one
			logging.Sugar.Warnw("Detection received after window expired",
				"puck_id", detection.PuckID,
				"elapsed", elapsed)
			a.finalizeArbitration()
			a.startArbitrationWindow(detection)
		}
	}

	return nil
}

// startArbitrationWindow begins a new arbitration window
func (a *Arbitrator) startArbitrationWindow(firstDetection WakeWordDetection) {
	a.activeWindow = &ArbitrationWindow{
		StartTime:     time.Now(),
		ExpectedPucks: make(map[string]bool),
		ReceivedFrom:  make(map[string]bool),
	}

	a.pendingDetections = []WakeWordDetection{firstDetection}
	a.activeWindow.ReceivedFrom[firstDetection.PuckID] = true

	// Set timer for window expiration
	a.windowTimer = time.AfterFunc(ArbitrationWindowDuration, func() {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		a.finalizeArbitration()
	})

	logging.Sugar.Infow("Started arbitration window",
		"start_time", a.activeWindow.StartTime,
		"first_puck", firstDetection.PuckID,
		"window_duration", ArbitrationWindowDuration)
}

// addDetectionToWindow adds a detection to the current window
func (a *Arbitrator) addDetectionToWindow(detection WakeWordDetection) {
	// Check for duplicate from same puck
	for _, existing := range a.pendingDetections {
		if existing.PuckID == detection.PuckID {
			// Update with higher confidence detection
			if detection.Confidence > existing.Confidence {
				logging.Sugar.Infow("Updating detection with higher confidence",
					"puck_id", detection.PuckID,
					"old_confidence", existing.Confidence,
					"new_confidence", detection.Confidence)

				// Replace the existing detection
				for i, d := range a.pendingDetections {
					if d.PuckID == detection.PuckID {
						a.pendingDetections[i] = detection
						break
					}
				}
			}
			return
		}
	}

	// Add new detection
	a.pendingDetections = append(a.pendingDetections, detection)
	a.activeWindow.ReceivedFrom[detection.PuckID] = true

	logging.Sugar.Infow("Added detection to arbitration window",
		"puck_id", detection.PuckID,
		"total_detections", len(a.pendingDetections))
}

// finalizeArbitration completes the arbitration and selects winner
func (a *Arbitrator) finalizeArbitration() {
	if a.activeWindow == nil || len(a.pendingDetections) == 0 {
		return
	}

	startTime := time.Now()

	// Stop the timer
	if a.windowTimer != nil {
		a.windowTimer.Stop()
	}

	// Calculate scores for each detection
	scoredDetections := make([]ScoredDetection, len(a.pendingDetections))
	for i, detection := range a.pendingDetections {
		score := a.calculateScore(detection)
		scoredDetections[i] = ScoredDetection{
			Detection: detection,
			Score:     score,
		}
	}

	// Find winner (highest score)
	winner := scoredDetections[0]
	for _, scored := range scoredDetections[1:] {
		if scored.Score > winner.Score {
			winner = scored
		}
	}

	// Create result
	result := &ArbitrationResult{
		WinnerPuckID:    winner.Detection.PuckID,
		WinnerScore:     winner.Score,
		AllDetections:   a.pendingDetections,
		ArbitrationTime: time.Since(startTime),
		DecisionReason:  a.getDecisionReason(scoredDetections, winner),
	}

	// Update statistics
	a.updateStats(result)

	logging.Sugar.Infow("Arbitration completed",
		"winner_puck", result.WinnerPuckID,
		"winner_score", result.WinnerScore,
		"total_detections", len(result.AllDetections),
		"decision_time", result.ArbitrationTime,
		"reason", result.DecisionReason)

	// Clear state
	a.activeWindow = nil
	a.pendingDetections = nil

	// Notify callback
	if a.onArbitrationComplete != nil {
		go a.onArbitrationComplete(result)
	}
}

// ScoredDetection pairs a detection with its calculated score
type ScoredDetection struct {
	Detection WakeWordDetection
	Score     float64
}

// calculateScore computes the arbitration score for a detection
func (a *Arbitrator) calculateScore(detection WakeWordDetection) float64 {
	// Base confidence score
	confidenceScore := detection.Confidence * a.confidenceWeight

	// Signal-to-noise ratio score
	snr := 1.0
	if detection.NoiseLevel > 0 {
		snr = detection.AudioLevel / detection.NoiseLevel
	}
	snrScore := normalizeScore(snr, 0.5, 10.0) * a.snrWeight

	// Proximity score (inverse of distance hint)
	proximityScore := 0.0
	if detection.DistanceHint > 0 {
		// Higher score for lower distance
		proximityScore = (1.0 / (1.0 + detection.DistanceHint)) * a.proximityWeight
	}

	totalScore := confidenceScore + snrScore + proximityScore

	logging.Sugar.Debugw("Calculated arbitration score",
		"puck_id", detection.PuckID,
		"confidence_score", confidenceScore,
		"snr_score", snrScore,
		"proximity_score", proximityScore,
		"total_score", totalScore)

	return totalScore
}

// normalizeScore normalizes a value to 0-1 range given min/max bounds
func normalizeScore(value, min, max float64) float64 {
	if value <= min {
		return 0.0
	}
	if value >= max {
		return 1.0
	}
	return (value - min) / (max - min)
}

// getDecisionReason generates a human-readable reason for the arbitration decision
func (a *Arbitrator) getDecisionReason(scored []ScoredDetection, winner ScoredDetection) string {
	if len(scored) == 1 {
		return "Single puck detection"
	}

	// Find the primary factor that led to victory
	if winner.Detection.Confidence >= 0.9 {
		return "High confidence detection"
	}

	// Check if significantly better SNR
	for _, other := range scored {
		if other.Detection.PuckID != winner.Detection.PuckID {
			winnerSNR := winner.Detection.AudioLevel / winner.Detection.NoiseLevel
			otherSNR := other.Detection.AudioLevel / other.Detection.NoiseLevel
			if winnerSNR > otherSNR*1.5 {
				return "Superior audio quality"
			}
		}
	}

	return "Best overall score"
}

// updateStats updates arbitration statistics
func (a *Arbitrator) updateStats(result *ArbitrationResult) {
	a.stats.TotalArbitrations++

	// Update average window time
	total := time.Duration(a.stats.TotalArbitrations)
	a.stats.AverageWindowTime = (a.stats.AverageWindowTime*(total-1) + result.ArbitrationTime) / total

	// Count single vs multi-puck decisions
	if len(result.AllDetections) == 1 {
		a.stats.SinglePuckDecisions++
	} else {
		a.stats.MultiPuckDecisions++
	}

	// Update confidence distribution
	bucket := fmt.Sprintf("%.1f", result.WinnerScore)
	a.stats.ConfidenceDistribution[bucket]++
}

// SetArbitrationCompleteCallback sets the callback for when arbitration completes
func (a *Arbitrator) SetArbitrationCompleteCallback(callback func(*ArbitrationResult)) {
	a.onArbitrationComplete = callback
}

// GetStats returns current arbitration statistics
func (a *Arbitrator) GetStats() ArbitrationStats {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.stats
}

// SerializeWakeWordDetection converts a wake word detection to binary frame data
func SerializeWakeWordDetection(detection WakeWordDetection) ([]byte, error) {
	data, err := json.Marshal(detection)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal wake word detection: %w", err)
	}
	return data, nil
}

// DeserializeWakeWordDetection converts binary frame data to wake word detection
func DeserializeWakeWordDetection(data []byte) (*WakeWordDetection, error) {
	var detection WakeWordDetection
	if err := json.Unmarshal(data, &detection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wake word detection: %w", err)
	}
	return &detection, nil
}

// CreateWakeWordFrame creates a binary frame for wake word detection
func CreateWakeWordFrame(detection WakeWordDetection, sessionID uint32, sequence uint32) (*transport.Frame, error) {
	data, err := SerializeWakeWordDetection(detection)
	if err != nil {
		return nil, err
	}

	return transport.NewFrame(
		transport.FrameTypeWakeWord,
		sessionID,
		sequence,
		uint64(time.Now().UnixMicro()),
		data,
	), nil
}