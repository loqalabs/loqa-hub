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
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrInvalidSkillID is returned when a skill ID format is invalid
	ErrInvalidSkillID = errors.New("invalid skill ID")
	
	// skillIDPattern validates skill IDs to only allow safe characters
	skillIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// SanitizeLogInput removes newline characters to prevent log injection attacks
// This function should be used for all user-controlled data before logging
func SanitizeLogInput(input string) string {
	sanitized := strings.ReplaceAll(input, "\n", "")
	sanitized = strings.ReplaceAll(sanitized, "\r", "")
	return sanitized
}

// ValidateSkillID ensures that a skill ID contains only safe characters
// and prevents path traversal attacks. Only allows alphanumeric ASCII 
// characters, dashes, and underscores.
func ValidateSkillID(skillID string) error {
	// Check for empty skill ID
	if skillID == "" {
		return ErrInvalidSkillID
	}
	
	// Check for path separators or parent directory references (CodeQL recommendation)
	if strings.Contains(skillID, "/") || strings.Contains(skillID, "\\") || strings.Contains(skillID, "..") {
		return ErrInvalidSkillID
	}
	
	// Validate against allowed character pattern
	if !skillIDPattern.MatchString(skillID) {
		return ErrInvalidSkillID
	}
	
	return nil
}