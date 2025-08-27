package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Loqa hub
type Config struct {
	Server   ServerConfig
	Whisper  WhisperConfig
	Logging  LoggingConfig
	NATS     NATSConfig
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host         string
	Port         int
	GRPCPort     int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// WhisperConfig holds Whisper-related configuration
type WhisperConfig struct {
	ModelPath    string
	Language     string
	Temperature  float32
	MaxTokens    int
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
}

// NATSConfig holds NATS messaging configuration
type NATSConfig struct {
	URL             string
	Subject         string
	MaxReconnect    int
	ReconnectWait   time.Duration
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
		Whisper: WhisperConfig{
			ModelPath:   getEnvString("WHISPER_MODEL_PATH", "/models/ggml-base.en.bin"),
			Language:    getEnvString("WHISPER_LANGUAGE", "en"),
			Temperature: getEnvFloat32("WHISPER_TEMPERATURE", 0.0),
			MaxTokens:   getEnvInt("WHISPER_MAX_TOKENS", 224),
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

	if c.Whisper.ModelPath == "" {
		return fmt.Errorf("whisper model path cannot be empty")
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