package grpc

/**
 * @title KDC Service - gRPC Server
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Bootstrapping the gRPC server and managing its lifecycle.
 */

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "mini_banking/pkg/pb/kdc"
)

type Server struct {
	grpcServer *grpc.Server
	port       string
}

func NewServer(handler *Handler, port string) *Server {
	// Initialize the underlying gRPC server
	grpcServer := grpc.NewServer()

	// Register the KDC Service Handler
	pb.RegisterKDCServiceServer(grpcServer, handler)

	// Enable health check protocol
	healthcheck := health.NewServer()
	healthcheck.SetServingStatus("kdc.KDCService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthcheck.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)

	// Enable Server Reflection for easier debugging via grpcui / grpcurl
	reflection.Register(grpcServer)

	return &Server{
		grpcServer: grpcServer,
		port:       port,
	}
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.port, err)
	}

	log.Printf("[KDC] Starting gRPC Server on %s\n", s.port)
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC: %w", err)
	}

	return nil
}

func (s *Server) Stop() {
	log.Println("[KDC] Shutting down gRPC Server gracefully...")
	s.grpcServer.GracefulStop()
	log.Println("[KDC] Server stopped.")
}
