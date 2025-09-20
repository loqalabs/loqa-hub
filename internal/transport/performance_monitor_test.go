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
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/stretchr/testify/assert"
)

func setupPerformanceTestLogging() {
	if err := logging.Initialize(); err != nil {
		panic("Failed to initialize logging: " + err.Error())
	}
}

func TestNewPerformanceMonitor(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	assert.NotNil(t, pm)
	assert.Equal(t, time.Hour, pm.minProcessingTime) // Should be initialized to high value
	assert.NotZero(t, pm.lastThroughputCheck)
	assert.Equal(t, uint64(0), pm.framesProcessed)
}

func TestRecordFrameProcessing(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record some frame processing times
	pm.RecordFrameProcessing(5 * time.Millisecond)
	pm.RecordFrameProcessing(10 * time.Millisecond)
	pm.RecordFrameProcessing(15 * time.Millisecond)

	metrics := pm.GetFrameProcessingMetrics()
	assert.Equal(t, uint64(3), metrics.FramesProcessed)
	assert.Equal(t, 10*time.Millisecond, metrics.AverageProcessingTime)
	assert.Equal(t, 15*time.Millisecond, metrics.MaxProcessingTime)
	assert.Equal(t, 5*time.Millisecond, metrics.MinProcessingTime)
}

func TestRecordSessionCreated(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	establishTime := 2 * time.Second
	pm.RecordSessionCreated(establishTime)

	assert.Equal(t, uint64(1), pm.sessionsCreated)
	assert.Equal(t, establishTime, pm.connectionEstablishTime)
}

func TestRecordSessionClosed(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record multiple session closures
	pm.RecordSessionClosed(30 * time.Second)
	pm.RecordSessionClosed(60 * time.Second)

	assert.Equal(t, uint64(2), pm.sessionsClosed)
	assert.Equal(t, 45*time.Second, pm.averageSessionDuration) // Average of 30s and 60s
}

func TestRecordErrors(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record frame processing first so error rates can be calculated
	pm.RecordFrameProcessing(1 * time.Millisecond)
	pm.RecordFrameProcessing(2 * time.Millisecond)

	// Record various types of errors
	pm.RecordValidationError()
	pm.RecordValidationError()
	pm.RecordHandlerError()
	pm.RecordBufferOverflow()
	pm.RecordDeserializationError()

	metrics := pm.GetFrameProcessingMetrics()
	assert.Equal(t, uint64(2), metrics.ValidationErrors)
	assert.Equal(t, uint64(1), metrics.HandlerErrors)
	assert.Equal(t, uint64(1), metrics.BufferOverflows)

	status := pm.GetPerformanceStatus()
	assert.Equal(t, float64(100), status["validation_error_rate"]) // 2 errors / 2 frames * 100
	assert.Equal(t, uint64(1), status["buffer_overflows"])
}

func TestGetFrameProcessingMetrics(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record some data
	pm.RecordFrameProcessing(1 * time.Millisecond)
	pm.RecordFrameProcessing(3 * time.Millisecond)
	pm.RecordValidationError()
	pm.RecordHandlerError()
	pm.RecordBufferOverflow()

	metrics := pm.GetFrameProcessingMetrics()

	assert.Equal(t, uint64(2), metrics.FramesProcessed)
	assert.Equal(t, 2*time.Millisecond, metrics.AverageProcessingTime)
	assert.Equal(t, 3*time.Millisecond, metrics.MaxProcessingTime)
	assert.Equal(t, 1*time.Millisecond, metrics.MinProcessingTime)
	assert.Equal(t, uint64(1), metrics.ValidationErrors)
	assert.Equal(t, uint64(1), metrics.HandlerErrors)
	assert.Equal(t, uint64(1), metrics.BufferOverflows)
}

func TestGetPerformanceStatus(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record comprehensive test data
	pm.RecordFrameProcessing(5 * time.Millisecond)
	pm.RecordFrameProcessing(15 * time.Millisecond) // Average: 10ms
	pm.RecordSessionCreated(3 * time.Second)
	pm.RecordSessionClosed(45 * time.Second)
	pm.RecordValidationError()
	pm.RecordHandlerError()
	pm.RecordDeserializationError()

	status := pm.GetPerformanceStatus()

	// Verify all status fields are present
	assert.Equal(t, uint64(2), status["frames_processed"])
	assert.Equal(t, int64(10), status["average_processing_time_ms"])
	assert.Equal(t, int64(15), status["max_processing_time_ms"])
	assert.Equal(t, int64(5), status["min_processing_time_ms"])
	assert.Equal(t, uint64(1), status["sessions_created"])
	assert.Equal(t, uint64(1), status["sessions_closed"])
	assert.Equal(t, uint64(0), status["active_sessions"])
	assert.Equal(t, float64(45), status["average_session_duration_s"])
	assert.Equal(t, float64(50), status["validation_error_rate"]) // 1/2 * 100
	assert.Equal(t, float64(50), status["handler_error_rate"])    // 1/2 * 100
	assert.Equal(t, float64(50), status["deserialization_error_rate"]) // 1/2 * 100
	assert.NotNil(t, status["recommendations"])
}

