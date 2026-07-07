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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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

	ctx = kdc.WithAuditMeta(ctx, traceID(ctx), callerIP(ctx))
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

	ctx = kdc.WithAuditMeta(ctx, traceID(ctx), callerIP(ctx))
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

/**
 * @description ListAuditEvents serves the admin dashboard read path for the
 * key-issuance audit log. It bypasses the AP exchange on purpose: the RPC is
 * only reachable on the internal TLS network and admin authn/authz is enforced
 * by the API Gateway before forwarding.
 */
func (h *Handler) ListAuditEvents(ctx context.Context, req *pb.ListAuditEventsRequest) (*pb.ListAuditEventsResponse, error) {
	filter := kdc.AuditFilter{
		Action:     req.GetAction(),
		ClientID:   req.GetClientId(),
		CertSerial: req.GetCertSerial(),
		RequestID:  req.GetRequestId(),
		Limit:      int(req.GetLimit()),
		Offset:     int(req.GetOffset()),
	}
	if req.GetFromUnix() > 0 {
		filter.From = time.Unix(req.GetFromUnix(), 0).UTC()
	}
	if req.GetToUnix() > 0 {
		filter.To = time.Unix(req.GetToUnix(), 0).UTC()
	}
	records, total, err := h.svc.ListAuditEvents(ctx, filter)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	events := make([]*pb.KdcAuditRecord, 0, len(records))
	for _, rec := range records {
		events = append(events, &pb.KdcAuditRecord{
			Id:            rec.ID,
			Action:        rec.Action,
			ClientId:      rec.ClientID,
			CertSerial:    rec.CertSerial,
			Scope:         rec.Scope,
			Reason:        rec.Reason,
			RequestId:     rec.RequestID,
			Ip:            rec.IP,
			MetadataJson:  rec.MetadataJSON,
			CreatedAtUnix: rec.CreatedAt.Unix(),
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
	return &pb.ListAuditEventsResponse{Events: events, Total: int32(total), Limit: limit, Offset: offset}, nil
}

/**
 * @description VerifyAuditChain replays the KDC audit hash chain and reports the
 * first tampering. Admin read path, guarded at the gateway (internal TLS only).
 */
func (h *Handler) VerifyAuditChain(ctx context.Context, _ *pb.VerifyAuditChainRequest) (*pb.VerifyAuditChainResponse, error) {
	result, err := h.svc.VerifyAuditChain(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "verify audit chain failed")
	}
	return &pb.VerifyAuditChainResponse{
		Ok:        result.OK,
		Checked:   int32(result.Checked),
		BrokenSeq: result.BrokenSeq,
		BrokenId:  result.BrokenID,
		Detail:    result.Detail,
	}, nil
}

// traceID reads the gateway trace id from gRPC metadata "x-request-id" so the
// KDC audit trail correlates with the same request across services.
func traceID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get("x-request-id"); len(v) > 0 {
		return v[0]
	}
	return ""
}

// callerIP returns the caller IP for the audit trail, preferring the
// gateway-forwarded "x-forwarded-for" header over the direct peer address.
func callerIP(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-forwarded-for"); len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return p.Addr.String()
	}
	return ""
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
