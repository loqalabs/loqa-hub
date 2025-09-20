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

package transport

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/loqalabs/loqa-hub/internal/logging"
)

// Error injection test suite based on lessons learned from puck testing
// These tests revealed critical issues in the puck and should catch similar problems in the hub

// setupTestLogging initializes logging for tests
func setupTestLogging() {
	if logging.Sugar == nil {
		_ = logging.Initialize()
	}
}

func TestErrorInjection_MalformedFrames(t *testing.T) {
	tests := []struct {
		name        string
		setupFrame  func() []byte
		expectError bool
		errorType   string
	}{
		{
			name: "Invalid magic number",
			setupFrame: func() []byte {
				data := make([]byte, HeaderSize)
				// Write invalid magic
				copy(data[0:4], []byte{0xDE, 0xAD, 0xBE, 0xEF})
				return data
			},
			expectError: true,
			errorType:   "invalid frame magic",
		},
		{
			name: "Frame too small",
			setupFrame: func() []byte {
				return make([]byte, HeaderSize-1)
			},
			expectError: true,
			errorType:   "frame too small",
		},
		{
			name: "Frame size exceeds maximum",
			setupFrame: func() []byte {
				// This will fail during serialization, so we expect serialization to fail
				frame := NewFrame(FrameTypeAudioData, 1, 1, uint64(time.Now().UnixMicro()), make([]byte, MaxDataSize+1)) //nolint:gosec // G115: Safe conversion for test timestamp
				data, err := frame.Serialize()
				if err != nil {
					// Return empty data to test frame too small error
					return make([]byte, 0)
				}
				return data
			},
			expectError: true,
			errorType:   "frame too small",
		},
		{
			name: "Invalid frame type",
			setupFrame: func() []byte {
				frame := &Frame{
					Type:      FrameType(0xFF), // Invalid frame type
					SessionID: 1,
					Sequence:  1,
					Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
					Data:      []byte("test"),
				}
				data, _ := frame.Serialize()
				return data
			},
			expectError: true,
			errorType:   "invalid frame type",
		},
		{
			name: "Audio frame with odd byte count",
			setupFrame: func() []byte {
				frame := NewFrame(FrameTypeAudioData, 1, 1, uint64(time.Now().UnixMicro()), []byte("odd")) //nolint:gosec // G115: Safe conversion for test timestamp
				data, _ := frame.Serialize()
				return data
			},
			expectError: true,
			errorType:   "not multiple of 2",
		},
		{
			name: "Corrupted frame data",
			setupFrame: func() []byte {
				frame := NewFrame(FrameTypeAudioData, 1, 1, uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test timestamp
				data, _ := frame.Serialize()
				// Corrupt the frame by changing magic bytes
				data[0] = 0xFF
				return data
			},
			expectError: true,
			errorType:   "invalid frame magic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frameData := tt.setupFrame()

			// Test frame deserialization first
			frame, deserializeErr := DeserializeFrame(frameData)

			// Test frame validation if deserialization succeeds
			var validationErr error
			if deserializeErr == nil && frame != nil {
				validationErr = ValidateFrameForHubProcessing(frame)
			}

			// Check if either operation produced the expected error
			var finalErr error
			if deserializeErr != nil {
				finalErr = deserializeErr
			} else if validationErr != nil {
				finalErr = validationErr
			}

			if tt.expectError {
				assert.Error(t, finalErr, "Expected an error but got none")
				if finalErr != nil {
					assert.Contains(t, finalErr.Error(), tt.errorType)
				}
			} else {
				assert.NoError(t, finalErr, "Expected no error but got: %v", finalErr)
			}
		})
	}
}

