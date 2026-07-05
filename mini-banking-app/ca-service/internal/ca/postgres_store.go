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

func (s *PostgresStore) UpsertIssuer(ctx context.Context, record IssuerRecord) error {
	if strings.TrimSpace(record.IssuerID) == "" {
		return fmt.Errorf("%w: issuer_id is required", ErrInvalidInput)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ca_issuers (
			issuer_id,
			parent_issuer_id,
			common_name,
			cert_role,
			serial_number,
			certificate_pem,
			fingerprint_sha256,
			subject_key_id,
			authority_key_id,
			not_before,
			not_after,
			status,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14
		)
		ON CONFLICT (issuer_id) DO UPDATE SET
			parent_issuer_id = EXCLUDED.parent_issuer_id,
			common_name = EXCLUDED.common_name,
			cert_role = EXCLUDED.cert_role,
			serial_number = EXCLUDED.serial_number,
			certificate_pem = EXCLUDED.certificate_pem,
			fingerprint_sha256 = EXCLUDED.fingerprint_sha256,
			subject_key_id = EXCLUDED.subject_key_id,
			authority_key_id = EXCLUDED.authority_key_id,
			not_before = EXCLUDED.not_before,
			not_after = EXCLUDED.not_after,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
	`,
		record.IssuerID,
		nullableString(record.ParentIssuerID),
		record.CommonName,
		record.CertRole,
		record.SerialNumber,
		record.CertificatePEM,
		record.FingerprintSHA256,
		nullableString(record.SubjectKeyID),
		nullableString(record.AuthorityKeyID),
		record.NotBefore,
		record.NotAfter,
		defaultString(record.Status, IssuerStatusActive),
		zeroTimeDefault(record.CreatedAt, time.Now().UTC()),
		zeroTimeDefault(record.UpdatedAt, time.Now().UTC()),
	)
	if err != nil {
		return fmt.Errorf("upsert CA issuer metadata: %w", err)
	}
	return nil
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
		  AND cert_type = 'client'
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
			cert_type,
			issuer_id,
			issuer_common_name,
			issuer_serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			chain_pem,
			chain_fingerprints,
			fingerprint_sha256,
			is_ca,
			key_usage,
			extended_key_usage,
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
			$8, $9, $10, $11, $12::jsonb, $13, $14,
			$15::text[], $16::text[], $17, $18, $19,
			$20, $21, $22, $23, $24
		)
	`,
		record.SerialNumber,
		record.CertType,
		nullableString(record.IssuerID),
		record.IssuerCommonName,
		nullableString(record.IssuerSerial),
		record.OwnerID,
		record.SubjectCN,
		record.SubjectEmail,
		record.PublicKeyPEM,
		record.CertificatePEM,
		nullableString(record.ChainPEM),
		jsonStringSlice(record.ChainFingerprints),
		record.FingerprintSHA256,
		record.IsCA,
		textArrayLiteral(record.KeyUsage),
		textArrayLiteral(record.ExtendedKeyUsage),
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
			cert_type,
			issuer_id,
			issuer_common_name,
			issuer_serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			chain_pem,
			COALESCE(chain_fingerprints::text, '[]'),
			fingerprint_sha256,
			is_ca,
			COALESCE(array_to_json(key_usage)::text, '[]'),
			COALESCE(array_to_json(extended_key_usage)::text, '[]'),
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
			cert_type,
			issuer_id,
			issuer_common_name,
			issuer_serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			chain_pem,
			COALESCE(chain_fingerprints::text, '[]'),
			fingerprint_sha256,
			is_ca,
			COALESCE(array_to_json(key_usage)::text, '[]'),
			COALESCE(array_to_json(extended_key_usage)::text, '[]'),
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
	var certType string
	err = tx.QueryRowContext(ctx, `
		SELECT status, cert_type
		FROM certificates
		WHERE serial_number = $1
		FOR UPDATE
	`, serial).Scan(&currentStatus, &certType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrCertificateNotFound, serial)
	}
	if err != nil {
		return nil, fmt.Errorf("lock certificate for revoke: %w", err)
	}
	if currentStatus == string(CertStatusRevoked) {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrAlreadyRevoked, serial)
	}
	if certType != CertTypeClient {
		return nil, fmt.Errorf("%w: cert_type=%s serial_number=%s", ErrCertificateNotRevokable, certType, serial)
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
			cert_type,
			issuer_id,
			issuer_common_name,
			issuer_serial_number,
			owner_id,
			subject_cn,
			subject_email,
			public_key_pem,
			certificate_pem,
			chain_pem,
			COALESCE(chain_fingerprints::text, '[]'),
			fingerprint_sha256,
			is_ca,
			COALESCE(array_to_json(key_usage)::text, '[]'),
			COALESCE(array_to_json(extended_key_usage)::text, '[]'),
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
			cert_type,
			issuer_id,
			action,
			performed_by,
			reason,
			performed_at,
			metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		event.SerialNumber,
		nullableString(event.CertType),
		nullableString(event.IssuerID),
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

type certificateScanner interface {
	Scan(dest ...any) error
}

func scanCertificate(scanner certificateScanner) (*CertificateRecord, error) {
	var record CertificateRecord
	var status string
	var issuerID sql.NullString
	var issuerSerial sql.NullString
	var ownerID sql.NullString
	var subjectEmail sql.NullString
	var publicKeyPEM sql.NullString
	var chainPEM sql.NullString
	var chainFingerprints string
	var keyUsage string
	var extendedKeyUsage string
	var revokedAt sql.NullTime
	var revocationReason sql.NullString

	err := scanner.Scan(
		&record.SerialNumber,
		&record.CertType,
		&issuerID,
		&record.IssuerCommonName,
		&issuerSerial,
		&ownerID,
		&record.SubjectCN,
		&subjectEmail,
		&publicKeyPEM,
		&record.CertificatePEM,
		&chainPEM,
		&chainFingerprints,
		&record.FingerprintSHA256,
		&record.IsCA,
		&keyUsage,
		&extendedKeyUsage,
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
	record.IssuerID = issuerID.String
	record.IssuerSerial = issuerSerial.String
	record.OwnerID = ownerID.String
	record.SubjectEmail = subjectEmail.String
	record.PublicKeyPEM = publicKeyPEM.String
	record.ChainPEM = chainPEM.String
	record.ChainFingerprints = decodeStringSliceJSON(chainFingerprints)
	record.KeyUsage = decodeStringSliceJSON(keyUsage)
	record.ExtendedKeyUsage = decodeStringSliceJSON(extendedKeyUsage)
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
	if certType := strings.TrimSpace(strings.ToLower(filter.CertType)); certType != "" {
		args = append(args, certType)
		clauses = append(clauses, "cert_type = $"+fmt.Sprint(len(args)))
	}
	if issuerID := strings.TrimSpace(filter.IssuerID); issuerID != "" {
		args = append(args, issuerID)
		clauses = append(clauses, "issuer_id = $"+fmt.Sprint(len(args)))
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

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func zeroTimeDefault(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}

func nullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func jsonStringSlice(values []string) any {
	if len(values) == 0 {
		return nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	return data
}

func decodeStringSliceJSON(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func textArrayLiteral(values []string) any {
	if len(values) == 0 {
		return nil
	}
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		escaped = append(escaped, `"`+value+`"`)
	}
	return "{" + strings.Join(escaped, ",") + "}"
}
