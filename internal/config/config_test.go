package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear all environment variables that could affect the test
	clearEnvVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Test server defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.GRPCPort != 50051 {
		t.Errorf("Server.GRPCPort = %d, want %d", cfg.Server.GRPCPort, 50051)
	}
	if cfg.Server.DBPath != "./data/loqa-hub.db" {
		t.Errorf("Server.DBPath = %q, want %q", cfg.Server.DBPath, "./data/loqa-hub.db")
	}

	// Test STT defaults
	if cfg.STT.URL != "http://stt:8000" {
		t.Errorf("STT.URL = %q, want %q", cfg.STT.URL, "http://stt:8000")
	}
	if cfg.STT.Language != "en" {
		t.Errorf("STT.Language = %q, want %q", cfg.STT.Language, "en")
	}
	if cfg.STT.Temperature != 0.0 {
		t.Errorf("STT.Temperature = %f, want %f", cfg.STT.Temperature, 0.0)
	}

	// Test TTS defaults
	if cfg.TTS.URL != "http://localhost:8880/v1" {
		t.Errorf("TTS.URL = %q, want %q", cfg.TTS.URL, "http://localhost:8880/v1")
	}
	if cfg.TTS.Voice != "af_bella" {
		t.Errorf("TTS.Voice = %q, want %q", cfg.TTS.Voice, "af_bella")
	}
	if cfg.TTS.Speed != 1.0 {
		t.Errorf("TTS.Speed = %f, want %f", cfg.TTS.Speed, 1.0)
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "STT language configuration",
			envVars: map[string]string{
				"STT_LANGUAGE": "es",
				"STT_URL":      "http://custom-stt:9000",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.STT.Language != "es" {
					t.Errorf("STT.Language = %q, want %q", cfg.STT.Language, "es")
				}
				if cfg.STT.URL != "http://custom-stt:9000" {
					t.Errorf("STT.URL = %q, want %q", cfg.STT.URL, "http://custom-stt:9000")
				}
			},
		},
		{
			name: "Server configuration",
			envVars: map[string]string{
				"LOQA_HOST":      "127.0.0.1",
				"LOQA_PORT":      "3000",
				"LOQA_GRPC_PORT": "50052",
				"LOQA_DB_PATH":   "/custom/path/db.sqlite",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.Host != "127.0.0.1" {
					t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
				}
				if cfg.Server.Port != 3000 {
					t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 3000)
				}
				if cfg.Server.GRPCPort != 50052 {
					t.Errorf("Server.GRPCPort = %d, want %d", cfg.Server.GRPCPort, 50052)
				}
				if cfg.Server.DBPath != "/custom/path/db.sqlite" {
					t.Errorf("Server.DBPath = %q, want %q", cfg.Server.DBPath, "/custom/path/db.sqlite")
				}
			},
		},
		{
			name: "TTS configuration",
			envVars: map[string]string{
				"TTS_URL":              "http://custom-tts:8881/v1",
				"TTS_VOICE":            "en_male",
				"TTS_SPEED":            "1.5",
				"TTS_FORMAT":           "wav",
				"TTS_MAX_CONCURRENT":   "15",
				"TTS_NORMALIZE":        "false",
				"TTS_TIMEOUT":          "15s",
				"TTS_FALLBACK_ENABLED": "false",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.TTS.URL != "http://custom-tts:8881/v1" {
					t.Errorf("TTS.URL = %q, want %q", cfg.TTS.URL, "http://custom-tts:8881/v1")
				}
				if cfg.TTS.Voice != "en_male" {
					t.Errorf("TTS.Voice = %q, want %q", cfg.TTS.Voice, "en_male")
				}
				if cfg.TTS.Speed != 1.5 {
					t.Errorf("TTS.Speed = %f, want %f", cfg.TTS.Speed, 1.5)
				}
				if cfg.TTS.ResponseFormat != "wav" {
					t.Errorf("TTS.ResponseFormat = %q, want %q", cfg.TTS.ResponseFormat, "wav")
				}
				if cfg.TTS.MaxConcurrent != 15 {
					t.Errorf("TTS.MaxConcurrent = %d, want %d", cfg.TTS.MaxConcurrent, 15)
				}
				if cfg.TTS.Normalize != false {
					t.Errorf("TTS.Normalize = %v, want %v", cfg.TTS.Normalize, false)
				}
				if cfg.TTS.Timeout != 15*time.Second {
					t.Errorf("TTS.Timeout = %v, want %v", cfg.TTS.Timeout, 15*time.Second)
				}
				if cfg.TTS.FallbackEnabled != false {
					t.Errorf("TTS.FallbackEnabled = %v, want %v", cfg.TTS.FallbackEnabled, false)
				}
			},
		},
		{
			name: "Language variations",
			envVars: map[string]string{
				"STT_LANGUAGE": "fr",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.STT.Language != "fr" {
					t.Errorf("STT.Language = %q, want %q", cfg.STT.Language, "fr")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment and set test vars
			clearEnvVars()
			for key, value := range tt.envVars {
				_ = os.Setenv(key, value)
			}
			defer clearEnvVars()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			tt.validate(t, cfg)
		})
	}
}

