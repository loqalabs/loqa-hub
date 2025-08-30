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