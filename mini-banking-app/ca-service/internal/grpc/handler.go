package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

// requestIDFromContext reads the gateway trace id from gRPC metadata key
// "x-request-id". The trace id is a transport concern, so it never appears as
// a proto message field; audit writers stash it in the event metadata.
func requestIDFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-request-id")
	if len(values) == 0 {
		values = md.Get("request-id")
	}
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (h *Handler) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	record, err := h.svc.RegisterUser(ctx, ca.RegisterInput{
		CSRPem:       req.GetCsrPem(),
		OwnerID:      req.GetOwnerId(),
		SubjectCN:    req.GetFullName(),
		SubjectEmail: req.GetSubjectEmail(),
		Role:         toDomainRole(req.GetRole()),
		RequestID:    requestIDFromContext(ctx),
	})
	if err != nil {
		return nil, toStatusError("register user", err)
	}
	return &pb.RegisterUserResponse{
		CertificatePem:     record.CertificatePEM,
		SerialNumber:       record.SerialNumber,
		NotBeforeUnix:      record.NotBefore.Unix(),
		NotAfterUnix:       record.NotAfter.Unix(),
		FingerprintSha256:  record.FingerprintSHA256,
		CertType:           record.CertType,
		IssuerId:           record.IssuerID,
		IssuerCommonName:   record.IssuerCommonName,
		IssuerSerialNumber: record.IssuerSerial,
		ChainPem:           record.ChainPEM,
		ChainFingerprints:  cloneStrings(record.ChainFingerprints),
	}, nil
}

