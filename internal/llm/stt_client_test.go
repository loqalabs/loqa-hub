package llm

import (
	"testing"
)

func TestPostProcessTranscription(t *testing.T) {
	client := &STTClient{}

	tests := []struct {
		name                      string
		input                     string
		expectedCleanedText       string
		expectedWakeWord          bool
		expectedWakeWordVariant   string
		expectedNeedsConfirmation bool
	}{
		{
			name:                      "Standard wake word with command",
			input:                     "Hey Loqa turn on the lights",
			expectedCleanedText:       "turn on the lights",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey loqa",
			expectedNeedsConfirmation: false,
		},
		{
			name:                      "Wake word variant - Hey Luca",
			input:                     "Hey Luca turn off the music",
			expectedCleanedText:       "turn off the music",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey luca",
			expectedNeedsConfirmation: false,
		},
		{
			name:                      "Wake word variant - Hey Luka",
			input:                     "Hey Luka what time is it",
			expectedCleanedText:       "what time is it",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey luka",
			expectedNeedsConfirmation: false,
		},
		{
			name:                      "Just wake word, no command",
			input:                     "Hey Loqa",
			expectedCleanedText:       "",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey loqa",
			expectedNeedsConfirmation: true,
		},
		{
			name:                      "No wake word",
			input:                     "turn on the lights",
			expectedCleanedText:       "turn on the lights",
			expectedWakeWord:          false,
			expectedWakeWordVariant:   "",
			expectedNeedsConfirmation: false,
		},
		{
			name:                      "Very short command (low confidence)",
			input:                     "on",
			expectedCleanedText:       "on",
			expectedWakeWord:          false,
			expectedWakeWordVariant:   "",
			expectedNeedsConfirmation: true,
		},
		{
			name:                      "Nonsensical patterns",
			input:                     "???",
			expectedCleanedText:       "???",
			expectedWakeWord:          false,
			expectedWakeWordVariant:   "",
			expectedNeedsConfirmation: true,
		},
		{
			name:                      "Repeated characters (stammering)",
			input:                     "Hey Loqa aaaaaah",
			expectedCleanedText:       "aaaaaah",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey loqa",
			expectedNeedsConfirmation: false, // Still above confidence threshold due to wake word boost
		},
		{
			name:                      "Case insensitive wake word",
			input:                     "HEY LOQA TURN ON LIGHTS",
			expectedCleanedText:       "TURN ON LIGHTS",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey loqa",
			expectedNeedsConfirmation: false,
		},
		{
			name:                      "Wake word with punctuation",
			input:                     "Hey Loqa, turn on the lights",
			expectedCleanedText:       "turn on the lights",
			expectedWakeWord:          true,
			expectedWakeWordVariant:   "hey loqa",
			expectedNeedsConfirmation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.postProcessTranscription(tt.input)

			if result.CleanedText != tt.expectedCleanedText {
				t.Errorf("CleanedText = %q, want %q", result.CleanedText, tt.expectedCleanedText)
			}

			if result.WakeWordDetected != tt.expectedWakeWord {
				t.Errorf("WakeWordDetected = %v, want %v", result.WakeWordDetected, tt.expectedWakeWord)
			}

			if result.WakeWordVariant != tt.expectedWakeWordVariant {
				t.Errorf("WakeWordVariant = %q, want %q", result.WakeWordVariant, tt.expectedWakeWordVariant)
			}

			if result.NeedsConfirmation != tt.expectedNeedsConfirmation {
				t.Errorf("NeedsConfirmation = %v, want %v", result.NeedsConfirmation, tt.expectedNeedsConfirmation)
			}

			// Confidence should be between 0 and 1
			if result.ConfidenceEstimate < 0.0 || result.ConfidenceEstimate > 1.0 {
				t.Errorf("ConfidenceEstimate = %f, should be between 0.0 and 1.0", result.ConfidenceEstimate)
			}

			// Original text should always be preserved
			if result.OriginalText != tt.input {
				t.Errorf("OriginalText = %q, want %q", result.OriginalText, tt.input)
			}
		})
	}
}

func TestEstimateConfidence(t *testing.T) {
	client := &STTClient{}

	tests := []struct {
		name    string
		input   string
		minConf float64
		maxConf float64
	}{
		{
			name:    "Empty string",
			input:   "",
			minConf: 0.0,
			maxConf: 0.0,
		},
		{
			name:    "Normal command",
			input:   "turn on the lights",
			minConf: 0.7,
			maxConf: 1.0,
		},
		{
			name:    "Very short",
			input:   "on",
			minConf: 0.0,
			maxConf: 0.6,
		},
		{
			name:    "With wake word",
			input:   "Hey Loqa turn on lights",
			minConf: 0.8,
			maxConf: 1.0,
		},
		{
			name:    "Nonsensical",
			input:   "...",
			minConf: 0.0,
			maxConf: 0.7,
		},
		{
			name:    "Repeated characters",
			input:   "aaaaaah",
			minConf: 0.0,
			maxConf: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := client.estimateConfidence(tt.input)

			if confidence < tt.minConf || confidence > tt.maxConf {
				t.Errorf("estimateConfidence(%q) = %f, want between %f and %f", tt.input, confidence, tt.minConf, tt.maxConf)
			}
		})
	}
}
