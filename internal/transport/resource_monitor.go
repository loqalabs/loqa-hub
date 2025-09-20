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
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// ResourceMonitor tracks system resources and detects leaks
// Based on lessons learned from puck testing
type ResourceMonitor struct {
	startGoroutines int
	startMemory     uint64
	maxGoroutines   int
	maxMemoryMB     uint64
	checkInterval   time.Duration
	mu              sync.RWMutex

	// Metrics
	metrics ResourceMetrics
}

// ResourceMetrics holds current resource usage metrics
type ResourceMetrics struct {
	Goroutines      int
	MemoryMB        uint64
	GCCycles        uint32
	LastGCPause     time.Duration
	ActiveSessions  int
	TotalFrames     uint64
	DroppedFrames   uint64
}

// NewResourceMonitor creates a new resource monitor with baseline measurements
func NewResourceMonitor() *ResourceMonitor {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rm := &ResourceMonitor{
		startGoroutines: runtime.NumGoroutine(),
		startMemory:     m.Alloc / 1024 / 1024, // Convert to MB
		maxGoroutines:   1000,                   // Reasonable limit for hub
		maxMemoryMB:     500,                    // 500MB limit
		checkInterval:   30 * time.Second,
	}

	logging.Sugar.Infow("ResourceMonitor initialized",
		"baseline_goroutines", rm.startGoroutines,
		"baseline_memory_mb", rm.startMemory)

	return rm
}

// UpdateMetrics updates current resource metrics
func (rm *ResourceMonitor) UpdateMetrics(activeSessions int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rm.metrics = ResourceMetrics{
		Goroutines:     runtime.NumGoroutine(),
		MemoryMB:       m.Alloc / 1024 / 1024,
		GCCycles:       m.NumGC,
		//nolint:gosec // Safe conversion for GC pause measurement
		LastGCPause:    time.Duration(m.PauseNs[(m.NumGC+255)%256]),
		ActiveSessions: activeSessions,
	}
}

// GetMetrics returns current resource metrics
func (rm *ResourceMonitor) GetMetrics() ResourceMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.metrics
}

// CheckResourceLeaks detects potential resource leaks
func (rm *ResourceMonitor) CheckResourceLeaks() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var warnings []string

	// Check goroutine leaks (learned from puck testing)
	goroutineDiff := rm.metrics.Goroutines - rm.startGoroutines
	if goroutineDiff > 50 { // More generous than puck since hub handles more connections
		warnings = append(warnings,
			fmt.Sprintf("Potential goroutine leak: %d goroutines (started with %d)",
				rm.metrics.Goroutines, rm.startGoroutines))
	}

	// Check memory leaks
	memoryDiff := rm.metrics.MemoryMB - rm.startMemory
	if memoryDiff > 100 { // 100MB increase from baseline
		warnings = append(warnings,
			fmt.Sprintf("Significant memory increase: %dMB (started with %dMB)",
				rm.metrics.MemoryMB, rm.startMemory))
	}

	// Check if we're hitting limits
	if rm.metrics.Goroutines > rm.maxGoroutines {
		warnings = append(warnings,
			fmt.Sprintf("Goroutine limit exceeded: %d (max %d)",
				rm.metrics.Goroutines, rm.maxGoroutines))
	}

	if rm.metrics.MemoryMB > rm.maxMemoryMB {
		warnings = append(warnings,
			fmt.Sprintf("Memory limit exceeded: %dMB (max %dMB)",
				rm.metrics.MemoryMB, rm.maxMemoryMB))
	}

	// Check GC pressure
	if rm.metrics.LastGCPause > 100*time.Millisecond {
		warnings = append(warnings,
			fmt.Sprintf("High GC pause detected: %v", rm.metrics.LastGCPause))
	}

	return warnings
}

// ForceCleanup forces garbage collection and connection cleanup
// Similar to the cleanup patterns we developed for puck testing
func (rm *ResourceMonitor) ForceCleanup() {
	logging.Sugar.Info("ResourceMonitor: Forcing cleanup")

	// Force garbage collection
	runtime.GC()
	runtime.GC() // Second GC to clean up finalizers

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)

	logging.Sugar.Info("ResourceMonitor: Cleanup completed")
}

// IsHealthy returns true if resource usage is within acceptable limits
func (rm *ResourceMonitor) IsHealthy() bool {
	warnings := rm.CheckResourceLeaks()
	return len(warnings) == 0
}

// GetHealthStatus returns a health status summary
func (rm *ResourceMonitor) GetHealthStatus() map[string]interface{} {
	metrics := rm.GetMetrics()
	warnings := rm.CheckResourceLeaks()

	return map[string]interface{}{
		"healthy":           len(warnings) == 0,
		"warnings":          warnings,
		"goroutines":        metrics.Goroutines,
		"memory_mb":         metrics.MemoryMB,
		"active_sessions":   metrics.ActiveSessions,
		"gc_cycles":         metrics.GCCycles,
		"last_gc_pause_ns":  int64(metrics.LastGCPause),
	}
}