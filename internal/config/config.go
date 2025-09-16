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

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Loqa hub
type Config struct {
	Server    ServerConfig
	STT       STTConfig
	TTS       TTSConfig
	Streaming StreamingConfig
	Logging   LoggingConfig
	NATS      NATSConfig
	Privacy   PrivacyConfig
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host         string
	Port         int
	GRPCPort     int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// STTConfig holds Speech-to-Text service configuration
type STTConfig struct {
	URL         string // REST API URL for OpenAI-compatible STT service
	Language    string
	Temperature float32
	MaxTokens   int
}

// TTSConfig holds Text-to-Speech service configuration
type TTSConfig struct {
	URL             string        // REST API URL for OpenAI-compatible TTS service
	Voice           string        // Default voice to use (e.g., "af_bella")
	Speed           float32       // Speech speed (1.0 = normal)
	ResponseFormat  string        // Audio format (mp3, wav, opus, flac)
	Normalize       bool          // Enable text normalization
	MaxConcurrent   int           // Maximum concurrent TTS requests
	Timeout         time.Duration // Request timeout
	FallbackEnabled bool          // Enable fallback to legacy TTS if available
}

// StreamingConfig holds real-time streaming configuration
type StreamingConfig struct {
	Enabled              bool          // Enable streaming LLM responses
	OllamaURL           string        // Ollama API URL for streaming
	Model               string        // LLM model to use for streaming
	MaxBufferTime       time.Duration // Maximum time to buffer tokens before synthesis
	MaxTokensPerPhrase  int           // Maximum tokens to buffer per phrase
	AudioConcurrency    int           // Number of parallel TTS synthesis workers
	VisualFeedbackDelay time.Duration // Delay before showing visual tokens (for smoothness)
	InterruptTimeout    time.Duration // Timeout for graceful stream interruption
	FallbackEnabled     bool          // Enable fallback to non-streaming mode on errors
	MetricsEnabled      bool          // Enable streaming performance metrics
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
}

// NATSConfig holds NATS messaging configuration
type NATSConfig struct {
	URL           string
	Subject       string
	MaxReconnect  int
	ReconnectWait time.Duration
}

// PrivacyConfig holds privacy and data retention configuration
type PrivacyConfig struct {
	// DataRetention controls how long voice event data is kept
	DataRetention time.Duration

	// ZeroPersistence disables all persistent storage of voice events (RAM only)
	ZeroPersistence bool

	// AutoCleanupEnabled enables automatic cleanup of old voice events
	AutoCleanupEnabled bool

	// CleanupInterval controls how often the cleanup job runs
	CleanupInterval time.Duration
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Host:         getEnvString("LOQA_HOST", "0.0.0.0"),
			Port:         getEnvInt("LOQA_PORT", 8080),
			GRPCPort:     getEnvInt("LOQA_GRPC_PORT", 50051),
			ReadTimeout:  getEnvDuration("LOQA_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("LOQA_WRITE_TIMEOUT", 30*time.Second),
		},
		STT: STTConfig{
			URL:         getEnvString("STT_URL", "http://stt:8000"),
			Language:    getEnvString("STT_LANGUAGE", "en"),
			Temperature: getEnvFloat32("STT_TEMPERATURE", 0.0),
			MaxTokens:   getEnvInt("STT_MAX_TOKENS", 224),
		},
		TTS: TTSConfig{
			URL:             getEnvString("TTS_URL", "http://localhost:8880/v1"),
			Voice:           getEnvString("TTS_VOICE", "af_bella"),
			Speed:           getEnvFloat32("TTS_SPEED", 1.0),
			ResponseFormat:  getEnvString("TTS_FORMAT", "mp3"),
			Normalize:       getEnvBool("TTS_NORMALIZE", true),
			MaxConcurrent:   getEnvInt("TTS_MAX_CONCURRENT", 10),
			Timeout:         getEnvDuration("TTS_TIMEOUT", 10*time.Second),
			FallbackEnabled: getEnvBool("TTS_FALLBACK_ENABLED", true),
		},
		Streaming: StreamingConfig{
			Enabled:              getEnvBool("STREAMING_ENABLED", false), // Default disabled for gradual rollout
			OllamaURL:           getEnvString("OLLAMA_URL", "http://ollama:11434"),
			Model:               getEnvString("STREAMING_MODEL", "llama3.2:3b"),
			MaxBufferTime:       getEnvDuration("STREAMING_MAX_BUFFER_TIME", 2*time.Second),
			MaxTokensPerPhrase:  getEnvInt("STREAMING_MAX_TOKENS_PER_PHRASE", 50),
			AudioConcurrency:    getEnvInt("STREAMING_AUDIO_CONCURRENCY", 3),
			VisualFeedbackDelay: getEnvDuration("STREAMING_VISUAL_DELAY", 50*time.Millisecond),
			InterruptTimeout:    getEnvDuration("STREAMING_INTERRUPT_TIMEOUT", 500*time.Millisecond),
			FallbackEnabled:     getEnvBool("STREAMING_FALLBACK_ENABLED", true),
			MetricsEnabled:      getEnvBool("STREAMING_METRICS_ENABLED", true),
		},
		Logging: LoggingConfig{
			Level:  getEnvString("LOG_LEVEL", "info"),
			Format: getEnvString("LOG_FORMAT", "json"),
		},
		NATS: NATSConfig{
			URL:           getEnvString("NATS_URL", "nats://localhost:4222"),
			Subject:       getEnvString("NATS_SUBJECT", "loqa.commands"),
			MaxReconnect:  getEnvInt("NATS_MAX_RECONNECT", 10),
			ReconnectWait: getEnvDuration("NATS_RECONNECT_WAIT", 2*time.Second),
		},
		Privacy: PrivacyConfig{
			DataRetention:      getEnvDuration("LOQA_DATA_RETENTION", 30*24*time.Hour), // 30 days default
			ZeroPersistence:    getEnvBool("LOQA_ZERO_PERSISTENCE", false),             // false by default
			AutoCleanupEnabled: getEnvBool("LOQA_AUTO_CLEANUP", true),                  // true by default
			CleanupInterval:    getEnvDuration("LOQA_CLEANUP_INTERVAL", 24*time.Hour),  // daily cleanup
		},
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}

	if c.STT.URL == "" {
		return fmt.Errorf("STT URL must be provided")
	}

	if c.TTS.URL == "" {
		return fmt.Errorf("TTS URL must be provided")
	}

	if c.TTS.MaxConcurrent <= 0 {
		return fmt.Errorf("TTS max concurrent must be positive: %d", c.TTS.MaxConcurrent)
	}

	if c.TTS.Speed <= 0 {
		return fmt.Errorf("TTS speed must be positive: %f", c.TTS.Speed)
	}

	if c.Privacy.DataRetention < 0 {
		return fmt.Errorf("data retention duration cannot be negative: %s", c.Privacy.DataRetention)
	}

	if c.Privacy.CleanupInterval <= 0 {
		return fmt.Errorf("cleanup interval must be positive: %s", c.Privacy.CleanupInterval)
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat32(key string, defaultValue float32) float32 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(floatValue)
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
