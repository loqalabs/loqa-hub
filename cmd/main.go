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

package main

import (
	"log"
	"os"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/server"
)

func main() {
	// Initialize structured logging
	if err := logging.Initialize(); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	port := getEnv("LOQA_HUB_PORT", "3000")
	grpcPort := getEnv("LOQA_GRPC_PORT", "50051")
	sttURL := getEnv("STT_URL", "http://stt:8000")
	asrURL := getEnv("ASR_HOST", "http://localhost:5001")
	intentURL := getEnv("INTENT_HOST", "http://localhost:5003")
	ttsURL := getEnv("TTS_HOST", "http://localhost:5002")
	dbPath := getEnv("DB_PATH", "./data/loqa-hub.db")

	cfg := server.Config{
		Port:      port,
		GRPCPort:  grpcPort,
		STTURL:    sttURL,
		ASRURL:    asrURL,
		IntentURL: intentURL,
		TTSURL:    ttsURL,
		DBPath:    dbPath,
	}

	srv := server.New(cfg)

	logging.Sugar.Infow("🚀 loqa-hub starting",
		"http_port", port,
		"grpc_port", grpcPort,
		"db_path", dbPath,
	)

	if err := srv.Start(); err != nil {
		logging.LogError(err, "Failed to start server")
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnv(key string, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
