# Mini_App_Banking - Database Documentation
---

##  Chi tiết các bảng

### 1. **users** - Thông tin khách hàng

#### Chức năng
Lưu trữ thông tin cơ bản của khách hàng/người dùng hệ thống ngân hàng.

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `id` | UUID | PK | Định danh duy nhất, tự động sinh bằng uuid_generate_v4() |
| `full_name` | VARCHAR(100) | NOT NULL | Họ và tên đầy đủ theo giấy tờ tùy thân |
| `identity_number` | VARCHAR(50) | UNIQUE, NOT NULL | Số CMND/CCCD/Hộ chiếu - duy nhất cho mỗi người |
| `address` | VARCHAR(255) | NOT NULL | Địa chỉ thường trú |
| `date_of_birth` | DATE | NOT NULL | Ngày sinh (dùng để kiểm tra độ tuổi mở tài khoản) |
| `email` | VARCHAR(120) | UNIQUE, NOT NULL | Email - dùng cho khôi phục mật khẩu & thông báo |
| `phone` | VARCHAR(20) | UNIQUE | Số điện thoại - dùng gửi OTP (nullable nếu chỉ dùng email) |
| `cert_serial` | VARCHAR(255) | | Số serial của chứng chỉ công khai hiện tại (denormalized từ user_certificates) |
| `status` | VARCHAR(10) | CHECK IN ('active', 'locked') | Trạng thái tài khoản: active/locked |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian tạo tài khoản |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian cập nhật lần cuối (auto-update) |

####  Các trường đáng chú ý

- **`identity_number`**: Duy nhất (UNIQUE) vì mỗi người dân chỉ có một CMND/CCCD
- **`email`**: Duy nhất để sử dụng làm tài khoản login/recovery
- **`cert_serial`**: Denormalized để tra cứu nhanh chứng chỉ hiện tại mà không cần JOIN bảng user_certificates
- **`status`**: Chỉ có 2 trạng thái: `active` (hoạt động) hoặc `locked` (khóa tài khoản)

---

### 2. **accounts** - Tài khoản ngân hàng

#### Chức năng
Lưu trữ thông tin tài khoản ngân hàng. Mỗi khách hàng có thể sở hữu nhiều tài khoản.

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `id` | UUID | PK | Định danh duy nhất của tài khoản |
| `user_id` | UUID | FK → users(id), NOT NULL | Tham chiếu đến chủ sở hữu tài khoản |
| `account_number` | VARCHAR(30) | UNIQUE, NOT NULL | Số tài khoản (định dạng quốc gia/IBAN) |
| `balance` | BIGINT | NOT NULL, CHECK >= 0 | Số dư hiện tại (đơn vị: cents) |
| `daily_transfer_limit` | BIGINT | NOT NULL, DEFAULT 50000000 | Hạn mức chuyển khoản ngày (đơn vị: cents) |
| `currency` | VARCHAR(3) | DEFAULT 'VND', NOT NULL | Loại tiền tệ (VND/USD/EUR...) |
| `status` | VARCHAR(10) | CHECK IN ('active', 'locked', 'frozen') | Trạng thái: active/locked/frozen |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Ngày mở tài khoản |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Cập nhật lần cuối (auto-update) |

####  Các trường đáng chú ý

- **`balance`**: 
  - **Kiểu BIGINT thay vì DECIMAL**: Tránh vấn đề floating-point precision
  - **Đơn vị cents**: 1 VND = 1 cent → 100.000 VND = 100000000 cents
  - **Ví dụ**: Số dư 100.000 VND được lưu là `100000000` (BIGINT)
  - **Lợi ích**: Tính toán tiền tệ chính xác, không lỗi làm tròn

- **`daily_transfer_limit`**: 
  - Default 50.000.000 cents = 500.000 VND
  - Phục vụ Domain Validation tại Bank Service (chống gian lận)
  - Có thể cập nhật tùy theo VIP tier của khách

- **`status`**: 
  - `active`: Tài khoản bình thường
  - `locked`: Chủ tài khoản bị khóa
  - `frozen`: Tài khoản bị đóng băng (ví dụ: do phát hiện gian lận)


---

### 3. **user_certificates** - Chứng chỉ công khai (PKI)

