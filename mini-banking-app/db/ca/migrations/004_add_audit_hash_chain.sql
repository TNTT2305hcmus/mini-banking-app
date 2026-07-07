-- Tamper-evidence for the CA certificate audit log: a hash chain over events.
--
-- seq gives a stable total order; each row stores prev_hash and
-- hash = SHA256(prev_hash | action | serial_number | performed_by | reason).
-- Deleting, reordering or modifying a hashed field breaks the chain, which the
-- verify endpoint detects by replaying it.

ALTER TABLE certificate_audit_log
    ADD COLUMN IF NOT EXISTS seq       BIGSERIAL,
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS hash      VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_ca_audit_seq ON certificate_audit_log(seq);
