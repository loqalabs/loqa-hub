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

package skills

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// BuiltinExecutor handles loading and execution of built-in skills
type BuiltinExecutor struct {
	logger        *zap.Logger
	skillRegistry map[string]SkillFactory
	loadedSkills  map[string]SkillPlugin
	mu            sync.RWMutex
}

// SkillFactory is a function that creates a new skill instance
type SkillFactory func(logger *zap.Logger) SkillPlugin

// NewBuiltinExecutor creates a new built-in skill executor
func NewBuiltinExecutor(logger *zap.Logger) *BuiltinExecutor {
	return &BuiltinExecutor{
		logger:        logger,
		skillRegistry: make(map[string]SkillFactory),
		loadedSkills:  make(map[string]SkillPlugin),
	}
}

// RegisterSkill registers a built-in skill factory
func (e *BuiltinExecutor) RegisterSkill(skillID string, factory SkillFactory) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.skillRegistry[skillID] = factory
	e.logger.Info("Registered built-in skill", zap.String("skill", skillID))
}

// LoadSkill loads a built-in skill
func (e *BuiltinExecutor) LoadSkill(manifest *SkillManifest, config *SkillConfig) (SkillPlugin, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	skillID := manifest.ID

	// Check if skill is already loaded
	if skill, exists := e.loadedSkills[skillID]; exists {
		return skill, nil
	}

	// Find skill factory
	factory, exists := e.skillRegistry[skillID]
	if !exists {
		return nil, fmt.Errorf("built-in skill %s not registered", skillID)
	}

	// Create skill instance
	skill := factory(e.logger)

	// Store loaded skill
	e.loadedSkills[skillID] = skill

	return skill, nil
}

// UnloadSkill unloads a built-in skill
func (e *BuiltinExecutor) UnloadSkill(skillID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.loadedSkills, skillID)
	return nil
}

// ListLoadedSkills returns a list of loaded skill IDs
func (e *BuiltinExecutor) ListLoadedSkills() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	skills := make([]string, 0, len(e.loadedSkills))
	for skillID := range e.loadedSkills {
		skills = append(skills, skillID)
	}

	return skills
}
