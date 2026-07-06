package bank

import (
	"context"
	"fmt"
)

// auditActions mirrors the CHECK constraint on bank_audit_log.action. Adding a
// value here requires an ALTER of that constraint in a DB migration first.
var auditActions = map[string]bool{
	"transfer_completed":   true,
	"transfer_rejected":    true,
	"replay_detected":      true,
	"invalid_signature":    true,
	"certificate_rejected": true,
	"forbidden_ownership":  true,
	"insufficient_funds":   true,
}

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

// Audit records an event on a best-effort basis; storage failures are swallowed
// so auditing never breaks the request path.
func (s *Service) Audit(ctx context.Context, e AuditEvent) {
	if s.repo == nil || s.repo.db == nil || e.Action == "" {
		return
	}
	if err := s.repo.InsertAudit(ctx, s.repo.db, e); err != nil {
		fmt.Printf("[BANK] warning: cannot insert audit event action=%s request_id=%s: %v\n", e.Action, e.RequestID, err)
	}
}

// ListAuditEvents is the read side of bank_audit_log for the admin dashboard.
// It is read-only and never records an audit event itself.
func (s *Service) ListAuditEvents(ctx context.Context, f AuditListFilter) ([]AuditRecord, int, error) {
	if s.repo == nil || s.repo.db == nil {
		return nil, 0, ErrNotConfigured
	}
	if f.Action != "" && !auditActions[f.Action] {
		return nil, 0, fmt.Errorf("%w: unsupported audit action %q", ErrBadRequest, f.Action)
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.To.Before(f.From) {
		return nil, 0, fmt.Errorf("%w: audit filter to must not be before from", ErrBadRequest)
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return s.repo.ListAudit(ctx, s.repo.db, f)
}
