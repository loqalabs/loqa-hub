/*
Copyright (c) 2024 Loqa Labs

Licensed under the AGPLv3 License.
This file is part of the loqa-hub.
*/

package llm

// TranscriptionResult contains the result of speech-to-text processing
type TranscriptionResult struct {
	Text               string
	ConfidenceEstimate float64
	WakeWordDetected   bool
	WakeWordVariant    string
	NeedsConfirmation  bool
}

// Transcriber defines the interface for speech-to-text transcription services
type Transcriber interface {
	// Transcribe converts audio samples to text
	Transcribe(audioData []float32, sampleRate int) (string, error)

	// TranscribeWithConfidence converts audio samples to text with confidence information
	TranscribeWithConfidence(audioData []float32, sampleRate int) (*TranscriptionResult, error)

	// Close cleans up resources
	Close() error
}