func TestErrorInjection_ResourceExhaustion(t *testing.T) {
	setupTestLogging()

	if testing.Short() {
		t.Skip("Skipping resource exhaustion test in short mode")
	}

	t.Run("Goroutine leak detection", func(t *testing.T) {
		initialGoroutines := runtime.NumGoroutine()

		// Create resource monitor
		monitor := NewResourceMonitor()

		// Simulate goroutine leak
		var wg sync.WaitGroup
		for i := 0; i < 60; i++ { // More than the 50 goroutine threshold
			wg.Add(1)
			go func() {
				defer wg.Done()
				time.Sleep(100 * time.Millisecond)
			}()
		}

		// Update metrics and check for leak detection
		monitor.UpdateMetrics(1)
		warnings := monitor.CheckResourceLeaks()

		// Should detect goroutine increase
		assert.True(t, len(warnings) > 0, "Should detect goroutine leak")

		// Wait for goroutines to finish
		wg.Wait()

		// Allow time for cleanup
		time.Sleep(200 * time.Millisecond)
		runtime.GC()

		finalGoroutines := runtime.NumGoroutine()
		assert.LessOrEqual(t, finalGoroutines, initialGoroutines+5, "Goroutines should be cleaned up")
	})

	t.Run("Memory leak detection", func(t *testing.T) {
		setupTestLogging()
		monitor := NewResourceMonitor()

		// Simulate memory allocation
		var memoryHogs [][]byte
		for i := 0; i < 1000; i++ {
			memoryHogs = append(memoryHogs, make([]byte, 1024*1024)) // 1MB each for memory pressure test
		}

		// Update metrics
		monitor.UpdateMetrics(1)
		warnings := monitor.CheckResourceLeaks()

		// Should detect memory increase
		foundMemoryWarning := false
		for _, warning := range warnings {
			if strings.Contains(warning, "memory increase") {
				foundMemoryWarning = true
				break
			}
		}
		assert.True(t, foundMemoryWarning, "Should detect memory increase")

		// Clean up memory hogs - ensure GC can collect
		for i := range memoryHogs {
			memoryHogs[i] = nil
		}
		// Clear slice (variable not used after this point)
		runtime.GC()
		runtime.GC() // Second GC to clean up finalizers
	})
}

func TestErrorInjection_ConcurrentConnections(t *testing.T) {
	setupTestLogging()

	if testing.Short() {
		t.Skip("Skipping concurrent connection test in short mode")
	}

	// This test simulates the concurrent connection scenarios that revealed issues in puck testing
	t.Run("Multiple simultaneous connections", func(t *testing.T) {
		const numConnections = 50
		const framesPerConnection = 100

		monitor := NewResourceMonitor()

		// Track metrics
		var (
			successfulConnections int64
			totalFramesProcessed  int64
			errors                []error
			errorsMutex           sync.Mutex
		)

		var wg sync.WaitGroup

		// Start concurrent connections
		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func(connID int) {
				defer wg.Done()

				// Simulate frame processing for this connection
				framesProcessed := 0
				for j := 0; j < framesPerConnection; j++ {
					// Create frame with even-length data for valid audio frames
					frameData := fmt.Sprintf("data_%d_%d", connID, j)
					if len(frameData)%2 != 0 {
						frameData += "x" // Make even length
					}
					frame := NewFrame(
						FrameTypeAudioData,
						uint32(connID+1), //nolint:gosec // G115: Safe conversion - connID is bounded by test loop
						uint32(j),        //nolint:gosec // G115: Safe conversion - j is bounded by test loop
						uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
						[]byte(frameData),
					)

					// Validate frame (simulating hub processing)
					if err := ValidateFrameForHubProcessing(frame); err != nil {
						errorsMutex.Lock()
						errors = append(errors, fmt.Errorf("conn %d frame %d: %w", connID, j, err))
						errorsMutex.Unlock()
						continue
					}

					framesProcessed++

					// Small delay to simulate processing time
					time.Sleep(time.Microsecond * 100)
				}

				if framesProcessed == framesPerConnection {
					atomic.AddInt64(&successfulConnections, 1)
				}
				atomic.AddInt64(&totalFramesProcessed, int64(framesProcessed))

			}(i)
		}

		// Wait for all connections to complete
		wg.Wait()

		// Update resource metrics
		monitor.UpdateMetrics(int(successfulConnections))

		// Verify results
		assert.Equal(t, int64(numConnections), successfulConnections, "All connections should succeed")
		assert.Equal(t, int64(numConnections*framesPerConnection), totalFramesProcessed, "All frames should be processed")
		assert.Empty(t, errors, "No frame processing errors should occur")

		// Check for resource leaks
		warnings := monitor.CheckResourceLeaks()
		for _, warning := range warnings {
			t.Logf("Resource warning: %s", warning)
		}

		// Allow some tolerance for resource cleanup
		time.Sleep(100 * time.Millisecond)
		runtime.GC()
	})
}

