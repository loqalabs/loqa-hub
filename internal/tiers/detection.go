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

package tiers

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// PerformanceTier represents system capability levels
type PerformanceTier string

const (
	TierBasic    PerformanceTier = "basic"    // Reflex-only, minimal processing
	TierStandard PerformanceTier = "standard" // Local LLM, full features
	TierPro      PerformanceTier = "pro"      // Advanced processing, experimental features
)

// ServiceAvailability tracks availability of different services
type ServiceAvailability struct {
	STT  bool `json:"stt_available"`
	TTS  bool `json:"tts_available"`
	LLM  bool `json:"llm_available"`
	NATS bool `json:"nats_available"`
}

// SystemCapabilities represents detected system capabilities
type SystemCapabilities struct {
	Tier              PerformanceTier     `json:"tier"`
	Services          ServiceAvailability `json:"services"`
	Hardware          HardwareInfo        `json:"hardware"`
	Performance       PerformanceMetrics  `json:"performance"`
	LastDetected      time.Time           `json:"last_detected"`
	Degraded          bool                `json:"degraded"`
	DegradationReason string              `json:"degradation_reason,omitempty"`
}

// HardwareInfo contains system hardware information
type HardwareInfo struct {
	CPUCores     int    `json:"cpu_cores"`
	MemoryGB     int    `json:"memory_gb"`
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

// PerformanceMetrics tracks system performance
type PerformanceMetrics struct {
	AvgCPUUsage    float64       `json:"avg_cpu_usage"`
	AvgMemoryUsage float64       `json:"avg_memory_usage"`
	STTLatency     time.Duration `json:"stt_latency"`
	TTSLatency     time.Duration `json:"tts_latency"`
	LLMLatency     time.Duration `json:"llm_latency"`
	LastMeasured   time.Time     `json:"last_measured"`
}

// TierDetector handles performance tier detection and graceful degradation
type TierDetector struct {
	mutex        sync.RWMutex
	capabilities SystemCapabilities

	// Service URLs for health checks
	sttURL  string
	ttsURL  string
	llmURL  string
	natsURL string

	// Configuration
	detectionInterval time.Duration
	healthTimeout     time.Duration
	performanceWindow time.Duration

	// Callbacks
	onTierChange  func(old, new PerformanceTier)
	onDegradation func(reason string)
}

// SLARequirements defines performance requirements for each tier
var SLARequirements = map[PerformanceTier]struct {
	MaxSTTLatency time.Duration
	MaxTTSLatency time.Duration
	MaxLLMLatency time.Duration
	MinCPUCores   int
	MinMemoryGB   int
}{
	TierBasic: {
		MaxSTTLatency: 1000 * time.Millisecond,
		MaxTTSLatency: 500 * time.Millisecond,
		MaxLLMLatency: 5000 * time.Millisecond, // Reflex-only, no LLM required
		MinCPUCores:   2,
		MinMemoryGB:   2,
	},
	TierStandard: {
		MaxSTTLatency: 800 * time.Millisecond,
		MaxTTSLatency: 300 * time.Millisecond,
		MaxLLMLatency: 3000 * time.Millisecond,
		MinCPUCores:   4,
		MinMemoryGB:   8,
	},
	TierPro: {
		MaxSTTLatency: 500 * time.Millisecond,
		MaxTTSLatency: 200 * time.Millisecond,
		MaxLLMLatency: 1500 * time.Millisecond,
		MinCPUCores:   8,
		MinMemoryGB:   16,
	},
}

// NewTierDetector creates a new tier detection system
func NewTierDetector(sttURL, ttsURL, llmURL, natsURL string) *TierDetector {
	td := &TierDetector{
		sttURL:            sttURL,
		ttsURL:            ttsURL,
		llmURL:            llmURL,
		natsURL:           natsURL,
		detectionInterval: 30 * time.Second,
		healthTimeout:     5 * time.Second,
		performanceWindow: 5 * time.Minute,
	}

	// Initialize with basic capabilities
	td.capabilities = SystemCapabilities{
		Tier:         TierBasic,
		Services:     ServiceAvailability{},
		Hardware:     td.detectHardware(),
		Performance:  PerformanceMetrics{LastMeasured: time.Now()},
		LastDetected: time.Now(),
		Degraded:     false,
	}

	return td
}

// Start begins continuous tier detection
func (td *TierDetector) Start(ctx context.Context) {
	// Perform initial detection
	td.detectCapabilities(ctx)

	// Start periodic detection
	ticker := time.NewTicker(td.detectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			td.detectCapabilities(ctx)
		}
	}
}

