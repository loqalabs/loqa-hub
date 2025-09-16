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

package llm

import (
	"encoding/json"
	"sync"
	"time"
)

// StreamingMetricsCollector aggregates and tracks streaming performance metrics
type StreamingMetricsCollector struct {
	enabled            bool
	sessionMetrics     map[string]*StreamingMetrics
	aggregateMetrics   *AggregateMetrics
	mu                 sync.RWMutex
	startTime          time.Time
}

// AggregateMetrics holds system-wide streaming performance data
type AggregateMetrics struct {
	TotalSessions         int           `json:"total_sessions"`
	CompletedSessions     int           `json:"completed_sessions"`
	InterruptedSessions   int           `json:"interrupted_sessions"`
	AverageFirstToken     time.Duration `json:"average_first_token"`
	AverageFirstPhrase    time.Duration `json:"average_first_phrase"`
	AverageCompletion     time.Duration `json:"average_completion"`
	TotalTokens           int           `json:"total_tokens"`
	TotalPhrases          int           `json:"total_phrases"`
	StreamingEnabled      bool          `json:"streaming_enabled"`
	FallbackUsage         int           `json:"fallback_usage"`
	ErrorRate             float64       `json:"error_rate"`
	ThroughputTokensPerSec float64      `json:"throughput_tokens_per_sec"`
	LastUpdated           time.Time     `json:"last_updated"`
}

// StreamingPerformanceReport provides detailed performance analysis
type StreamingPerformanceReport struct {
	Summary              *AggregateMetrics                `json:"summary"`
	RecentSessions       []*StreamingSessionReport       `json:"recent_sessions"`
	PerformanceTrends    *PerformanceTrends               `json:"performance_trends"`
	RecommendedSettings  *RecommendedSettings             `json:"recommended_settings"`
	HealthStatus         string                           `json:"health_status"`
}

// StreamingSessionReport provides detailed session analysis
type StreamingSessionReport struct {
	SessionID            string        `json:"session_id"`
	StartTime            time.Time     `json:"start_time"`
	FirstTokenLatency    time.Duration `json:"first_token_latency"`
	FirstPhraseLatency   time.Duration `json:"first_phrase_latency"`
	TotalDuration        time.Duration `json:"total_duration"`
	TokenCount           int           `json:"token_count"`
	PhraseCount          int           `json:"phrase_count"`
	BufferOverflows      int           `json:"buffer_overflows"`
	InterruptCount       int           `json:"interrupt_count"`
	WasInterrupted       bool          `json:"was_interrupted"`
	CompletedNaturally   bool          `json:"completed_naturally"`
	ErrorEncountered     bool          `json:"error_encountered"`
	TokensPerSecond      float64       `json:"tokens_per_second"`
	QualityScore         float64       `json:"quality_score"`
}

// PerformanceTrends tracks streaming performance over time
type PerformanceTrends struct {
	LastHourSessions     int           `json:"last_hour_sessions"`
	LastHourAvgLatency   time.Duration `json:"last_hour_avg_latency"`
	LastHourErrorRate    float64       `json:"last_hour_error_rate"`
	TrendDirection       string        `json:"trend_direction"` // "improving", "stable", "degrading"
	LatencyTrend         []float64     `json:"latency_trend"`   // Last 10 measurements
	ThroughputTrend      []float64     `json:"throughput_trend"`
}

// RecommendedSettings provides optimization suggestions
type RecommendedSettings struct {
	OptimalBufferTime      time.Duration `json:"optimal_buffer_time"`
	OptimalConcurrency     int           `json:"optimal_concurrency"`
	RecommendStreaming     bool          `json:"recommend_streaming"`
	EstimatedImprovement   string        `json:"estimated_improvement"`
	ConfigurationChanges   []string      `json:"configuration_changes"`
}

// NewStreamingMetricsCollector creates a new metrics collector
func NewStreamingMetricsCollector(enabled bool) *StreamingMetricsCollector {
	return &StreamingMetricsCollector{
		enabled:        enabled,
		sessionMetrics: make(map[string]*StreamingMetrics),
		aggregateMetrics: &AggregateMetrics{
			StreamingEnabled: enabled,
			LastUpdated:      time.Now(),
		},
		startTime: time.Now(),
	}
}

// RecordSessionStart records the start of a streaming session
func (smc *StreamingMetricsCollector) RecordSessionStart(sessionID string) {
	if !smc.enabled {
		return
	}

	smc.mu.Lock()
	defer smc.mu.Unlock()

	metrics := &StreamingMetrics{StartTime: time.Now()}
	smc.sessionMetrics[sessionID] = metrics
	smc.aggregateMetrics.TotalSessions++
}

// RecordSessionMetrics records final metrics for a completed session
func (smc *StreamingMetricsCollector) RecordSessionMetrics(sessionID string, metrics *StreamingMetrics) {
	if !smc.enabled || metrics == nil {
		return
	}

	smc.mu.Lock()
	defer smc.mu.Unlock()

	// Store session metrics
	smc.sessionMetrics[sessionID] = metrics

	// Update aggregate metrics
	smc.updateAggregateMetrics(metrics)

	// Clean up old session data (keep last 100 sessions)
	if len(smc.sessionMetrics) > 100 {
		smc.cleanupOldSessions()
	}
}

