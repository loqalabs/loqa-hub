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
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

func TestNewTierDetector(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	sttURL := "http://localhost:8000"
	ttsURL := "http://localhost:8880"
	llmURL := "http://localhost:11434"
	natsURL := "nats://localhost:4222"

	td := NewTierDetector(sttURL, ttsURL, llmURL, natsURL)

	// Verify configuration
	if td.sttURL != sttURL {
		t.Errorf("sttURL = %s, want %s", td.sttURL, sttURL)
	}
	if td.ttsURL != ttsURL {
		t.Errorf("ttsURL = %s, want %s", td.ttsURL, ttsURL)
	}
	if td.llmURL != llmURL {
		t.Errorf("llmURL = %s, want %s", td.llmURL, llmURL)
	}
	if td.natsURL != natsURL {
		t.Errorf("natsURL = %s, want %s", td.natsURL, natsURL)
	}

	// Verify default settings
	if td.detectionInterval != 30*time.Second {
		t.Errorf("detectionInterval = %v, want 30s", td.detectionInterval)
	}
	if td.healthTimeout != 5*time.Second {
		t.Errorf("healthTimeout = %v, want 5s", td.healthTimeout)
	}
	if td.performanceWindow != 5*time.Minute {
		t.Errorf("performanceWindow = %v, want 5m", td.performanceWindow)
	}

	// Verify initial capabilities
	caps := td.GetCapabilities()
	if caps.Tier != TierBasic {
		t.Errorf("Initial tier = %v, want %v", caps.Tier, TierBasic)
	}
	if caps.Hardware.CPUCores != runtime.NumCPU() {
		t.Errorf("CPU cores = %d, want %d", caps.Hardware.CPUCores, runtime.NumCPU())
	}
	if caps.Hardware.Architecture != runtime.GOARCH {
		t.Errorf("Architecture = %s, want %s", caps.Hardware.Architecture, runtime.GOARCH)
	}
	if caps.Hardware.OS != runtime.GOOS {
		t.Errorf("OS = %s, want %s", caps.Hardware.OS, runtime.GOOS)
	}
	if caps.Degraded {
		t.Error("Initial state should not be degraded")
	}
}

func TestTierDetector_DetectHardware(t *testing.T) {
	td := &TierDetector{}
	hardware := td.detectHardware()

	// Verify hardware detection
	if hardware.CPUCores <= 0 {
		t.Errorf("CPU cores = %d, want > 0", hardware.CPUCores)
	}
	if hardware.MemoryGB < 0 {
		t.Errorf("Memory = %d GB, want >= 0", hardware.MemoryGB)
	}
	if hardware.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
	if hardware.OS == "" {
		t.Error("OS should not be empty")
	}

	// Check expected values
	if hardware.CPUCores != runtime.NumCPU() {
		t.Errorf("CPU cores = %d, want %d", hardware.CPUCores, runtime.NumCPU())
	}
	if hardware.Architecture != runtime.GOARCH {
		t.Errorf("Architecture = %s, want %s", hardware.Architecture, runtime.GOARCH)
	}
	if hardware.OS != runtime.GOOS {
		t.Errorf("OS = %s, want %s", hardware.OS, runtime.GOOS)
	}
}

func TestTierDetector_CheckServiceHealth(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	// Test healthy service
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	if !td.checkServiceHealth(context.Background(), healthyServer.URL) {
		t.Error("Should return true for healthy service")
	}

	// Test unhealthy service
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	if td.checkServiceHealth(context.Background(), unhealthyServer.URL) {
		t.Error("Should return false for unhealthy service")
	}

	// Test unreachable service
	if td.checkServiceHealth(context.Background(), "http://invalid.localhost:99999") {
		t.Error("Should return false for unreachable service")
	}

	// Test timeout
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	td.healthTimeout = 10 * time.Millisecond
	if td.checkServiceHealth(context.Background(), slowServer.URL) {
		t.Error("Should return false for slow service when timeout is short")
	}
}

func TestTierDetector_MeasureServiceLatency(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	// Test normal service
	normalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer normalServer.Close()

	latency := td.measureServiceLatency(context.Background(), normalServer.URL)
	if latency < 50*time.Millisecond || latency > 200*time.Millisecond {
		t.Errorf("Latency = %v, want between 50ms and 200ms", latency)
	}

	// Test error case
	errorLatency := td.measureServiceLatency(context.Background(), "http://invalid.localhost:99999")
	if errorLatency != time.Hour {
		t.Errorf("Error latency = %v, want %v", errorLatency, time.Hour)
	}
}

