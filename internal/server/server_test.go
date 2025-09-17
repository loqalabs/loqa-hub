package server

import (
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
)

func TestNew_WithGlobalConfig(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			GRPCPort:     50051,
			DBPath:       ":memory:", // Use in-memory SQLite for testing
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

	if server.grpcServer == nil {
		t.Error("Server gRPC server not initialized")
	}

	if server.database == nil {
		t.Error("Server database not initialized")
	}

	if server.eventsStore == nil {
		t.Error("Server events store not initialized")
	}

	if server.apiHandler == nil {
		t.Error("Server API handler not initialized")
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
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host:         "0.0.0.0",
					Port:         8080,
					GRPCPort:     50051,
					DBPath:       ":memory:",
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