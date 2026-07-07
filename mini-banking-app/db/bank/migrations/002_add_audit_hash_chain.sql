-- Tamper-evidence for the bank audit log: a hash chain over the events.
--
-- seq gives a stable total order; each row stores prev_hash and
-- hash = SHA256(prev_hash | action | user_id | account_id | transaction_id |
-- cert_serial | request_id | reason). Deleting, reordering or modifying a
-- hashed field breaks the chain, which the verify endpoint detects by replay.

ALTER TABLE bank_audit_log
    ADD COLUMN IF NOT EXISTS seq       BIGSERIAL,
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS hash      VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_bank_audit_seq ON bank_audit_log(seq);
