-- KDC audit log schema for Mini Banking.
--
-- The KDC is otherwise stateless (TGT/service tickets are encrypted blobs and
-- replay markers live in Redis). This table is the durable audit trail for the
-- key-issuance domain: every AS/TGS ticket granted or rejected is recorded here.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS kdc_audit_log (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    action      VARCHAR(30) NOT NULL
                CHECK (action IN (
                    'as_ticket_issued',
                    'as_rejected',
                    'tgs_ticket_issued',
                    'tgs_rejected'
                )),
    client_id   VARCHAR(255),
    cert_serial VARCHAR(128),
    scope       VARCHAR(64),
    reason      TEXT,
    request_id  VARCHAR(64),
    ip          VARCHAR(64),
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kdc_audit_created_at
    ON kdc_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kdc_audit_action
    ON kdc_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_kdc_audit_client_id
    ON kdc_audit_log(client_id);
CREATE INDEX IF NOT EXISTS idx_kdc_audit_request_id
    ON kdc_audit_log(request_id);
