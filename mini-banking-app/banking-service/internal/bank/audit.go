package bank

import "context"

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
	_ = s.repo.InsertAudit(ctx, s.repo.db, e)
}
