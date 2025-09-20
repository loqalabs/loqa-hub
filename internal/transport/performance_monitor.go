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
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// PerformanceMonitor tracks performance metrics for the transport layer
// Based on lessons learned from puck testing - monitors key performance indicators
type PerformanceMonitor struct {
	mutex sync.RWMutex

	// Frame processing metrics
	framesProcessed    uint64
	totalProcessingTime time.Duration
	maxProcessingTime   time.Duration
	minProcessingTime   time.Duration

	// Connection metrics
	connectionEstablishTime time.Duration
	averageSessionDuration  time.Duration
	sessionsCreated         uint64
	sessionsClosed          uint64

	// Error tracking
	validationErrors   uint64
	handlerErrors      uint64
	bufferOverflows    uint64
	deserializationErrors uint64

	// Throughput tracking
	lastThroughputCheck time.Time
	framesInLastPeriod  uint64
	currentThroughput   float64 // frames per second

	// Performance recommendations
	recommendations []string
}

// FrameProcessingMetrics holds metrics for frame processing performance
type FrameProcessingMetrics struct {
	FramesProcessed     uint64
	AverageProcessingTime time.Duration
	MaxProcessingTime   time.Duration
	MinProcessingTime   time.Duration
	CurrentThroughput   float64
	ValidationErrors    uint64
	HandlerErrors       uint64
	BufferOverflows     uint64
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{
		minProcessingTime:   time.Hour, // Initialize to high value
		lastThroughputCheck: time.Now(),
		recommendations:     make([]string, 0),
	}
}

// RecordFrameProcessing records metrics for frame processing
func (pm *PerformanceMonitor) RecordFrameProcessing(processingTime time.Duration) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.framesProcessed++
	pm.totalProcessingTime += processingTime

	// Update min/max processing times
	if processingTime > pm.maxProcessingTime {
		pm.maxProcessingTime = processingTime
	}
	if processingTime < pm.minProcessingTime || pm.minProcessingTime == time.Hour {
		pm.minProcessingTime = processingTime
	}

	pm.framesInLastPeriod++

	// Update throughput every 10 seconds
	now := time.Now()
	if now.Sub(pm.lastThroughputCheck) >= 10*time.Second {
		duration := now.Sub(pm.lastThroughputCheck).Seconds()
		pm.currentThroughput = float64(pm.framesInLastPeriod) / duration
		pm.framesInLastPeriod = 0
		pm.lastThroughputCheck = now

		// Generate performance recommendations
		pm.updateRecommendations()
	}
}

// RecordSessionCreated records metrics for session creation
func (pm *PerformanceMonitor) RecordSessionCreated(establishTime time.Duration) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.sessionsCreated++
	pm.connectionEstablishTime = establishTime
}

// RecordSessionClosed records metrics for session closure
func (pm *PerformanceMonitor) RecordSessionClosed(sessionDuration time.Duration) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.sessionsClosed++

	// Update average session duration
	if pm.sessionsClosed > 0 {
		//nolint:gosec // Safe conversion for session duration calculation
		pm.averageSessionDuration = (pm.averageSessionDuration*time.Duration(pm.sessionsClosed-1) + sessionDuration) / time.Duration(pm.sessionsClosed)
	} else {
		pm.averageSessionDuration = sessionDuration
	}
}

// RecordValidationError records a frame validation error
func (pm *PerformanceMonitor) RecordValidationError() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.validationErrors++
}

// RecordHandlerError records a frame handler error
func (pm *PerformanceMonitor) RecordHandlerError() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.handlerErrors++
}

// RecordBufferOverflow records a buffer overflow event
func (pm *PerformanceMonitor) RecordBufferOverflow() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.bufferOverflows++
}

// RecordDeserializationError records a frame deserialization error
func (pm *PerformanceMonitor) RecordDeserializationError() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.deserializationErrors++
}

// GetFrameProcessingMetrics returns current frame processing metrics
func (pm *PerformanceMonitor) GetFrameProcessingMetrics() FrameProcessingMetrics {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var avgProcessingTime time.Duration
	if pm.framesProcessed > 0 {
		//nolint:gosec // Safe conversion for average calculation
		avgProcessingTime = pm.totalProcessingTime / time.Duration(pm.framesProcessed)
	}

	return FrameProcessingMetrics{
		FramesProcessed:       pm.framesProcessed,
		AverageProcessingTime: avgProcessingTime,
		MaxProcessingTime:     pm.maxProcessingTime,
		MinProcessingTime:     pm.minProcessingTime,
		CurrentThroughput:     pm.currentThroughput,
		ValidationErrors:      pm.validationErrors,
		HandlerErrors:         pm.handlerErrors,
		BufferOverflows:       pm.bufferOverflows,
	}
}

