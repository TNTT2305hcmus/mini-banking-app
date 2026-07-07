package kdc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// auditGenesis seeds the hash chain; auditChainHash links each event to the
// previous one. Fields must be hashed and re-verified in the same order.
const auditGenesis = "genesis"

// kdcAuditLockKey serializes concurrent audit inserts (via a Postgres advisory
// lock) so the chain reads a consistent previous hash.
const kdcAuditLockKey int64 = 0x6b64635f61756469 // "kdc_audi"

func auditChainHash(prev string, fields ...string) string {
	h := sha256.New()
	h.Write([]byte(prev))
	for _, f := range fields {
		h.Write([]byte{0x1f})
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// auditHashFields is the exact, ordered field set covered by the chain hash for
// one event (excludes timestamp/metadata, which do not round-trip byte-stable).
func auditHashFields(action, clientID, certSerial, scope, reason, requestID string) []string {
	return []string{action, clientID, certSerial, scope, reason, requestID}
}

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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize chain appends so the previous hash is read consistently.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, kdcAuditLockKey); err != nil {
		return err
	}
	var prev sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT hash FROM kdc_audit_log ORDER BY seq DESC LIMIT 1`).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	prevHash := auditGenesis
	if prev.Valid && prev.String != "" {
		prevHash = prev.String
	}
	hash := auditChainHash(prevHash, auditHashFields(string(e.Action), e.ClientID, e.CertSerial, e.Scope, e.Reason, e.RequestID)...)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO kdc_audit_log(action, client_id, cert_serial, scope, reason, request_id, ip, metadata, created_at, prev_hash, hash)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, $11)`,
		string(e.Action), e.ClientID, e.CertSerial, e.Scope, e.Reason, e.RequestID, e.IP, string(metadata), created.UTC(), prevHash, hash)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ChainVerification is the result of replaying an audit hash chain.
type ChainVerification struct {
	OK        bool
	Checked   int
	BrokenSeq int64
	BrokenID  string
	Detail    string
}

// VerifyChain replays the kdc_audit_log hash chain in seq order and reports the
// first inconsistency (a modified field, a deleted or reordered row). Rows with
// no hash (pre-chain) are skipped.
func (r *AuditRepository) VerifyChain(ctx context.Context) (ChainVerification, error) {
	if r == nil || r.db == nil {
		return ChainVerification{OK: true}, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT seq, id::text, COALESCE(prev_hash, ''), COALESCE(hash, ''),
		        action, COALESCE(client_id, ''), COALESCE(cert_serial, ''),
		        COALESCE(scope, ''), COALESCE(reason, ''), COALESCE(request_id, '')
		 FROM kdc_audit_log ORDER BY seq`)
	if err != nil {
		return ChainVerification{}, err
	}
	defer rows.Close()

	running := ""
	first := true
	checked := 0
	for rows.Next() {
		var seq int64
		var id, prevHash, hash, action, clientID, certSerial, scope, reason, requestID string
		if err := rows.Scan(&seq, &id, &prevHash, &hash, &action, &clientID, &certSerial, &scope, &reason, &requestID); err != nil {
			return ChainVerification{}, err
		}
		if hash == "" {
			continue // pre-chain row, not covered
		}
		if first {
			running = prevHash
			first = false
		}
		expect := auditChainHash(running, auditHashFields(action, clientID, certSerial, scope, reason, requestID)...)
		if prevHash != running {
			return ChainVerification{OK: false, Checked: checked, BrokenSeq: seq, BrokenID: id, Detail: "prev_hash does not match the previous row"}, nil
		}
		if expect != hash {
			return ChainVerification{OK: false, Checked: checked, BrokenSeq: seq, BrokenID: id, Detail: "hash does not match the row contents"}, nil
		}
		running = hash
		checked++
	}
	if err := rows.Err(); err != nil {
		return ChainVerification{}, err
	}
	return ChainVerification{OK: true, Checked: checked}, nil
}

// VerifyAuditChain exposes the chain verification through the service facade.
func (s *Service) VerifyAuditChain(ctx context.Context) (ChainVerification, error) {
	if s == nil || s.auditRepo == nil {
		return ChainVerification{OK: true}, nil
	}
	return s.auditRepo.VerifyChain(ctx)
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

// auditBestEffort inserts an event and swallows storage failures (logging a
// warning), so auditing can never break AS/TGS ticket issuance.
func auditBestEffort(ctx context.Context, repo *AuditRepository, e AuditEvent) {
	if repo == nil || e.Action == "" {
		return
	}
	if err := repo.InsertAudit(ctx, e); err != nil {
		fmt.Printf("[KDC] warning: cannot insert audit event action=%s request_id=%s: %v\n", e.Action, e.RequestID, err)
	}
}

// AuditSink records a single key-issuance event. AS/TGS services hold one and
// call it at their decision points; a nil sink is a no-op.
type AuditSink func(ctx context.Context, e AuditEvent)

// Audit records an event best-effort via the service's audit repository.
func (s *Service) Audit(ctx context.Context, e AuditEvent) {
	if s == nil {
		return
	}
	auditBestEffort(ctx, s.auditRepo, e)
}

// auditReason turns a KDC domain error into a stable lower_snake reason string
// (e.g. REPLAY_DETECTED -> "replay_detected") for the audit trail.
func auditReason(err error) string {
	if err == nil {
		return ""
	}
	return strings.ToLower(string(ErrorCodeOf(err)))
}

// --- trace context: request_id / ip travel from the gRPC handler as context
// values (transport concern), never as service method params. ---

type auditCtxKey int

const (
	ctxKeyRequestID auditCtxKey = iota
	ctxKeyIP
)

// WithAuditMeta attaches the gateway trace id and caller IP for downstream audit.
func WithAuditMeta(ctx context.Context, requestID, ip string) context.Context {
	if requestID != "" {
		ctx = context.WithValue(ctx, ctxKeyRequestID, requestID)
	}
	if ip != "" {
		ctx = context.WithValue(ctx, ctxKeyIP, ip)
	}
	return ctx
}

func auditRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

func auditIP(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyIP).(string); ok {
		return v
	}
	return ""
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
