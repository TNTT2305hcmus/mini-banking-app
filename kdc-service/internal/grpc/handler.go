package grpc

/**
 * @title KDC Service - gRPC Handler
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Implementing the KDCServiceServer interface and mapping gRPC requests to service logic.
 */

import (
	"context"
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
	if req.ClientId == "" || req.CertSn == "" || len(req.PreAuthSignature) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	// @note 1.5. Replay Attack Prevention - Check Nonce in Redis
	if err := h.svc.CheckAndStoreNonce(ctx, req.Nonce1); err != nil {
		fmt.Printf("[KDC] Replay attack or Redis error for %s: %v\n", req.ClientId, err)
		return nil, status.Error(codes.PermissionDenied, "invalid nonce or replay attack detected")
	}

	// @note 2. Prepare data to verify (ID_c || ID_tgs || Nonce1 || TS)
	// @note This format must match the Client's signing logic.
	dataToVerify := []byte(fmt.Sprintf("%s|%s|%x|%d",
		req.ClientId, req.TgsId, req.Nonce1, req.Timestamp))

	// @note 3. Call Service to verify Pre-Authentication
	err := h.svc.VerifyPreAuthSignature(ctx, req.CertSn, req.PreAuthSignature, dataToVerify)
	if err != nil {
		// @note Log error and return PermissionDenied or Unauthenticated
		fmt.Printf("[KDC] Pre-auth failed for %s: %v\n", req.ClientId, err)
		return nil, status.Error(codes.Unauthenticated, "pre-authentication failed")
	}

	// @note 4. Generate Session Key K_{c,tgs}
	sessionKey, err := h.svc.GenerateSessionKey()
	if err != nil {
		fmt.Printf("[KDC] Failed to generate session key for %s: %v\n", req.ClientId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	fmt.Println(sessionKey)

	// @todo 5. Generate TGT (Ticket-Granting Ticket)
	tgt, err := h.svc.GenerateEncryptedTGT(req.ClientId, sessionKey)
	if err != nil {
		fmt.Printf("[KDC] Failed to generate TGT for %s: %v\n", req.ClientId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	fmt.Printf("TGT generated for %s", req.ClientId)

	// @todo 6. Build AS_REP (Encrypted with Client PubKey)
	as_rep, err := h.svc.BuildAS_REP(ctx, sessionKey, tgt, req.Nonce1, req.CertSn)
	if err != nil {
		fmt.Printf("[KDC] Failed to sign TGT for %s: %v\n", req.ClientId, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &pb.ASResponse{
		EncryptedPayload: as_rep,
		TgtExpiryUnix:    time.Now().Add(time.Duration(ENV.LoadEnv().TGTExp) * time.Minute).Unix(),
	}, nil
}

/**
 * @description RequestServiceTicket handles Phase 3: TGS Exchange.
 */
func (h *Handler) RequestServiceTicket(ctx context.Context, req *pb.TGSRequest) (*pb.TGSResponse, error) {
	// @todo Implement TGS Exchange logic
	return nil, status.Error(codes.Unimplemented, "method RequestServiceTicket not implemented")
}
