-- Migration: Remove audio_hash column for privacy compliance
-- This migration removes the audio_hash field to ensure zero persistence of audio fingerprints
-- Issue: https://github.com/loqalabs/loqa/issues/36

-- Drop the audio_hash index first
DROP INDEX IF EXISTS idx_voice_events_audio_hash;

-- Remove the audio_hash column
-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
BEGIN TRANSACTION;

-- Create new table without audio_hash
CREATE TABLE voice_events_new (
    -- Core identification
    uuid TEXT PRIMARY KEY NOT NULL,
    request_id TEXT NOT NULL,
    relay_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Audio metadata (without audio_hash)
    audio_duration REAL NOT NULL DEFAULT 0.0,
    sample_rate INTEGER NOT NULL DEFAULT 16000,
    wake_word_detected BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- Processing results
    transcription TEXT NOT NULL DEFAULT '',
    intent TEXT NOT NULL DEFAULT 'unknown',
    entities TEXT NOT NULL DEFAULT '{}', -- JSON string
    confidence REAL NOT NULL DEFAULT 0.0,
    
    -- Response data
    response_text TEXT NOT NULL DEFAULT '',
    processing_time_ms INTEGER NOT NULL DEFAULT 0,
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT DEFAULT NULL,
    
    -- Metadata
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints
    CONSTRAINT chk_confidence CHECK (confidence >= 0.0 AND confidence <= 1.0),
    CONSTRAINT chk_processing_time CHECK (processing_time_ms >= 0),
    CONSTRAINT chk_audio_duration CHECK (audio_duration >= 0.0)
);

-- Copy data from old table (excluding audio_hash)
INSERT INTO voice_events_new (
    uuid, request_id, relay_id, timestamp,
    audio_duration, sample_rate, wake_word_detected,
    transcription, intent, entities, confidence,
    response_text, processing_time_ms, success, error_message,
    created_at
)
SELECT 
    uuid, request_id, relay_id, timestamp,
    audio_duration, sample_rate, wake_word_detected,
    transcription, intent, entities, confidence,
    response_text, processing_time_ms, success, error_message,
    created_at
FROM voice_events;

-- Drop the old table
DROP TABLE voice_events;

-- Rename new table to original name
ALTER TABLE voice_events_new RENAME TO voice_events;

-- Recreate indexes (excluding audio_hash index)
CREATE INDEX IF NOT EXISTS idx_voice_events_timestamp ON voice_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_voice_events_relay_id ON voice_events(relay_id);
CREATE INDEX IF NOT EXISTS idx_voice_events_intent ON voice_events(intent);
CREATE INDEX IF NOT EXISTS idx_voice_events_success ON voice_events(success);
CREATE INDEX IF NOT EXISTS idx_voice_events_created_at ON voice_events(created_at DESC);

-- Composite indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_voice_events_relay_timestamp ON voice_events(relay_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_voice_events_intent_confidence ON voice_events(intent, confidence DESC);

COMMIT;