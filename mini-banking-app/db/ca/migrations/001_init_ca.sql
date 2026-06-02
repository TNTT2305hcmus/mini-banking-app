-- CA database schema for Mini Banking.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS certificates (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    serial_number      VARCHAR(128) NOT NULL UNIQUE,
    owner_id           VARCHAR(255) NOT NULL,
    subject_cn         VARCHAR(255) NOT NULL,
    subject_email      VARCHAR(255) NOT NULL,
    public_key_pem     TEXT NOT NULL,
    certificate_pem    TEXT NOT NULL,
    fingerprint_sha256 VARCHAR(64) NOT NULL UNIQUE,
    not_before         TIMESTAMPTZ NOT NULL,
    not_after          TIMESTAMPTZ NOT NULL,
    status             VARCHAR(10) NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'revoked', 'expired')),
    issued_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at         TIMESTAMPTZ,
    revocation_reason  TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cert_revoked_requires_time
        CHECK (status != 'revoked' OR revoked_at IS NOT NULL),
    CONSTRAINT cert_revoked_requires_reason
        CHECK (status != 'revoked' OR revocation_reason IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS certificate_audit_log (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    serial_number VARCHAR(128) NOT NULL,
    action        VARCHAR(30) NOT NULL
                  CHECK (action IN ('issued', 'revoked', 'looked_up', 'revocation_checked', 'verify_certificate')),
    performed_by  VARCHAR(255) NOT NULL,
    reason        TEXT,
    performed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata      JSONB,
    CONSTRAINT audit_revoke_requires_reason
        CHECK (action != 'revoked' OR reason IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_serial
    ON certificates(serial_number);
CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_fingerprint
    ON certificates(fingerprint_sha256);
CREATE INDEX IF NOT EXISTS idx_certs_owner_id
    ON certificates(owner_id);
CREATE INDEX IF NOT EXISTS idx_certs_subject_email
    ON certificates(subject_email);
CREATE INDEX IF NOT EXISTS idx_certs_status
    ON certificates(status);
CREATE INDEX IF NOT EXISTS idx_certs_not_after
    ON certificates(not_after);
CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_one_active_per_owner
    ON certificates(owner_id) WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_audit_serial
    ON certificate_audit_log(serial_number);
CREATE INDEX IF NOT EXISTS idx_audit_performed_at
    ON certificate_audit_log(performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action
    ON certificate_audit_log(action);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_certificates_updated_at ON certificates;
CREATE TRIGGER trg_certificates_updated_at
    BEFORE UPDATE ON certificates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
