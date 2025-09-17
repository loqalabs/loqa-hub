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

package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/loqalabs/loqa-hub/internal/api"
	"github.com/loqalabs/loqa-hub/internal/config"
	grpcservice "github.com/loqalabs/loqa-hub/internal/grpc"
	"github.com/loqalabs/loqa-hub/internal/storage"
	pb "github.com/loqalabs/loqa-proto/go/audio"
	"google.golang.org/grpc"
)

type Server struct {
	cfg          *config.Config
	mux          *http.ServeMux
	grpcServer   *grpc.Server
	audioService *grpcservice.AudioService
	database     *storage.Database
	eventsStore  *storage.VoiceEventsStore
	apiHandler   *api.VoiceEventsHandler
}

func New(cfg *config.Config) *Server {
	return NewWithOptions(cfg, true)
}

func NewWithOptions(cfg *config.Config, enableHealthChecks bool) *Server {
	mux := http.NewServeMux()

	// Initialize database
	dbConfig := storage.DatabaseConfig{
		Path: cfg.Server.DBPath,
	}
	database, err := storage.NewDatabase(dbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create voice events store
	eventsStore := storage.NewVoiceEventsStore(database)

	// Create API handler
	apiHandler := api.NewVoiceEventsHandler(eventsStore)

	// Create gRPC server and audio service
	grpcServer := grpc.NewServer()

	var audioService *grpcservice.AudioService

	log.Printf("🎙️  Using STT service at: %s", cfg.STT.URL)
	log.Printf("🔊 Using TTS service at: %s", cfg.TTS.URL)
	audioService, err = grpcservice.NewAudioServiceWithTTSAndOptions(cfg.STT.URL, cfg.STT.Language, cfg.TTS, eventsStore, enableHealthChecks)

	if err != nil {
		log.Fatalf("Failed to create audio service: %v", err)
	}

	// Register audio service with gRPC server
	pb.RegisterAudioServiceServer(grpcServer, audioService)

	s := &Server{
		cfg:          cfg,
		mux:          mux,
		grpcServer:   grpcServer,
		audioService: audioService,
		database:     database,
		eventsStore:  eventsStore,
		apiHandler:   apiHandler,
	}
	s.routes()
	return s
}

func (s *Server) Start() error {
	// Start gRPC server in a goroutine
	go func() {
		grpcPort := strconv.Itoa(s.cfg.Server.GRPCPort)

		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("Failed to listen on gRPC port %s: %v", grpcPort, err)
		}

		log.Printf("🎙️  gRPC server listening on :%s", grpcPort)
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start HTTP server with security timeouts
	httpPort := strconv.Itoa(s.cfg.Server.Port)
	log.Printf("🌐 HTTP server listening on :%s", httpPort)
	server := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)

	// Voice Events API
	s.mux.HandleFunc("/api/voice-events", s.apiHandler.HandleVoiceEvents)
	s.mux.HandleFunc("/api/voice-events/", s.apiHandler.HandleVoiceEventByID)

	// future: /wake, /stream, /session, etc.
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	log.Println("Health check received")
	if _, err := fmt.Fprintln(w, "ok"); err != nil {
		log.Printf("Failed to write health check response: %v", err)
	}
}
