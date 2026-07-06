-- CA database schema for Mini Banking.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS ca_issuers (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    issuer_id          VARCHAR(64) NOT NULL UNIQUE,
    parent_issuer_id   VARCHAR(64),
    common_name        VARCHAR(255) NOT NULL,
    cert_role          VARCHAR(32) NOT NULL
                       CHECK (cert_role IN ('root_ca', 'grpc_transport_ca', 'client_ca')),
    serial_number      VARCHAR(128) NOT NULL UNIQUE,
    certificate_pem    TEXT NOT NULL,
    fingerprint_sha256 VARCHAR(64) NOT NULL UNIQUE,
    subject_key_id     VARCHAR(128),
    authority_key_id   VARCHAR(128),
    not_before         TIMESTAMPTZ NOT NULL,
    not_after          TIMESTAMPTZ NOT NULL,
    status             VARCHAR(16) NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'retired', 'expired', 'compromised')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT issuer_root_has_no_parent
        CHECK (cert_role != 'root_ca' OR parent_issuer_id IS NULL),
    CONSTRAINT issuer_intermediate_has_parent
        CHECK (cert_role = 'root_ca' OR parent_issuer_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS certificates (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    serial_number      VARCHAR(128) NOT NULL UNIQUE,
    cert_type          VARCHAR(20) NOT NULL
                       CHECK (cert_type IN ('root_ca', 'intermediate_ca', 'service_tls', 'client')),
    issuer_id          VARCHAR(64),
    issuer_common_name VARCHAR(255) NOT NULL,
    issuer_serial_number VARCHAR(128),
    owner_id           VARCHAR(255),
    subject_cn         VARCHAR(255) NOT NULL,
    subject_email      VARCHAR(255),
    public_key_pem     TEXT,
    certificate_pem    TEXT NOT NULL,
    chain_pem          TEXT,
    chain_fingerprints JSONB,
    fingerprint_sha256 VARCHAR(64) NOT NULL UNIQUE,
    is_ca              BOOLEAN NOT NULL DEFAULT FALSE,
    key_usage          TEXT[],
    extended_key_usage TEXT[],
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
        CHECK (status != 'revoked' OR revocation_reason IS NOT NULL),
    CONSTRAINT cert_client_requires_identity
        CHECK (
            cert_type != 'client'
            OR (owner_id IS NOT NULL AND subject_email IS NOT NULL AND public_key_pem IS NOT NULL)
        ),
    CONSTRAINT cert_ca_type_matches_basic_constraints
        CHECK (
            (cert_type IN ('root_ca', 'intermediate_ca') AND is_ca = TRUE)
            OR (cert_type IN ('service_tls', 'client') AND is_ca = FALSE)
        )
);

CREATE TABLE IF NOT EXISTS certificate_audit_log (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    serial_number VARCHAR(128) NOT NULL,
    cert_type     VARCHAR(20),
    issuer_id     VARCHAR(64),
    action        VARCHAR(30) NOT NULL
                  CHECK (action IN (
                      'issuer_provisioned',
                      'issued',
                      'revoked',
                      'looked_up',
                      'verify_certificate',
                      'chain_verified'
                  )),
    performed_by  VARCHAR(255) NOT NULL,
    reason        TEXT,
    performed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata      JSONB,
    CONSTRAINT audit_revoke_requires_reason
        CHECK (action != 'revoked' OR reason IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ca_issuers_issuer_id
    ON ca_issuers(issuer_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ca_issuers_serial
    ON ca_issuers(serial_number);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ca_issuers_fingerprint
    ON ca_issuers(fingerprint_sha256);
CREATE INDEX IF NOT EXISTS idx_ca_issuers_role
    ON ca_issuers(cert_role);
CREATE INDEX IF NOT EXISTS idx_ca_issuers_status
    ON ca_issuers(status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_serial
    ON certificates(serial_number);
CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_fingerprint
    ON certificates(fingerprint_sha256);
CREATE INDEX IF NOT EXISTS idx_certs_owner_id
    ON certificates(owner_id);
CREATE INDEX IF NOT EXISTS idx_certs_subject_email
    ON certificates(subject_email);
CREATE INDEX IF NOT EXISTS idx_certs_type
    ON certificates(cert_type);
CREATE INDEX IF NOT EXISTS idx_certs_issuer_id
    ON certificates(issuer_id);
CREATE INDEX IF NOT EXISTS idx_certs_status
    ON certificates(status);
CREATE INDEX IF NOT EXISTS idx_certs_not_after
    ON certificates(not_after);
CREATE UNIQUE INDEX IF NOT EXISTS idx_certs_one_active_per_owner
    ON certificates(owner_id)
    WHERE cert_type = 'client' AND status = 'active';

CREATE INDEX IF NOT EXISTS idx_audit_serial
    ON certificate_audit_log(serial_number);
CREATE INDEX IF NOT EXISTS idx_audit_performed_at
    ON certificate_audit_log(performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action
    ON certificate_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_cert_type
    ON certificate_audit_log(cert_type);
CREATE INDEX IF NOT EXISTS idx_audit_issuer_id
    ON certificate_audit_log(issuer_id);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ca_issuers_updated_at ON ca_issuers;
CREATE TRIGGER trg_ca_issuers_updated_at
    BEFORE UPDATE ON ca_issuers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_certificates_updated_at ON certificates;
CREATE TRIGGER trg_certificates_updated_at
    BEFORE UPDATE ON certificates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
