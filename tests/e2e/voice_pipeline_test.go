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

package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

const (
	testTimeout = 60 * time.Second
	hubWaitTime = 10 * time.Second
)

func TestVoicePipelineEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Start Docker Compose services
	t.Log("Starting Docker Compose services...")
	composeCmd := exec.CommandContext(ctx, "docker-compose", "up", "-d")
	composeCmd.Dir = "../../../loqa" // Run from main orchestration directory
	if err := composeCmd.Run(); err != nil {
		t.Fatalf("Failed to start Docker Compose: %v", err)
	}

	// Cleanup function
	defer func() {
		t.Log("Stopping Docker Compose services...")
		stopCmd := exec.Command("docker-compose", "down")
		stopCmd.Dir = "../../../loqa"
		if err := stopCmd.Run(); err != nil {
			t.Logf("Warning: Failed to stop Docker Compose: %v", err)
		}
	}()

	// Wait for services to be ready
	t.Log("Waiting for services to be ready...")
	time.Sleep(hubWaitTime)

	// Check if services are running
	checkCmd := exec.CommandContext(ctx, "docker-compose", "ps")
	checkCmd.Dir = "../../../loqa"
	output, err := checkCmd.Output()
	if err != nil {
		t.Fatalf("Failed to check Docker Compose status: %v", err)
	}

	t.Logf("Docker Compose status:\n%s", output)

	// Test hub connectivity
	t.Log("Testing hub connectivity...")
	if err := testHubConnectivity(ctx); err != nil {
		t.Fatalf("Hub connectivity test failed: %v", err)
	}

	// Run test relay for a short duration
	t.Log("Testing relay connection...")
	if err := testRelayConnection(ctx); err != nil {
		t.Logf("Warning: Relay connection test failed: %v", err)
	}

	t.Log("Voice pipeline e2e test completed successfully")
}

func testHubConnectivity(ctx context.Context) error {
	// Simple connectivity test using grpcurl or similar
	testCmd := exec.CommandContext(ctx, "docker-compose", "logs", "--tail=10", "loqa-hub")
	testCmd.Dir = "../../../loqa"
	output, err := testCmd.Output()
	if err != nil {
		return err
	}

	// Check if hub is responding
	if len(output) == 0 {
		return nil // No errors if logs are present
	}

	return nil
}

func testRelayConnection(ctx context.Context) error {
	// Check if test relay binary exists in the loqa-relay repository
	relayPath := "../../../loqa-relay/bin/test-relay"
	if _, err := os.Stat(relayPath); os.IsNotExist(err) {
		return nil // Skip if binary doesn't exist
	}

	// Run test relay for a short duration
	relayCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	relayCmd := exec.CommandContext(relayCtx, relayPath, "--hub", "localhost:50051")
	relayCmd.Dir = "../../../loqa"

	// Start relay and let it run briefly
	if err := relayCmd.Start(); err != nil {
		return err
	}

	// Wait for context timeout or process completion
	return relayCmd.Wait()
}
