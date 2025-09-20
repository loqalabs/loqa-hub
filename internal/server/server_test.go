package server

import (
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/logging"
)

func TestNew_WithHTTPStreamingConfig(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Create a test configuration for new HTTP/1.1 streaming architecture
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "0.0.0.0",
			Port:         3000,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
		STT: config.STTConfig{
			URL:         "http://localhost:8000",
			Language:    "en",
			Temperature: 0.0,
		},
		TTS: config.TTSConfig{
			URL:             "http://localhost:8880/v1",
			Voice:           "af_bella",
			Speed:           1.0,
			ResponseFormat:  "mp3",
			Normalize:       true,
			MaxConcurrent:   10,
			Timeout:         10 * time.Second,
			FallbackEnabled: true,
		},
		Streaming: config.StreamingConfig{
			Enabled:             false, // Disabled for testing
			OllamaURL:           "http://localhost:11434",
			Model:               "llama3.2:3b",
			MaxBufferTime:       2 * time.Second,
			MaxTokensPerPhrase:  50,
			AudioConcurrency:    3,
			VisualFeedbackDelay: 50 * time.Millisecond,
			InterruptTimeout:    500 * time.Millisecond,
			FallbackEnabled:     true,
			MetricsEnabled:      false, // Disabled for testing
		},
		NATS: config.NATSConfig{
			URL:           "nats://localhost:4222",
			Subject:       "loqa.test",
			MaxReconnect:  10,
			ReconnectWait: 2 * time.Second,
		},
	}

	// Test that server creation doesn't panic and returns a valid server
	// Use NewWithOptions to skip health checks during testing
	server := NewWithOptions(cfg, false)
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify server has the expected configuration
	if server.cfg != cfg {
		t.Error("Server configuration not set correctly")
	}

	// Verify server components are initialized
	if server.mux == nil {
		t.Error("Server mux not initialized")
	}

	if server.streamTransport == nil {
		t.Error("Server HTTP streaming transport not initialized")
	}

	if server.arbitrator == nil {
		t.Error("Server arbitrator not initialized")
	}

	if server.intentProcessor == nil {
		t.Error("Server intent processor not initialized")
	}

	if server.tierDetector == nil {
		t.Error("Server tier detector not initialized")
	}

	// Note: audioService may be nil if STT/TTS services are not available during tests
	// This is acceptable for unit testing server initialization
}

func TestNew_STTLanguageConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		language string
		expected string
	}{
		{
			name:     "English language",
			language: "en",
			expected: "en",
		},
		{
			name:     "Spanish language",
			language: "es",
			expected: "es",
		},
		{
			name:     "French language",
			language: "fr",
			expected: "fr",
		},
		{
			name:     "Empty language passed through as-is",
			language: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize logging for test
			if err := logging.Initialize(); err != nil {
				t.Fatalf("Failed to initialize logging: %v", err)
			}
			defer logging.Close()
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host:         "0.0.0.0",
					Port:         3000,
					ReadTimeout:  15 * time.Second,
					WriteTimeout: 15 * time.Second,
				},
				STT: config.STTConfig{
					URL:         "http://localhost:8000",
					Language:    tt.language,
					Temperature: 0.0,
				},
				TTS: config.TTSConfig{
					URL:             "http://localhost:8880/v1",
					Voice:           "af_bella",
					Speed:           1.0,
					ResponseFormat:  "mp3",
					Normalize:       true,
					MaxConcurrent:   10,
					Timeout:         10 * time.Second,
					FallbackEnabled: true,
				},
			}

			// Test that server creation works with different language configurations
			// Use NewWithOptions to skip health checks during testing
			server := NewWithOptions(cfg, false)
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			// Verify the language configuration is passed through
			if server.cfg.STT.Language != tt.expected {
				t.Errorf("Expected STT language %q, got %q", tt.expected, server.cfg.STT.Language)
			}
		})
	}
}