func (h *Handler) VerifyCertificate(ctx context.Context, req *pb.VerifyCertificateRequest) (*pb.VerifyCertificateResponse, error) {
	record, err := h.svc.VerifyCertificate(ctx, ca.VerifyInput{
		SerialNumber:          req.GetSerialNumber(),
		Caller:                req.GetCaller(),
		IncludeCertificatePEM: req.GetIncludeCertificatePem(),
		IncludePublicKeyPEM:   req.GetIncludePublicKeyPem(),
		RequestID:             requestIDFromContext(ctx),
	})
	if err != nil {
		return nil, toStatusError("verify certificate", err)
	}

	resp := &pb.VerifyCertificateResponse{
		Status:             toProtoStatus(record.Status),
		OwnerId:            record.OwnerID,
		SubjectEmail:       record.SubjectEmail,
		FingerprintSha256:  record.FingerprintSHA256,
		NotBeforeUnix:      record.NotBefore.Unix(),
		NotAfterUnix:       record.NotAfter.Unix(),
		RevocationReason:   record.RevocationReason,
		Role:               toProtoRole(record.Role),
		CertType:           record.CertType,
		IssuerId:           record.IssuerID,
		IssuerCommonName:   record.IssuerCommonName,
		IssuerSerialNumber: record.IssuerSerial,
		ChainPem:           record.ChainPEM,
		ChainFingerprints:  cloneStrings(record.ChainFingerprints),
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
		OwnerId:        record.OwnerID,
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
		CertType:     req.GetCertType(),
		IssuerID:     req.GetIssuerId(),
		OwnerID:      req.GetOwnerId(),
		SubjectEmail: req.GetSubjectEmail(),
		SerialNumber: req.GetSerialNumber(),
		Limit:        int(req.GetLimit()),
		Offset:       int(req.GetOffset()),
		RequestID:    requestIDFromContext(ctx),
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
	record, err := h.svc.GetCertificateDetail(ctx, req.GetSerialNumber(), requestIDFromContext(ctx), req.GetPerformedBy())
	if err != nil {
		return nil, toStatusError("get certificate detail", err)
	}
	return &pb.GetCertificateDetailResponse{Certificate: toProtoMetadata(*record)}, nil
}

func (h *Handler) RevokeCertificate(ctx context.Context, req *pb.RevokeCertificateRequest) (*pb.RevokeCertificateResponse, error) {
	record, err := h.svc.RevokeCertificate(ctx, req.GetSerialNumber(), req.GetReason(), requestIDFromContext(ctx), req.GetPerformedBy())
	if err != nil {
		return nil, toStatusError("revoke certificate", err)
	}
	return &pb.RevokeCertificateResponse{Certificate: toProtoMetadata(*record)}, nil
}

func (h *Handler) ListAuditEvents(ctx context.Context, req *pb.ListAuditEventsRequest) (*pb.ListAuditEventsResponse, error) {
	filter := ca.AuditFilter{
		SerialNumber: req.GetSerialNumber(),
		Action:       req.GetAction(),
		PerformedBy:  req.GetPerformedByFilter(),
		Limit:        int(req.GetLimit()),
		Offset:       int(req.GetOffset()),
	}
	if req.GetFromUnix() > 0 {
		filter.From = time.Unix(req.GetFromUnix(), 0).UTC()
	}
	if req.GetToUnix() > 0 {
		filter.To = time.Unix(req.GetToUnix(), 0).UTC()
	}
	events, total, err := h.svc.ListAuditEvents(ctx, filter)
	if err != nil {
		return nil, toStatusError("list audit events", err)
	}
	records := make([]*pb.AuditEventRecord, 0, len(events))
	for _, event := range events {
		records = append(records, &pb.AuditEventRecord{
			SerialNumber:    event.SerialNumber,
			Action:          string(event.Action),
			PerformedBy:     event.PerformedBy,
			Reason:          event.Reason,
			PerformedAtUnix: event.PerformedAt.Unix(),
			Metadata:        event.Metadata,
			CertType:        event.CertType,
			IssuerId:        event.IssuerID,
		})
	}
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.GetOffset()
	if offset < 0 {
		offset = 0
	}
	return &pb.ListAuditEventsResponse{
		Events: records,
		Total:  int32(total),
		Limit:  limit,
		Offset: offset,
	}, nil
}

// AppendAuditEvent records a Registration Authority / admin-auth event submitted
// by the API Gateway. The service layer whitelists the action so lifecycle
// events (issued/revoked/...) cannot be forged through this RPC.
func (h *Handler) AppendAuditEvent(ctx context.Context, req *pb.AppendAuditEventRequest) (*pb.AppendAuditEventResponse, error) {
	err := h.svc.AppendExternalAudit(ctx, ca.AuditEvent{
		Action:       ca.AuditAction(req.GetAction()),
		SerialNumber: req.GetSerialNumber(),
		PerformedBy:  req.GetPerformedBy(),
		Reason:       req.GetReason(),
		Metadata:     req.GetMetadata(),
	})
	if err != nil {
		return nil, toStatusError("append audit event", err)
	}
	return &pb.AppendAuditEventResponse{Recorded: true}, nil
}

func toProtoMetadata(record ca.CertificateRecord) *pb.CertificateMetadata {
	out := &pb.CertificateMetadata{
		SerialNumber:       record.SerialNumber,
		OwnerId:            record.OwnerID,
		SubjectCn:          record.SubjectCN,
		SubjectEmail:       record.SubjectEmail,
		FingerprintSha256:  record.FingerprintSHA256,
		Status:             toProtoStatus(record.Status),
		NotBeforeUnix:      record.NotBefore.Unix(),
		NotAfterUnix:       record.NotAfter.Unix(),
		IssuedAtUnix:       record.IssuedAt.Unix(),
		RevocationReason:   record.RevocationReason,
		CertType:           record.CertType,
		IssuerId:           record.IssuerID,
		IssuerCommonName:   record.IssuerCommonName,
		IssuerSerialNumber: record.IssuerSerial,
		ChainPem:           record.ChainPEM,
		ChainFingerprints:  cloneStrings(record.ChainFingerprints),
		IsCa:               record.IsCA,
		KeyUsage:           cloneStrings(record.KeyUsage),
		ExtendedKeyUsage:   cloneStrings(record.ExtendedKeyUsage),
	}
	if record.RevokedAt != nil {
		out.RevokedAtUnix = record.RevokedAt.Unix()
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
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

func toDomainRole(value pb.IdentityRole) ca.IdentityRole {
	switch value {
	case pb.IdentityRole_IDENTITY_ROLE_CUSTOMER:
		return ca.IdentityRoleCustomer
	case pb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN:
		return ca.IdentityRoleBankAdmin
	default:
		return ca.IdentityRoleUnknown
	}
}

func toProtoRole(value ca.IdentityRole) pb.IdentityRole {
	if value == ca.IdentityRoleBankAdmin {
		return pb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN
	}
	return pb.IdentityRole_IDENTITY_ROLE_CUSTOMER
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
	case errors.Is(err, ca.ErrCertificateNotRevokable):
		return status.Errorf(codes.FailedPrecondition, "%s: %v", operation, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}