// detectCapabilities performs full system capability detection
func (td *TierDetector) detectCapabilities(ctx context.Context) {
	td.mutex.Lock()
	defer td.mutex.Unlock()

	oldTier := td.capabilities.Tier

	// Detect service availability
	services := td.detectServiceAvailability(ctx)

	// Measure performance
	performance := td.measurePerformance(ctx)

	// Determine tier based on capabilities
	newTier := td.determineTier(services, performance)

	// Check for degradation
	degraded, degradationReason := td.checkDegradation(services, performance, newTier)

	// Update capabilities
	td.capabilities = SystemCapabilities{
		Tier:              newTier,
		Services:          services,
		Hardware:          td.capabilities.Hardware, // Hardware doesn't change
		Performance:       performance,
		LastDetected:      time.Now(),
		Degraded:          degraded,
		DegradationReason: degradationReason,
	}

	logging.Sugar.Infow("Tier detection completed",
		"tier", newTier,
		"degraded", degraded,
		"stt_available", services.STT,
		"tts_available", services.TTS,
		"llm_available", services.LLM,
		"stt_latency", performance.STTLatency,
		"tts_latency", performance.TTSLatency,
		"llm_latency", performance.LLMLatency)

	// Notify tier change
	if oldTier != newTier && td.onTierChange != nil {
		go td.onTierChange(oldTier, newTier)
	}

	// Notify degradation
	if degraded && td.onDegradation != nil {
		go td.onDegradation(degradationReason)
	}
}

// detectServiceAvailability checks availability of all services
func (td *TierDetector) detectServiceAvailability(ctx context.Context) ServiceAvailability {
	healthCtx, cancel := context.WithTimeout(ctx, td.healthTimeout)
	defer cancel()

	services := ServiceAvailability{}

	// Check STT service
	services.STT = td.checkServiceHealth(healthCtx, td.sttURL+"/health")

	// Check TTS service
	services.TTS = td.checkServiceHealth(healthCtx, td.ttsURL+"/health")

	// Check LLM service
	services.LLM = td.checkServiceHealth(healthCtx, td.llmURL+"/api/health")

	// NATS health check would require NATS client, simplified for now
	services.NATS = true // Assume available for architecture migration

	return services
}

// checkServiceHealth performs HTTP health check
func (td *TierDetector) checkServiceHealth(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: td.healthTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// measurePerformance measures current system performance
func (td *TierDetector) measurePerformance(ctx context.Context) PerformanceMetrics {
	metrics := PerformanceMetrics{
		LastMeasured: time.Now(),
	}

	// Measure service latencies (simplified for architecture migration)
	if td.capabilities.Services.STT {
		metrics.STTLatency = td.measureServiceLatency(ctx, td.sttURL+"/health")
	}

	if td.capabilities.Services.TTS {
		metrics.TTSLatency = td.measureServiceLatency(ctx, td.ttsURL+"/health")
	}

	if td.capabilities.Services.LLM {
		metrics.LLMLatency = td.measureServiceLatency(ctx, td.llmURL+"/api/health")
	}

	// CPU and memory usage would require system monitoring
	// Simplified for architecture migration
	metrics.AvgCPUUsage = 50.0    // Placeholder
	metrics.AvgMemoryUsage = 60.0 // Placeholder

	return metrics
}

// measureServiceLatency measures response time for a service
func (td *TierDetector) measureServiceLatency(ctx context.Context, url string) time.Duration {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return time.Hour // High latency for errors
	}

	client := &http.Client{Timeout: td.healthTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return time.Hour // High latency for errors
	}
	defer func() { _ = resp.Body.Close() }()

	return time.Since(start)
}

