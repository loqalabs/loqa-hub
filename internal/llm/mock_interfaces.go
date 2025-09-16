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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPClient interface for dependency injection in tests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// MockHTTPClient implements HTTPClient for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return nil, fmt.Errorf("no mock function provided")
}

// CreateMockHTTPClient creates a mock client that returns standard responses
func CreateMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Parse the request to determine response
			body := req.Body
			if body != nil {
				defer func() {
					_ = body.Close() // Ignore close errors in mock
				}()
			}

			// Default mock response for command parsing
			response := OllamaResponse{
				Response: `{"intent":"lights.control","entities":{"action":"turn_on","location":"bedroom"},"confidence":0.9}`,
				Done:     true,
			}

			jsonBytes, _ := json.Marshal(response)
			resp := &http.Response{
				StatusCode: 200,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(jsonBytes))),
			}
			resp.Header.Set("Content-Type", "application/json")
			return resp, nil
		},
	}
}

// CreateMockHTTPClientWithError creates a mock client that returns errors
func CreateMockHTTPClientWithError(errMsg string) *MockHTTPClient {
	return &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("%s", errMsg)
		},
	}
}

// LLMService interface for dependency injection
type LLMService interface {
	Query(ctx context.Context, prompt string) (string, error)
	QueryStreaming(ctx context.Context, prompt string) (<-chan string, error)
}

// MockLLMService implements LLMService for testing
type MockLLMService struct {
	QueryFunc          func(ctx context.Context, prompt string) (string, error)
	QueryStreamingFunc func(ctx context.Context, prompt string) (<-chan string, error)
}

func (m *MockLLMService) Query(ctx context.Context, prompt string) (string, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, prompt)
	}
	// Default response
	return `{"intent":"lights.control","entities":{"action":"turn_on","location":"bedroom"},"confidence":0.9}`, nil
}

func (m *MockLLMService) QueryStreaming(ctx context.Context, prompt string) (<-chan string, error) {
	if m.QueryStreamingFunc != nil {
		return m.QueryStreamingFunc(ctx, prompt)
	}
	// Default streaming response
	ch := make(chan string, 1)
	ch <- `{"intent":"lights.control","entities":{"action":"turn_on","location":"bedroom"},"confidence":0.9}`
	close(ch)
	return ch, nil
}

// CreateMockLLMService creates a mock LLM service with standard responses
func CreateMockLLMService() *MockLLMService {
	return &MockLLMService{
		QueryFunc: func(ctx context.Context, prompt string) (string, error) {
			// Analyze prompt to provide appropriate mock response
			if strings.Contains(prompt, "weather") {
				return `{"intent":"weather.query","entities":{"location":"current"},"confidence":0.85}`, nil
			}
			if strings.Contains(prompt, "lights") {
				return `{"intent":"lights.control","entities":{"action":"turn_on","location":"bedroom"},"confidence":0.9}`, nil
			}
			if strings.Contains(prompt, "music") {
				return `{"intent":"music.play","entities":{"genre":"jazz"},"confidence":0.8}`, nil
			}
			// Default response
			return `{"intent":"unknown","entities":{},"confidence":0.3}`, nil
		},
		QueryStreamingFunc: func(ctx context.Context, prompt string) (<-chan string, error) {
			ch := make(chan string, 1)
			if strings.Contains(prompt, "lights") {
				ch <- `{"intent":"lights.control","entities":{"action":"turn_on","location":"bedroom"},"confidence":0.9}`
			} else {
				ch <- `{"intent":"unknown","entities":{},"confidence":0.3}`
			}
			close(ch)
			return ch, nil
		},
	}
}

// CreateMockLLMServiceWithError creates a mock LLM service that returns errors
func CreateMockLLMServiceWithError(errMsg string) *MockLLMService {
	return &MockLLMService{
		QueryFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", fmt.Errorf("%s", errMsg)
		},
		QueryStreamingFunc: func(ctx context.Context, prompt string) (<-chan string, error) {
			return nil, fmt.Errorf("%s", errMsg)
		},
	}
}