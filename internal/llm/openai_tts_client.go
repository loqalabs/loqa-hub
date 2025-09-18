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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"go.uber.org/zap"
)

// OpenAITTSRequest represents a request to an OpenAI-compatible TTS API
type OpenAITTSRequest struct {
	Model   string         `json:"model"`
	Input   string         `json:"input"`
	Voice   string         `json:"voice"`
	Format  string         `json:"response_format"`
	Speed   float32        `json:"speed,omitempty"`
	Options map[string]any `json:"normalization_options,omitempty"`
}

// OpenAITTSVoicesResponse represents the response from the voices endpoint
type OpenAITTSVoicesResponse struct {
	Voices []string `json:"voices"`
}

// OpenAITTSClient implements TextToSpeech interface for OpenAI-compatible TTS services
type OpenAITTSClient struct {
	baseURL         string
	client          *http.Client
	config          config.TTSConfig
	semaphore       chan struct{} // Limits concurrent requests
	mu              sync.RWMutex
	cachedVoices    []string
	voicesCacheTime time.Time
}

// NewOpenAITTSClient creates a new OpenAI-compatible TTS client
func NewOpenAITTSClient(cfg config.TTSConfig) (*OpenAITTSClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("TTS URL cannot be empty")
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	// Create semaphore to limit concurrent requests
	semaphore := make(chan struct{}, cfg.MaxConcurrent)

	ttsClient := &OpenAITTSClient{
		baseURL:   strings.TrimSuffix(cfg.URL, "/"),
		client:    client,
		config:    cfg,
		semaphore: semaphore,
	}

	// Test connection
	if err := ttsClient.testConnection(); err != nil {
		return nil, fmt.Errorf("failed to connect to TTS service: %w", err)
	}

	if logging.Sugar != nil {
		logging.Sugar.Infow("🔊 TTS client initialized",
			"url", cfg.URL,
			"voice", cfg.Voice,
			"max_concurrent", cfg.MaxConcurrent,
		)
	}

	return ttsClient, nil
}

// Synthesize converts text to speech using OpenAI-compatible TTS
func (c *OpenAITTSClient) Synthesize(text string, options *TTSOptions) (*TTSResult, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// Acquire semaphore slot for concurrency control
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("TTS synthesis queue full, request timed out")
	}

	startTime := time.Now()

	// Prepare request options
	voice := c.config.Voice
	speed := c.config.Speed
	format := c.config.ResponseFormat
	normalize := c.config.Normalize

	if options != nil {
		if options.Voice != "" {
			voice = options.Voice
		}
		if options.Speed > 0 {
			speed = options.Speed
		}
		if options.ResponseFormat != "" {
			format = options.ResponseFormat
		}
		normalize = options.Normalize
	}

	// Build request payload
	request := OpenAITTSRequest{
		Model:  "tts-1",
		Input:  text,
		Voice:  voice,
		Format: format,
		Speed:  speed,
	}

	// Add normalization options if needed
	if !normalize {
		request.Options = map[string]any{
			"normalize": false,
		}
	}

	// Serialize request
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TTS request: %w", err)
	}

	if logging.Logger != nil {
		logging.LogTTSOperation("synthesis_start",
			zap.String("voice", voice),
			zap.Int("text_length", len(text)),
			zap.String("format", format),
			zap.Float32("speed", speed),
		)
	}

	// Make HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	// Note: Do not defer cancel() here as the context is needed to read the response body

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/audio/speech", bytes.NewBuffer(requestBody))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/*")

	resp, err := c.client.Do(req)
	if err != nil {
		cancel()
		if logging.Logger != nil {
			logging.LogError(err, "TTS HTTP request failed",
				zap.String("voice", voice),
				zap.Int("text_length", len(text)),
			)
		}
		return nil, fmt.Errorf("TTS HTTP request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
		cancel()
		if logging.Logger != nil {
			logging.LogWarn("TTS request failed",
				zap.Int("status_code", resp.StatusCode),
				zap.String("response_body", string(body)),
			)
		}
		return nil, fmt.Errorf("TTS request failed with status %d: %s", resp.StatusCode, string(body))
	}

	processingTime := time.Since(startTime)

	if logging.Logger != nil {
		logging.LogTTSOperation("synthesis_complete",
			zap.String("voice", voice),
			zap.Int("text_length", len(text)),
			zap.Duration("processing_time", processingTime),
			zap.String("content_type", resp.Header.Get("Content-Type")),
			zap.Int64("content_length", resp.ContentLength),
		)
	}

	return &TTSResult{
		Audio:       resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
		Length:      resp.ContentLength,
		Cleanup:     cancel,
	}, nil
}

// GetAvailableVoices returns the list of available voices
func (c *OpenAITTSClient) GetAvailableVoices() ([]string, error) {
	c.mu.RLock()
	// Return cached voices if they're fresh (cache for 1 hour)
	if len(c.cachedVoices) > 0 && time.Since(c.voicesCacheTime) < time.Hour {
		voices := make([]string, len(c.cachedVoices))
		copy(voices, c.cachedVoices)
		c.mu.RUnlock()
		return voices, nil
	}
	c.mu.RUnlock()

	// Fetch voices from API
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/audio/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create voices request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch voices: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voices request failed with status %d", resp.StatusCode)
	}

	var voicesResponse OpenAITTSVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&voicesResponse); err != nil {
		return nil, fmt.Errorf("failed to decode voices response: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.cachedVoices = make([]string, len(voicesResponse.Voices))
	copy(c.cachedVoices, voicesResponse.Voices)
	c.voicesCacheTime = time.Now()
	c.mu.Unlock()

	if logging.Sugar != nil {
		logging.Sugar.Debugw("🔊 Retrieved available voices",
			"count", len(voicesResponse.Voices),
			"voices", voicesResponse.Voices,
		)
	}

	return voicesResponse.Voices, nil
}

// Close cleans up resources
func (c *OpenAITTSClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

// testConnection tests the connection to the TTS service
func (c *OpenAITTSClient) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/audio/voices", nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service health check failed with status %d", resp.StatusCode)
	}

	return nil
}
