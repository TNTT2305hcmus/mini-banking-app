package grpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "mini-banking/pkg/pb/bank"
)

type Server struct {
	grpcServer *grpc.Server
	port       string
}

func NewServer(handler *Handler, port string) *Server {
	grpcSrv := grpc.NewServer()

	pb.RegisterBankServiceServer(grpcSrv, handler)

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("bank.BankService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(grpcSrv)

	return &Server{grpcServer: grpcSrv, port: port}
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
