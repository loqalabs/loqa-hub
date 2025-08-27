package server

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"
	pb "github.com/loqalabs/loqa-proto/go"
	grpcservice "github.com/loqalabs/loqa-hub/internal/grpc"
)

type Config struct {
	Port      string
	GRPCPort  string
	ModelPath string
	ASRURL    string
	IntentURL string
	TTSURL    string
}

type Server struct {
	cfg         Config
	mux         *http.ServeMux
	grpcServer  *grpc.Server
	audioService *grpcservice.AudioService
}

func New(cfg Config) *Server {
	mux := http.NewServeMux()

	// Create gRPC server and audio service
	grpcServer := grpc.NewServer()
	audioService, err := grpcservice.NewAudioService(cfg.ModelPath)
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
	}
	s.routes()
	return s
}

func (s *Server) Start() error {
	// Start gRPC server in a goroutine
	go func() {
		grpcPort := s.cfg.GRPCPort
		if grpcPort == "" {
			grpcPort = "50051"
		}

		lis, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("Failed to listen on gRPC port %s: %v", grpcPort, err)
		}

		log.Printf("🎙️  gRPC server listening on :%s", grpcPort)
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start HTTP server
	log.Printf("🌐 HTTP server listening on :%s", s.cfg.Port)
	return http.ListenAndServe(":"+s.cfg.Port, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	// future: /wake, /stream, /session, etc.
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	log.Println("Health check received")
	fmt.Fprintln(w, "ok")
}
