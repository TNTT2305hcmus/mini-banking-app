package grpc

/**
 * @title KDC Service - gRPC Handler
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Implementing the KDCServiceServer interface and mapping gRPC requests to service logic.
 */

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"kdc-service/internal/kdc"
	pb "mini_banking/pkg/pb/kdc"
)

/**
 * @description Handler implements the gRPC KDCServiceServer interface.
 */
type Handler struct {
	pb.UnimplementedKDCServiceServer
	svc *kdc.Service
}

/**
 * @description NewHandler creates a new gRPC handler for the KDC Service.
 */
func NewHandler(svc *kdc.Service) *Handler {
	return &Handler{
		svc: svc,
	}
}

/**
 * @description RequestTGT handles Phase 2: AS Exchange.
 */
func (h *Handler) RequestTGT(ctx context.Context, req *pb.ASRequest) (*pb.ASResponse, error) {
	if req == nil ||
		req.OwnerId == "" ||
		req.CertSn == "" ||
		len(req.Nonce) == 0 ||
		req.Timestamp == 0 ||
		len(req.Signature) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	asRep, tgtExpiryUnix, err := h.svc.IssueTGT(ctx, req.OwnerId, req.CertSn, req.Nonce, req.Timestamp, req.Signature)
	if err != nil {
		fmt.Printf("[KDC] AS exchange failed for %s: %v\n", req.OwnerId, err)
		return nil, kdcErrorToStatus(err)
	}

	return &pb.ASResponse{
		AsRep:            asRep,
		TgtExpiresAtUnix: tgtExpiryUnix,
	}, nil
}

/**
 * @description RequestServiceTicket handles Phase 3: TGS Exchange.
 */
func (h *Handler) RequestServiceTicket(ctx context.Context, req *pb.TGSRequest) (*pb.TGSResponse, error) {
	if req == nil ||
		req.CertSn == "" ||
		req.ServiceId == "" ||
		len(req.Tgt) == 0 ||
		len(req.Authenticator) == 0 ||
		req.Scope == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	resp, err := h.svc.RequestServiceTicket(ctx, kdc.TGSRequest{
		CertSN:         req.CertSn,
		ServiceID:      req.ServiceId,
		TGTCiphertext:  req.Tgt,
		Authenticator:  req.Authenticator,
		RequestedScope: req.Scope,
	})
	if err != nil {
		fmt.Printf("[KDC] TGS exchange failed for service %s: %v\n", req.ServiceId, err)
		return nil, kdcErrorToStatus(err)
	}

	return &pb.TGSResponse{
		TgsRep:              resp.EncryptedPayload,
		TicketExpiresAtUnix: resp.TicketExpiryUnix,
		Scope:               req.Scope,
		ServiceId:           req.ServiceId,
	}, nil
}

func kdcErrorToStatus(err error) error {
	switch kdc.ErrorCodeOf(err) {
	case kdc.ErrAuthInvalid, kdc.ErrTGTExpired, kdc.ErrRequestExpired, kdc.ErrReplayDetected:
		return status.Error(codes.Unauthenticated, "invalid or expired authentication material")
	case kdc.ErrIdentityMismatch, kdc.ErrCertRevoked, kdc.ErrCertExpired, kdc.ErrScopeDenied:
		return status.Error(codes.PermissionDenied, "request is not authorized")
	case kdc.ErrCertNotFound, kdc.ErrServiceUnknown, kdc.ErrInvalidScope:
		return status.Error(codes.InvalidArgument, "invalid service ticket request")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
