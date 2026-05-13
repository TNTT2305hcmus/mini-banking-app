/**
 * @file init.sql
 * @description Database schema for the Mini App Banking system.
 * Includes user management, accounts, digital certificates (PKI), and transaction control 
 * with advanced security mechanisms (Anti-Replay Attack, Idempotency, Hash-chaining).
 * @version 1.0.0
 */

-- =============================================================
-- EXTENSIONS
-- =============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================================
-- TABLES
-- =============================================================

/**
 * @table users
 * @description Stores customer information, including personal identification data and system status.
 * * @property {UUID} id - Primary key, auto-generated using UUID v4.
 * @property {VARCHAR(100)} full_name - Full name of the user.
 * @property {VARCHAR(50)} identity_number - Identity card/Passport number, used for legal identification (Unique).
 * @property {VARCHAR(255)} address - Current contact address.
 * @property {DATE} date_of_birth - Date of birth.
 * @property {VARCHAR(120)} email - Primary contact email address (Unique).
 * @property {VARCHAR(20)} phone - Contact phone number (Unique).
 * @property {VARCHAR(255)} cert_serial - Serial number of the current digital certificate associated with the user.
 * @property {VARCHAR(10)} status - User account status, limited to the set: 'active', 'locked'.
 * @property {TIMESTAMPTZ} created_at - Record creation timestamp.
 * @property {TIMESTAMPTZ} updated_at - Timestamp of the most recent record update.
 */
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

/**
 * @table accounts
 * @description Stores bank account information, balances, and transaction limits associated with a User.
 * * @property {UUID} id - Primary key of the account (UUID).
 * @property {UUID} user_id - Foreign key linking to the users table. RESTRICT to prevent accidental deletion of users with active accounts.
 * @property {VARCHAR(30)} account_number - Bank account number identifying transactions (Unique).
 * @property {BIGINT} balance - Current available balance (Always >= 0).
 * @property {VARCHAR(3)} currency - ISO 4217 standard currency code, default 'VND'.
 * @property {BIGINT} daily_transfer_limit - Maximum daily transfer limit.
 * @property {VARCHAR(10)} status - Account status: 'active', 'locked', 'frozen'.
 * @property {TIMESTAMPTZ} created_at - Account creation timestamp.
 * @property {TIMESTAMPTZ} updated_at - Timestamp of the most recent account fluctuation.
 */
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

/**
 * @table user_certificates
 * @description Manages the user's Public Key Infrastructure (PKI). Stores the Public Key to verify digital signatures on transactions.
 * * @property {UUID} id - Primary key identifying the certificate in the system.
 * @property {UUID} user_id - Foreign key pointing to the certificate owner.
 * @property {VARCHAR(255)} serial_number - Unique Serial number of the digital certificate issued by the CA.
 * @property {TEXT} public_key_pem - Public Key string in PEM format used to verify the signature.
 * @property {VARCHAR(10)} status - Validity status of the certificate: 'active', 'revoked', 'expired'.
 * @property {TIMESTAMPTZ} not_after - Expiration timestamp of the digital certificate.
 * @property {TIMESTAMPTZ} created_at - Timestamp when the certificate was added to the system.
 */
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

/**
 * @table transactions
 * @description Core table managing the status, cash flow, and integrity of transactions through hashing and signatures.
 * * @property {UUID} id - Primary key identifying the transaction.
 * @property {UUID} from_account_id - Foreign key, source account for deduction.
 * @property {UUID} to_account_id - Foreign key, destination account for receiving funds.
 * @property {VARCHAR(30)} from_account_number - Snapshot of the source account number at the time of transaction.
 * @property {VARCHAR(30)} to_account_number - Snapshot of the destination account number at the time of transaction.
 * @property {BIGINT} amount - Transaction amount (Must be > 0).
 * @property {VARCHAR(3)} currency - Currency type.
 * @property {VARCHAR(10)} status - Transaction lifecycle: 'pending', 'completed', 'failed'.
 * @property {TEXT} description - Transfer description/message.
 * @property {VARCHAR(255)} payload_hash - Hash code of the request payload (ensures Integrity).
 * @property {TEXT} client_signature - Digital signature from the user (ensures Non-repudiation).
 * @property {VARCHAR(50)} scope - Transaction scope (e.g., internal, external, payment).
 * @property {VARCHAR(64)} nonce - One-time random number string (protects against Replay Attacks).
 * @property {VARCHAR(64)} idempotency_key - Idempotency key (Prevents duplicate transaction processing).
 * @property {VARCHAR(255)} previous_hash - Hash code of the immediately preceding transaction (Blockchain-style Audit mechanism).
 * @property {VARCHAR(255)} current_hash - Hash code of this exact transaction after completion.
 * @property {TIMESTAMPTZ} created_at - Transaction creation timestamp.
 * @property {TIMESTAMPTZ} completed_at - Transaction completion/failure timestamp.
 */
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

/**
 * @table transaction_details
 * @description Stores the history of transaction state transitions, serving auditing purposes (Audit Log).
 * * @property {UUID} id - Primary key identifying the history record.
 * @property {UUID} transaction_id - Links to the original transaction (Deleting the transaction will delete the log - CASCADE).
 * @property {VARCHAR(50)} status_before - Status before the change.
 * @property {VARCHAR(50)} status_after - Status after the change.
 * @property {VARCHAR(100)} changed_by - System, service, or admin performing the state change.
 * @property {TIMESTAMPTZ} changed_at - Timestamp of the state transition.
 * @property {TEXT} notes - Error notes or reasons for rejection (if any).
 */
CREATE TABLE transaction_details (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id  UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    status_before   VARCHAR(50) NOT NULL,
    status_after    VARCHAR(50) NOT NULL,
    changed_by      VARCHAR(100) NOT NULL,
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes           TEXT
);

/**
 * @table used_nonces
 * @description Temporary storage table for used Nonces for verification and prevention of Replay Attacks.
 * * @property {VARCHAR(64)} nonce - Nonce value (Primary Key due to absolute uniqueness).
 * @property {TIMESTAMPTZ} used_at - Timestamp when the Nonce was recorded.
 * @property {TIMESTAMPTZ} expires_at - Timestamp when the Nonce expires (can use cronjob/TTL to clean up expired rows).
 */
CREATE TABLE IF NOT EXISTS used_nonces (
    nonce       VARCHAR(64) PRIMARY KEY,
    used_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

-- =============================================================
-- INDEXES
-- =============================================================
-- Indexing helps accelerate query speed on commonly used fields.

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

-- =============================================================
-- FUNCTIONS & TRIGGERS
-- =============================================================

/**
 * @function update_updated_at_column
 * @description Shared Trigger function to automatically reassign the timestamp value for the `updated_at` column with the current system time.
 * @returns {Trigger} Returns the NEW record object with the updated data.
 */
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN 
    NEW.updated_at = NOW(); 
    RETURN NEW; 
END;
$$ LANGUAGE plpgsql;

/**
 * @description Attaches the time update Trigger to the users table. Activates before the UPDATE action.
 */
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

/**
 * @description Attaches the time update Trigger to the accounts table. Activates before the UPDATE action.
 */
CREATE TRIGGER trg_accounts_updated_at
    BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();