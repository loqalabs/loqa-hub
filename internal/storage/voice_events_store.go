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

package storage

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/loqalabs/loqa-hub/internal/events"
)

// VoiceEventsStore handles database operations for voice events
type VoiceEventsStore struct {
	db *Database
}

// NewVoiceEventsStore creates a new voice events store
func NewVoiceEventsStore(db *Database) *VoiceEventsStore {
	return &VoiceEventsStore{db: db}
}

// Insert stores a new voice event in the database
func (s *VoiceEventsStore) Insert(event *events.VoiceEvent) error {
	if err := event.IsValid(); err != nil {
		return fmt.Errorf("invalid voice event: %w", err)
	}

	entitiesJSON, err := event.EntitiesJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize entities: %w", err)
	}

	query := `
		INSERT INTO voice_events (
			uuid, request_id, puck_id, timestamp,
			audio_hash, audio_duration, sample_rate, wake_word_detected,
			transcription, intent, entities, confidence,
			response_text, processing_time_ms, success, error_message
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?
		)`

	_, err = s.db.DB().Exec(query,
		event.UUID, event.RequestID, event.PuckID, event.Timestamp,
		event.AudioHash, event.AudioDuration, event.SampleRate, event.WakeWordDetected,
		event.Transcription, event.Intent, entitiesJSON, event.Confidence,
		event.ResponseText, event.ProcessingTime, event.Success, event.ErrorMessage,
	)

	if err != nil {
		return fmt.Errorf("failed to insert voice event: %w", err)
	}

	log.Printf("📝 Stored voice event: %s (PuckID: %s, Intent: %s)", 
		event.UUID, event.PuckID, event.Intent)
	return nil
}

// GetByUUID retrieves a voice event by its UUID
func (s *VoiceEventsStore) GetByUUID(uuid string) (*events.VoiceEvent, error) {
	query := `
		SELECT uuid, request_id, puck_id, timestamp,
			   audio_hash, audio_duration, sample_rate, wake_word_detected,
			   transcription, intent, entities, confidence,
			   response_text, processing_time_ms, success, error_message
		FROM voice_events 
		WHERE uuid = ?`

	row := s.db.DB().QueryRow(query, uuid)
	return s.scanVoiceEvent(row)
}

// List retrieves voice events with pagination and filtering
func (s *VoiceEventsStore) List(options ListOptions) ([]*events.VoiceEvent, error) {
	query, args := s.buildListQuery(options)
	
	rows, err := s.db.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query voice events: %w", err)
	}
	defer rows.Close()

	var eventsList []*events.VoiceEvent
	for rows.Next() {
		event, err := s.scanVoiceEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voice event: %w", err)
		}
		eventsList = append(eventsList, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voice events: %w", err)
	}

	return eventsList, nil
}

// Count returns the total number of voice events matching the filter
func (s *VoiceEventsStore) Count(options ListOptions) (int64, error) {
	// Build count query using same filters
	options.Limit = 0
	options.Offset = 0
	query, args := s.buildListQuery(options)
	
	// Replace SELECT fields with COUNT(*)
	countQuery := "SELECT COUNT(*) FROM (" + query + ") as filtered"
	
	var count int64
	err := s.db.DB().QueryRow(countQuery, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count voice events: %w", err)
	}

	return count, nil
}

// GetRecentByPuck retrieves recent events for a specific puck
func (s *VoiceEventsStore) GetRecentByPuck(puckID string, limit int) ([]*events.VoiceEvent, error) {
	options := ListOptions{
		PuckID: puckID,
		Limit:  limit,
	}
	return s.List(options)
}