#### Chức năng
Lưu trữ chứng chỉ công khai X.509 được cấp bởi CA Service. Dùng để xác minh chữ ký số của client khi thực hiện giao dịch.

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `id` | UUID | PK | Định danh duy nhất của chứng chỉ |
| `user_id` | UUID | FK → users(id), NOT NULL | Tham chiếu đến chủ sở hữu chứng chỉ |
| `serial_number` | VARCHAR(255) | UNIQUE, NOT NULL | Số serial của chứng chỉ X.509 (cấp bởi CA) |
| `public_key_pem` | TEXT | NOT NULL | Khóa công khai (pubKey_c) ở định dạng PEM |
| `status` | VARCHAR(10) | CHECK IN ('active', 'revoked', 'expired') | Trạng thái: active/revoked/expired |
| `not_after` | TIMESTAMPTZ | NOT NULL | Ngày hết hạn của chứng chỉ |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian lưu vào database |

#### Các trường đáng chú ý

- **`serial_number`**: 
  - Duy nhất (UNIQUE) vì mỗi chứng chỉ có số serial khác nhau
  - Được cấp bởi Certificate Authority (CA)
  - Dùng để tra cứu CRL (Certificate Revocation List)

- **`public_key_pem`**: 
  - Lưu khóa công khai dạng PEM (Human-readable text format)
  - Dùng để xác minh chữ ký: Verify(signature, message, pubKey_pem)
  - Ví dụ:
    ```
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAJv7L2...
    ...
    -----END CERTIFICATE-----
    ```

- **`status`**: 
  - `active`: Chứng chỉ còn hiệu lực
  - `revoked`: Chứng chỉ bị thu hồi sớm (ví dụ: key bị lộ)
  - `expired`: Chứng chỉ hết hạn (>not_after)

- **Bảng riêng**: 1 user có thể có nhiều chứng chỉ (revoke cái cũ → cấp cái mới)

---

### 4. **transactions** - Giao dịch chuyển khoản

#### Chức năng
Lưu thông tin của từng giao dịch chuyển khoản giữa 2 tài khoản. 
**Tính chất Immutable Ledger**: Chỉ INSERT, không UPDATE/DELETE (blockchain-lite).

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `id` | UUID | PK | Định danh duy nhất của giao dịch |
| `from_account_id` | UUID | FK → accounts(id) | Tài khoản gửi tiền |
| `to_account_id` | UUID | FK → accounts(id) | Tài khoản nhận tiền |
| `from_account_number` | VARCHAR(30) | | Số tài khoản gửi (denormalized để query nhanh) |
| `to_account_number` | VARCHAR(30) | | Số tài khoản nhận (denormalized để query nhanh) |
| `amount` | BIGINT | NOT NULL, CHECK > 0 | Số tiền gửi (đơn vị: cents) |
| `currency` | VARCHAR(3) | DEFAULT 'VND' | Mã tiền tệ (VND, USD, ...) |
| `status` | VARCHAR(10) | CHECK IN ('pending', 'completed', 'failed') | Trạng thái giao dịch |
| `description` | TEXT | | Mô tả/ghi chú giao dịch (tùy chọn) |
| `payload_hash` | VARCHAR(255) | | SHA-256 hash của Payload gốc (trước khi ký) |
| `client_signature` | TEXT | | Chữ ký số: Sign(Payload, priv_c) - chống chối bỏ |
| `scope` | VARCHAR(50) | | Phạm vi giao dịch (ví dụ: "transfer:internal") |
| `nonce` | VARCHAR(64) | UNIQUE | Nonce từ AP Exchange - chống replay attack |
| `idempotency_key` | VARCHAR(64) | UNIQUE | Khóa idempotency - chống double-spend |
| `previous_hash` | VARCHAR(255) | | SHA-256 của giao dịch trước (hash chaining) |
| `current_hash` | VARCHAR(255) | | SHA-256 của giao dịch hiện tại (hash chaining) |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian tạo giao dịch |
| `completed_at` | TIMESTAMPTZ | | Thời gian hoàn thành (khi status = completed) |

####  Các trường đáng chú ý

- **`amount`**: 
  - Kiểu BIGINT (cents), tương tự accounts.balance
  - Phải > 0 (không cho phép chuyển 0 hoặc số âm)

