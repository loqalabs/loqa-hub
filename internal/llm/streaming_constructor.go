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
	"fmt"
	"log"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
)

// StreamingComponents holds all streaming-related components
type StreamingComponents struct {
	Parser           *StreamingCommandParser
	AudioPipeline    *StreamingAudioPipeline
	InterruptHandler *StreamingInterruptHandler
	MetricsCollector *StreamingMetricsCollector
}

// NewStreamingComponents creates and initializes all streaming components
func NewStreamingComponents(cfg *config.Config, ttsClient TextToSpeech) (*StreamingComponents, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	streamingCfg := &cfg.Streaming

	log.Printf("🌊 Initializing streaming components (enabled: %v)", streamingCfg.Enabled)

	// Create streaming command parser
	streamingParser := NewStreamingCommandParser(
		streamingCfg.OllamaURL,
		streamingCfg.Model,
		streamingCfg.Enabled,
	)

	// Create TTS options from configuration
	ttsOptions := &TTSOptions{
		Voice:          cfg.TTS.Voice,
		Speed:          cfg.TTS.Speed,
		ResponseFormat: cfg.TTS.ResponseFormat,
		Normalize:      cfg.TTS.Normalize,
	}

	// Create streaming audio pipeline
	audioPipeline := NewStreamingAudioPipeline(ttsClient, ttsOptions)

	// Update concurrency settings
	if streamingCfg.AudioConcurrency > 0 {
		audioPipeline.maxConcurrent = streamingCfg.AudioConcurrency
	}

	// Create interrupt handler with configured timeouts
	interruptHandler := NewStreamingInterruptHandler(
		streamingCfg.InterruptTimeout,
		streamingCfg.InterruptTimeout*2, // Force timeout is 2x grace period
	)

	// Create metrics collector
	metricsCollector := NewStreamingMetricsCollector(streamingCfg.MetricsEnabled)

	components := &StreamingComponents{
		Parser:           streamingParser,
		AudioPipeline:    audioPipeline,
		InterruptHandler: interruptHandler,
		MetricsCollector: metricsCollector,
	}

	// Test streaming connection if enabled
	if streamingCfg.Enabled {
		if err := streamingParser.TestStreamingConnection(); err != nil {
			if streamingCfg.FallbackEnabled {
				log.Printf("⚠️ Streaming test failed, fallback enabled: %v", err)
			} else {
				return nil, fmt.Errorf("streaming test failed and fallback disabled: %w", err)
			}
		}
	}

	log.Printf("✅ Streaming components initialized successfully")
	return components, nil
}

// UpdateConfiguration updates streaming components with new configuration
func (sc *StreamingComponents) UpdateConfiguration(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	streamingCfg := &cfg.Streaming

	// Update parser configuration
	sc.Parser.enabled = streamingCfg.Enabled
	sc.Parser.ollamaURL = streamingCfg.OllamaURL
	sc.Parser.model = streamingCfg.Model

	// Update TTS options
	ttsOptions := &TTSOptions{
		Voice:          cfg.TTS.Voice,
		Speed:          cfg.TTS.Speed,
		ResponseFormat: cfg.TTS.ResponseFormat,
		Normalize:      cfg.TTS.Normalize,
	}
	sc.AudioPipeline.UpdateTTSOptions(ttsOptions)

	// Update concurrency
	if streamingCfg.AudioConcurrency > 0 {
		sc.AudioPipeline.maxConcurrent = streamingCfg.AudioConcurrency
	}

	// Update interrupt handler timeouts
	sc.InterruptHandler.gracePeriod = streamingCfg.InterruptTimeout
	sc.InterruptHandler.forceTimeout = streamingCfg.InterruptTimeout * 2

	// Update metrics collector
	sc.MetricsCollector.enabled = streamingCfg.MetricsEnabled

	log.Printf("🔄 Updated streaming configuration")
	return nil
}