// GetByAudioHash finds events with the same audio hash (potential duplicates)
func (s *VoiceEventsStore) GetByAudioHash(audioHash string) ([]*events.VoiceEvent, error) {
	query := `
		SELECT uuid, request_id, puck_id, timestamp,
			   audio_hash, audio_duration, sample_rate, wake_word_detected,
			   transcription, intent, entities, confidence,
			   response_text, processing_time_ms, success, error_message
		FROM voice_events 
		WHERE audio_hash = ?
		ORDER BY timestamp DESC`

	rows, err := s.db.DB().Query(query, audioHash)
	if err != nil {
		return nil, fmt.Errorf("failed to query by audio hash: %w", err)
	}
	defer rows.Close()

	var eventsList []*events.VoiceEvent
	for rows.Next() {
		event, err := s.scanVoiceEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voice event: %w", err)
		}
		eventsList = append(eventsList, event)
	}

	return eventsList, nil
}

// Delete removes a voice event by UUID
func (s *VoiceEventsStore) Delete(uuid string) error {
	result, err := s.db.DB().Exec("DELETE FROM voice_events WHERE uuid = ?", uuid)
	if err != nil {
		return fmt.Errorf("failed to delete voice event: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("voice event not found: %s", uuid)
	}

	log.Printf("🗑️  Deleted voice event: %s", uuid)
	return nil
}

// ListOptions defines filtering and pagination options
type ListOptions struct {
	// Filtering
	PuckID    string
	Intent    string
	Success   *bool // nil = all, true = success only, false = errors only
	StartTime *time.Time
	EndTime   *time.Time
	
	// Pagination  
	Limit  int
	Offset int
	
	// Sorting
	SortBy    string // "timestamp", "confidence", "processing_time"
	SortOrder string // "ASC", "DESC"
}

// buildListQuery constructs the SQL query based on ListOptions
func (s *VoiceEventsStore) buildListQuery(options ListOptions) (string, []interface{}) {
	query := `
		SELECT uuid, request_id, puck_id, timestamp,
			   audio_hash, audio_duration, sample_rate, wake_word_detected,
			   transcription, intent, entities, confidence,
			   response_text, processing_time_ms, success, error_message
		FROM voice_events WHERE 1=1`
	
	var args []interface{}

	// Apply filters
	if options.PuckID != "" {
		query += " AND puck_id = ?"
		args = append(args, options.PuckID)
	}

	if options.Intent != "" {
		query += " AND intent = ?"
		args = append(args, options.Intent)
	}

	if options.Success != nil {
		query += " AND success = ?"
		args = append(args, *options.Success)
	}

	if options.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, options.StartTime)
	}

	if options.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, options.EndTime)
	}

	// Apply sorting
	sortBy := options.SortBy
	if sortBy == "" {
		sortBy = "timestamp"
	}
	
	sortOrder := options.SortOrder
	if sortOrder == "" {
		sortOrder = "DESC"
	}
	
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Apply pagination
	if options.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, options.Limit)
		
		if options.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, options.Offset)
		}
	}

	return query, args
}

// scanVoiceEvent scans a database row into a VoiceEvent struct
func (s *VoiceEventsStore) scanVoiceEvent(scanner interface{}) (*events.VoiceEvent, error) {
	var event events.VoiceEvent
	var entitiesJSON string

	var row interface {
		Scan(dest ...interface{}) error
	}

	switch v := scanner.(type) {
	case *sql.Row:
		row = v
	case *sql.Rows:
		row = v
	default:
		return nil, fmt.Errorf("unsupported scanner type")
	}

	err := row.Scan(
		&event.UUID, &event.RequestID, &event.PuckID, &event.Timestamp,
		&event.AudioHash, &event.AudioDuration, &event.SampleRate, &event.WakeWordDetected,
		&event.Transcription, &event.Intent, &entitiesJSON, &event.Confidence,
		&event.ResponseText, &event.ProcessingTime, &event.Success, &event.ErrorMessage,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("voice event not found")
		}
		return nil, err
	}

	// Parse entities JSON
	if err := event.SetEntitiesFromJSON(entitiesJSON); err != nil {
		return nil, fmt.Errorf("failed to parse entities JSON: %w", err)
	}

	return &event, nil
}