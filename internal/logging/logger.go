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
	"os"
	"strings"

	"go.uber.org/zap"
)

var (
	// Global logger instance
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "json", "console"
}

// Initialize sets up the global logger based on environment variables
func Initialize() error {
	config := LogConfig{
		Level:  getEnvOrDefault("LOG_LEVEL", "info"),
		Format: getEnvOrDefault("LOG_FORMAT", "console"),
	}

	return InitializeWithConfig(config)
}

// InitializeWithConfig sets up the global logger with provided configuration
func InitializeWithConfig(config LogConfig) error {
	var zapConfig zap.Config

	// Configure base settings based on format
	switch strings.ToLower(config.Format) {
	case "json":
		zapConfig = zap.NewProductionConfig()
	case "console":
		zapConfig = zap.NewDevelopmentConfig()
	default:
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	level, err := zap.ParseAtomicLevel(strings.ToLower(config.Level))
	if err != nil {
		// Default to info level if parsing fails
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	zapConfig.Level = level

	// Build logger
	logger, err := zapConfig.Build(
		zap.AddCallerSkip(1), // Skip the wrapper functions
		zap.AddStacktrace(zap.ErrorLevel),
	)
	if err != nil {
		return err
	}

	// Set global instances
	Logger = logger
	Sugar = logger.Sugar()

	Sugar.Infof("🚀 Structured logging initialized (level: %s, format: %s)",
		config.Level, config.Format)

	return nil
}

// Sync flushes any buffered log entries
func Sync() {
	if Logger != nil {
		if err := Logger.Sync(); err != nil {
			// Logger.Sync() can fail on some systems, especially in tests
			// This is usually not critical, so we just ignore the error
			_ = err
		}
	}
}

// Close cleans up the logger
func Close() {
	Sync()
}

// Helper functions for common logging patterns

// LogVoiceEvent logs a voice event with structured fields
func LogVoiceEvent(event interface{}, message string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.String("component", "voice_pipeline"),
	}

	// Add event-specific fields if it's a voice event
	switch v := event.(type) {
	case interface{ GetUUID() string }:
		if uuid := v.GetUUID(); uuid != "" {
			baseFields = append(baseFields, zap.String("event_uuid", uuid))
		}
	}

	allFields := append(baseFields, fields...)
	Logger.Info(message, allFields...)
}

// LogAudioProcessing logs audio processing events
func LogAudioProcessing(relayID, stage string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.String("component", "audio_processing"),
		zap.String("relay_id", relayID),
		zap.String("stage", stage),
	}

	allFields := append(baseFields, fields...)
	Logger.Info("Audio processing", allFields...)
}

// LogNATSEvent logs NATS messaging events
func LogNATSEvent(subject, action string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.String("component", "messaging"),
		zap.String("subject", subject),
		zap.String("action", action),
	}

	allFields := append(baseFields, fields...)
	Logger.Info("NATS event", allFields...)
}

// LogDatabaseOperation logs database operations
func LogDatabaseOperation(operation, table string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.String("component", "database"),
		zap.String("operation", operation),
		zap.String("table", table),
	}

	allFields := append(baseFields, fields...)
	Logger.Info("Database operation", allFields...)
}

// LogError logs errors with context
func LogError(err error, message string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.Error(err),
	}

	allFields := append(baseFields, fields...)
	Logger.Error(message, allFields...)
}

// LogWarn logs warnings with context
func LogWarn(message string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	Logger.Warn(message, fields...)
}

// LogTTSOperation logs text-to-speech operations
func LogTTSOperation(operation string, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	baseFields := []zap.Field{
		zap.String("component", "tts"),
		zap.String("operation", operation),
	}

	allFields := append(baseFields, fields...)
	Logger.Info("TTS operation", allFields...)
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
