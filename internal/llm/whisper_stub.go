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

//go:build !whisper

package llm

import "fmt"

// WhisperTranscriber stub implementation when whisper is disabled
type WhisperTranscriber struct {
	modelPath string
}

// NewWhisperTranscriber creates a stub transcriber when whisper is disabled
func NewWhisperTranscriber(modelPath string) (*WhisperTranscriber, error) {
	return &WhisperTranscriber{
		modelPath: modelPath,
	}, nil
}

// Transcribe stub implementation returns empty transcription
func (wt *WhisperTranscriber) Transcribe(audioData []float32, sampleRate int) (string, error) {
	return "", fmt.Errorf("whisper transcription disabled (build with -tags whisper to enable)")
}

// Close stub implementation
func (wt *WhisperTranscriber) Close() {
	// Nothing to clean up in stub
}