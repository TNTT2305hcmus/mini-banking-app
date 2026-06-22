package grpc

/**
 * @title KDC Service - gRPC Handler
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Implementing the KDCServiceServer interface and mapping gRPC requests to service logic.
 */

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ENV "kdc-service/internal/config"
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
	// @note 1. Basic validation
	if req == nil ||
		req.OwnerId == "" ||
		req.CertSn == "" ||
		len(req.Nonce) == 0 ||
		req.Timestamp == 0 ||
		len(req.Signature) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	if !isFreshTimestamp(req.Timestamp, 5*time.Minute) {
		return nil, status.Error(codes.Unauthenticated, "stale request")
	}

	// @note 1.5. Replay Attack Prevention - Check request identity in Redis
	if err := h.svc.CheckAndStoreASReplay(ctx, req.OwnerId, req.Nonce, req.Timestamp); err != nil {
		fmt.Printf("[KDC] Replay attack or Redis error for %s: %v\n", req.OwnerId, err)
		return nil, status.Error(codes.PermissionDenied, "invalid nonce or replay attack detected")
	}

	dataToVerify, err := buildASCanonicalPayload(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid AS request")
	}

	// @note 3. Call Service to verify Pre-Authentication
	err = h.svc.VerifyPreAuthSignature(ctx, req.CertSn, req.Signature, dataToVerify)
	if err != nil {
		// @note Log error and return PermissionDenied or Unauthenticated
		fmt.Printf("[KDC] Pre-auth failed for %s: %v\n", req.OwnerId, err)
		return nil, status.Error(codes.Unauthenticated, "pre-authentication failed")
	}

	// @note 4. Generate Session Key K_{c,tgs}
	sessionKey, err := h.svc.GenerateSessionKey()
	if err != nil {
		fmt.Printf("[KDC] Failed to generate session key for %s: %v\n", req.OwnerId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	fmt.Printf("[KDC] Generated session key for %s\n", req.OwnerId)

	// @todo 5. Generate TGT (Ticket-Granting Ticket)
	tgt, err := h.svc.GenerateEncryptedTGT(req.OwnerId, sessionKey, req.CertSn)
	if err != nil {
		fmt.Printf("[KDC] Failed to generate TGT for %s: %v\n", req.OwnerId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	fmt.Printf("[KDC] TGT generated for %s\n", req.OwnerId)

	// @todo 6. Build AS_REP (Encrypted with Client PubKey)
	asRep, err := h.svc.BuildAS_REP(ctx, sessionKey, tgt, req.Nonce, req.CertSn)
	if err != nil {
		fmt.Printf("[KDC] Failed to sign TGT for %s: %v\n", req.OwnerId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &pb.ASResponse{
		AsRep:            asRep,
		TgtExpiresAtUnix: time.Now().Add(ENV.LoadEnv().TGTExp).Unix(),
	}, nil
}

type asCanonicalPayload struct {
	CertSn    string `json:"cert_sn"`
	OwnerId   string `json:"owner_id"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

func buildASCanonicalPayload(req *pb.ASRequest) ([]byte, error) {
	return json.Marshal(asCanonicalPayload{
		CertSn:    req.CertSn,
		OwnerId:   req.OwnerId,
		Nonce:     base64.StdEncoding.EncodeToString(req.Nonce),
		Timestamp: req.Timestamp,
	})
}

func isFreshTimestamp(ts int64, window time.Duration) bool {
	now := time.Now().UTC()
	delta := now.Sub(time.Unix(ts, 0).UTC())
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
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
