-- Voice Events Database Schema
-- This schema supports full traceability of voice interactions

CREATE TABLE IF NOT EXISTS voice_events (
    -- Core identification
    uuid TEXT PRIMARY KEY NOT NULL,
    request_id TEXT NOT NULL,
    relay_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Audio metadata
    audio_hash TEXT NOT NULL,
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

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_voice_events_timestamp ON voice_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_voice_events_relay_id ON voice_events(relay_id);
CREATE INDEX IF NOT EXISTS idx_voice_events_intent ON voice_events(intent);
CREATE INDEX IF NOT EXISTS idx_voice_events_success ON voice_events(success);
CREATE INDEX IF NOT EXISTS idx_voice_events_audio_hash ON voice_events(audio_hash);
CREATE INDEX IF NOT EXISTS idx_voice_events_created_at ON voice_events(created_at DESC);

-- Composite indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_voice_events_relay_timestamp ON voice_events(relay_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_voice_events_intent_confidence ON voice_events(intent, confidence DESC);