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

package logging

import (
	"errors"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInitialize(t *testing.T) {
	// Save original environment
	originalLevel := os.Getenv("LOG_LEVEL")
	originalFormat := os.Getenv("LOG_FORMAT")
	defer func() {
		_ = os.Setenv("LOG_LEVEL", originalLevel)
		_ = os.Setenv("LOG_FORMAT", originalFormat)
	}()

	tests := []struct {
		name      string
		logLevel  string
		logFormat string
		wantErr   bool
	}{
		{
			name:      "Default values",
			logLevel:  "",
			logFormat: "",
			wantErr:   false,
		},
		{
			name:      "Info level console format",
			logLevel:  "info",
			logFormat: "console",
			wantErr:   false,
		},
		{
			name:      "Debug level JSON format",
			logLevel:  "debug",
			logFormat: "json",
			wantErr:   false,
		},
		{
			name:      "Error level JSON format",
			logLevel:  "error",
			logFormat: "json",
			wantErr:   false,
		},
		{
			name:      "Warn level console format",
			logLevel:  "warn",
			logFormat: "console",
			wantErr:   false,
		},
		{
			name:      "Invalid format defaults to console",
			logLevel:  "info",
			logFormat: "invalid",
			wantErr:   false,
		},
		{
			name:      "Invalid level defaults to info",
			logLevel:  "invalid",
			logFormat: "console",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.logLevel != "" {
				_ = os.Setenv("LOG_LEVEL", tt.logLevel)
			} else {
				_ = os.Unsetenv("LOG_LEVEL")
			}
			if tt.logFormat != "" {
				_ = os.Setenv("LOG_FORMAT", tt.logFormat)
			} else {
				_ = os.Unsetenv("LOG_FORMAT")
			}

			err := Initialize()

			if tt.wantErr {
				if err == nil {
					t.Error("Initialize() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Initialize() unexpected error: %v", err)
				return
			}

			// Verify logger was initialized
			if Logger == nil {
				t.Error("Logger should not be nil after initialization")
			}
			if Sugar == nil {
				t.Error("Sugar should not be nil after initialization")
			}

			// Clean up
			Close()
		})
	}
}

func TestInitializeWithConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  LogConfig
		wantErr bool
	}{
		{
			name: "Console format info level",
			config: LogConfig{
				Level:  "info",
				Format: "console",
			},
			wantErr: false,
		},
		{
			name: "JSON format debug level",
			config: LogConfig{
				Level:  "debug",
				Format: "json",
			},
			wantErr: false,
		},
		{
			name: "Invalid format defaults to console",
			config: LogConfig{
				Level:  "info",
				Format: "invalid",
			},
			wantErr: false,
		},
		{
			name: "Invalid level defaults to info",
			config: LogConfig{
				Level:  "invalid",
				Format: "console",
			},
			wantErr: false,
		},
		{
			name: "Empty config uses defaults",
			config: LogConfig{
				Level:  "",
				Format: "",
			},
			wantErr: false,
		},
		{
			name: "Case insensitive",
			config: LogConfig{
				Level:  "INFO",
				Format: "JSON",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitializeWithConfig(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("InitializeWithConfig() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("InitializeWithConfig() unexpected error: %v", err)
				return
			}

			// Verify logger was initialized
			if Logger == nil {
				t.Error("Logger should not be nil after initialization")
			}
			if Sugar == nil {
				t.Error("Sugar should not be nil after initialization")
			}

			// Clean up
			Close()
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	// Set up test logger with observer
	core, recorded := observer.New(zapcore.InfoLevel)
	Logger = zap.New(core)
	Sugar = Logger.Sugar()

	defer func() {
		Close()
		Logger = nil
		Sugar = nil
	}()

	t.Run("LogVoiceEvent", func(t *testing.T) {
		// Test with mock voice event
		mockEvent := &mockVoiceEvent{uuid: "test-uuid-123"}
		LogVoiceEvent(mockEvent, "Test voice event", zap.String("extra", "field"))

		logs := recorded.All()
		if len(logs) == 0 {
			t.Error("Expected log entry but got none")
			return
		}

		log := logs[len(logs)-1]
		if log.Message != "Test voice event" {
			t.Errorf("Expected message 'Test voice event', got %q", log.Message)
		}

		// Check for expected fields
		hasComponent := false
		hasEventUUID := false
		hasExtra := false
		for _, field := range log.Context {
			switch field.Key {
			case "component":
				if field.String != "voice_pipeline" {
					t.Errorf("Expected component 'voice_pipeline', got %q", field.String)
				}
				hasComponent = true
			case "event_uuid":
				if field.String != "test-uuid-123" {
					t.Errorf("Expected event_uuid 'test-uuid-123', got %q", field.String)
				}
				hasEventUUID = true
			case "extra":
				if field.String != "field" {
					t.Errorf("Expected extra 'field', got %q", field.String)
				}
				hasExtra = true
			}
		}

		if !hasComponent {
			t.Error("Missing component field")
		}
		if !hasEventUUID {
			t.Error("Missing event_uuid field")
		}
		if !hasExtra {
			t.Error("Missing extra field")
		}
	})

	t.Run("LogAudioProcessing", func(t *testing.T) {
		LogAudioProcessing("relay-123", "transcription", zap.Int("duration_ms", 500))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Message != "Audio processing" {
			t.Errorf("Expected message 'Audio processing', got %q", log.Message)
		}

		// Verify expected fields
		fields := make(map[string]interface{})
		for _, field := range log.Context {
			switch field.Type {
			case zapcore.StringType:
				fields[field.Key] = field.String
			case zapcore.Int64Type:
				fields[field.Key] = field.Integer
			}
		}

		if fields["component"] != "audio_processing" {
			t.Errorf("Expected component 'audio_processing', got %v", fields["component"])
		}
		if fields["relay_id"] != "relay-123" {
			t.Errorf("Expected relay_id 'relay-123', got %v", fields["relay_id"])
		}
		if fields["stage"] != "transcription" {
			t.Errorf("Expected stage 'transcription', got %v", fields["stage"])
		}
		if fields["duration_ms"] != int64(500) {
			t.Errorf("Expected duration_ms 500, got %v", fields["duration_ms"])
		}
	})

	t.Run("LogNATSEvent", func(t *testing.T) {
		LogNATSEvent("loqa.commands", "publish", zap.String("message_id", "msg-456"))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Message != "NATS event" {
			t.Errorf("Expected message 'NATS event', got %q", log.Message)
		}

		// Check for NATS-specific fields
		hasMessaging := false
		hasSubject := false
		hasAction := false
		for _, field := range log.Context {
			switch field.Key {
			case "component":
				if field.String != "messaging" {
					t.Errorf("Expected component 'messaging', got %q", field.String)
				}
				hasMessaging = true
			case "subject":
				if field.String != "loqa.commands" {
					t.Errorf("Expected subject 'loqa.commands', got %q", field.String)
				}
				hasSubject = true
			case "action":
				if field.String != "publish" {
					t.Errorf("Expected action 'publish', got %q", field.String)
				}
				hasAction = true
			}
		}

		if !hasMessaging || !hasSubject || !hasAction {
			t.Error("Missing required NATS event fields")
		}
	})

	t.Run("LogDatabaseOperation", func(t *testing.T) {
		LogDatabaseOperation("INSERT", "voice_events", zap.Int("affected_rows", 1))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Message != "Database operation" {
			t.Errorf("Expected message 'Database operation', got %q", log.Message)
		}

		// Check database-specific fields
		fields := make(map[string]interface{})
		for _, field := range log.Context {
			switch field.Type {
			case zapcore.StringType:
				fields[field.Key] = field.String
			case zapcore.Int64Type:
				fields[field.Key] = field.Integer
			}
		}

		if fields["component"] != "database" {
			t.Errorf("Expected component 'database', got %v", fields["component"])
		}
		if fields["operation"] != "INSERT" {
			t.Errorf("Expected operation 'INSERT', got %v", fields["operation"])
		}
		if fields["table"] != "voice_events" {
			t.Errorf("Expected table 'voice_events', got %v", fields["table"])
		}
	})

	t.Run("LogError", func(t *testing.T) {
		testErr := errors.New("test error")
		LogError(testErr, "Something went wrong", zap.String("context", "test"))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Level != zapcore.ErrorLevel {
			t.Errorf("Expected error level, got %v", log.Level)
		}
		if log.Message != "Something went wrong" {
			t.Errorf("Expected message 'Something went wrong', got %q", log.Message)
		}

		// Check for error field
		hasError := false
		for _, field := range log.Context {
			if field.Key == "error" {
				hasError = true
				// Error field type depends on zap version, could be string or interface
				if field.Type == zapcore.ErrorType || field.Type == zapcore.StringType {
					if field.String != "test error" && field.Interface != nil {
						// Check if it's the error we expect
						if err, ok := field.Interface.(error); ok && err.Error() != "test error" {
							t.Errorf("Expected error 'test error', got %v", err.Error())
						}
					}
				}
				break
			}
		}
		if !hasError {
			t.Error("Missing error field")
		}
	})

	t.Run("LogWarn", func(t *testing.T) {
		LogWarn("Warning message", zap.String("warning_type", "deprecation"))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Level != zapcore.WarnLevel {
			t.Errorf("Expected warn level, got %v", log.Level)
		}
		if log.Message != "Warning message" {
			t.Errorf("Expected message 'Warning message', got %q", log.Message)
		}
	})

	t.Run("LogTTSOperation", func(t *testing.T) {
		LogTTSOperation("synthesize", zap.String("voice", "af_bella"), zap.Int("text_length", 50))

		logs := recorded.All()
		log := logs[len(logs)-1]
		if log.Message != "TTS operation" {
			t.Errorf("Expected message 'TTS operation', got %q", log.Message)
		}

		// Check TTS-specific fields
		fields := make(map[string]interface{})
		for _, field := range log.Context {
			switch field.Type {
			case zapcore.StringType:
				fields[field.Key] = field.String
			case zapcore.Int64Type:
				fields[field.Key] = field.Integer
			}
		}

		if fields["component"] != "tts" {
			t.Errorf("Expected component 'tts', got %v", fields["component"])
		}
		if fields["operation"] != "synthesize" {
			t.Errorf("Expected operation 'synthesize', got %v", fields["operation"])
		}
	})
}

func TestLoggingFunctions_NilLogger(t *testing.T) {
	// Test that logging functions handle nil logger gracefully
	originalLogger := Logger
	originalSugar := Sugar
	defer func() {
		Logger = originalLogger
		Sugar = originalSugar
	}()

	Logger = nil
	Sugar = nil

	// These should not panic when Logger is nil
	t.Run("Functions with nil logger", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Function panicked with nil logger: %v", r)
			}
		}()

		LogVoiceEvent(nil, "test")
		LogAudioProcessing("relay", "stage")
		LogNATSEvent("subject", "action")
		LogDatabaseOperation("op", "table")
		LogError(errors.New("test"), "message")
		LogWarn("warning")
		LogTTSOperation("operation")
		Sync() // Should not panic
	})
}

