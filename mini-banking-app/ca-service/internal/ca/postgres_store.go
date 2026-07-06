package ca

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateCertificate(ctx context.Context, record CertificateRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create certificate transaction: %w", err)
	}
	defer tx.Rollback()

	var existingSerial string
	err = tx.QueryRowContext(ctx, `
		SELECT serial_number
		FROM certificates
		WHERE owner_id = $1
		  AND status = 'active'
		  AND not_after > NOW()
		LIMIT 1
	`, record.OwnerID).Scan(&existingSerial)
	if err == nil {
		return fmt.Errorf("%w: owner_id=%s serial_number=%s", ErrActiveCertificateExists, record.OwnerID, existingSerial)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check active certificate for owner: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO certificates (
			serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			fingerprint_sha256,
			not_before,
			not_after,
			status,
			issued_at,
			revoked_at,
			revocation_reason,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14, $15
		)
	`,
		record.SerialNumber,
		record.OwnerID,
		record.SubjectCN,
		record.SubjectEmail,
		record.PublicKeyPEM,
		record.CertificatePEM,
		record.FingerprintSHA256,
		record.NotBefore,
		record.NotAfter,
		string(record.Status),
		record.IssuedAt,
		nullableTime(record.RevokedAt),
		nullableString(record.RevocationReason),
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert certificate metadata: %w", err)
	}
	return tx.Commit()
}

func (s *PostgresStore) GetCertificate(ctx context.Context, serial string) (*CertificateRecord, error) {
	record, err := scanCertificate(s.db.QueryRowContext(ctx, `
		SELECT
			serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			fingerprint_sha256,
			not_before,
			not_after,
			status,
			issued_at,
			revoked_at,
			revocation_reason,
			created_at,
			updated_at
		FROM certificates
		WHERE serial_number = $1
	`, serial))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrCertificateNotFound, serial)
	}
	if err != nil {
		return nil, fmt.Errorf("get certificate metadata: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) ListCertificates(ctx context.Context, filter ListFilter) ([]CertificateRecord, int, error) {
	where, args := certificateListWhere(filter)
	countSQL := "SELECT COUNT(*) FROM certificates" + where

	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count certificates: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	query := `
		SELECT
			serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			fingerprint_sha256,
			not_before,
			not_after,
			status,
			issued_at,
			revoked_at,
			revocation_reason,
			created_at,
			updated_at
		FROM certificates` + where + `
		ORDER BY issued_at DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()

	var records []CertificateRecord
	for rows.Next() {
		record, err := scanCertificate(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan certificate list row: %w", err)
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate certificate list: %w", err)
	}
	return records, total, nil
}

func (s *PostgresStore) RevokeCertificate(ctx context.Context, serial, reason string, revokedAt time.Time) (*CertificateRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin revoke transaction: %w", err)
	}
	defer tx.Rollback()

	var currentStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM certificates
		WHERE serial_number = $1
		FOR UPDATE
	`, serial).Scan(&currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrCertificateNotFound, serial)
	}
	if err != nil {
		return nil, fmt.Errorf("lock certificate for revoke: %w", err)
	}
	if currentStatus == string(CertStatusRevoked) {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrAlreadyRevoked, serial)
	}

	record, err := scanCertificate(tx.QueryRowContext(ctx, `
		UPDATE certificates
		SET status = 'revoked',
		    revoked_at = $2,
		    revocation_reason = $3,
		    updated_at = NOW()
		WHERE serial_number = $1
		RETURNING
			serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			fingerprint_sha256,
			not_before,
			not_after,
			status,
			issued_at,
			revoked_at,
			revocation_reason,
			created_at,
			updated_at
	`, serial, revokedAt.UTC(), reason))
	if err != nil {
		return nil, fmt.Errorf("update revoked certificate: %w", err)
	}
	return record, tx.Commit()
}

func (s *PostgresStore) AppendAudit(ctx context.Context, event AuditEvent) error {
	if event.PerformedAt.IsZero() {
		event.PerformedAt = time.Now().UTC()
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	if event.Metadata == nil {
		metadata = nil
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO certificate_audit_log (
			serial_number,
			action,
			performed_by,
			reason,
			performed_at,
			metadata
		) VALUES ($1, $2, $3, $4, $5, $6)
	`,
		event.SerialNumber,
		string(event.Action),
		event.PerformedBy,
		nullableString(event.Reason),
		event.PerformedAt.UTC(),
		metadata,
	)
	if err != nil {
		return fmt.Errorf("insert certificate audit event: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListAudit(ctx context.Context, filter AuditFilter) ([]AuditEvent, int, error) {
	where, args := auditListWhere(filter)
	countSQL := "SELECT COUNT(*) FROM certificate_audit_log" + where

	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit events: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	query := `
		SELECT serial_number, action, performed_by, reason, performed_at, metadata
		FROM certificate_audit_log` + where + `
		ORDER BY performed_at DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var action string
		var reason sql.NullString
		var metadata []byte
		if err := rows.Scan(&event.SerialNumber, &action, &event.PerformedBy, &reason, &event.PerformedAt, &metadata); err != nil {
			return nil, 0, fmt.Errorf("scan audit event row: %w", err)
		}
		event.Action = AuditAction(action)
		if reason.Valid {
			event.Reason = reason.String
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, 0, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		event.PerformedAt = event.PerformedAt.UTC()
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, total, nil
}

func auditListWhere(filter AuditFilter) (string, []any) {
	var clauses []string
	var args []any

	if serial := strings.TrimSpace(filter.SerialNumber); serial != "" {
		args = append(args, "%"+serial+"%")
		clauses = append(clauses, "serial_number ILIKE $"+fmt.Sprint(len(args)))
	}
	if action := strings.TrimSpace(filter.Action); action != "" {
		args = append(args, action)
		clauses = append(clauses, "action = $"+fmt.Sprint(len(args)))
	}
	if performedBy := strings.TrimSpace(filter.PerformedBy); performedBy != "" {
		args = append(args, "%"+performedBy+"%")
		clauses = append(clauses, "performed_by ILIKE $"+fmt.Sprint(len(args)))
	}
	if !filter.From.IsZero() {
		args = append(args, filter.From.UTC())
		clauses = append(clauses, "performed_at >= $"+fmt.Sprint(len(args)))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To.UTC())
		clauses = append(clauses, "performed_at < $"+fmt.Sprint(len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

type certificateScanner interface {
	Scan(dest ...any) error
}

func scanCertificate(scanner certificateScanner) (*CertificateRecord, error) {
	var record CertificateRecord
	var status string
	var revokedAt sql.NullTime
	var revocationReason sql.NullString

	err := scanner.Scan(
		&record.SerialNumber,
		&record.OwnerID,
		&record.SubjectCN,
		&record.SubjectEmail,
		&record.PublicKeyPEM,
		&record.CertificatePEM,
		&record.FingerprintSHA256,
		&record.NotBefore,
		&record.NotAfter,
		&status,
		&record.IssuedAt,
		&revokedAt,
		&revocationReason,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	record.Status = CertStatus(status)
	if revokedAt.Valid {
		value := revokedAt.Time.UTC()
		record.RevokedAt = &value
	}
	if revocationReason.Valid {
		record.RevocationReason = revocationReason.String
	}
	return &record, nil
}

func certificateListWhere(filter ListFilter) (string, []any) {
	var clauses []string
	var args []any

	status := strings.TrimSpace(strings.ToLower(filter.Status))
	switch status {
	case "":
	case string(CertStatusActive):
		clauses = append(clauses, "status = 'active' AND not_after > NOW()")
	case string(CertStatusRevoked):
		clauses = append(clauses, "status = 'revoked'")
	case string(CertStatusExpired):
		clauses = append(clauses, "(status = 'expired' OR (status = 'active' AND not_after <= NOW()))")
	default:
		args = append(args, status)
		clauses = append(clauses, "status = $"+fmt.Sprint(len(args)))
	}

	if ownerID := strings.TrimSpace(filter.OwnerID); ownerID != "" {
		args = append(args, "%"+ownerID+"%")
		clauses = append(clauses, "owner_id ILIKE $"+fmt.Sprint(len(args)))
	}
	email := strings.TrimSpace(filter.SubjectEmail)
	if email == "" {
		email = strings.TrimSpace(filter.SubjectEmail)
	}
	if email != "" {
		args = append(args, "%"+email+"%")
		clauses = append(clauses, "subject_email ILIKE $"+fmt.Sprint(len(args)))
	}
	if serial := strings.TrimSpace(filter.SerialNumber); serial != "" {
		args = append(args, "%"+serial+"%")
		clauses = append(clauses, "serial_number ILIKE $"+fmt.Sprint(len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func nullableString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}