func TestTierDetector_DetectServiceAvailability(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Create test servers
	sttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer sttServer.Close()

	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ttsServer.Close()

	// LLM server returns error
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer llmServer.Close()

	td := NewTierDetector(sttServer.URL, ttsServer.URL, llmServer.URL, "nats://localhost:4222")

	services := td.detectServiceAvailability(context.Background())

	// Verify results
	if !services.STT {
		t.Error("STT service should be available")
	}
	if !services.TTS {
		t.Error("TTS service should be available")
	}
	if services.LLM {
		t.Error("LLM service should be unavailable (returns 500)")
	}
	if !services.NATS {
		t.Error("NATS should be assumed available")
	}
}

func TestTierDetector_DetermineTier(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	// Mock high-end hardware
	td.capabilities.Hardware = HardwareInfo{
		CPUCores: 16,
		MemoryGB: 32,
	}

	tests := []struct {
		name         string
		services     ServiceAvailability
		performance  PerformanceMetrics
		expectedTier PerformanceTier
	}{
		{
			name: "Pro tier - all services available, excellent performance",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 300 * time.Millisecond,
				TTSLatency: 100 * time.Millisecond,
				LLMLatency: 1000 * time.Millisecond,
			},
			expectedTier: TierPro,
		},
		{
			name: "Standard tier - all services available, good performance",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 600 * time.Millisecond,
				TTSLatency: 250 * time.Millisecond,
				LLMLatency: 2500 * time.Millisecond,
			},
			expectedTier: TierStandard,
		},
		{
			name: "Basic tier - LLM unavailable",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: false, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 300 * time.Millisecond,
				TTSLatency: 100 * time.Millisecond,
				LLMLatency: 1000 * time.Millisecond,
			},
			expectedTier: TierBasic,
		},
		{
			name: "Basic tier - poor performance",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 2000 * time.Millisecond, // Exceeds all limits
				TTSLatency: 1000 * time.Millisecond,
				LLMLatency: 5000 * time.Millisecond,
			},
			expectedTier: TierBasic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier := td.determineTier(tt.services, tt.performance)
			if tier != tt.expectedTier {
				t.Errorf("determineTier() = %v, want %v", tier, tt.expectedTier)
			}
		})
	}
}

func TestTierDetector_DetermineTier_LowEndHardware(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	// Mock low-end hardware
	td.capabilities.Hardware = HardwareInfo{
		CPUCores: 2,
		MemoryGB: 1,
	}

	// Even with excellent services and performance, hardware limits to Basic
	services := ServiceAvailability{
		STT: true, TTS: true, LLM: true, NATS: true,
	}
	performance := PerformanceMetrics{
		STTLatency: 100 * time.Millisecond,
		TTSLatency: 50 * time.Millisecond,
		LLMLatency: 500 * time.Millisecond,
	}

	tier := td.determineTier(services, performance)
	if tier != TierBasic {
		t.Errorf("Low-end hardware should limit to Basic tier, got %v", tier)
	}
}

