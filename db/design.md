# 📚 Mini_App_Banking - Database Documentation

**Ngày cập nhật:** 2024  
**Phiên bản:** 1.0

---

## 📋 Mục lục

1. [Tổng quan](#tổng-quan)
2. [Kiến trúc Database](#kiến-trúc-database)
3. [Chi tiết các bảng](#chi-tiết-các-bảng)
4. [Mối quan hệ (ER Diagram)](#mối-quan-hệ)
5. [Những điểm đáng chú ý](#những-điểm-đáng-chú-ý)
6. [Best Practices](#best-practices)

---

## 🎯 Tổng quan

Database `Mini_App_Banking` được thiết kế để hỗ trợ hệ thống ngân hàng mini với các tính năng chính:

- ✅ Quản lý người dùng và tài khoản ngân hàng
- ✅ Xử lý giao dịch chuyển khoản an toàn
- ✅ Xác thực số (PKI - Public Key Infrastructure)
- ✅ Kiểm toán không thể phủ nhận (Non-repudiation)
- ✅ Phòng chống replay attack (Nonce)
- ✅ Phòng chống double-spend (Idempotency Key)
- ✅ Hash chaining cho immutable ledger

---

## 🏗️ Kiến trúc Database

```
┌─────────────────────────────────────────────────────────┐
│               BANKING DATABASE SYSTEM                   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌──────────────┐         ┌──────────────────────┐      │
│  │    users     │◄───────►│  user_certificates   │      │
│  └──────────────┘         └──────────────────────┘      │
│         │                                               │
│         │ (1:N)                                         │
│         ▼                                               │
│  ┌──────────────┐                                       │
│  │   accounts   │                                       │
│  └──────────────┘                                       │
│         │                                               │
│         │ (1:N)                                         │
│         ▼                                               │
│  ┌──────────────────────┐                               │
│  │   transactions       │                               │
│  │ (Immutable Ledger)   │                               │
│  └──────────────────────┘                               │
│         │                                               │
│         │ (1:N)                                         │
│         ▼                                               │
│  ┌──────────────────────┐                               │
│  │ transaction_details  │                               │
│  │ (Status History)     │                               │
│  └──────────────────────┘                               │
│                                                         │
│  ┌──────────────────────┐                               │
│  │   used_nonces        │  (Anti-replay cache)          │
│  └──────────────────────┘                               │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 📊 Chi tiết các bảng

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

#### 🔍 Các trường đáng chú ý

- **`identity_number`**: Duy nhất (UNIQUE) vì mỗi người dân chỉ có một CMND/CCCD
- **`email`**: Duy nhất để sử dụng làm tài khoản login/recovery
- **`cert_serial`**: Denormalized để tra cứu nhanh chứng chỉ hiện tại mà không cần JOIN bảng user_certificates
- **`status`**: Chỉ có 2 trạng thái: `active` (hoạt động) hoặc `locked` (khóa tài khoản)

#### Ví dụ dữ liệu

```sql
INSERT INTO users (email, full_name, identity_number, date_of_birth, phone, status)
VALUES ('alice@example.com', 'Alice Nguyen', '123456789', '1990-05-15', '0901000001', 'active');
```

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

#### 🔍 Các trường đáng chú ý

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

#### Ví dụ dữ liệu

```sql
INSERT INTO accounts (user_id, account_number, balance, daily_transfer_limit, status)
VALUES (
    'a0000000-0000-0000-0000-000000000001',
    '1000000001',
    100000000,  -- 1.000.000 VND
    50000000,   -- 500.000 VND hạn mức ngày
    'active'
);
```

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

#### 🔍 Các trường đáng chú ý

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

#### Ví dụ dữ liệu

```sql
INSERT INTO user_certificates (user_id, serial_number, public_key_pem, status, not_after)
VALUES (
    'a0000000-0000-0000-0000-000000000001',
    '2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A',
    '-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----',
    'active',
    '2025-12-31'::TIMESTAMPTZ
);
```

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

#### 🔍 Các trường đáng chú ý

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

#### Ví dụ dữ liệu

```sql
INSERT INTO transactions (
    from_account_id,
    to_account_id,
    from_account_number,
    to_account_number,
    amount,
    currency,
    status,
    payload_hash,
    client_signature,
    scope,
    nonce,
    idempotency_key,
    previous_hash,
    current_hash
) VALUES (
    'acc-uuid-001',
    'acc-uuid-002',
    '1000000001',
    '1000000002',
    5000000,  -- 50.000 VND
    'VND',
    'completed',
    '3a5c...hash...',
    'MIGfMA0GCSqGSIb3...signature...',
    'transfer:internal',
    'nonce-12345',
    'idempotency-abc123',
    'prev-hash-xyz',
    'current-hash-abc'
);
```

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

#### 🔍 Các trường đáng chú ý

- **Audit trail**: Mỗi lần giao dịch thay đổi trạng thái tự động thêm 1 record vào bảng này
- **`changed_by`**: Dùng để audit ai/cái gì thay đổi (system automation vs manual intervention)
- **`notes`**: Lý do thay đổi (ví dụ: tại sao failed?)

#### Ví dụ dữ liệu

```sql
-- Transaction pending → completed
INSERT INTO transaction_details (
    transaction_id,
    status_before,
    status_after,
    changed_by,
    notes
) VALUES (
    'txn-uuid-001',
    'pending',
    'completed',
    'system',
    'Successfully processed'
);

-- Transaction pending → failed
INSERT INTO transaction_details (
    transaction_id,
    status_before,
    status_after,
    changed_by,
    notes
) VALUES (
    'txn-uuid-002',
    'pending',
    'failed',
    'system',
    'Insufficient balance in from_account'
);
```

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

#### 🔍 Các trường đáng chú ý

- **`expires_at`**: 
  - Được tính: `used_at + 5 minutes`
  - Index trên `expires_at` để dễ dàng cleanup old nonces
  - Sau khi hết hạn, nonce có thể tái sử dụng (ít nhất là 5 phút sau)

- **Dual storage**: 
  - Redis (primary cache): Nhanh, in-memory
  - PostgreSQL (persistent backup): Dùng khi Redis restart
  - Reconciliation process: Khôi phục nonce từ DB khi Redis khởi động lại

#### Ví dụ dữ liệu

```sql
INSERT INTO used_nonces (nonce, used_at, expires_at)
VALUES (
    'nonce-12345',
    NOW(),
    NOW() + INTERVAL '5 minutes'
);
```

---

## 🔗 Mối quan hệ (ER Diagram)

### Quan hệ 1:N

```
users (1) ─────── (N) accounts
  │
  │
  └──────────── (N) user_certificates

accounts (1) ─────── (N) transactions
                       │
                       └──── (N) transaction_details
```

### Chi tiết quan hệ

| Quan hệ | Từ bảng | Đến bảng | Ý nghĩa | ON DELETE |
|--------|---------|----------|---------|-----------|
| 1:N | users | accounts | 1 user có N accounts | RESTRICT |
| 1:N | users | user_certificates | 1 user có N certificates | RESTRICT |
| 1:N | accounts | transactions | 1 account có N transactions | (no FK từ account) |
| 1:N | transactions | transaction_details | 1 transaction có N details | CASCADE |

---

## ⚠️ Những điểm đáng chú ý

### 1. 💰 Tiền tệ: BIGINT thay vì DECIMAL

**Vấn đề**: DECIMAL/FLOAT có precision issues với tiền tệ
```
-- ❌ DECIMAL(19,2) - có thể có lỗi làm tròn
0.1 + 0.2 = 0.30000000000000004

-- ✅ BIGINT (cents)
10 + 20 = 30 (luôn chính xác)
```

**Giải pháp**: Lưu tiền dưới dạng **cents** (BIGINT)
- 1 VND = 1 cent
- 100.000 VND = 100000000 (BIGINT)
- Tính toán: số tiền = balance_cents / 100

### 2. 🔐 Chứng chỉ & Chữ ký số (PKI)

**Luồng xác thực**:
```
1. Client sinh key pair: (priv_c, pubKey_c)
2. CA Service cấp chứng chỉ X.509 (public_key_pem)
3. Lưu vào user_certificates
4. Khách hàng ký giao dịch: signature = Sign(Payload, priv_c)
5. Bank verify: Verify(signature, Payload, pubKey_c from cert) = True
```

**Lợi ích**:
- Non-repudiation: Khách không thể phủ nhận đã ký
- Authentication: Xác thực danh tính người ký
- Integrity: Đảm bảo payload không bị thay đổi

### 3. 🚫 Chống Replay Attack (Nonce)

**Vấn đề**: Attacker ghi lại giao dịch → gửi lại
```
TIME 1: Client → Bank: Giao dịch T (signature = X)
        Bank: Verify & process ✓
        
TIME 2: Attacker → Bank: Giao dịch T (signature = X) [copy từ lúc 1]
        Bank: ??? Verify cũng đúng... process lại???
```

**Giải pháp**: Nonce (Number used ONCE)
- Bank gửi nonce mới cho mỗi request
- Client include nonce vào Payload trước khi ký
- Verify: nonce chưa từng dùng trước đó
- Nonce hết hạn sau 5 phút

### 4. 🛡️ Chống Double-Spend (Idempotency Key)

**Vấn đề**: Client timeout → tự động retry
```
TIME 1: Client gửi giao dịch T, chờ response
        → Timeout (network lag)
        
TIME 2: Client retry giao dịch T (signature mới, vì nonce mới)
        → Tạo giao dịch T thứ 2???
```

**Giải pháp**: Idempotency Key
- Client sinh: `idempotency_key = UUID hoặc (timestamp + random)`
- Gửi cùng mỗi request
- Bank: `SELECT * FROM transactions WHERE idempotency_key = 'abc123'`
- Nếu tìm thấy → trả lại kết quả cũ (idempotent)
- Nếu không → xử lý giao dịch mới

### 5. ⛓️ Immutable Ledger (Hash Chaining)

**Ý tưởng**: Kiểu blockchain-lite
```
Transaction 1:
  current_hash_1 = SHA-256(previous_hash_0 || payload_1 || signature_1)
  
Transaction 2:
  current_hash_2 = SHA-256(current_hash_1 || payload_2 || signature_2)
  
Transaction 3:
  current_hash_3 = SHA-256(current_hash_2 || payload_3 || signature_3)
```

**Lợi ích**:
- Bảo vệ toàn vẹn: Nếu thay đổi txn 1 → current_hash_1 thay đổi → tất cả txn sau đó invalid
- Chống làm giả: Hacker muốn thay đổi txn cũ phải thay đổi tất cả txn sau

**Limitation**: Phòng chống nếu hacker có access DB và code, nhưng không phòng chống nếu hacker thay đổi **tất cả** txn (bao gồm previous_hash_0)

### 6. 📝 Denormalization: account_number trong transactions

**Tại sao**?
```sql
-- ❌ Phải JOIN để lấy account_number
SELECT t.id, a1.account_number as from_account_number, a2.account_number as to_account_number
FROM transactions t
JOIN accounts a1 ON t.from_account_id = a1.id
JOIN accounts a2 ON t.to_account_id = a2.id;

-- ✅ Lưu trực tiếp (denormalized)
SELECT t.id, t.from_account_number, t.to_account_number
FROM transactions t;
```

**Tradeoff**:
- ✅ Query nhanh (không cần JOIN)
- ✅ Lịch sử số account ngay cả khi account thay đổi
- ❌ Dữ liệu dư thừa (redundancy)

### 7. 🔄 Status CHECK constraints

```sql
-- Users
CHECK (status IN ('active', 'locked'))

-- Accounts
CHECK (status IN ('active', 'locked', 'frozen'))

-- Transactions
CHECK (status IN ('pending', 'completed', 'failed'))

-- Certificates
CHECK (status IN ('active', 'revoked', 'expired'))
```

Tác dụng: Database level validation → tránh dữ liệu invalid

### 8. 📊 Auto-update updated_at

```sql
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();
```

Mỗi lần UPDATE users/accounts → `updated_at` tự động set = NOW()

---

## ✅ Best Practices

### 1. **Truy vấn tài khoản của người dùng**

```sql
SELECT a.* 
FROM accounts a
WHERE a.user_id = $1 AND a.status = 'active';
```

### 2. **Lấy giao dịch gần đây của account**

```sql
SELECT * 
FROM transactions 
WHERE (from_account_id = $1 OR to_account_id = $1)
  AND created_at >= NOW() - INTERVAL '30 days'
ORDER BY created_at DESC
LIMIT 50;
```

### 3. **Lấy lịch sử trạng thái giao dịch**

```sql
SELECT * 
FROM transaction_details 
WHERE transaction_id = $1
ORDER BY changed_at ASC;
```

### 4. **Kiểm tra chứng chỉ còn hiệu lực**

```sql
SELECT * 
FROM user_certificates 
WHERE user_id = $1 
  AND status = 'active' 
  AND not_after > NOW();
```

### 5. **Cleanup old nonces**

```sql
-- Chạy periodically (ví dụ: mỗi 6 giờ)
DELETE FROM used_nonces 
WHERE expires_at < NOW();
```

### 6. **Cập nhật balance (atomic transaction)**

```sql
BEGIN;
  SELECT balance FROM accounts WHERE id = $1 FOR UPDATE;  -- Lock
  -- Kiểm tra balance >= amount
  UPDATE accounts SET balance = balance - $2 WHERE id = $1;
  UPDATE accounts SET balance = balance + $2 WHERE id = $3;
COMMIT;
```

### 7. **Tạo giao dịch mới**

```sql
INSERT INTO transactions (
    from_account_id, to_account_id, 
    from_account_number, to_account_number,
    amount, currency, status,
    payload_hash, client_signature, scope,
    nonce, idempotency_key,
    previous_hash, current_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, 'pending',
    $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- Sau đó, INSERT vào transaction_details
INSERT INTO transaction_details (
    transaction_id, status_before, status_after, changed_by
) VALUES ($1, null, 'pending', 'system');
```

### 8. **Cập nhật status giao dịch**

```sql
BEGIN;
  UPDATE transactions 
  SET status = 'completed', completed_at = NOW()
  WHERE id = $1;
  
  INSERT INTO transaction_details (
      transaction_id, status_before, status_after, changed_by
  ) VALUES ($1, 'pending', 'completed', 'system');
COMMIT;
```

---

## 🔍 Indexing Strategy

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

**Lợi ích**:
- ✅ Lookup user accounts: `idx_accounts_user_id`
- ✅ Lookup giao dịch: `idx_transactions_from_account`, `idx_transactions_to_account`
- ✅ Kiểm tra idempotency: `idx_transactions_idempotency_key`
- ✅ Cleanup nonces: `idx_used_nonces_expires_at`

---

## 📌 Tóm tắt

| Bảng | Chức năng | Tính chất | Quan trọng |
|------|-----------|----------|-----------|
| **users** | Khách hàng | Có thể UPDATE | Identity (identity_number UNIQUE) |
| **accounts** | Tài khoản | Immutable số dư (chỉ UPDATE từ transactions) | Balance lưu BIGINT (cents) |
| **user_certificates** | Chứng chỉ PKI | Append-only (add new, revoke cũ) | Xác thực chữ ký số |
| **transactions** | Giao dịch | Immutable Ledger (INSERT only) | Hash chaining, signature, nonce |
| **transaction_details** | Lịch sử trạng thái | Append-only | Audit trail |
| **used_nonces** | Cache nonce | Cleanup tự động | Chống replay attack |

---

**Tài liệu này được cập nhật lần cuối:** 2024  
**Phiên bản:** 1.0