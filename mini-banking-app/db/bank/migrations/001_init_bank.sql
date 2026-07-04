-- Bank database schema for Mini Banking.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email      VARCHAR(255) NOT NULL UNIQUE,
    full_name  VARCHAR(255) NOT NULL,
    status     VARCHAR(10) NOT NULL DEFAULT 'active'
               CHECK (status IN ('active', 'locked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS accounts (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    account_number       VARCHAR(30) NOT NULL UNIQUE,
    balance              BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    daily_transfer_limit BIGINT NOT NULL DEFAULT 50000000,
    currency             VARCHAR(3) NOT NULL DEFAULT 'VND',
    status               VARCHAR(10) NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'locked', 'frozen')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transactions (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    from_account_id     UUID REFERENCES accounts(id),
    to_account_id       UUID REFERENCES accounts(id),
    from_account_number VARCHAR(30),
    to_account_number   VARCHAR(30),
    amount              BIGINT NOT NULL CHECK (amount > 0),
    currency            VARCHAR(3) NOT NULL DEFAULT 'VND',
    status              VARCHAR(10) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'completed', 'failed')),
    description         TEXT,
    payload_hash        VARCHAR(64) NOT NULL,
    client_signature    TEXT NOT NULL,
    cert_serial         VARCHAR(128) NOT NULL,
    scope               VARCHAR(50) NOT NULL,
    nonce               VARCHAR(255) NOT NULL UNIQUE,
    idempotency_key     VARCHAR(255) NOT NULL UNIQUE,
    previous_hash       VARCHAR(64),
    current_hash        VARCHAR(64),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    CONSTRAINT diff_accounts CHECK (
        from_account_id IS NULL
        OR to_account_id IS NULL
        OR from_account_id != to_account_id
    )
);

CREATE TABLE IF NOT EXISTS used_nonces (
    nonce      VARCHAR(255) PRIMARY KEY,
    used_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS bank_audit_log (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    action         VARCHAR(40) NOT NULL
                   CHECK (action IN (
                       'transfer_completed',
                       'transfer_rejected',
                       'replay_detected',
                       'invalid_signature',
                       'certificate_rejected',
                       'forbidden_ownership',
                       'insufficient_funds'
                   )),
    user_id        UUID,
    account_id     UUID,
    transaction_id UUID,
    cert_serial    VARCHAR(128),
    request_id     VARCHAR(64),
    reason         TEXT,
    metadata       JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ledger_state (
    id                  VARCHAR(20) PRIMARY KEY,
    last_hash           VARCHAR(64) NOT NULL DEFAULT 'genesis',
    last_transaction_id UUID,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO ledger_state(id, last_hash)
VALUES ('main', 'genesis')
ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_accounts_user_id
    ON accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_txn_from_account
    ON transactions(from_account_id);
CREATE INDEX IF NOT EXISTS idx_txn_to_account
    ON transactions(to_account_id);
CREATE INDEX IF NOT EXISTS idx_txn_created_at
    ON transactions(created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_txn_nonce
    ON transactions(nonce);
CREATE UNIQUE INDEX IF NOT EXISTS idx_txn_idem_key
    ON transactions(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_used_nonces_expires_at
    ON used_nonces(expires_at);
CREATE INDEX IF NOT EXISTS idx_bank_audit_created_at
    ON bank_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bank_audit_action
    ON bank_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_bank_audit_user_id
    ON bank_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_bank_audit_request_id
    ON bank_audit_log(request_id);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_accounts_updated_at ON accounts;
CREATE TRIGGER trg_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