func TestSync(t *testing.T) {
	// Initialize a test logger
	config := LogConfig{Level: "info", Format: "console"}
	err := InitializeWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Sync should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Sync() panicked: %v", r)
		}
	}()

	Sync()
	Close()
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "Environment variable set",
			key:          "TEST_ENV_VAR",
			defaultValue: "default",
			envValue:     "env_value",
			expected:     "env_value",
		},
		{
			name:         "Environment variable not set",
			key:          "TEST_ENV_VAR_NOT_SET",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "Empty environment variable",
			key:          "TEST_ENV_VAR_EMPTY",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if provided
			if tt.envValue != "" {
				_ = os.Setenv(tt.key, tt.envValue)
				defer func() { _ = os.Unsetenv(tt.key) }()
			} else {
				_ = os.Unsetenv(tt.key)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefault(%q, %q) = %q, want %q", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestLogConfig(t *testing.T) {
	config := LogConfig{
		Level:  "debug",
		Format: "json",
	}

	if config.Level != "debug" {
		t.Errorf("Expected Level 'debug', got %q", config.Level)
	}
	if config.Format != "json" {
		t.Errorf("Expected Format 'json', got %q", config.Format)
	}
}

// Mock voice event for testing
type mockVoiceEvent struct {
	uuid string
}

func (m *mockVoiceEvent) GetUUID() string {
	return m.uuid
}

// Test with different log levels to ensure they work correctly
func TestLogLevels(t *testing.T) {
	logLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range logLevels {
		t.Run("Level_"+level, func(t *testing.T) {
			config := LogConfig{
				Level:  level,
				Format: "console",
			}

			err := InitializeWithConfig(config)
			if err != nil {
				t.Errorf("Failed to initialize with level %s: %v", level, err)
			}

			if Logger == nil {
				t.Errorf("Logger should not be nil for level %s", level)
			}

			Close()
		})
	}
}

// Test that initialization logs contain expected content
func TestInitializationLogging(t *testing.T) {
	// Capture output during initialization
	config := LogConfig{
		Level:  "info",
		Format: "console",
	}

	err := InitializeWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// The initialization should have logged a message about being initialized
	// We can't easily capture this without more complex setup, but we can verify
	// that the logger is working correctly by ensuring Sugar is not nil
	if Sugar == nil {
		t.Error("Sugar logger should be initialized and functional")
	}

	Close()
}

// Benchmark logging performance
func BenchmarkLogging(b *testing.B) {
	config := LogConfig{Level: "info", Format: "json"}
	_ = InitializeWithConfig(config)
	defer Close()

	b.Run("LogVoiceEvent", func(b *testing.B) {
		event := &mockVoiceEvent{uuid: "benchmark-uuid"}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			LogVoiceEvent(event, "Benchmark voice event")
		}
	})

	b.Run("LogError", func(b *testing.B) {
		err := errors.New("benchmark error")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			LogError(err, "Benchmark error")
		}
	})

	b.Run("Sugar.Infow", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			Sugar.Infow("Benchmark message", "key", "value")
		}
	})
}