- **`payload_hash`**: 
  - SHA-256 hash của Payload gốc
  - Được tính **trước khi ký** (chứ không phải sau)
  - Dùng để kiểm tra tính toàn vẹn: Verify(client_signature, payload_hash, pubKey_c)

- **`client_signature`**: 
  - Chữ ký số: Sign(Payload, priv_c) do client tạo
  - **Chống chối bỏ (Non-repudiation)**: Client không thể phủ nhận đã ký giao dịch
  - Bank verify: Verify(signature, payload, pubKey) = True

- **`nonce`**: 
  - Unique cho mỗi giao dịch
  - Đến từ AP Exchange
  - **Chống replay attack**: Attacker không thể gửi lại giao dịch cũ
  - Redis là cache chính (TTL 5 phút), bảng này là persistent backup

- **`idempotency_key`**: 
  - Client sinh ra (ví dụ: UUID hoặc timestamp + random)
  - **Chống double-spend**: Nếu client retry (network timeout), hệ thống detect được
  - Ví dụ: Giao dịch với idempotency_key = "abc123" lần đầu → success
    - Lần 2 gửi = "abc123" → trả lại kết quả cũ, không tạo giao dịch mới

- **`previous_hash` & `current_hash`**: 
  - **Hash Chaining** (kiểu blockchain-lite):
    ```
    current_hash_n = SHA-256(previous_hash_{n-1} || payload_hash || client_signature)
    ```
  - **Immutable Ledger**: Không thể thay đổi giao dịch cũ mà không làm thay đổi hash của tất cả giao dịch sau
  - Bảo vệ toàn vẹn dữ liệu

- **`status`**: 
  - `pending`: Chờ xử lý
  - `completed`: Hoàn thành thành công
  - `failed`: Thất bại (ví dụ: saldo không đủ)
---

### 5. **transaction_details** - Lịch sử thay đổi trạng thái giao dịch

#### Chức năng
Lưu lịch sử mỗi lần trạng thái của giao dịch thay đổi (ví dụ: pending → processing → completed hoặc pending → failed).

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `id` | UUID | PK | Định danh duy nhất của record chi tiết |
| `transaction_id` | UUID | FK → transactions(id), NOT NULL | Tham chiếu giao dịch |
| `status_before` | VARCHAR(50) | NOT NULL | Trạng thái trước đó |
| `status_after` | VARCHAR(50) | NOT NULL | Trạng thái mới |
| `changed_by` | VARCHAR(100) | NOT NULL | Người/hệ thống thay đổi (ví dụ: "system", "admin", username) |
| `changed_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian thay đổi |
| `notes` | TEXT | | Ghi chú (ví dụ: lý do thất bại: "Insufficient balance") |

####  Các trường đáng chú ý

- **Audit trail**: Mỗi lần giao dịch thay đổi trạng thái tự động thêm 1 record vào bảng này
- **`changed_by`**: Dùng để audit ai/cái gì thay đổi (system automation vs manual intervention)
- **`notes`**: Lý do thay đổi (ví dụ: tại sao failed?)

---

### 6. **used_nonces** - Cache Nonce (Anti-replay)

#### Chức năng
Persistent backup cho Redis Nonce store. Giữ lại nonce đã sử dụng để chống replay attack, đặc biệt khi Redis restart.

#### Cấu trúc bảng

| Trường | Kiểu dữ liệu | Ràng buộc | Mô tả |
|--------|-------------|----------|-------|
| `nonce` | VARCHAR(64) | PK | Nonce value (duy nhất) |
| `used_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời gian nonce được sử dụng |
| `expires_at` | TIMESTAMPTZ | NOT NULL | Thời gian nonce hết hiệu lực (TTL: 5 phút) |

#### Các trường đáng chú ý

- **`expires_at`**: 
  - Được tính: `used_at + 5 minutes`
  - Index trên `expires_at` để dễ dàng cleanup old nonces
  - Sau khi hết hạn, nonce có thể tái sử dụng (ít nhất là 5 phút sau)

- **Dual storage**: 
  - Redis (primary cache): Nhanh, in-memory
  - PostgreSQL (persistent backup): Dùng khi Redis restart
  - Reconciliation process: Khôi phục nonce từ DB khi Redis khởi động lại

---

## Indexing Strategy

Các index đã được tạo để tối ưu hóa:

```sql
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
```