package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/ca-service/internal/ca"
	pb "mini_banking/pkg/pb/ca"
)

type Handler struct {
	pb.UnimplementedCAServiceServer
	svc *ca.Service
}

func NewHandler(svc *ca.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	record, err := h.svc.RegisterUser(ctx, ca.RegisterInput{
		CSRPem:  req.GetCsrPem(),
		OwnerID: req.GetOwnerId(),
	})
	if err != nil {
		return nil, toStatusError("register user", err)
	}
	return &pb.RegisterUserResponse{
		CertificatePem:    record.CertificatePEM,
		SerialNumber:      record.SerialNumber,
		NotBeforeUnix:     record.NotBefore.Unix(),
		NotAfterUnix:      record.NotAfter.Unix(),
		FingerprintSha256: record.FingerprintSHA256,
	}, nil
}

func (h *Handler) VerifyCertificate(ctx context.Context, req *pb.VerifyCertificateRequest) (*pb.VerifyCertificateResponse, error) {
	record, err := h.svc.VerifyCertificate(ctx, ca.VerifyInput{
		SerialNumber:          req.GetSerialNumber(),
		Caller:                req.GetCaller(),
		IncludeCertificatePEM: req.GetIncludeCertificatePem(),
		IncludePublicKeyPEM:   req.GetIncludePublicKeyPem(),
	})
	if err != nil {
		return nil, toStatusError("verify certificate", err)
	}

	resp := &pb.VerifyCertificateResponse{
		Status:            toProtoStatus(record.Status),
		OwnerId:           record.OwnerID,
		FingerprintSha256: record.FingerprintSHA256,
		NotBeforeUnix:     record.NotBefore.Unix(),
		NotAfterUnix:      record.NotAfter.Unix(),
		RevocationReason:  record.RevocationReason,
	}
	if record.RevokedAt != nil {
		resp.RevokedAtUnix = record.RevokedAt.Unix()
	}
	if req.GetIncludeCertificatePem() {
		resp.CertificatePem = record.CertificatePEM
	}
	if req.GetIncludePublicKeyPem() {
		resp.PublicKeyPem = record.PublicKeyPEM
	}
	return resp, nil
}

func (h *Handler) GetCertificate(ctx context.Context, req *pb.GetCertificateRequest) (*pb.GetCertificateResponse, error) {
	record, err := h.svc.GetCertificate(ctx, req.GetSerialNumber())
	if err != nil {
		return nil, toStatusError("get certificate", err)
	}
	return &pb.GetCertificateResponse{
		CertificatePem: record.CertificatePEM,
		UserId:         record.OwnerID,
		Status:         toProtoStatus(record.Status),
		NotAfterUnix:   record.NotAfter.Unix(),
		PublicKeyPem:   record.PublicKeyPEM,
	}, nil
}

func (h *Handler) CheckRevocation(ctx context.Context, req *pb.CheckRevocationRequest) (*pb.CheckRevocationResponse, error) {
	record, err := h.svc.CheckRevocation(ctx, req.GetSerialNumber())
	if err != nil {
		return nil, toStatusError("check revocation", err)
	}
	resp := &pb.CheckRevocationResponse{
		Status: toProtoStatus(record.Status),
		Reason: record.RevocationReason,
	}
	if record.RevokedAt != nil {
		resp.RevokedAt = record.RevokedAt.Unix()
	}
	return resp, nil
}

func (h *Handler) ListCertificates(ctx context.Context, req *pb.ListCertificatesRequest) (*pb.ListCertificatesResponse, error) {
	records, total, err := h.svc.ListCertificates(ctx, ca.ListFilter{
		Status:       req.GetStatus(),
		Email:        req.GetEmail(),
		SerialNumber: req.GetSerialNumber(),
		Limit:        int(req.GetLimit()),
		Offset:       int(req.GetOffset()),
		PerformedBy:  req.GetPerformedBy(),
	})
	if err != nil {
		return nil, toStatusError("list certificates", err)
	}
	certificates := make([]*pb.CertificateMetadata, 0, len(records))
	for i := range records {
		certificates = append(certificates, toProtoMetadata(records[i]))
	}
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return &pb.ListCertificatesResponse{
		Certificates: certificates,
		Total:        int32(total),
		Limit:        limit,
		Offset:       req.GetOffset(),
	}, nil
}

func (h *Handler) GetCertificateDetail(ctx context.Context, req *pb.GetCertificateDetailRequest) (*pb.GetCertificateDetailResponse, error) {
	record, err := h.svc.GetCertificateDetail(ctx, req.GetSerialNumber(), "", req.GetPerformedBy())
	if err != nil {
		return nil, toStatusError("get certificate detail", err)
	}
	return &pb.GetCertificateDetailResponse{Certificate: toProtoMetadata(*record)}, nil
}

func (h *Handler) RevokeCertificate(ctx context.Context, req *pb.RevokeCertificateRequest) (*pb.RevokeCertificateResponse, error) {
	record, err := h.svc.RevokeCertificate(ctx, req.GetSerialNumber(), req.GetReason(), "", req.GetPerformedBy())
	if err != nil {
		return nil, toStatusError("revoke certificate", err)
	}
	return &pb.RevokeCertificateResponse{Certificate: toProtoMetadata(*record)}, nil
}

func toProtoMetadata(record ca.CertificateRecord) *pb.CertificateMetadata {
	out := &pb.CertificateMetadata{
		SerialNumber:      record.SerialNumber,
		OwnerId:           record.OwnerID,
		SubjectCn:         record.SubjectCN,
		SubjectEmail:      record.SubjectEmail,
		FingerprintSha256: record.FingerprintSHA256,
		Status:            toProtoStatus(record.Status),
		NotBeforeUnix:     record.NotBefore.Unix(),
		NotAfterUnix:      record.NotAfter.Unix(),
		IssuedAtUnix:      record.IssuedAt.Unix(),
		RevocationReason:  record.RevocationReason,
	}
	if record.RevokedAt != nil {
		out.RevokedAtUnix = record.RevokedAt.Unix()
	}
	return out
}

func toProtoStatus(value ca.CertStatus) pb.CertStatus {
	switch value {
	case ca.CertStatusActive:
		return pb.CertStatus_CERT_STATUS_ACTIVE
	case ca.CertStatusRevoked:
		return pb.CertStatus_CERT_STATUS_REVOKED
	case ca.CertStatusExpired:
		return pb.CertStatus_CERT_STATUS_EXPIRED
	default:
		return pb.CertStatus_CERT_STATUS_UNKNOWN
	}
}

func toStatusError(operation string, err error) error {
	switch {
	case errors.Is(err, ca.ErrInvalidInput):
		return status.Errorf(codes.InvalidArgument, "%s: %v", operation, err)
	case errors.Is(err, ca.ErrInvalidCSR):
		return status.Errorf(codes.InvalidArgument, "%s: %v", operation, err)
	case errors.Is(err, ca.ErrCSRIdentityMismatch):
		return status.Errorf(codes.InvalidArgument, "%s: %v", operation, err)
	case errors.Is(err, ca.ErrActiveCertificateExists):
		return status.Errorf(codes.AlreadyExists, "%s: %v", operation, err)
	case errors.Is(err, ca.ErrCertificateNotFound):
		return status.Errorf(codes.NotFound, "%s: %v", operation, err)
	case errors.Is(err, ca.ErrAlreadyRevoked):
		return status.Errorf(codes.AlreadyExists, "%s: %v", operation, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}