func TestTierDetector_CheckDegradation(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	tests := []struct {
		name           string
		services       ServiceAvailability
		performance    PerformanceMetrics
		tier           PerformanceTier
		expectDegraded bool
		expectReason   string
	}{
		{
			name: "No degradation - all good",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 300 * time.Millisecond,
				TTSLatency: 100 * time.Millisecond,
				LLMLatency: 1000 * time.Millisecond,
			},
			tier:           TierPro,
			expectDegraded: false,
		},
		{
			name: "STT unavailable",
			services: ServiceAvailability{
				STT: false, TTS: true, LLM: true, NATS: true,
			},
			performance:    PerformanceMetrics{},
			tier:           TierStandard,
			expectDegraded: true,
			expectReason:   "STT service unavailable",
		},
		{
			name: "TTS unavailable",
			services: ServiceAvailability{
				STT: true, TTS: false, LLM: true, NATS: true,
			},
			performance:    PerformanceMetrics{},
			tier:           TierStandard,
			expectDegraded: true,
			expectReason:   "TTS service unavailable",
		},
		{
			name: "LLM unavailable for Standard tier",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: false, NATS: true,
			},
			performance:    PerformanceMetrics{},
			tier:           TierStandard,
			expectDegraded: true,
			expectReason:   "LLM service unavailable",
		},
		{
			name: "LLM unavailable for Basic tier - OK",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: false, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 500 * time.Millisecond,
				TTSLatency: 200 * time.Millisecond,
			},
			tier:           TierBasic,
			expectDegraded: false,
		},
		{
			name: "STT latency exceeds SLA",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 1000 * time.Millisecond, // Exceeds Pro limit of 500ms
				TTSLatency: 100 * time.Millisecond,
				LLMLatency: 1000 * time.Millisecond,
			},
			tier:           TierPro,
			expectDegraded: true,
			expectReason:   "STT latency 1000ms exceeds SLA",
		},
		{
			name: "TTS latency exceeds SLA",
			services: ServiceAvailability{
				STT: true, TTS: true, LLM: true, NATS: true,
			},
			performance: PerformanceMetrics{
				STTLatency: 300 * time.Millisecond,
				TTSLatency: 500 * time.Millisecond, // Exceeds Pro limit of 200ms
				LLMLatency: 1000 * time.Millisecond,
			},
			tier:           TierPro,
			expectDegraded: true,
			expectReason:   "TTS latency 500ms exceeds SLA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			degraded, reason := td.checkDegradation(tt.services, tt.performance, tt.tier)

			if degraded != tt.expectDegraded {
				t.Errorf("checkDegradation() degraded = %v, want %v", degraded, tt.expectDegraded)
			}

			if tt.expectDegraded && reason != tt.expectReason {
				t.Errorf("checkDegradation() reason = %q, want %q", reason, tt.expectReason)
			}

			if !tt.expectDegraded && reason != "" {
				t.Errorf("checkDegradation() reason should be empty when not degraded, got %q", reason)
			}
		})
	}
}

func TestTierDetector_IsFeatureAvailable(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	tests := []struct {
		tier      PerformanceTier
		feature   string
		available bool
	}{
		// Basic tier
		{TierBasic, "reflex_only", true},
		{TierBasic, "local_llm", false},
		{TierBasic, "streaming_responses", false},
		{TierBasic, "unknown_feature", false},

		// Standard tier
		{TierStandard, "reflex_only", true},
		{TierStandard, "local_llm", true},
		{TierStandard, "streaming_responses", false},
		{TierStandard, "unknown_feature", true},

		// Pro tier
		{TierPro, "reflex_only", true},
		{TierPro, "local_llm", true},
		{TierPro, "streaming_responses", true},
		{TierPro, "unknown_feature", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier)+"_"+tt.feature, func(t *testing.T) {
			// Set the tier
			td.mutex.Lock()
			td.capabilities.Tier = tt.tier
			td.mutex.Unlock()

			available := td.IsFeatureAvailable(tt.feature)
			if available != tt.available {
				t.Errorf("IsFeatureAvailable(%s) with tier %s = %v, want %v",
					tt.feature, tt.tier, available, tt.available)
			}
		})
	}
}

func TestTierDetector_GetCapabilities(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	caps := td.GetCapabilities()

	// Verify structure
	if caps.Tier == "" {
		t.Error("Tier should not be empty")
	}
	if caps.LastDetected.IsZero() {
		t.Error("LastDetected should be set")
	}
	if caps.Hardware.CPUCores <= 0 {
		t.Error("CPU cores should be > 0")
	}
}

func TestTierDetector_GetTier(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	tier := td.GetTier()
	if tier != TierBasic {
		t.Errorf("Initial tier = %v, want %v", tier, TierBasic)
	}

	// Change tier and verify
	td.mutex.Lock()
	td.capabilities.Tier = TierPro
	td.mutex.Unlock()

	tier = td.GetTier()
	if tier != TierPro {
		t.Errorf("Updated tier = %v, want %v", tier, TierPro)
	}
}

