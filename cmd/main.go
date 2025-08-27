package main

import (
	"log"
	"os"

	"github.com/loqalabs/loqa-hub/internal/server"
)

func main() {
	port := getEnv("LOQA_HUB_PORT", "3000")
	grpcPort := getEnv("LOQA_GRPC_PORT", "50051")
	modelPath := getEnv("MODEL_PATH", "/tmp/whisper.cpp/models/ggml-tiny.bin")
	asrURL := getEnv("ASR_HOST", "http://localhost:5001")
	intentURL := getEnv("INTENT_HOST", "http://localhost:5003")
	ttsURL := getEnv("TTS_HOST", "http://localhost:5002")

	cfg := server.Config{
		Port:      port,
		GRPCPort:  grpcPort,
		ModelPath: modelPath,
		ASRURL:    asrURL,
		IntentURL: intentURL,
		TTSURL:    ttsURL,
	}

	srv := server.New(cfg)

	log.Printf("🚀 loqa-hub listening on http://0.0.0.0:%s", port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnv(key string, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