func TestLoad_InvalidConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		envVars       map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name: "Invalid server port",
			envVars: map[string]string{
				"LOQA_PORT": "0",
			},
			expectError:   true,
			errorContains: "invalid server port",
		},
		{
			name: "Invalid gRPC port",
			envVars: map[string]string{
				"LOQA_GRPC_PORT": "99999",
			},
			expectError:   true,
			errorContains: "invalid gRPC port",
		},
		{
			name: "Valid configuration",
			envVars: map[string]string{
				"STT_LANGUAGE": "en",
				"LOQA_PORT":    "3000",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			for key, value := range tt.envVars {
				_ = os.Setenv(key, value)
			}
			defer clearEnvVars()

			_, err := Load()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSTTLanguageConfigurationBackwardsCompatibility(t *testing.T) {
	// Test that the STT language configuration works as expected
	tests := []struct {
		name        string
		sttLanguage string
		expected    string
	}{
		{
			name:        "Default English",
			sttLanguage: "",
			expected:    "en",
		},
		{
			name:        "Spanish",
			sttLanguage: "es",
			expected:    "es",
		},
		{
			name:        "French",
			sttLanguage: "fr",
			expected:    "fr",
		},
		{
			name:        "German",
			sttLanguage: "de",
			expected:    "de",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			if tt.sttLanguage != "" {
				_ = os.Setenv("STT_LANGUAGE", tt.sttLanguage)
			}
			defer clearEnvVars()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if cfg.STT.Language != tt.expected {
				t.Errorf("STT.Language = %q, want %q", cfg.STT.Language, tt.expected)
			}
		})
	}
}

// Helper function to clear environment variables used in tests
func clearEnvVars() {
	envVars := []string{
		"LOQA_HOST", "LOQA_PORT", "LOQA_GRPC_PORT", "LOQA_DB_PATH",
		"LOQA_READ_TIMEOUT", "LOQA_WRITE_TIMEOUT",
		"STT_URL", "STT_LANGUAGE", "STT_TEMPERATURE", "STT_MAX_TOKENS",
		"TTS_URL", "TTS_VOICE", "TTS_SPEED", "TTS_FORMAT", "TTS_NORMALIZE",
		"TTS_MAX_CONCURRENT", "TTS_TIMEOUT", "TTS_FALLBACK_ENABLED",
		"STREAMING_ENABLED", "STREAMING_MODEL", "STREAMING_MAX_BUFFER_TIME",
		"STREAMING_MAX_TOKENS_PER_PHRASE", "STREAMING_AUDIO_CONCURRENCY",
		"STREAMING_VISUAL_DELAY", "STREAMING_INTERRUPT_TIMEOUT",
		"STREAMING_FALLBACK_ENABLED", "STREAMING_METRICS_ENABLED",
		"OLLAMA_URL", "LOG_LEVEL", "LOG_FORMAT",
		"NATS_URL", "NATS_SUBJECT", "NATS_MAX_RECONNECT", "NATS_RECONNECT_WAIT",
		"LOQA_DATA_RETENTION", "LOQA_ZERO_PERSISTENCE", "LOQA_AUTO_CLEANUP", "LOQA_CLEANUP_INTERVAL",
	}

	for _, envVar := range envVars {
		_ = os.Unsetenv(envVar)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || indexOf(s, substr) >= 0)
}

// Helper function to find index of substring
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
