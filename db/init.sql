-- =============================================================
-- Mini_App_Banking - Database Schema
-- =============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1. Users
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    full_name       VARCHAR(100) NOT NULL,
    identity_number VARCHAR(50) NOT NULL UNIQUE,
    address         VARCHAR(255) NOT NULL,
    date_of_birth   DATE NOT NULL,
    email           VARCHAR(120) NOT NULL UNIQUE,  
    phone           VARCHAR(20) UNIQUE,
    cert_serial     VARCHAR(255),
    status          VARCHAR(10) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'locked')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Accounts 
CREATE TABLE IF NOT EXISTS accounts (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    account_number       VARCHAR(30) NOT NULL UNIQUE,
    balance              BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    currency             VARCHAR(3) NOT NULL DEFAULT 'VND',
    daily_transfer_limit BIGINT NOT NULL DEFAULT 50000000,
    status               VARCHAR(10) NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'locked', 'frozen')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. User Certificates 
CREATE TABLE IF NOT EXISTS user_certificates (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    serial_number   VARCHAR(255) NOT NULL UNIQUE,
    public_key_pem  TEXT NOT NULL,
    status          VARCHAR(10) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'revoked', 'expired')),
    not_after       TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. Transactions 
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
    payload_hash        VARCHAR(255),       
    client_signature    TEXT,           
    scope               VARCHAR(50),     
    nonce               VARCHAR(64) UNIQUE,
    idempotency_key     VARCHAR(64) UNIQUE,
    previous_hash       VARCHAR(255),
    current_hash        VARCHAR(255),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,

    CONSTRAINT diff_accounts CHECK (from_account_id != to_account_id)
);

-- 5. Transaction Details table
CREATE TABLE transaction_details (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id      UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    status_before       VARCHAR(50) NOT NULL,
    status_after        VARCHAR(50) NOT NULL,
    changed_by          VARCHAR(100) NOT NULL,
    changed_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes               TEXT
);

-- 6. Used Nonces 
CREATE TABLE IF NOT EXISTS used_nonces (
    nonce       VARCHAR(64) PRIMARY KEY,
    used_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

-- Indexes 
CREATE INDEX idx_accounts_user_id              ON accounts(user_id);
CREATE INDEX idx_user_certs_user_id            ON user_certificates(user_id);
CREATE INDEX idx_user_certs_serial             ON user_certificates(serial_number);
CREATE INDEX idx_transactions_from_account     ON transactions(from_account_id);
CREATE INDEX idx_transactions_to_account       ON transactions(to_account_id);
CREATE INDEX idx_transactions_created_at       ON transactions(created_at DESC);
CREATE INDEX idx_transactions_idempotency_key  ON transactions(idempotency_key);
CREATE INDEX idx_transactions_nonce            ON transactions(nonce);
CREATE INDEX idx_used_nonces_expires_at        ON used_nonces(expires_at);
CREATE INDEX idx_users_cert_serial             ON users(cert_serial);

-- ── Auto-update updated_at ────────────────────────────────────
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER trg_accounts_updated_at
    BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();