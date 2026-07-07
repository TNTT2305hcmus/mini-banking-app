package kdc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AuditAction mirrors the CHECK constraint on kdc_audit_log.action. Adding a
// value here requires an ALTER of that constraint in a DB migration first.
type AuditAction string

const (
	AuditASTicketIssued  AuditAction = "as_ticket_issued"
	AuditASRejected      AuditAction = "as_rejected"
	AuditTGSTicketIssued AuditAction = "tgs_ticket_issued"
	AuditTGSRejected     AuditAction = "tgs_rejected"
)

// AuditEvent is a key-issuance event recorded to kdc_audit_log. It is written on
// a best-effort basis by the AS/TGS flows so auditing never breaks ticket issuance.
type AuditEvent struct {
	Action     AuditAction
	ClientID   string
	CertSerial string
	Scope      string
	Reason     string
	RequestID  string
	IP         string
	Metadata   map[string]any
	CreatedAt  time.Time
}

// AuditRecord is a stored kdc_audit_log row returned to the admin read API.
// Nullable columns come back as empty strings; MetadataJSON is the raw JSONB text.
type AuditRecord struct {
	ID           string
	Action       string
	ClientID     string
	CertSerial   string
	Scope        string
	Reason       string
	RequestID    string
	IP           string
	MetadataJSON string
	CreatedAt    time.Time
}

// AuditFilter selects rows for the admin read API. Zero values mean "no filter";
// From/To bound created_at as a half-open range [From, To).
type AuditFilter struct {
	Action     string
	ClientID   string
	CertSerial string
	RequestID  string
	From       time.Time
	To         time.Time
	Limit      int
	Offset     int
}

// AuditRepository owns all SQL for kdc_audit_log. A nil repository or nil db
// makes every operation a no-op, so the KDC still runs when DATABASE_URL is
// not configured (audit is best-effort infrastructure, not a hard dependency).
type AuditRepository struct {
	db *sql.DB
}

func NewAuditRepository(db *sql.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) InsertAudit(ctx context.Context, e AuditEvent) error {
	if r == nil || r.db == nil || e.Action == "" {
		return nil
	}
	metadata := []byte("{}")
	if len(e.Metadata) > 0 {
		if b, err := json.Marshal(e.Metadata); err == nil {
			metadata = b
		}
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO kdc_audit_log(action, client_id, cert_serial, scope, reason, request_id, ip, metadata, created_at)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9)`,
		string(e.Action), e.ClientID, e.CertSerial, e.Scope, e.Reason, e.RequestID, e.IP, string(metadata), created.UTC())
	return err
}

func (r *AuditRepository) ListAudit(ctx context.Context, f AuditFilter) ([]AuditRecord, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	where := ` WHERE ($1 = '' OR action = $1)
	   AND ($2 = '' OR client_id = $2)
	   AND ($3 = '' OR cert_serial = $3)
	   AND ($4 = '' OR request_id = $4)
	   AND ($5::timestamptz IS NULL OR created_at >= $5)
	   AND ($6::timestamptz IS NULL OR created_at < $6)`
	args := []any{f.Action, f.ClientID, f.CertSerial, f.RequestID, nullableTime(f.From), nullableTime(f.To)}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kdc_audit_log`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id::text, action, COALESCE(client_id, ''), COALESCE(cert_serial, ''), COALESCE(scope, ''),
		        COALESCE(reason, ''), COALESCE(request_id, ''), COALESCE(ip, ''), COALESCE(metadata::text, '{}'), created_at
		 FROM kdc_audit_log`+where+`
		 ORDER BY created_at DESC
		 LIMIT $7 OFFSET $8`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []AuditRecord
	for rows.Next() {
		var rec AuditRecord
		if err := rows.Scan(&rec.ID, &rec.Action, &rec.ClientID, &rec.CertSerial, &rec.Scope,
			&rec.Reason, &rec.RequestID, &rec.IP, &rec.MetadataJSON, &rec.CreatedAt); err != nil {
			return nil, 0, err
		}
		rec.CreatedAt = rec.CreatedAt.UTC()
		records = append(records, rec)
	}
	return records, total, rows.Err()
}

func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

// Audit records an event best-effort: storage failures are logged but never
// propagated, so auditing cannot break AS/TGS ticket issuance.
func (s *Service) Audit(ctx context.Context, e AuditEvent) {
	if s == nil || s.auditRepo == nil {
		return
	}
	if err := s.auditRepo.InsertAudit(ctx, e); err != nil {
		fmt.Printf("[KDC] warning: cannot insert audit event action=%s request_id=%s: %v\n", e.Action, e.RequestID, err)
	}
}

// ListAuditEvents is the read side of kdc_audit_log for the admin dashboard.
// It is read-only and never records an audit event itself.
func (s *Service) ListAuditEvents(ctx context.Context, f AuditFilter) ([]AuditRecord, int, error) {
	if s == nil || s.auditRepo == nil {
		return nil, 0, nil
	}
	f.Action = strings.TrimSpace(strings.ToLower(f.Action))
	switch f.Action {
	case "", string(AuditASTicketIssued), string(AuditASRejected),
		string(AuditTGSTicketIssued), string(AuditTGSRejected):
	default:
		return nil, 0, fmt.Errorf("unsupported audit action %q", f.Action)
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.To.Before(f.From) {
		return nil, 0, fmt.Errorf("audit filter to must not be before from")
	}
	return s.auditRepo.ListAudit(ctx, f)
}
