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

package security

import (
	"strings"
	"testing"
)

func TestSanitizeLogInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Clean input",
			input:    "normal log message",
			expected: "normal log message",
		},
		{
			name:     "Single newline",
			input:    "line1\nline2",
			expected: "line1line2",
		},
		{
			name:     "Single carriage return",
			input:    "line1\rline2",
			expected: "line1line2",
		},
		{
			name:     "CRLF sequence",
			input:    "line1\r\nline2",
			expected: "line1line2",
		},
		{
			name:     "Multiple newlines",
			input:    "line1\n\nline2\nline3",
			expected: "line1line2line3",
		},
		{
			name:     "Mixed line endings",
			input:    "line1\nline2\rline3\r\nline4",
			expected: "line1line2line3line4",
		},
		{
			name:     "Log injection attempt",
			input:    "user_input\nERROR: fake error message",
			expected: "user_inputERROR: fake error message",
		},
		{
			name:     "Complex log injection with ANSI codes",
			input:    "normal\n\x1b[31mFAKE ERROR\x1b[0m\nmore text",
			expected: "normal\x1b[31mFAKE ERROR\x1b[0mmore text",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only newlines",
			input:    "\n\r\n\r",
			expected: "",
		},
		{
			name:     "Unicode characters preserved",
			input:    "Hello 世界\nSecond line",
			expected: "Hello 世界Second line",
		},
		{
			name:     "Special characters preserved",
			input:    "user@domain.com\npassword=secret!@#$%",
			expected: "user@domain.compassword=secret!@#$%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLogInput(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeLogInput(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Verify no newlines remain
			if strings.Contains(result, "\n") || strings.Contains(result, "\r") {
				t.Errorf("SanitizeLogInput(%q) still contains line breaks: %q", tt.input, result)
			}
		})
	}
}

func TestSanitizeLogInput_InjectionPrevention(t *testing.T) {
	// Test specific log injection attack patterns
	injectionTests := []struct {
		name           string
		maliciousInput string
		description    string
	}{
		{
			name:           "Fake error injection",
			maliciousInput: "normal input\nERROR: System compromised!",
			description:    "Attacker tries to inject fake error messages",
		},
		{
			name:           "Log level spoofing",
			maliciousInput: "user data\nFATAL: Authentication bypassed",
			description:    "Attacker tries to spoof high-severity log levels",
		},
		{
			name:           "Multi-line injection",
			maliciousInput: "legitimate\nINFO: Legitimate message\nERROR: Fake error\nWARN: Another fake",
			description:    "Multiple fake log entries in one injection",
		},
		{
			name:           "ANSI escape sequence injection",
			maliciousInput: "normal\n\x1b[31mERROR: Red fake error\x1b[0m",
			description:    "Using ANSI codes to make fake logs look real",
		},
		{
			name:           "Timestamp spoofing attempt",
			maliciousInput: "data\n2025-01-01T00:00:00Z ERROR: Fake timestamped error",
			description:    "Attacker tries to inject fake timestamps",
		},
	}

	for _, tt := range injectionTests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := SanitizeLogInput(tt.maliciousInput)

			// Verify no line breaks remain (primary defense)
			if strings.Contains(sanitized, "\n") || strings.Contains(sanitized, "\r") {
				t.Errorf("SanitizeLogInput failed to remove line breaks from: %q", tt.maliciousInput)
			}

			// Verify the injection is neutralized by concatenation
			if strings.Count(sanitized, "ERROR:") > strings.Count(tt.maliciousInput, "ERROR:") {
				t.Errorf("SanitizeLogInput appears to have duplicated content: %q -> %q", tt.maliciousInput, sanitized)
			}

			t.Logf("Test case: %s", tt.description)
			t.Logf("Input:     %q", tt.maliciousInput)
			t.Logf("Sanitized: %q", sanitized)
		})
	}
}

// Benchmark tests to ensure security functions don't impact performance
func BenchmarkSanitizeLogInput(b *testing.B) {
	testInput := "Normal log message with some\nmalicious\r\ncontent that needs sanitization"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizeLogInput(testInput)
	}
}
