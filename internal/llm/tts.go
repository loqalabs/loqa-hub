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

package llm

import (
	"io"
)

// TTSOptions holds options for text-to-speech synthesis
type TTSOptions struct {
	Voice          string  // Voice to use (e.g., "af_bella")
	Speed          float32 // Speech speed (1.0 = normal)
	ResponseFormat string  // Audio format (mp3, wav, opus, flac)
	Normalize      bool    // Enable text normalization
}

// TTSResult holds the result of text-to-speech synthesis
type TTSResult struct {
	Audio       io.Reader // Audio stream
	ContentType string    // MIME type of the audio
	Length      int64     // Audio length in bytes (-1 if unknown)
	Cleanup     func()    // Optional cleanup function for resources
}

// TextToSpeech defines the interface for text-to-speech synthesis services
type TextToSpeech interface {
	// Synthesize converts text to speech audio
	Synthesize(text string, options *TTSOptions) (*TTSResult, error)

	// GetAvailableVoices returns the list of available voices
	GetAvailableVoices() ([]string, error)

	// Close cleans up resources
	Close() error
}