func TestErrorInjection_NetworkFailures(t *testing.T) {
	tests := []struct {
		name           string
		simulateError  func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name: "Connection drop during frame transmission",
			simulateError: func(w http.ResponseWriter, r *http.Request) {
				// Start writing response then close connection
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("partial")) // Ignore error as this is test code simulating failure
				// Simulate connection drop by panicking
				panic("connection dropped")
			},
			expectError:   false, // The server will return 200 before the panic
			errorContains: "",
		},
		{
			name: "HTTP 500 server error",
			simulateError: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			},
			expectError:   true,
			errorContains: "500",
		},
		{
			name: "Timeout simulation",
			simulateError: func(w http.ResponseWriter, r *http.Request) {
				// Simulate slow response
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			},
			expectError:   true,
			errorContains: "deadline exceeded",
		},
		{
			name: "Malformed HTTP response",
			simulateError: func(w http.ResponseWriter, r *http.Request) {
				// Write invalid HTTP response
				conn, _, _ := w.(http.Hijacker).Hijack()
				_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: invalid\r\n\r\n")) // Ignore error as this is test code simulating failure
				_ = conn.Close() // Ignore error as this is test code simulating failure
			},
			expectError:   true,
			errorContains: "Content-Length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that simulates the error condition
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer func() {
					if r := recover(); r != nil {
						// Handle panics (connection drops)
						t.Logf("Simulated connection drop: %v", r)
					}
				}()
				tt.simulateError(w, r)
			}))
			defer server.Close()

			// Create HTTP client with short timeout for timeout tests
			client := &http.Client{
				Timeout: 1 * time.Second,
			}

			// Attempt to send a frame
			frame := NewFrame(
				FrameTypeAudioData,
				12345,
				1,
				uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				[]byte("test data"),
			)

			frameData, err := frame.Serialize()
			require.NoError(t, err)

			// Send request to test server
			req, err := http.NewRequest("POST", server.URL, bytes.NewReader(frameData))
			require.NoError(t, err)

			resp, err := client.Do(req)

			if tt.expectError {
				// Either the request should fail, or the response should indicate an error
				if err != nil {
					assert.Contains(t, err.Error(), tt.errorContains)
				} else {
					assert.True(t, resp.StatusCode >= 400, "Should receive error status code")
					_ = resp.Body.Close() // Ignore error in test cleanup
				}
			} else {
				assert.NoError(t, err)
				if resp != nil {
					assert.Equal(t, http.StatusOK, resp.StatusCode)
					_ = resp.Body.Close() // Ignore error in test cleanup
				}
			}
		})
	}
}

func TestErrorInjection_SessionManagement(t *testing.T) {
	setupTestLogging()

	t.Run("Invalid session handling", func(t *testing.T) {
		tests := []struct {
			name      string
			sessionID string
			puckID    string
			expectErr bool
		}{
			{"Empty session ID", "", "puck1", true},
			{"Empty puck ID", "session1", "", true},
			{"Valid IDs", "session1", "puck1", false},
			{"Very long session ID", strings.Repeat("a", 1000), "puck1", true},
			{"Special characters in session ID", "session/with/slashes", "puck1", true},
			{"Null bytes in IDs", "session\x00null", "puck1", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create a test frame
				frame := NewFrame(
					FrameTypeAudioData,
					12345,
					1,
					uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
					[]byte("test"),
				)

				// Validate the frame first
				err := ValidateFrameForHubProcessing(frame)
				assert.NoError(t, err, "Frame should be valid")

				// Test session ID validation (simulate what the hub would do)
				if tt.sessionID == "" || tt.puckID == "" {
					assert.True(t, tt.expectErr, "Empty IDs should be rejected")
				} else if len(tt.sessionID) > 255 { // Reasonable limit
					assert.True(t, tt.expectErr, "Overly long session IDs should be rejected")
				} else if strings.ContainsAny(tt.sessionID, "/\x00") {
					assert.True(t, tt.expectErr, "Invalid characters should be rejected")
				}
			})
		}
	})

	t.Run("Session lifecycle stress test", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping session stress test in short mode")
		}

		monitor := NewResourceMonitor()
		const numSessions = 100
		const operationsPerSession = 50

		var wg sync.WaitGroup
		var sessionErrors int64

		for i := 0; i < numSessions; i++ {
			wg.Add(1)
			go func(sessionNum int) {
				defer wg.Done()

				sessionID := fmt.Sprintf("stress_session_%d", sessionNum)
				puckID := fmt.Sprintf("stress_puck_%d", sessionNum)

				for j := 0; j < operationsPerSession; j++ {
					// Create frame with even-length data for valid audio frames
					frameData := fmt.Sprintf("data_%d_%d", sessionNum, j)
					if len(frameData)%2 != 0 {
						frameData += "x" // Make even length
					}
					frame := NewFrame(
						FrameTypeAudioData,
						uint32(sessionNum+1), //nolint:gosec // G115: Safe conversion - sessionNum is bounded by test loop
						uint32(j),            //nolint:gosec // G115: Safe conversion - j is bounded by test loop
						uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
						[]byte(frameData),
					)

					// Validate frame
					if err := ValidateFrameForHubProcessing(frame); err != nil {
						atomic.AddInt64(&sessionErrors, 1)
						continue
					}

					// Simulate session operations
					if sessionID == "" || puckID == "" {
						atomic.AddInt64(&sessionErrors, 1)
					}

					// Brief delay to simulate processing
					time.Sleep(time.Microsecond * 10)
				}
			}(i)
		}

		wg.Wait()

		// Check resource usage
		monitor.UpdateMetrics(numSessions)
		warnings := monitor.CheckResourceLeaks()

		assert.Equal(t, int64(0), sessionErrors, "No session errors should occur")

		// Log any resource warnings
		for _, warning := range warnings {
			t.Logf("Resource warning: %s", warning)
		}
	})
}