func TestTierDetector_Callbacks(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")

	// Test tier change callback
	var tierChangeCalled bool
	var oldTier, newTier PerformanceTier
	var wg sync.WaitGroup

	td.SetTierChangeCallback(func(old, new PerformanceTier) {
		tierChangeCalled = true
		oldTier = old
		newTier = new
		wg.Done()
	})

	// Test degradation callback
	var degradationCalled bool
	var degradationReason string

	td.SetDegradationCallback(func(reason string) {
		degradationCalled = true
		degradationReason = reason
		wg.Done()
	})

	// Simulate tier change
	wg.Add(1)
	td.mutex.Lock()
	oldCapsTier := td.capabilities.Tier
	td.capabilities.Tier = TierPro
	td.mutex.Unlock()

	if td.onTierChange != nil {
		go td.onTierChange(oldCapsTier, TierPro)
	}

	// Wait for callback
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !tierChangeCalled {
			t.Error("Tier change callback should be called")
		}
		if oldTier != TierBasic {
			t.Errorf("Old tier = %v, want %v", oldTier, TierBasic)
		}
		if newTier != TierPro {
			t.Errorf("New tier = %v, want %v", newTier, TierPro)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Tier change callback should be called within timeout")
	}

	// Simulate degradation
	wg.Add(1)
	if td.onDegradation != nil {
		go td.onDegradation("test degradation")
	}

	// Wait for degradation callback
	done2 := make(chan struct{})
	go func() {
		wg.Wait()
		close(done2)
	}()

	select {
	case <-done2:
		if !degradationCalled {
			t.Error("Degradation callback should be called")
		}
		if degradationReason != "test degradation" {
			t.Errorf("Degradation reason = %q, want %q", degradationReason, "test degradation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Degradation callback should be called within timeout")
	}
}

func TestTierDetector_Start_ContextCancellation(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	td := NewTierDetector("", "", "", "")
	td.detectionInterval = 10 * time.Millisecond // Fast interval for testing

	ctx, cancel := context.WithCancel(context.Background())

	// Start the detector in a goroutine
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		close(started)
		td.Start(ctx)
		close(finished)
	}()

	// Wait for start
	<-started

	// Cancel context after short delay
	time.Sleep(20 * time.Millisecond)
	cancel()

	// Wait for finish
	select {
	case <-finished:
		// Success - Start() returned when context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Error("Start() should return when context is cancelled")
	}
}

func TestSLARequirements(t *testing.T) {
	// Verify SLA requirements are properly defined
	tiers := []PerformanceTier{TierBasic, TierStandard, TierPro}

	for _, tier := range tiers {
		reqs, exists := SLARequirements[tier]
		if !exists {
			t.Errorf("SLA requirements not defined for tier %s", tier)
			continue
		}

		if reqs.MaxSTTLatency <= 0 {
			t.Errorf("MaxSTTLatency for %s should be > 0", tier)
		}
		if reqs.MaxTTSLatency <= 0 {
			t.Errorf("MaxTTSLatency for %s should be > 0", tier)
		}
		if reqs.MaxLLMLatency <= 0 {
			t.Errorf("MaxLLMLatency for %s should be > 0", tier)
		}
		if reqs.MinCPUCores <= 0 {
			t.Errorf("MinCPUCores for %s should be > 0", tier)
		}
		if reqs.MinMemoryGB <= 0 {
			t.Errorf("MinMemoryGB for %s should be > 0", tier)
		}
	}

	// Verify tier progression (more demanding requirements for higher tiers)
	basicReqs := SLARequirements[TierBasic]
	standardReqs := SLARequirements[TierStandard]
	proReqs := SLARequirements[TierPro]

	if standardReqs.MaxSTTLatency >= basicReqs.MaxSTTLatency {
		t.Error("Standard tier should have stricter STT latency than Basic")
	}
	if proReqs.MaxSTTLatency >= standardReqs.MaxSTTLatency {
		t.Error("Pro tier should have stricter STT latency than Standard")
	}

	if standardReqs.MinCPUCores <= basicReqs.MinCPUCores {
		t.Error("Standard tier should require more CPU cores than Basic")
	}
	if proReqs.MinCPUCores <= standardReqs.MinCPUCores {
		t.Error("Pro tier should require more CPU cores than Standard")
	}
}

func TestTierDetector_MeasurePerformance(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Create test servers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	td := NewTierDetector(server.URL, server.URL, server.URL, "")

	// Set services as available
	td.capabilities.Services = ServiceAvailability{
		STT: true,
		TTS: true,
		LLM: true,
	}

	metrics := td.measurePerformance(context.Background())

	// Verify metrics are set
	if metrics.LastMeasured.IsZero() {
		t.Error("LastMeasured should be set")
	}
	if metrics.STTLatency <= 0 {
		t.Error("STT latency should be measured")
	}
	if metrics.TTSLatency <= 0 {
		t.Error("TTS latency should be measured")
	}
	if metrics.LLMLatency <= 0 {
		t.Error("LLM latency should be measured")
	}
	if metrics.AvgCPUUsage != 50.0 {
		t.Errorf("AvgCPUUsage = %f, want 50.0 (placeholder)", metrics.AvgCPUUsage)
	}
	if metrics.AvgMemoryUsage != 60.0 {
		t.Errorf("AvgMemoryUsage = %f, want 60.0 (placeholder)", metrics.AvgMemoryUsage)
	}
}
