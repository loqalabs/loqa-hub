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
	"context"
	"testing"
	"time"
)

// Integration tests for multi-command parsing functionality
// These tests focus on the integration between the parser and command queue

func TestMultiCommandIntegrationFlow(t *testing.T) {
	// Create a mock parser that returns predefined multi-commands
	parser := &MockCommandParser{}

	testCases := []struct {
		name              string
		utterance         string
		mockMultiCommand  *MultiCommand
		expectedExecCount int
		expectedSuccess   bool
		description       string
	}{
		{
			name:      "successful_dual_command",
			utterance: "turn on the lights and play music",
			mockMultiCommand: &MultiCommand{
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
				OriginalText:     "turn on the lights and play music",
				CombinedResponse: "I'll turn on the lights and play music for you.",
			},
			expectedExecCount: 2,
			expectedSuccess:   true,
			description:       "should successfully execute two commands",
		},
		{
			name:      "single_command_as_multi",
			utterance: "turn on the lights",
			mockMultiCommand: &MultiCommand{
				Commands: []Command{
					{
						Intent:     "turn_on",
						Entities:   map[string]string{"device": "lights"},
						Confidence: 0.9,
						Response:   "Turning on the lights",
					},
				},
				IsMulti:          false,
				OriginalText:     "turn on the lights",
				CombinedResponse: "Turning on the lights",
			},
			expectedExecCount: 1,
			expectedSuccess:   true,
			description:       "should handle single command correctly",
		},
		{
			name:      "complex_three_command_chain",
			utterance: "turn on the lights then play music and set the temperature",
			mockMultiCommand: &MultiCommand{
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
					{
						Intent:     "set_temperature",
						Entities:   map[string]string{"device": "thermostat", "value": "72"},
						Confidence: 0.7,
						Response:   "Setting temperature to 72 degrees",
					},
				},
				IsMulti:          true,
				OriginalText:     "turn on the lights then play music and set the temperature",
				CombinedResponse: "I'll turn on the lights, play music, and set the temperature.",
			},
			expectedExecCount: 3,
			expectedSuccess:   true,
			description:       "should handle complex three-command chain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock parser to return the test multi-command
			parser.SetMockMultiCommand(tc.mockMultiCommand)

			// Create executor to track commands
			executor := NewMockCommandExecutor([]bool{}, 10*time.Millisecond)

			// Execute the multi-command flow
			multiCmd, err := parser.ParseMultiCommand(tc.utterance)
			if err != nil {
				t.Errorf("ParseMultiCommand failed: %v (%s)", err, tc.description)
				return
			}

			if multiCmd.IsMulti && len(multiCmd.Commands) > 1 {
				// Create and execute command queue
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				queue := NewCommandQueue(multiCmd.Commands, 2*time.Second, true)
				result, execErr := queue.Execute(ctx, executor)

				if execErr != nil {
					t.Errorf("Command queue execution failed: %v (%s)", execErr, tc.description)
					return
				}

				if result.Success != tc.expectedSuccess {
					t.Errorf("Execution success = %v, expected %v (%s)",
						result.Success, tc.expectedSuccess, tc.description)
				}

				if len(result.CompletedItems) != tc.expectedExecCount {
					t.Errorf("Completed commands = %d, expected %d (%s)",
						len(result.CompletedItems), tc.expectedExecCount, tc.description)
				}

				// Verify performance target (< 200ms per command)
				avgDuration := result.TotalDuration / time.Duration(len(multiCmd.Commands))
				if avgDuration > 200*time.Millisecond {
					t.Errorf("Average command duration %v exceeds 200ms target (%s)",
						avgDuration, tc.description)
				}

				// Verify response aggregation
				if result.CombinedResponse == "" {
					t.Errorf("Combined response should not be empty (%s)", tc.description)
				}
			} else {
				// Single command case - simulate execution
				if len(multiCmd.Commands) != tc.expectedExecCount {
					t.Errorf("Command count = %d, expected %d (%s)",
						len(multiCmd.Commands), tc.expectedExecCount, tc.description)
				}

				// For single commands, manually execute through the executor
				if len(multiCmd.Commands) > 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancel()

					if err := executor.ExecuteCommand(ctx, &multiCmd.Commands[0]); err != nil {
						t.Errorf("Single command execution failed: %v (%s)", err, tc.description)
					}
				}
			}

			// Verify all commands were processed
			executedCommands := executor.GetExecutedCommands()
			if len(executedCommands) != tc.expectedExecCount {
				t.Errorf("Executed commands = %d, expected %d (%s)",
					len(executedCommands), tc.expectedExecCount, tc.description)
			}
		})
	}
}

