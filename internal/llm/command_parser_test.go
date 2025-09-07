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
	"testing"
)

func TestDetectCompoundUtterance(t *testing.T) {
	cp := NewCommandParser("http://localhost:11434", "test-model")

	testCases := []struct {
		name        string
		utterance   string
		expected    bool
		description string
	}{
		{
			name:        "simple_and",
			utterance:   "turn on the lights and play music",
			expected:    true,
			description: "should detect 'and' conjunction",
		},
		{
			name:        "then_conjunction",
			utterance:   "turn off the tv then dim the bedroom lights",
			expected:    true,
			description: "should detect 'then' conjunction",
		},
		{
			name:        "after_that_conjunction",
			utterance:   "turn on the lights, after that play some music",
			expected:    true,
			description: "should detect 'after that' conjunction",
		},
		{
			name:        "single_command",
			utterance:   "turn on the lights",
			expected:    false,
			description: "should not detect compound in single command",
		},
		{
			name:        "false_positive_and",
			utterance:   "play rock and roll music",
			expected:    false,
			description: "should not detect compound when 'and' is part of entity",
		},
		{
			name:        "multiple_conjunctions",
			utterance:   "turn on the lights and play music and set the temperature",
			expected:    true,
			description: "should detect multiple conjunctions",
		},
		{
			name:        "comma_and",
			utterance:   "turn on the lights, and play music",
			expected:    true,
			description: "should detect comma-separated conjunctions",
		},
		{
			name:        "next_conjunction",
			utterance:   "turn off the lights next turn on the fan",
			expected:    true,
			description: "should detect 'next' conjunction",
		},
		{
			name:        "also_conjunction",
			utterance:   "turn on the lights also play music",
			expected:    true,
			description: "should detect 'also' conjunction",
		},
		{
			name:        "empty_string",
			utterance:   "",
			expected:    false,
			description: "should handle empty string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := cp.detectCompoundUtterance(tc.utterance)
			if result != tc.expected {
				t.Errorf("detectCompoundUtterance(%q) = %v, expected %v (%s)",
					tc.utterance, result, tc.expected, tc.description)
			}
		})
	}
}

func TestBuildMultiCommandPrompt(t *testing.T) {
	cp := NewCommandParser("http://localhost:11434", "test-model")

	utterance := "turn on the lights and play music"
	prompt := cp.buildMultiCommandPrompt(utterance)

	// Check that the prompt contains the utterance
	if !containsString(prompt, utterance) {
		t.Errorf("buildMultiCommandPrompt should contain the original utterance")
	}

	// Check that the prompt contains expected structure keywords
	expectedKeywords := []string{
		"is_multi",
		"commands",
		"intent",
		"entities",
		"confidence",
		"response",
		"combined_response",
	}

	for _, keyword := range expectedKeywords {
		if !containsString(prompt, keyword) {
			t.Errorf("buildMultiCommandPrompt should contain keyword '%s'", keyword)
		}
	}
}

