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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
)

func TestKokoroClient_NewKokoroClient(t *testing.T) {
	// Test server that responds successfully to /audio/voices
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"voices": ["af_bella", "af_sky"]}`))
		}
	}))
	defer server.Close()

	cfg := config.TTSConfig{
		URL:             server.URL,
		Voice:           "af_bella",
		Speed:           1.0,
		ResponseFormat:  "mp3",
		Normalize:       true,
		MaxConcurrent:   5,
		Timeout:         5 * time.Second,
		FallbackEnabled: true,
	}

	client, err := NewKokoroClient(cfg)
	if err != nil {
		t.Fatalf("Expected successful client creation, got error: %v", err)
	}

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Test getting voices
	voices, err := client.GetAvailableVoices()
	if err != nil {
		t.Fatalf("Expected successful voices retrieval, got error: %v", err)
	}

	expectedVoices := []string{"af_bella", "af_sky"}
	if len(voices) != len(expectedVoices) {
		t.Fatalf("Expected %d voices, got %d", len(expectedVoices), len(voices))
	}

	for i, voice := range voices {
		if voice != expectedVoices[i] {
			t.Errorf("Expected voice %s, got %s", expectedVoices[i], voice)
		}
	}

	// Clean up
	client.Close()
}

func TestKokoroClient_NewKokoroClient_InvalidURL(t *testing.T) {
	cfg := config.TTSConfig{
		URL:             "",
		Voice:           "af_bella",
		Speed:           1.0,
		ResponseFormat:  "mp3",
		Normalize:       true,
		MaxConcurrent:   5,
		Timeout:         5 * time.Second,
		FallbackEnabled: true,
	}

	_, err := NewKokoroClient(cfg)
	if err == nil {
		t.Fatal("Expected error for empty URL, got nil")
	}

	if !strings.Contains(err.Error(), "URL cannot be empty") {
		t.Errorf("Expected 'URL cannot be empty' error, got: %v", err)
	}
}

func TestKokoroClient_Synthesize(t *testing.T) {
	// Test server that responds to both voices and synthesis requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio/voices":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"voices": ["af_bella"]}`))
		case "/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("fake-mp3-data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.TTSConfig{
		URL:             server.URL,
		Voice:           "af_bella",
		Speed:           1.0,
		ResponseFormat:  "mp3",
		Normalize:       true,
		MaxConcurrent:   5,
		Timeout:         5 * time.Second,
		FallbackEnabled: true,
	}

	client, err := NewKokoroClient(cfg)
	if err != nil {
		t.Fatalf("Expected successful client creation, got error: %v", err)
	}
	defer client.Close()

	options := &TTSOptions{
		Voice:          "af_bella",
		Speed:          1.0,
		ResponseFormat: "mp3",
		Normalize:      true,
	}

	result, err := client.Synthesize("Hello world", options)
	if err != nil {
		t.Fatalf("Expected successful synthesis, got error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ContentType != "audio/mpeg" {
		t.Errorf("Expected content type 'audio/mpeg', got '%s'", result.ContentType)
	}

	// Read the audio data
	data := make([]byte, 100)
	n, _ := result.Audio.Read(data)
	if n == 0 {
		t.Error("Expected to read some audio data")
	}

	if !strings.Contains(string(data[:n]), "fake-mp3-data") {
		t.Error("Expected to find fake audio data in response")
	}
}

func TestKokoroClient_Synthesize_EmptyText(t *testing.T) {
	// We won't actually create a client for this test since we don't want
	// to make real connections. We'll test the validation directly.
	client := &KokoroClient{}

	_, err := client.Synthesize("", nil)
	if err == nil {
		t.Fatal("Expected error for empty text, got nil")
	}

	if !strings.Contains(err.Error(), "text cannot be empty") {
		t.Errorf("Expected 'text cannot be empty' error, got: %v", err)
	}
}
