package bank

import (
	"context"
	"fmt"
)

// AuditEvent is a security/business event recorded to bank_audit_log. It is used
// by both the gRPC handler (auth-layer events) and the service (business events).
type AuditEvent struct {
	Action        string
	UserID        string
	AccountID     string
	TransactionID string
	CertSerial    string
	RequestID     string
	Reason        string
	Metadata      map[string]any
}

type auditCtxKey int

const traceIDKey auditCtxKey = iota

// WithTraceID stores the gateway HTTP trace id (X-Request-ID) in the context so
// every audit event written below the handler can correlate with the same
// request across CA/KDC/Bank, even when the AP authenticator has not been
// decrypted yet and no protocol request_id exists.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

func traceIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey).(string)
	return v
}

// Audit records an event on a best-effort basis; storage failures are swallowed
// so auditing never breaks the request path.
func (s *Service) Audit(ctx context.Context, e AuditEvent) {
	if s.repo == nil || s.repo.db == nil || e.Action == "" {
		return
	}
	// Auth-layer failures that happen before the authenticator is decrypted have
	// no AP request_id; fall back to the gateway trace id so the SOC timeline can
	// still reach these rows. Events that do carry the AP request_id keep it and
	// expose the trace id via metadata instead.
	if trace := traceIDFrom(ctx); trace != "" {
		if e.RequestID == "" {
			e.RequestID = trace
		}
		if e.Metadata == nil {
			e.Metadata = map[string]any{}
		}
		if _, ok := e.Metadata["trace_id"]; !ok {
			e.Metadata["trace_id"] = trace
		}
	}
	if err := s.repo.InsertAudit(ctx, e); err != nil {
		fmt.Printf("[BANK] warning: cannot insert audit event action=%s request_id=%s: %v\n", e.Action, e.RequestID, err)
	}
}

// VerifyAuditChain replays the bank audit hash chain and reports the first tampering.
func (s *Service) VerifyAuditChain(ctx context.Context) (ChainVerification, error) {
	if s.repo == nil || s.repo.db == nil {
		return ChainVerification{OK: true}, nil
	}
	return s.repo.VerifyAuditChain(ctx)
}
