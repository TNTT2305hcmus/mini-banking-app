-- Tamper-evidence for the KDC audit log: a hash chain over the events.
--
-- seq gives a stable total order; each row stores prev_hash (the hash of the
-- previous row) and hash = SHA256(prev_hash | action | client_id | cert_serial |
-- scope | reason | request_id). Deleting, reordering or modifying a hashed
-- field breaks the chain, which the verify endpoint detects by replaying it.

ALTER TABLE kdc_audit_log
    ADD COLUMN IF NOT EXISTS seq       BIGSERIAL,
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS hash      VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_kdc_audit_seq ON kdc_audit_log(seq);