func TestGetRecommendations(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Initially should have no recommendations
	recommendations := pm.GetRecommendations()
	assert.Empty(t, recommendations)

	// Trigger high processing time
	for i := 0; i < 10; i++ {
		pm.RecordFrameProcessing(20 * time.Millisecond) // >10ms threshold
	}
	pm.updateRecommendations()

	recommendations = pm.GetRecommendations()
	assert.NotEmpty(t, recommendations)
	assert.Contains(t, recommendations[0], "Frame processing time is high")
}

func TestPerformanceRecommendations(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	tests := []struct {
		name           string
		setup          func()
		expectedInRec  string
	}{
		{
			name: "High processing time",
			setup: func() {
				for i := 0; i < 5; i++ {
					pm.RecordFrameProcessing(20 * time.Millisecond)
				}
			},
			expectedInRec: "Frame processing time is high",
		},
		{
			name: "High validation error rate",
			setup: func() {
				for i := 0; i < 20; i++ {
					pm.RecordFrameProcessing(1 * time.Millisecond)
				}
				for i := 0; i < 2; i++ { // 10% error rate
					pm.RecordValidationError()
				}
			},
			expectedInRec: "High frame validation error rate",
		},
		{
			name: "Buffer overflows",
			setup: func() {
				pm.RecordBufferOverflow()
			},
			expectedInRec: "Buffer overflows detected",
		},
		{
			name: "Slow connection establishment",
			setup: func() {
				pm.RecordSessionCreated(10 * time.Second) // >5s threshold
			},
			expectedInRec: "Slow connection establishment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm.Reset() // Start fresh for each test
			tt.setup()
			pm.updateRecommendations()

			recommendations := pm.GetRecommendations()
			found := false
			for _, rec := range recommendations {
				if strings.Contains(rec, tt.expectedInRec) {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected recommendation containing '%s' not found in: %v", tt.expectedInRec, recommendations)
		})
	}
}

func TestLogPerformanceSummary(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Record some data to make the summary meaningful
	pm.RecordFrameProcessing(5 * time.Millisecond)
	pm.RecordSessionCreated(1 * time.Second)

	// This should not panic and should log the summary
	pm.LogPerformanceSummary()
}

func TestReset(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Populate with data
	pm.RecordFrameProcessing(10 * time.Millisecond)
	pm.RecordSessionCreated(2 * time.Second)
	pm.RecordSessionClosed(30 * time.Second)
	pm.RecordValidationError()
	pm.RecordHandlerError()
	pm.RecordBufferOverflow()
	pm.RecordDeserializationError()

	// Verify data exists
	assert.Equal(t, uint64(1), pm.framesProcessed)
	assert.Equal(t, uint64(1), pm.sessionsCreated)

	// Reset
	pm.Reset()

	// Verify everything is reset
	assert.Equal(t, uint64(0), pm.framesProcessed)
	assert.Equal(t, time.Duration(0), pm.totalProcessingTime)
	assert.Equal(t, time.Duration(0), pm.maxProcessingTime)
	assert.Equal(t, time.Hour, pm.minProcessingTime) // Back to initial high value
	assert.Equal(t, uint64(0), pm.sessionsCreated)
	assert.Equal(t, uint64(0), pm.sessionsClosed)
	assert.Equal(t, uint64(0), pm.validationErrors)
	assert.Equal(t, uint64(0), pm.handlerErrors)
	assert.Equal(t, uint64(0), pm.bufferOverflows)
	assert.Equal(t, uint64(0), pm.deserializationErrors)
	assert.Empty(t, pm.recommendations)
}

func TestThroughputCalculation(t *testing.T) {
	setupPerformanceTestLogging()
	defer logging.Close()

	pm := NewPerformanceMonitor()

	// Simulate processing frames over time
	pm.RecordFrameProcessing(1 * time.Millisecond)
	pm.RecordFrameProcessing(1 * time.Millisecond)
	pm.RecordFrameProcessing(1 * time.Millisecond)

	// Manually trigger throughput calculation by setting last check time to past
	pm.lastThroughputCheck = time.Now().Add(-11 * time.Second)
	pm.RecordFrameProcessing(1 * time.Millisecond)

	// Should have calculated throughput
	assert.Greater(t, pm.currentThroughput, float64(0))
}