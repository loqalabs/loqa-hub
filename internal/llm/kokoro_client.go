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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"go.uber.org/zap"
)

// KokoroRequest represents a request to the Kokoro TTS API
type KokoroRequest struct {
	Model   string                 `json:"model"`
	Input   string                 `json:"input"`
	Voice   string                 `json:"voice"`
	Format  string                 `json:"response_format"`
	Speed   float32                `json:"speed,omitempty"`
	Options map[string]interface{} `json:"normalization_options,omitempty"`
}

// KokoroVoicesResponse represents the response from the voices endpoint
type KokoroVoicesResponse struct {
	Voices []string `json:"voices"`
}

// KokoroClient implements TextToSpeech interface for Kokoro-82M TTS
type KokoroClient struct {
	baseURL         string
	client          *http.Client
	config          config.TTSConfig
	semaphore       chan struct{} // Limits concurrent requests
	mu              sync.RWMutex
	cachedVoices    []string
	voicesCacheTime time.Time
}

// NewKokoroClient creates a new Kokoro TTS client
func NewKokoroClient(cfg config.TTSConfig) (*KokoroClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("Kokoro TTS URL cannot be empty")
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	// Create semaphore to limit concurrent requests
	semaphore := make(chan struct{}, cfg.MaxConcurrent)

	kokoroClient := &KokoroClient{
		baseURL:   strings.TrimSuffix(cfg.URL, "/"),
		client:    client,
		config:    cfg,
		semaphore: semaphore,
	}

	// Test connection
	if err := kokoroClient.testConnection(); err != nil {
		return nil, fmt.Errorf("failed to connect to Kokoro TTS service: %w", err)
	}

	if logging.Sugar != nil {
		logging.Sugar.Infow("🔊 Kokoro TTS client initialized",
			"url", cfg.URL,
			"voice", cfg.Voice,
			"max_concurrent", cfg.MaxConcurrent,
		)
	}

	return kokoroClient, nil
}

// Synthesize converts text to speech using Kokoro-82M
func (k *KokoroClient) Synthesize(text string, options *TTSOptions) (*TTSResult, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// Acquire semaphore slot for concurrency control
	select {
	case k.semaphore <- struct{}{}:
		defer func() { <-k.semaphore }()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("TTS synthesis queue full, request timed out")
	}

	startTime := time.Now()

	// Prepare request options
	voice := k.config.Voice
	speed := k.config.Speed
	format := k.config.ResponseFormat
	normalize := k.config.Normalize

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
	request := KokoroRequest{
		Model:  "kokoro",
		Input:  text,
		Voice:  voice,
		Format: format,
		Speed:  speed,
	}

	// Add normalization options if needed
	if !normalize {
		request.Options = map[string]interface{}{
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
	ctx, cancel := context.WithTimeout(context.Background(), k.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", k.baseURL+"/audio/speech", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/*")

	resp, err := k.client.Do(req)
	if err != nil {
		if logging.Logger != nil {
			logging.LogError(err, "Kokoro TTS HTTP request failed",
				zap.String("voice", voice),
				zap.Int("text_length", len(text)),
			)
		}
		return nil, fmt.Errorf("TTS HTTP request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if logging.Logger != nil {
			logging.LogWarn("Kokoro TTS request failed",
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
	}, nil
}

// GetAvailableVoices returns the list of available voices
func (k *KokoroClient) GetAvailableVoices() ([]string, error) {
	k.mu.RLock()
	// Return cached voices if they're fresh (cache for 1 hour)
	if len(k.cachedVoices) > 0 && time.Since(k.voicesCacheTime) < time.Hour {
		voices := make([]string, len(k.cachedVoices))
		copy(voices, k.cachedVoices)
		k.mu.RUnlock()
		return voices, nil
	}
	k.mu.RUnlock()

	// Fetch voices from API
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", k.baseURL+"/audio/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create voices request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch voices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voices request failed with status %d", resp.StatusCode)
	}

	var voicesResponse KokoroVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&voicesResponse); err != nil {
		return nil, fmt.Errorf("failed to decode voices response: %w", err)
	}

	// Update cache
	k.mu.Lock()
	k.cachedVoices = make([]string, len(voicesResponse.Voices))
	copy(k.cachedVoices, voicesResponse.Voices)
	k.voicesCacheTime = time.Now()
	k.mu.Unlock()

	if logging.Sugar != nil {
		logging.Sugar.Debugw("🔊 Retrieved available voices",
			"count", len(voicesResponse.Voices),
			"voices", voicesResponse.Voices,
		)
	}

	return voicesResponse.Voices, nil
}

// Close cleans up resources
func (k *KokoroClient) Close() error {
	k.client.CloseIdleConnections()
	return nil
}

// testConnection tests the connection to the Kokoro TTS service
func (k *KokoroClient) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", k.baseURL+"/audio/voices", nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service health check failed with status %d", resp.StatusCode)
	}

	return nil
}
