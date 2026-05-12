/**
 * @file init.sql
 * @description Lược đồ cơ sở dữ liệu cho hệ thống Mini App Banking.
 * Bao gồm quản lý người dùng, tài khoản, chứng thư số (PKI) và kiểm soát giao dịch 
 * với các cơ chế bảo mật nâng cao (chống Replay Attack, Idempotency, Hash-chaining).
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
 * @description Lưu trữ thông tin khách hàng, bao gồm dữ liệu định danh cá nhân và trạng thái hệ thống.
 * 
 * @property {UUID} id - Khóa chính, tự động sinh bằng UUID v4.
 * @property {VARCHAR(100)} full_name - Họ và tên đầy đủ của người dùng.
 * @property {VARCHAR(50)} identity_number - Số CMND/CCCD, dùng để định danh pháp lý (Unique).
 * @property {VARCHAR(255)} address - Địa chỉ liên hệ hiện tại.
 * @property {DATE} date_of_birth - Ngày tháng năm sinh.
 * @property {VARCHAR(120)} email - Địa chỉ email liên lạc chính (Unique).
 * @property {VARCHAR(20)} phone - Số điện thoại liên lạc (Unique).
 * @property {VARCHAR(255)} cert_serial - Số serial của chứng thư số hiện tại đang gắn kết với người dùng.
 * @property {VARCHAR(10)} status - Trạng thái tài khoản người dùng, giới hạn trong tập: 'active', 'locked'.
 * @property {TIMESTAMPTZ} created_at - Thời điểm khởi tạo bản ghi.
 * @property {TIMESTAMPTZ} updated_at - Thời điểm cập nhật bản ghi gần nhất.
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
 * @description Lưu trữ thông tin tài khoản ngân hàng, số dư và các giới hạn giao dịch liên quan đến một User.
 * 
 * @property {UUID} id - Khóa chính của tài khoản (UUID).
 * @property {UUID} user_id - Khóa ngoại liên kết tới bảng users. RESTRICT để tránh xóa nhầm user có tài khoản.
 * @property {VARCHAR(30)} account_number - Số tài khoản ngân hàng định danh giao dịch (Unique).
 * @property {BIGINT} balance - Số dư khả dụng hiện tại (Luôn >= 0).
 * @property {VARCHAR(3)} currency - Mã tiền tệ chuẩn ISO 4217, mặc định 'VND'.
 * @property {BIGINT} daily_transfer_limit - Hạn mức chuyển tiền tối đa trong ngày.
 * @property {VARCHAR(10)} status - Tình trạng tài khoản: 'active', 'locked', 'frozen'.
 * @property {TIMESTAMPTZ} created_at - Thời điểm khởi tạo tài khoản.
 * @property {TIMESTAMPTZ} updated_at - Thời điểm biến động tài khoản gần nhất.
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
 * @description Quản lý hạ tầng khóa công khai (PKI) của người dùng. Lưu trữ Public Key để xác thực chữ ký số trên giao dịch.
 * 
 * @property {UUID} id - Khóa chính định danh chứng thư trong hệ thống.
 * @property {UUID} user_id - Khóa ngoại trỏ đến chủ sở hữu chứng thư.
 * @property {VARCHAR(255)} serial_number - Số Serial duy nhất của chứng thư số do CA cấp.
 * @property {TEXT} public_key_pem - Chuỗi Public Key định dạng PEM dùng để verify signature.
 * @property {VARCHAR(10)} status - Trạng thái hiệu lực của chứng thư: 'active', 'revoked', 'expired'.
 * @property {TIMESTAMPTZ} not_after - Thời điểm hết hạn của chứng thư số.
 * @property {TIMESTAMPTZ} created_at - Thời điểm thêm chứng thư vào hệ thống.
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
 * @description Bảng lõi quản lý trạng thái, dòng tiền và tính toàn vẹn của các giao dịch thông qua mã băm và chữ ký.
 * 
 * @property {UUID} id - Khóa chính định danh giao dịch.
 * @property {UUID} from_account_id - Khóa ngoại, tài khoản nguồn trích tiền.
 * @property {UUID} to_account_id - Khóa ngoại, tài khoản đích nhận tiền.
 * @property {VARCHAR(30)} from_account_number - Snapshot số tài khoản nguồn tại thời điểm giao dịch.
 * @property {VARCHAR(30)} to_account_number - Snapshot số tài khoản đích tại thời điểm giao dịch.
 * @property {BIGINT} amount - Số tiền giao dịch (Phải > 0).
 * @property {VARCHAR(3)} currency - Loại tiền tệ.
 * @property {VARCHAR(10)} status - Vòng đời giao dịch: 'pending', 'completed', 'failed'.
 * @property {TEXT} description - Nội dung/lời nhắn chuyển tiền.
 * @property {VARCHAR(255)} payload_hash - Mã băm nội dung request (đảm bảo tính toàn vẹn - Integrity).
 * @property {TEXT} client_signature - Chữ ký số từ phía người dùng (chống chối bỏ - Non-repudiation).
 * @property {VARCHAR(50)} scope - Phạm vi giao dịch (VD: internal, external, payment).
 * @property {VARCHAR(64)} nonce - Chuỗi số ngẫu nhiên dùng một lần (bảo vệ chống Replay Attack).
 * @property {VARCHAR(64)} idempotency_key - Khóa lũy đẳng (Ngăn chặn xử lý trùng lặp giao dịch).
 * @property {VARCHAR(255)} previous_hash - Mã băm của giao dịch liền trước (cơ chế Audit dạng chuỗi khối).
 * @property {VARCHAR(255)} current_hash - Mã băm của chính giao dịch này sau khi hoàn tất.
 * @property {TIMESTAMPTZ} created_at - Thời điểm khởi tạo giao dịch.
 * @property {TIMESTAMPTZ} completed_at - Thời điểm hoàn tất/thất bại giao dịch.
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
 * @description Lưu trữ lịch sử chuyển đổi trạng thái của giao dịch, phục vụ mục đích kiểm toán (Audit Log).
 * 
 * @property {UUID} id - Khóa chính định danh dòng lịch sử.
 * @property {UUID} transaction_id - Liên kết với giao dịch gốc (Xóa giao dịch sẽ xóa log - CASCADE).
 * @property {VARCHAR(50)} status_before - Trạng thái trước khi thay đổi.
 * @property {VARCHAR(50)} status_after - Trạng thái sau khi thay đổi.
 * @property {VARCHAR(100)} changed_by - Hệ thống, dịch vụ, hoặc admin thực hiện việc thay đổi trạng thái.
 * @property {TIMESTAMPTZ} changed_at - Thời gian thực hiện chuyển đổi trạng thái.
 * @property {TEXT} notes - Ghi chú lỗi hoặc lý do từ chối (nếu có).
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
 * @description Bảng lưu trữ tạm thời các mã Nonce đã sử dụng để đối chiếu và ngăn chặn Replay Attack.
 * 
 * @property {VARCHAR(64)} nonce - Giá trị Nonce (Primary Key vì tính duy nhất tuyệt đối).
 * @property {TIMESTAMPTZ} used_at - Thời điểm Nonce được ghi nhận.
 * @property {TIMESTAMPTZ} expires_at - Thời điểm Nonce hết hiệu lực (có thể dùng cronjob/TTL để dọn dẹp các dòng hết hạn).
 */
CREATE TABLE IF NOT EXISTS used_nonces (
    nonce       VARCHAR(64) PRIMARY KEY,
    used_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

-- =============================================================
-- INDEXES
-- =============================================================
-- Lập chỉ mục giúp tăng tốc độ truy vấn trên các trường thông dụng

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
 * @description Hàm Trigger dùng chung để tự động gán lại giá trị thời gian cho cột `updated_at` bằng thời gian hệ thống hiện tại.
 * @returns {Trigger} Trả về đối tượng bản ghi NEW với dữ liệu đã được cập nhật.
 */
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN 
    NEW.updated_at = NOW(); 
    RETURN NEW; 
END;
$$ LANGUAGE plpgsql;

/**
 * @description Gắn Trigger cập nhật thời gian vào bảng users. Kích hoạt trước hành động UPDATE.
 */
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

/**
 * @description Gắn Trigger cập nhật thời gian vào bảng accounts. Kích hoạt trước hành động UPDATE.
 */
CREATE TRIGGER trg_accounts_updated_at
    BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();