// updateAggregateMetrics updates system-wide metrics
func (smc *StreamingMetricsCollector) updateAggregateMetrics(metrics *StreamingMetrics) {
	agg := smc.aggregateMetrics

	// Update completion stats
	if !metrics.CompletionTime.IsZero() {
		agg.CompletedSessions++
	}
	if metrics.InterruptCount > 0 {
		agg.InterruptedSessions++
	}

	// Update timing averages (simple moving average)
	if !metrics.FirstTokenTime.IsZero() {
		firstTokenLatency := metrics.FirstTokenTime.Sub(metrics.StartTime)
		if agg.AverageFirstToken == 0 {
			agg.AverageFirstToken = firstTokenLatency
		} else {
			agg.AverageFirstToken = (agg.AverageFirstToken + firstTokenLatency) / 2
		}
	}

	if !metrics.FirstPhraseTime.IsZero() {
		firstPhraseLatency := metrics.FirstPhraseTime.Sub(metrics.StartTime)
		if agg.AverageFirstPhrase == 0 {
			agg.AverageFirstPhrase = firstPhraseLatency
		} else {
			agg.AverageFirstPhrase = (agg.AverageFirstPhrase + firstPhraseLatency) / 2
		}
	}

	if !metrics.CompletionTime.IsZero() {
		completionTime := metrics.CompletionTime.Sub(metrics.StartTime)
		if agg.AverageCompletion == 0 {
			agg.AverageCompletion = completionTime
		} else {
			agg.AverageCompletion = (agg.AverageCompletion + completionTime) / 2
		}
	}

	// Update counts
	agg.TotalTokens += metrics.TokenCount
	agg.TotalPhrases += metrics.PhraseCount

	// Update throughput
	if !metrics.CompletionTime.IsZero() && metrics.TokenCount > 0 {
		duration := metrics.CompletionTime.Sub(metrics.StartTime)
		if duration > 0 {
			tokensPerSec := float64(metrics.TokenCount) / duration.Seconds()
			if agg.ThroughputTokensPerSec == 0 {
				agg.ThroughputTokensPerSec = tokensPerSec
			} else {
				agg.ThroughputTokensPerSec = (agg.ThroughputTokensPerSec + tokensPerSec) / 2
			}
		}
	}

	// Update error rate
	totalSessions := float64(agg.TotalSessions)
	if totalSessions > 0 {
		errorSessions := float64(agg.InterruptedSessions)
		agg.ErrorRate = errorSessions / totalSessions
	}

	agg.LastUpdated = time.Now()
}

// cleanupOldSessions removes old session data to prevent memory growth
func (smc *StreamingMetricsCollector) cleanupOldSessions() {
	// Find oldest sessions to remove
	oldestSessions := make([]string, 0)
	cutoff := time.Now().Add(-1 * time.Hour) // Keep last hour

	for sessionID, metrics := range smc.sessionMetrics {
		if metrics.StartTime.Before(cutoff) {
			oldestSessions = append(oldestSessions, sessionID)
		}
	}

	// Remove old sessions
	for _, sessionID := range oldestSessions {
		delete(smc.sessionMetrics, sessionID)
	}
}

// GetAggregateMetrics returns current aggregate metrics
func (smc *StreamingMetricsCollector) GetAggregateMetrics() *AggregateMetrics {
	if !smc.enabled {
		return &AggregateMetrics{StreamingEnabled: false}
	}

	smc.mu.RLock()
	defer smc.mu.RUnlock()

	// Return a copy to prevent external modification
	metrics := *smc.aggregateMetrics
	return &metrics
}

// GeneratePerformanceReport creates a comprehensive performance report
func (smc *StreamingMetricsCollector) GeneratePerformanceReport() *StreamingPerformanceReport {
	if !smc.enabled {
		return &StreamingPerformanceReport{
			Summary: &AggregateMetrics{StreamingEnabled: false},
			HealthStatus: "disabled",
		}
	}

	smc.mu.RLock()
	defer smc.mu.RUnlock()

	report := &StreamingPerformanceReport{
		Summary:             smc.aggregateMetrics,
		RecentSessions:      smc.generateRecentSessionReports(),
		PerformanceTrends:   smc.calculatePerformanceTrends(),
		RecommendedSettings: smc.generateRecommendations(),
		HealthStatus:        smc.assessHealthStatus(),
	}

	return report
}