// determineTier calculates appropriate tier based on capabilities
func (td *TierDetector) determineTier(services ServiceAvailability, performance PerformanceMetrics) PerformanceTier {
	hardware := td.capabilities.Hardware

	// Check if we meet Pro tier requirements
	proReqs := SLARequirements[TierPro]
	if hardware.CPUCores >= proReqs.MinCPUCores &&
		hardware.MemoryGB >= proReqs.MinMemoryGB &&
		services.STT && services.TTS && services.LLM &&
		performance.STTLatency <= proReqs.MaxSTTLatency &&
		performance.TTSLatency <= proReqs.MaxTTSLatency &&
		performance.LLMLatency <= proReqs.MaxLLMLatency {
		return TierPro
	}

	// Check if we meet Standard tier requirements
	standardReqs := SLARequirements[TierStandard]
	if hardware.CPUCores >= standardReqs.MinCPUCores &&
		hardware.MemoryGB >= standardReqs.MinMemoryGB &&
		services.STT && services.TTS && services.LLM &&
		performance.STTLatency <= standardReqs.MaxSTTLatency &&
		performance.TTSLatency <= standardReqs.MaxTTSLatency &&
		performance.LLMLatency <= standardReqs.MaxLLMLatency {
		return TierStandard
	}

	// Fall back to Basic tier
	return TierBasic
}

// checkDegradation determines if system is running in degraded mode
func (td *TierDetector) checkDegradation(services ServiceAvailability, performance PerformanceMetrics, tier PerformanceTier) (bool, string) {
	// Check for service failures
	if !services.STT {
		return true, "STT service unavailable"
	}
	if !services.TTS {
		return true, "TTS service unavailable"
	}

	// For Standard/Pro tiers, LLM is required
	if tier != TierBasic && !services.LLM {
		return true, "LLM service unavailable"
	}

	// Check SLA violations
	reqs := SLARequirements[tier]
	if performance.STTLatency > reqs.MaxSTTLatency {
		return true, fmt.Sprintf("STT latency %.0fms exceeds SLA", performance.STTLatency.Seconds()*1000)
	}
	if performance.TTSLatency > reqs.MaxTTSLatency {
		return true, fmt.Sprintf("TTS latency %.0fms exceeds SLA", performance.TTSLatency.Seconds()*1000)
	}

	return false, ""
}

// detectHardware detects system hardware capabilities
func (td *TierDetector) detectHardware() HardwareInfo {
	// Get basic runtime information
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)

	// Calculate memory in GB, checking for overflow
	memoryBytes := memStats.Sys
	memoryGB := memoryBytes / (1024 * 1024 * 1024)
	if memoryGB > 2147483647 { // Max int32
		memoryGB = 2147483647
	}

	return HardwareInfo{
		CPUCores:     runtime.NumCPU(),
		MemoryGB:     int(memoryGB), //nolint:gosec // G115: Safe conversion after bounds check above
		Architecture: runtime.GOARCH,
		OS:           runtime.GOOS,
	}
}

// GetCapabilities returns current system capabilities
func (td *TierDetector) GetCapabilities() SystemCapabilities {
	td.mutex.RLock()
	defer td.mutex.RUnlock()
	return td.capabilities
}

// GetTier returns current performance tier
func (td *TierDetector) GetTier() PerformanceTier {
	td.mutex.RLock()
	defer td.mutex.RUnlock()
	return td.capabilities.Tier
}

// IsFeatureAvailable checks if a feature is available in current tier
func (td *TierDetector) IsFeatureAvailable(feature string) bool {
	tier := td.GetTier()

	switch feature {
	case "local_llm":
		return tier == TierStandard || tier == TierPro
	case "streaming_responses":
		return tier == TierPro
	case "reflex_only":
		return true // Available in all tiers
	default:
		return tier != TierBasic // Most features available in Standard+
	}
}

// SetTierChangeCallback sets callback for tier changes
func (td *TierDetector) SetTierChangeCallback(callback func(old, new PerformanceTier)) {
	td.onTierChange = callback
}

// SetDegradationCallback sets callback for degradation events
func (td *TierDetector) SetDegradationCallback(callback func(reason string)) {
	td.onDegradation = callback
}
