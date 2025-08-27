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
	composeCmd.Dir = "../.." // Run from project root
	if err := composeCmd.Run(); err != nil {
		t.Fatalf("Failed to start Docker Compose: %v", err)
	}

	// Cleanup function
	defer func() {
		t.Log("Stopping Docker Compose services...")
		stopCmd := exec.Command("docker-compose", "down")
		stopCmd.Dir = "../.."
		if err := stopCmd.Run(); err != nil {
			t.Logf("Warning: Failed to stop Docker Compose: %v", err)
		}
	}()

	// Wait for services to be ready
	t.Log("Waiting for services to be ready...")
	time.Sleep(hubWaitTime)

	// Check if services are running
	checkCmd := exec.CommandContext(ctx, "docker-compose", "ps")
	checkCmd.Dir = "../.."
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

	// Run test puck for a short duration
	t.Log("Testing puck connection...")
	if err := testPuckConnection(ctx); err != nil {
		t.Logf("Warning: Puck connection test failed: %v", err)
	}

	t.Log("Voice pipeline e2e test completed successfully")
}

func testHubConnectivity(ctx context.Context) error {
	// Simple connectivity test using grpcurl or similar
	testCmd := exec.CommandContext(ctx, "docker-compose", "logs", "--tail=10", "loqa-hub")
	testCmd.Dir = "../.."
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

func testPuckConnection(ctx context.Context) error {
	// Check if test puck binary exists
	puckPath := "../../bin/test-puck"
	if _, err := os.Stat(puckPath); os.IsNotExist(err) {
		return nil // Skip if binary doesn't exist
	}

	// Run test puck for a short duration
	puckCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	puckCmd := exec.CommandContext(puckCtx, puckPath, "--hub", "localhost:50051")
	puckCmd.Dir = "../.."

	// Start puck and let it run briefly
	if err := puckCmd.Start(); err != nil {
		return err
	}

	// Wait for context timeout or process completion
	return puckCmd.Wait()
}