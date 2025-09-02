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

//go:build whisper

package llm

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// WhisperTranscriber handles speech-to-text using Whisper
type WhisperTranscriber struct {
	model     whisper.Model
	modelPath string
}

// NewWhisperTranscriber creates a new Whisper transcriber
func NewWhisperTranscriber(modelPath string) (*WhisperTranscriber, error) {
	// Check if model file exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("whisper model not found at %s", modelPath)
	}

	// Load the model
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load whisper model: %w", err)
	}

	log.Printf("✅ Whisper model loaded: %s", modelPath)
	return &WhisperTranscriber{
		model:     model,
		modelPath: modelPath,
	}, nil
}

// Transcribe converts audio samples to text
func (wt *WhisperTranscriber) Transcribe(audioData []float32, sampleRate int) (string, error) {
	if wt.model == nil {
		return "", fmt.Errorf("whisper model not initialized")
	}

	// Create a new context for this transcription
	ctx, err := wt.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("failed to create whisper context: %w", err)
	}

	// Process the audio data
	if err := ctx.Process(audioData, nil, nil, nil); err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	// Extract the transcription
	var transcript strings.Builder
	for {
		segment, err := ctx.NextSegment()
		if err != nil {
			break
		}
		transcript.WriteString(segment.Text)
	}

	result := strings.TrimSpace(transcript.String())
	log.Printf("🧠 Whisper transcription: \"%s\"", result)
	return result, nil
}

// Close cleans up the Whisper model
func (wt *WhisperTranscriber) Close() error {
	if wt.model != nil {
		wt.model.Close()
		log.Println("🧠 Whisper model closed")
	}
	return nil
}