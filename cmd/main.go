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

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/server"
)

func main() {
	// Initialize structured logging
	if err := logging.Initialize(); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Load global configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	srv := server.New(cfg)

	logging.Sugar.Infow("🚀 loqa-hub starting with HTTP/1.1 streaming architecture",
		"http_port", cfg.Server.Port,
		"db_path", cfg.Server.DBPath,
		"architecture", "HTTP/1.1 Binary Streaming",
	)

	if err := srv.Start(); err != nil {
		logging.LogError(err, "Failed to start server")
		log.Fatalf("Failed to start server: %v", err)
	}
}
