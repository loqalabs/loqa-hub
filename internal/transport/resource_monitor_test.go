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
	"runtime"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/stretchr/testify/assert"
)

func setupResourceTestLogging() {
	if err := logging.Initialize(); err != nil {
		panic("Failed to initialize logging: " + err.Error())
	}
}

func TestNewResourceMonitor(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	assert.NotNil(t, rm)
	assert.Greater(t, rm.startGoroutines, 0)
	assert.GreaterOrEqual(t, rm.startMemory, uint64(0))
	assert.Equal(t, 1000, rm.maxGoroutines)
	assert.Equal(t, uint64(500), rm.maxMemoryMB)
	assert.Equal(t, 30*time.Second, rm.checkInterval)
}

func TestUpdateMetrics(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	activeSessions := 5
	rm.UpdateMetrics(activeSessions)

	metrics := rm.GetMetrics()
	assert.Greater(t, metrics.Goroutines, 0)
	assert.GreaterOrEqual(t, metrics.MemoryMB, uint64(0))
	assert.GreaterOrEqual(t, metrics.GCCycles, uint32(0))
	assert.GreaterOrEqual(t, metrics.LastGCPause, time.Duration(0))
	assert.Equal(t, activeSessions, metrics.ActiveSessions)
}

func TestGetMetrics(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Update with some sessions
	rm.UpdateMetrics(3)

	metrics := rm.GetMetrics()
	assert.Equal(t, 3, metrics.ActiveSessions)
	assert.Greater(t, metrics.Goroutines, 0)
	assert.GreaterOrEqual(t, metrics.MemoryMB, uint64(0))
}

func TestCheckResourceLeaks_Normal(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()
	rm.UpdateMetrics(1)

	warnings := rm.CheckResourceLeaks()
	// With normal resource usage, should have no warnings
	assert.Empty(t, warnings)
}

func TestCheckResourceLeaks_GoroutineLeak(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Simulate goroutine leak by manually setting high goroutine count
	rm.UpdateMetrics(1)
	rm.metrics.Goroutines = rm.startGoroutines + 60 // More than 50 threshold

	warnings := rm.CheckResourceLeaks()
	assert.NotEmpty(t, warnings)

	found := false
	for _, warning := range warnings {
		if contains(warning, "Potential goroutine leak") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected goroutine leak warning")
}

func TestCheckResourceLeaks_MemoryLeak(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Simulate memory increase
	rm.UpdateMetrics(1)
	rm.metrics.MemoryMB = rm.startMemory + 150 // More than 100MB threshold

	warnings := rm.CheckResourceLeaks()
	assert.NotEmpty(t, warnings)

	found := false
	for _, warning := range warnings {
		if contains(warning, "Significant memory increase") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected memory leak warning")
}

func TestCheckResourceLeaks_LimitsExceeded(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Simulate exceeding limits
	rm.UpdateMetrics(1)
	rm.metrics.Goroutines = rm.maxGoroutines + 100
	rm.metrics.MemoryMB = rm.maxMemoryMB + 100

	warnings := rm.CheckResourceLeaks()
	assert.NotEmpty(t, warnings)
	assert.GreaterOrEqual(t, len(warnings), 2) // Should have both goroutine and memory warnings

	goroutineWarning := false
	memoryWarning := false
	for _, warning := range warnings {
		if contains(warning, "Goroutine limit exceeded") {
			goroutineWarning = true
		}
		if contains(warning, "Memory limit exceeded") {
			memoryWarning = true
		}
	}
	assert.True(t, goroutineWarning, "Expected goroutine limit warning")
	assert.True(t, memoryWarning, "Expected memory limit warning")
}

func TestCheckResourceLeaks_HighGCPause(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Simulate high GC pause
	rm.UpdateMetrics(1)
	rm.metrics.LastGCPause = 150 * time.Millisecond // More than 100ms threshold

	warnings := rm.CheckResourceLeaks()
	assert.NotEmpty(t, warnings)

	found := false
	for _, warning := range warnings {
		if contains(warning, "High GC pause detected") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected high GC pause warning")
}

func TestForceCleanup(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// This should not panic and should log cleanup messages
	rm.ForceCleanup()
}

func TestIsHealthy(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Initially should be healthy
	rm.UpdateMetrics(1)
	assert.True(t, rm.IsHealthy())

	// Simulate unhealthy condition
	rm.metrics.Goroutines = rm.startGoroutines + 60 // Trigger goroutine leak warning
	assert.False(t, rm.IsHealthy())
}

func TestGetHealthStatus(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()
	rm.UpdateMetrics(2)

	status := rm.GetHealthStatus()

	// Verify all expected fields are present
	assert.Contains(t, status, "healthy")
	assert.Contains(t, status, "warnings")
	assert.Contains(t, status, "goroutines")
	assert.Contains(t, status, "memory_mb")
	assert.Contains(t, status, "active_sessions")
	assert.Contains(t, status, "gc_cycles")
	assert.Contains(t, status, "last_gc_pause_ns")

	// Verify types and values
	assert.IsType(t, true, status["healthy"])
	assert.IsType(t, []string{}, status["warnings"])
	assert.IsType(t, 0, status["goroutines"])
	assert.IsType(t, uint64(0), status["memory_mb"])
	assert.Equal(t, 2, status["active_sessions"])
}

func TestGetHealthStatus_Unhealthy(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()
	rm.UpdateMetrics(1)

	// Make it unhealthy
	rm.metrics.Goroutines = rm.startGoroutines + 60

	status := rm.GetHealthStatus()
	assert.False(t, status["healthy"].(bool))

	warnings := status["warnings"].([]string)
	assert.NotEmpty(t, warnings)
}

func TestResourceMonitor_ConcurrentAccess(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Test concurrent access to ensure thread safety
	done := make(chan bool, 2)

	// Goroutine 1: Update metrics
	go func() {
		for i := 0; i < 100; i++ {
			rm.UpdateMetrics(i % 10)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Check health status
	go func() {
		for i := 0; i < 100; i++ {
			_ = rm.GetHealthStatus()
			_ = rm.CheckResourceLeaks()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Should still be functional
	rm.UpdateMetrics(5)
	assert.True(t, rm.IsHealthy())
}

func TestResourceMonitor_ActualGC(t *testing.T) {
	setupResourceTestLogging()
	defer logging.Close()

	rm := NewResourceMonitor()

	// Force GC and update metrics
	runtime.GC()
	runtime.GC()
	rm.UpdateMetrics(1)

	metrics := rm.GetMetrics()
	assert.Greater(t, metrics.GCCycles, uint32(0))
	assert.GreaterOrEqual(t, metrics.LastGCPause, time.Duration(0))
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}