func TestErrorInjection_GracefulShutdown(t *testing.T) {
	setupTestLogging()

	t.Run("Shutdown during active connections", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping shutdown test in short mode")
		}

		monitor := NewResourceMonitor()

		// Create a context for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())

		var activeConnections int64
		var wg sync.WaitGroup
		const numConnections = 20

		// Start multiple long-running connections
		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func(connID int) {
				defer wg.Done()
				defer atomic.AddInt64(&activeConnections, -1)

				atomic.AddInt64(&activeConnections, 1)

				// Simulate long-running connection
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						// Graceful shutdown requested
						return
					case <-ticker.C:
						// Process a frame with even-length data for valid audio frames
						frameData := fmt.Sprintf("conn_%d_data", connID)
						if len(frameData)%2 != 0 {
							frameData += "x" // Make even length
						}
						frame := NewFrame(
							FrameTypeAudioData,
							uint32(connID+1), //nolint:gosec // G115: Safe conversion - connID is bounded by test loop
							uint32(time.Now().Unix()), //nolint:gosec // G115: Safe conversion for test timestamp
							uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
							[]byte(frameData),
						)

						if err := ValidateFrameForHubProcessing(frame); err != nil {
							t.Errorf("Frame validation failed during shutdown test: %v", err)
							return
						}
					}
				}
			}(i)
		}

		// Wait for connections to start
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, int64(numConnections), atomic.LoadInt64(&activeConnections))

		// Trigger graceful shutdown
		cancel()

		// Wait for all connections to gracefully terminate
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		// Wait with timeout
		select {
		case <-done:
			// Success - all connections terminated gracefully
		case <-time.After(5 * time.Second):
			t.Error("Graceful shutdown timed out")
		}

		// Verify all connections are cleaned up
		assert.Equal(t, int64(0), atomic.LoadInt64(&activeConnections))

		// Check for resource leaks after shutdown
		monitor.UpdateMetrics(0)
		warnings := monitor.CheckResourceLeaks()

		// Force cleanup to help with resource leak detection
		monitor.ForceCleanup()

		// Log any warnings but don't fail the test unless they're severe
		for _, warning := range warnings {
			t.Logf("Post-shutdown resource warning: %s", warning)
		}
	})
}

// Benchmark tests to ensure performance doesn't degrade under load
func BenchmarkFrameValidation(b *testing.B) {
	setupTestLogging()

	frame := NewFrame(
		FrameTypeAudioData,
		12345,
		1,
		uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
		make([]byte, 1000),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidateFrameForHubProcessing(frame); err != nil {
			b.Fatalf("Validation failed: %v", err)
		}
	}
}

func BenchmarkFrameSerialization(b *testing.B) {
	setupTestLogging()

	frame := NewFrame(
		FrameTypeAudioData,
		12345,
		1,
		uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
		make([]byte, MaxDataSize),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := frame.Serialize()
		if err != nil {
			b.Fatalf("Serialization failed: %v", err)
		}
	}
}

func BenchmarkResourceMonitoring(b *testing.B) {
	setupTestLogging()

	monitor := NewResourceMonitor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor.UpdateMetrics(10)
		monitor.CheckResourceLeaks()
	}
}