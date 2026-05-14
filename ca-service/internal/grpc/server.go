package grpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "mini-banking/ca-service/internal/grpc/pb"
)

// Server wraps grpc.Server với cấu hình cho CA Service.
type Server struct {
	grpcServer *grpc.Server
	port       string
}

// NewServer tạo gRPC server với:
//   - CAService handler
//   - Health check (grpc.health.v1) — dùng cho Docker healthcheck và service mesh
//   - Reflection — cho phép dùng grpcurl để debug mà không cần proto file
func NewServer(handler *Handler, port string) *Server {
	grpcSrv := grpc.NewServer(
	// Interceptor có thể thêm ở đây sau:
	// grpc.UnaryInterceptor(loggingInterceptor),
	)

	// Đăng ký CA Service
	pb.RegisterCAServiceServer(grpcSrv, handler)

	// Health check — các service khác (KDC, Bank) dùng để check CA ready chưa
	// Trả SERVING khi CA đang hoạt động bình thường
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("ca.CAService", grpc_health_v1.HealthCheckResponse_SERVING)
	// "" = overall health của toàn server
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Reflection cho phép:
	// grpcurl -plaintext localhost:50051 list
	// grpcurl -plaintext localhost:50051 ca.CAService/CheckRevocation
	reflection.Register(grpcSrv)

	return &Server{
		grpcServer: grpcSrv,
		port:       port,
	}
}

// Start lắng nghe và serve. Block cho đến khi có lỗi.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", s.port, err)
	}

	fmt.Printf("[CA] gRPC server listening on :%s\n", s.port)
	return s.grpcServer.Serve(lis)
}

// Stop graceful shutdown.
func (s *Server) Stop() {
	fmt.Println("[CA] Shutting down gRPC server...")
	s.grpcServer.GracefulStop()
}
