package grpc

import (
	"crypto/tls"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "mini-banking/pkg/pb/bank"
)

type SecurityConfig struct {
	ServerCertPath string
	ServerKeyPath  string
}

type Server struct {
	grpcServer *grpc.Server
	port       string
}

func NewServer(handler *Handler, port string, security SecurityConfig) (*Server, error) {
	transportCreds, err := newTLSTransportCredentials(security)
	if err != nil {
		return nil, err
	}

	grpcSrv := grpc.NewServer(
		grpc.Creds(transportCreds),
	)

	pb.RegisterBankServiceServer(grpcSrv, handler)

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("bank.BankService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(grpcSrv)

	return &Server{grpcServer: grpcSrv, port: port}, nil
}

func newTLSTransportCredentials(security SecurityConfig) (credentials.TransportCredentials, error) {
	if security.ServerCertPath == "" {
		return nil, fmt.Errorf("BANK_TLS_CERT_PATH is required")
	}
	if security.ServerKeyPath == "" {
		return nil, fmt.Errorf("BANK_TLS_KEY_PATH is required")
	}

	serverCert, err := tls.LoadX509KeyPair(security.ServerCertPath, security.ServerKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load Bank gRPC server certificate/key: %w", err)
	}

	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
	}), nil
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", s.port, err)
	}
	fmt.Printf("[Bank] gRPC server listening on :%s\n", s.port)
	return s.grpcServer.Serve(lis)
}

func (s *Server) Stop() {
	fmt.Println("[Bank] Shutting down gRPC server...")
	s.grpcServer.GracefulStop()
}