// GetPerformanceStatus returns comprehensive performance status
func (pm *PerformanceMonitor) GetPerformanceStatus() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var avgProcessingTime time.Duration
	if pm.framesProcessed > 0 {
		//nolint:gosec // Safe conversion for average calculation
		avgProcessingTime = pm.totalProcessingTime / time.Duration(pm.framesProcessed)
	}

	// Calculate error rates
	var validationErrorRate, handlerErrorRate, deserializationErrorRate float64
	if pm.framesProcessed > 0 {
		validationErrorRate = float64(pm.validationErrors) / float64(pm.framesProcessed) * 100
		handlerErrorRate = float64(pm.handlerErrors) / float64(pm.framesProcessed) * 100
		deserializationErrorRate = float64(pm.deserializationErrors) / float64(pm.framesProcessed) * 100
	}

	return map[string]interface{}{
		"frames_processed":          pm.framesProcessed,
		"average_processing_time_ms": avgProcessingTime.Milliseconds(),
		"max_processing_time_ms":    pm.maxProcessingTime.Milliseconds(),
		"min_processing_time_ms":    pm.minProcessingTime.Milliseconds(),
		"current_throughput_fps":    pm.currentThroughput,
		"sessions_created":          pm.sessionsCreated,
		"sessions_closed":           pm.sessionsClosed,
		"active_sessions":           pm.sessionsCreated - pm.sessionsClosed,
		"average_session_duration_s": pm.averageSessionDuration.Seconds(),
		"validation_error_rate":     validationErrorRate,
		"handler_error_rate":        handlerErrorRate,
		"deserialization_error_rate": deserializationErrorRate,
		"buffer_overflows":          pm.bufferOverflows,
		"recommendations":           pm.recommendations,
	}
}

// updateRecommendations generates performance recommendations based on current metrics
func (pm *PerformanceMonitor) updateRecommendations() {
	pm.recommendations = nil // Clear previous recommendations

	// Average processing time recommendations
	var avgProcessingTime time.Duration
	if pm.framesProcessed > 0 {
		//nolint:gosec // Safe conversion for average calculation
		avgProcessingTime = pm.totalProcessingTime / time.Duration(pm.framesProcessed)
	}

	if avgProcessingTime > 10*time.Millisecond {
		pm.recommendations = append(pm.recommendations,
			"Frame processing time is high (>10ms). Consider optimizing frame handlers.")
	}

	// Throughput recommendations
	if pm.currentThroughput < 50 && pm.framesProcessed > 100 {
		pm.recommendations = append(pm.recommendations,
			"Low throughput detected (<50 fps). Check for processing bottlenecks.")
	}

	// Error rate recommendations
	if pm.framesProcessed > 0 {
		validationErrorRate := float64(pm.validationErrors) / float64(pm.framesProcessed) * 100
		if validationErrorRate > 5 {
			pm.recommendations = append(pm.recommendations,
				"High frame validation error rate (>5%). Check puck firmware or network issues.")
		}

		handlerErrorRate := float64(pm.handlerErrors) / float64(pm.framesProcessed) * 100
		if handlerErrorRate > 1 {
			pm.recommendations = append(pm.recommendations,
				"High frame handler error rate (>1%). Review frame handler implementations.")
		}
	}

	// Buffer overflow recommendations
	if pm.bufferOverflows > 0 {
		pm.recommendations = append(pm.recommendations,
			"Buffer overflows detected. Consider increasing buffer sizes or improving processing speed.")
	}

	// Session management recommendations
	if pm.sessionsCreated > 0 && pm.sessionsClosed > 0 {
		sessionCloseRate := float64(pm.sessionsClosed) / float64(pm.sessionsCreated) * 100
		if sessionCloseRate > 90 {
			pm.recommendations = append(pm.recommendations,
				"High session close rate. Check for connection stability issues.")
		}
	}

	// Connection time recommendations
	if pm.connectionEstablishTime > 5*time.Second {
		pm.recommendations = append(pm.recommendations,
			"Slow connection establishment (>5s). Check network latency and server load.")
	}
}

// GetRecommendations returns current performance recommendations
func (pm *PerformanceMonitor) GetRecommendations() []string {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	recommendations := make([]string, len(pm.recommendations))
	copy(recommendations, pm.recommendations)
	return recommendations
}

// LogPerformanceSummary logs a performance summary
func (pm *PerformanceMonitor) LogPerformanceSummary() {
	status := pm.GetPerformanceStatus()

	logging.Sugar.Infow("Performance Summary",
		"frames_processed", status["frames_processed"],
		"avg_processing_time_ms", status["average_processing_time_ms"],
		"current_throughput_fps", status["current_throughput_fps"],
		"active_sessions", status["active_sessions"],
		"validation_error_rate", status["validation_error_rate"],
		"buffer_overflows", status["buffer_overflows"])

	recommendations := pm.GetRecommendations()
	if len(recommendations) > 0 {
		logging.Sugar.Warnw("Performance Recommendations", "recommendations", recommendations)
	}
}

// Reset resets all performance metrics (useful for testing)
func (pm *PerformanceMonitor) Reset() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.framesProcessed = 0
	pm.totalProcessingTime = 0
	pm.maxProcessingTime = 0
	pm.minProcessingTime = time.Hour
	pm.connectionEstablishTime = 0
	pm.averageSessionDuration = 0
	pm.sessionsCreated = 0
	pm.sessionsClosed = 0
	pm.validationErrors = 0
	pm.handlerErrors = 0
	pm.bufferOverflows = 0
	pm.deserializationErrors = 0
	pm.lastThroughputCheck = time.Now()
	pm.framesInLastPeriod = 0
	pm.currentThroughput = 0
	pm.recommendations = nil
}