// generateRecentSessionReports creates reports for recent sessions
func (smc *StreamingMetricsCollector) generateRecentSessionReports() []*StreamingSessionReport {
	reports := make([]*StreamingSessionReport, 0)
	recentCutoff := time.Now().Add(-10 * time.Minute)

	for sessionID, metrics := range smc.sessionMetrics {
		if metrics.StartTime.After(recentCutoff) {
			report := &StreamingSessionReport{
				SessionID:          sessionID,
				StartTime:          metrics.StartTime,
				TokenCount:         metrics.TokenCount,
				PhraseCount:        metrics.PhraseCount,
				BufferOverflows:    metrics.BufferOverflows,
				InterruptCount:     metrics.InterruptCount,
				WasInterrupted:     metrics.InterruptCount > 0,
				CompletedNaturally: !metrics.CompletionTime.IsZero(),
			}

			// Calculate latencies
			if !metrics.FirstTokenTime.IsZero() {
				report.FirstTokenLatency = metrics.FirstTokenTime.Sub(metrics.StartTime)
			}
			if !metrics.FirstPhraseTime.IsZero() {
				report.FirstPhraseLatency = metrics.FirstPhraseTime.Sub(metrics.StartTime)
			}
			if !metrics.CompletionTime.IsZero() {
				report.TotalDuration = metrics.CompletionTime.Sub(metrics.StartTime)
				if report.TotalDuration > 0 && metrics.TokenCount > 0 {
					report.TokensPerSecond = float64(metrics.TokenCount) / report.TotalDuration.Seconds()
				}
			}

			// Calculate quality score (0-1 based on performance factors)
			report.QualityScore = smc.calculateQualityScore(metrics)

			reports = append(reports, report)
		}
	}

	return reports
}

// calculateQualityScore computes a quality score for a session
func (smc *StreamingMetricsCollector) calculateQualityScore(metrics *StreamingMetrics) float64 {
	score := 1.0

	// Penalize for interrupts
	if metrics.InterruptCount > 0 {
		score -= 0.2 * float64(metrics.InterruptCount)
	}

	// Penalize for buffer overflows
	if metrics.BufferOverflows > 0 {
		score -= 0.1 * float64(metrics.BufferOverflows)
	}

	// Reward for fast first token
	if !metrics.FirstTokenTime.IsZero() {
		latency := metrics.FirstTokenTime.Sub(metrics.StartTime)
		if latency < 500*time.Millisecond {
			score += 0.1
		} else if latency > 2*time.Second {
			score -= 0.1
		}
	}

	// Reward for completion
	if !metrics.CompletionTime.IsZero() {
		score += 0.1
	}

	// Ensure score is between 0 and 1
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// calculatePerformanceTrends analyzes performance trends
func (smc *StreamingMetricsCollector) calculatePerformanceTrends() *PerformanceTrends {
	// For now, return basic trends - this could be enhanced with more sophisticated analysis
	return &PerformanceTrends{
		LastHourSessions:   len(smc.sessionMetrics),
		LastHourAvgLatency: smc.aggregateMetrics.AverageFirstToken,
		LastHourErrorRate:  smc.aggregateMetrics.ErrorRate,
		TrendDirection:     "stable", // Could be calculated based on historical data
	}
}

// generateRecommendations provides optimization suggestions
func (smc *StreamingMetricsCollector) generateRecommendations() *RecommendedSettings {
	agg := smc.aggregateMetrics
	recommendations := &RecommendedSettings{
		OptimalBufferTime:  2 * time.Second, // Default
		OptimalConcurrency: 3,               // Default
		RecommendStreaming: true,
	}

	// Analyze performance and adjust recommendations
	if agg.AverageFirstToken > 1*time.Second {
		recommendations.ConfigurationChanges = append(recommendations.ConfigurationChanges,
			"Consider reducing STREAMING_MAX_BUFFER_TIME to improve responsiveness")
		recommendations.OptimalBufferTime = 1 * time.Second
	}

	if agg.ErrorRate > 0.1 {
		recommendations.ConfigurationChanges = append(recommendations.ConfigurationChanges,
			"High error rate detected - consider enabling STREAMING_FALLBACK_ENABLED")
		recommendations.RecommendStreaming = false
	}

	if agg.ThroughputTokensPerSec < 10 {
		recommendations.ConfigurationChanges = append(recommendations.ConfigurationChanges,
			"Low throughput - consider increasing STREAMING_AUDIO_CONCURRENCY")
		recommendations.OptimalConcurrency = 5
	}

	return recommendations
}

// assessHealthStatus determines overall streaming health
func (smc *StreamingMetricsCollector) assessHealthStatus() string {
	agg := smc.aggregateMetrics

	if agg.ErrorRate > 0.2 {
		return "critical"
	}
	if agg.ErrorRate > 0.1 || agg.AverageFirstToken > 2*time.Second {
		return "warning"
	}
	if agg.CompletedSessions > 0 {
		return "healthy"
	}

	return "unknown"
}

// ExportMetrics exports metrics in JSON format
func (smc *StreamingMetricsCollector) ExportMetrics() ([]byte, error) {
	report := smc.GeneratePerformanceReport()
	return json.MarshalIndent(report, "", "  ")
}

// Reset clears all collected metrics
func (smc *StreamingMetricsCollector) Reset() {
	smc.mu.Lock()
	defer smc.mu.Unlock()

	smc.sessionMetrics = make(map[string]*StreamingMetrics)
	smc.aggregateMetrics = &AggregateMetrics{
		StreamingEnabled: smc.enabled,
		LastUpdated:      time.Now(),
	}
	smc.startTime = time.Now()
}