package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/ca-service/internal/ca"
	pb "mini-banking/ca-service/internal/grpc/pb"
)

// Handler implement interface CAServiceServer được generate từ ca.proto.
// Nhiệm vụ duy nhất: map gRPC request → ca.Service → gRPC response.
// Không chứa business logic.
type Handler struct {
	pb.UnimplementedCAServiceServer // embed để forward-compatible khi thêm RPC mới
	svc                             *ca.Service
}

// NewHandler tạo gRPC handler với CA Service đã inject.
func NewHandler(svc *ca.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterUser nhận CSR PEM, ký và trả X.509 cert.
func (h *Handler) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	if req.CsrPem == "" {
		return nil, status.Error(codes.InvalidArgument, "csr_pem is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	certPEM, serialHex, notAfter, err := h.svc.RegisterUser(req.CsrPem, req.UserId)
	if err != nil {
		// Phân loại lỗi để trả đúng gRPC status code
		if isCSRSignatureError(err) {
			return nil, status.Errorf(codes.InvalidArgument, "CSR verification failed: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "register user: %v", err)
	}

	return &pb.RegisterUserResponse{
		CertificatePem: certPEM,
		SerialNumber:   serialHex,
		NotAfterUnix:   notAfter,
	}, nil
}

// GetCertificate trả về cert và trạng thái theo serial number.
func (h *Handler) GetCertificate(ctx context.Context, req *pb.GetCertificateRequest) (*pb.GetCertificateResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	certPEM, userID, certStatus, notAfter, err := h.svc.GetCertificate(req.SerialNumber)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		}
		return nil, status.Errorf(codes.Internal, "get certificate: %v", err)
	}

	return &pb.GetCertificateResponse{
		CertificatePem: certPEM,
		UserId:         userID,
		Status:         pb.CertStatus(certStatus),
		NotAfterUnix:   notAfter,
	}, nil
}

// CheckRevocation kiểm tra trạng thái revocation của cert.
// KDC gọi trong TGS Exchange, Bank gọi trước BEGIN TRANSACTION.
func (h *Handler) CheckRevocation(ctx context.Context, req *pb.CheckRevocationRequest) (*pb.CheckRevocationResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	certStatus, reason, revokedAt, err := h.svc.CheckRevocation(req.SerialNumber)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		}
		return nil, status.Errorf(codes.Internal, "check revocation: %v", err)
	}

	return &pb.CheckRevocationResponse{
		Status:    pb.CertStatus(certStatus),
		Reason:    reason,
		RevokedAt: revokedAt,
	}, nil
}

// RevokeCertificate thu hồi cert.
// Trả codes.NotFound nếu serial không tồn tại.
// Trả codes.AlreadyExists nếu cert đã bị revoke trước đó.
func (h *Handler) RevokeCertificate(ctx context.Context, req *pb.RevokeCertificateRequest) (*pb.RevokeCertificateResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	if err := h.svc.RevokeCertificate(req.SerialNumber, req.Reason); err != nil {
		switch {
		case strings.HasPrefix(err.Error(), "not_found"):
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		case strings.HasPrefix(err.Error(), "already_revoked"):
			return nil, status.Errorf(codes.AlreadyExists, "certificate already revoked: %s", req.SerialNumber)
		default:
			return nil, status.Errorf(codes.Internal, "revoke certificate: %v", err)
		}
	}

	return &pb.RevokeCertificateResponse{}, nil
}

// ── Error helpers ─────────────────────────────────────────────

func isCSRSignatureError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "signature") ||
		strings.Contains(msg, "verification failed") ||
		strings.Contains(msg, "invalid CSR")
}

func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "not found")
}