// ProcessStreamingCommand processes a transcription using streaming
func (sc *StreamingComponents) ProcessStreamingCommand(transcription, sessionID string) (*StreamingResult, error) {
	// Record session start in metrics
	sc.MetricsCollector.RecordSessionStart(sessionID)

	// Create streaming context
	ctx := sc.InterruptHandler.activeStreams[sessionID]
	if ctx == nil {
		return nil, fmt.Errorf("no context found for session %s", sessionID)
	}

	// Start streaming command parsing
	result, err := sc.Parser.ParseCommandStreaming(ctx.Context, transcription)
	if err != nil {
		return nil, fmt.Errorf("streaming command parsing failed: %w", err)
	}

	// Register session for interrupt management
	sc.InterruptHandler.RegisterSession(
		sessionID,
		ctx.Context,
		ctx.Cancel,
		result,
		nil, // Audio pipeline context will be set later
	)

	// Start audio pipeline if we have phrases to synthesize
	go sc.processAudioStream(sessionID, result)

	return result, nil
}

// processAudioStream manages the streaming audio synthesis pipeline
func (sc *StreamingComponents) processAudioStream(sessionID string, result *StreamingResult) {
	// Start audio pipeline
	pipelineCtx, err := sc.AudioPipeline.StartPipeline(sessionID, result.AudioPhrases)
	if err != nil {
		log.Printf("❌ Failed to start audio pipeline for session %s: %v", sessionID, err)
		select {
		case result.ErrorChan <- err:
		default:
		}
		return
	}

	// Update interrupt handler with pipeline context
	if session, exists := sc.InterruptHandler.activeStreams[sessionID]; exists {
		session.AudioPipeline = pipelineCtx
	}

	// Monitor for completion or errors
	go func() {
		defer sc.AudioPipeline.StopPipeline(sessionID)

		for {
			select {
			case audioChunk, ok := <-pipelineCtx.AudioChunks:
				if !ok {
					// Pipeline finished
					return
				}
				if audioChunk.IsLast {
					// Final audio chunk received
					sc.recordSessionCompletion(sessionID, result)
					return
				}
				// Audio chunk available for streaming to client
				// This would be handled by the gRPC service layer

			case err := <-pipelineCtx.ErrorChan:
				log.Printf("❌ Audio pipeline error for session %s: %v", sessionID, err)
				select {
				case result.ErrorChan <- err:
				default:
				}
				return

			case <-result.FinalCommand:
				// Command parsing completed, let audio finish
				// Continue processing until audio pipeline completes
			}
		}
	}()
}

// recordSessionCompletion records final metrics for a completed session
func (sc *StreamingComponents) recordSessionCompletion(sessionID string, result *StreamingResult) {
	if result.Metrics != nil {
		sc.MetricsCollector.RecordSessionMetrics(sessionID, result.Metrics)
	}

	log.Printf("✅ Streaming session %s completed", sessionID)
}

// Shutdown gracefully shuts down all streaming components
func (sc *StreamingComponents) Shutdown() {
	log.Printf("🔄 Shutting down streaming components...")

	// Stop all active sessions
	sc.InterruptHandler.Shutdown()

	// Stop all audio pipelines
	activePipelines := sc.AudioPipeline.GetActivePipelines()
	for _, pipelineID := range activePipelines {
		sc.AudioPipeline.StopPipeline(pipelineID)
	}

	// Allow time for graceful shutdown
	time.Sleep(1 * time.Second)

	log.Printf("✅ Streaming components shutdown completed")
}

// GetHealthStatus returns the health status of streaming components
func (sc *StreamingComponents) GetHealthStatus() *StreamingHealthStatus {
	return &StreamingHealthStatus{
		ParserEnabled:   sc.Parser.enabled,
		ActiveSessions:  len(sc.InterruptHandler.GetActiveSessionIDs()),
		ActivePipelines: len(sc.AudioPipeline.GetActivePipelines()),
		MetricsEnabled:  sc.MetricsCollector.enabled,
		OverallHealth:   sc.MetricsCollector.assessHealthStatus(),
		LastHealthCheck: time.Now(),
	}
}

// StreamingHealthStatus provides health information for streaming components
type StreamingHealthStatus struct {
	ParserEnabled   bool      `json:"parser_enabled"`
	ActiveSessions  int       `json:"active_sessions"`
	ActivePipelines int       `json:"active_pipelines"`
	MetricsEnabled  bool      `json:"metrics_enabled"`
	OverallHealth   string    `json:"overall_health"`
	LastHealthCheck time.Time `json:"last_health_check"`
}
