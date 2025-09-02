/*
Copyright (c) 2024 Loqa Labs

Licensed under the AGPLv3 License.
This file is part of the loqa-hub.
*/

package llm

// Transcriber defines the interface for speech-to-text transcription services
type Transcriber interface {
	// Transcribe converts audio samples to text
	Transcribe(audioData []float32, sampleRate int) (string, error)
	
	// Close cleans up resources
	Close() error
}