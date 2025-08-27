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
func (wt *WhisperTranscriber) Close() {
	if wt.model != nil {
		wt.model.Close()
		log.Println("🧠 Whisper model closed")
	}
}