func TestParseMultiCommandResponse(t *testing.T) {
	cp := NewCommandParser("http://localhost:11434", "test-model")

	testCases := []struct {
		name          string
		response      string
		originalText  string
		expectError   bool
		expectedMulti bool
		expectedCount int
		description   string
	}{
		{
			name: "valid_multi_command",
			response: `{
				"is_multi": true,
				"commands": [
					{
						"intent": "turn_on",
						"entities": {"device": "lights"},
						"confidence": 0.9,
						"response": "Turning on the lights"
					},
					{
						"intent": "turn_on", 
						"entities": {"device": "music"},
						"confidence": 0.8,
						"response": "Playing music"
					}
				],
				"combined_response": "I'll turn on the lights and play music for you."
			}`,
			originalText:  "turn on the lights and play music",
			expectError:   false,
			expectedMulti: true,
			expectedCount: 2,
			description:   "should parse valid multi-command response",
		},
		{
			name: "valid_single_command",
			response: `{
				"is_multi": false,
				"commands": [
					{
						"intent": "turn_on",
						"entities": {"device": "lights"},
						"confidence": 0.9,
						"response": "Turning on the lights"
					}
				],
				"combined_response": "Turning on the lights"
			}`,
			originalText:  "turn on the lights",
			expectError:   false,
			expectedMulti: false,
			expectedCount: 1,
			description:   "should parse valid single command response",
		},
		{
			name:          "invalid_json",
			response:      "not a valid json response",
			originalText:  "turn on lights",
			expectError:   true,
			expectedMulti: false,
			expectedCount: 0,
			description:   "should return error for invalid JSON",
		},
		{
			name: "missing_fields",
			response: `{
				"commands": [
					{
						"intent": "turn_on"
					}
				]
			}`,
			originalText:  "turn on lights",
			expectError:   false,
			expectedMulti: false,
			expectedCount: 1,
			description:   "should handle missing fields with defaults",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := cp.parseMultiCommandResponse(tc.response, tc.originalText)

			if tc.expectError {
				if err == nil {
					t.Errorf("parseMultiCommandResponse should return error for case: %s", tc.description)
				}
				return
			}

			if err != nil {
				t.Errorf("parseMultiCommandResponse returned unexpected error: %v (%s)", err, tc.description)
				return
			}

			if result.IsMulti != tc.expectedMulti {
				t.Errorf("IsMulti = %v, expected %v (%s)", result.IsMulti, tc.expectedMulti, tc.description)
			}

			if len(result.Commands) != tc.expectedCount {
				t.Errorf("Command count = %d, expected %d (%s)", len(result.Commands), tc.expectedCount, tc.description)
			}

			if result.OriginalText != tc.originalText {
				t.Errorf("OriginalText = %q, expected %q (%s)", result.OriginalText, tc.originalText, tc.description)
			}

			// Validate command defaults
			for i, cmd := range result.Commands {
				if cmd.Intent == "" {
					t.Errorf("Command %d should have default intent", i)
				}
				if cmd.Entities == nil {
					t.Errorf("Command %d should have initialized entities map", i)
				}
				if cmd.Confidence < 0 || cmd.Confidence > 1 {
					t.Errorf("Command %d confidence should be in range 0-1, got %f", i, cmd.Confidence)
				}
				if cmd.Response == "" {
					t.Errorf("Command %d should have default response", i)
				}
			}
		})
	}
}

func TestCreateCombinedCommand(t *testing.T) {
	cp := NewCommandParser("http://localhost:11434", "test-model")

	testCases := []struct {
		name           string
		multiCmd       *MultiCommand
		expectedIntent string
		description    string
	}{
		{
			name: "empty_commands",
			multiCmd: &MultiCommand{
				Commands: []Command{},
				IsMulti:  false,
			},
			expectedIntent: "unknown",
			description:    "should handle empty commands list",
		},
		{
			name: "single_command",
			multiCmd: &MultiCommand{
				Commands: []Command{
					{
						Intent:     "turn_on",
						Entities:   map[string]string{"device": "lights"},
						Confidence: 0.9,
						Response:   "Turning on the lights",
					},
				},
				IsMulti: false,
			},
			expectedIntent: "turn_on",
			description:    "should return single command unchanged",
		},
		{
			name: "multiple_commands",
			multiCmd: &MultiCommand{
				Commands: []Command{
					{
						Intent:     "turn_on",
						Entities:   map[string]string{"device": "lights"},
						Confidence: 0.9,
						Response:   "Turning on the lights",
					},
					{
						Intent:     "turn_on",
						Entities:   map[string]string{"device": "music"},
						Confidence: 0.8,
						Response:   "Playing music",
					},
				},
				IsMulti:          true,
				CombinedResponse: "I'll turn on the lights and play music.",
			},
			expectedIntent: "multi_turn_on",
			description:    "should combine multiple commands",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := cp.createCombinedCommand(tc.multiCmd)

			if result.Intent != tc.expectedIntent {
				t.Errorf("Intent = %q, expected %q (%s)", result.Intent, tc.expectedIntent, tc.description)
			}

			if result.Entities == nil {
				t.Errorf("Entities should be initialized")
			}

			if result.Confidence < 0 || result.Confidence > 1 {
				t.Errorf("Confidence should be in range 0-1, got %f", result.Confidence)
			}

			if result.Response == "" {
				t.Errorf("Response should not be empty")
			}
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func containsString(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) == 0 {
		return false
	}

	// Simple case-insensitive contains check
	for i := 0; i <= len(haystack)-len(needle); i++ {
		found := true
		for j := 0; j < len(needle); j++ {
			h := haystack[i+j]
			n := needle[j]
			// Simple case conversion
			if h >= 'A' && h <= 'Z' {
				h = h + 32
			}
			if n >= 'A' && n <= 'Z' {
				n = n + 32
			}
			if h != n {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}