func TestMultiCommandRollbackIntegration(t *testing.T) {
	// Test rollback functionality in integration
	commands := []Command{
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
		{
			Intent:     "turn_off",
			Entities:   map[string]string{"device": "tv"},
			Confidence: 0.7,
			Response:   "Turning off TV",
		},
	}

	// Make the second command fail to trigger rollback
	executor := NewMockCommandExecutor([]bool{false, true, false}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	queue := NewCommandQueue(commands, 2*time.Second, true)
	result, err := queue.Execute(ctx, executor)

	if err != nil {
		t.Errorf("Command queue execution should not return error: %v", err)
	}

	if result.Success {
		t.Error("Execution should fail due to second command failure")
	}

	if !result.RollbackOccurred {
		t.Error("Rollback should have occurred")
	}

	if len(result.CompletedItems) != 1 {
		t.Errorf("Should have 1 completed item before failure, got %d", len(result.CompletedItems))
	}

	if result.FailedItem == nil || result.FailedItem.Index != 1 {
		t.Error("Failed item should be the second command (index 1)")
	}

	// Verify rollback command was executed
	executedCommands := executor.GetExecutedCommands()
	if len(executedCommands) < 3 {
		t.Errorf("Should have at least 3 executed commands (including rollback), got %d", len(executedCommands))
	}

	// Last command should be a rollback (turn_off for the turn_on command)
	if len(executedCommands) >= 3 {
		rollbackCmd := executedCommands[len(executedCommands)-1]
		if rollbackCmd.Intent != "turn_off" {
			t.Errorf("Rollback command intent = %q, expected turn_off", rollbackCmd.Intent)
		}
	}
}

func TestMultiCommandPerformanceIntegration(t *testing.T) {
	// Test performance requirements for multi-command execution
	commands := make([]Command, 5) // Create 5 commands
	for i := 0; i < 5; i++ {
		commands[i] = Command{
			Intent:     "turn_on",
			Entities:   map[string]string{"device": "device" + string(rune('1'+i))},
			Confidence: 0.9,
			Response:   "Command " + string(rune('1'+i)) + " executed",
		}
	}

	executor := NewMockCommandExecutor([]bool{}, 50*time.Millisecond) // 50ms per command

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue := NewCommandQueue(commands, 5*time.Second, false)
	startTime := time.Now()
	result, err := queue.Execute(ctx, executor)
	totalTime := time.Since(startTime)

	if err != nil {
		t.Errorf("Command queue execution failed: %v", err)
	}

	if !result.Success {
		t.Error("All commands should execute successfully")
	}

	if len(result.CompletedItems) != 5 {
		t.Errorf("Should have 5 completed commands, got %d", len(result.CompletedItems))
	}

	// Verify performance target: each additional command should add < 200ms
	// First command can take longer due to setup, but additional commands should be fast
	if len(result.CompletedItems) > 1 {
		for i := 1; i < len(result.CompletedItems); i++ {
			item := result.CompletedItems[i]
			if item.Duration > 200*time.Millisecond {
				t.Errorf("Command %d duration %v exceeds 200ms target", i, item.Duration)
			}
		}
	}

	// Total execution time should be reasonable
	expectedMaxTime := time.Duration(len(commands)) * 200 * time.Millisecond
	if totalTime > expectedMaxTime {
		t.Errorf("Total execution time %v exceeds expected maximum %v", totalTime, expectedMaxTime)
	}

	t.Logf("Performance test completed: %d commands in %v (avg: %v per command)",
		len(commands), totalTime, totalTime/time.Duration(len(commands)))
}

// MockCommandParser for integration testing
type MockCommandParser struct {
	mockMultiCommand *MultiCommand
}

func (m *MockCommandParser) SetMockMultiCommand(multiCmd *MultiCommand) {
	m.mockMultiCommand = multiCmd
}

func (m *MockCommandParser) ParseMultiCommand(transcription string) (*MultiCommand, error) {
	if m.mockMultiCommand != nil {
		return m.mockMultiCommand, nil
	}

	// Default single command response
	return &MultiCommand{
		Commands: []Command{
			{
				Intent:     "unknown",
				Entities:   make(map[string]string),
				Confidence: 0.5,
				Response:   "I'm not sure what you want me to do.",
			},
		},
		IsMulti:          false,
		OriginalText:     transcription,
		CombinedResponse: "I'm not sure what you want me to do.",
	}, nil
}
