package grpc

/**
 * @title KDC Service - gRPC Server
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Bootstrapping the gRPC server and managing its lifecycle.
 */

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	// Load server certificate
	serverCert, err := tls.LoadX509KeyPair(
		"certs/grpc/kdc-server.crt",
		"certs/grpc/kdc-server.key",
	)
	if err != nil {
		panic(fmt.Sprintf("load server cert failed: %v", err))
	}

	// Load CA certificate
	caPem, err := os.ReadFile("certs/grpc/grpc-ca.crt")
	if err != nil {
		panic(fmt.Sprintf("load ca cert failed: %v", err))
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPem) {
		panic("failed to append CA cert")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS13,
	}

	creds := credentials.NewTLS(tlsConfig)

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
	)

	pb.RegisterKDCServiceServer(grpcServer, handler)

	healthcheck := health.NewServer()
	healthcheck.SetServingStatus(
		"kdc.KDCService",
		grpc_health_v1.HealthCheckResponse_SERVING,
	)
	healthcheck.SetServingStatus(
		"",
		grpc_health_v1.HealthCheckResponse_SERVING,
	)

	grpc_health_v1.RegisterHealthServer(
		grpcServer,
		healthcheck,
	)

	reflection.Register(grpcServer)

	return &Server{
		grpcServer: grpcServer,
		port:       port,
	}
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+s.port)
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
