# MINI BANKING DATABASE DESIGN

## Các database được sử dụng

| Thành phần | Vai trò |
|---|---|
| CA PostgreSQL DB | Lưu toàn bộ certificate metadata, trạng thái revocation và audit log cho Admin Dashboard |
| Bank PostgreSQL DB | Lưu tài khoản, giao dịch, audit log bảo mật và immutable ledger với hash chaining |
| Redis | In-memory store cho OTP TTL, nonce replay cache, rate limit counter và revocation cache ngắn hạn |

---

## CA Database

CA DB là nguồn sự thật duy nhất cho certificate lifecycle và issuer metadata. KDC và Bank Service đều gọi CA qua gRPC để lookup, verify chain metadata và revocation check — không có bảng certificate nào ở Bank DB.

Theo kiến trúc CA mới, DB **không lưu private key** của Root CA, gRPC Transport CA hoặc Client CA. Private key nằm trong filesystem/secret store/KMS tùy môi trường. DB chỉ lưu public certificate, issuer metadata, chain, trạng thái lifecycle và audit để phục vụ CA Service, KDC/Bank lookup và Admin Dashboard.

### Bảng tóm tắt danh sách table

| Bảng | Mục đích |
|---|---|
| `ca_issuers` | Registry metadata của Root CA và Intermediate CA: gRPC Transport CA, Client CA |
| `certificates` | Registry đầy đủ mọi X.509 certificate được quản lý: issuer CA, service TLS cert, user/client cert |
| `certificate_audit_log` | Audit trail cho mọi thao tác nhạy cảm: issue, revoke, lookup |

---

### ca_issuers

- **Mục đích**: Lưu public metadata của các CA issuer trong hierarchy để Admin Dashboard và CA Service biết certificate nào được ký bởi issuer nào.
- **Không lưu**: private key, passphrase, seed, KMS secret reference có quyền ký trực tiếp.
- **Constraint**:
  - `issuer_id` UNIQUE: định danh ổn định trong code/config, ví dụ `root-ca`, `grpc-transport-ca`, `client-ca`.
  - `serial_number` và `fingerprint_sha256` UNIQUE.
  - Root CA có `parent_issuer_id = NULL`; Intermediate CA có `parent_issuer_id = 'root-ca'`.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Internal primary key |
| `issuer_id` | VARCHAR(64) | UNIQUE, NOT NULL | Stable ID: `root-ca`, `grpc-transport-ca`, `client-ca` |
| `parent_issuer_id` | VARCHAR(64) | NULL | Issuer cha; NULL với Root CA |
| `common_name` | VARCHAR(255) | NOT NULL | CN của CA certificate |
| `cert_role` | VARCHAR(32) | NOT NULL, CHECK IN ('root_ca', 'grpc_transport_ca', 'client_ca') | Vai trò trong hierarchy |
| `serial_number` | VARCHAR(128) | UNIQUE, NOT NULL | Serial của CA certificate |
| `certificate_pem` | TEXT | NOT NULL | Public CA certificate ở định dạng PEM |
| `fingerprint_sha256` | VARCHAR(64) | UNIQUE, NOT NULL | Fingerprint của CA certificate |
| `subject_key_id` | VARCHAR(128) | NULL | Subject Key Identifier, dùng trace chain |
| `authority_key_id` | VARCHAR(128) | NULL | Authority Key Identifier |
| `not_before` | TIMESTAMPTZ | NOT NULL | Thời điểm CA cert có hiệu lực |
| `not_after` | TIMESTAMPTZ | NOT NULL | Thời điểm CA cert hết hạn |
| `status` | VARCHAR(16) | NOT NULL, DEFAULT 'active', CHECK IN ('active', 'retired', 'expired', 'compromised') | Trạng thái issuer |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm ghi metadata |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Cập nhật tự động qua trigger |

**Index:**
```sql
CREATE UNIQUE INDEX idx_ca_issuers_issuer_id
    ON ca_issuers(issuer_id);
CREATE UNIQUE INDEX idx_ca_issuers_serial
    ON ca_issuers(serial_number);
CREATE UNIQUE INDEX idx_ca_issuers_fingerprint
    ON ca_issuers(fingerprint_sha256);
CREATE INDEX idx_ca_issuers_role
    ON ca_issuers(cert_role);
CREATE INDEX idx_ca_issuers_status
    ON ca_issuers(status);
```

---

### certificates

- **Mục đích**: Lưu mỗi X.509 certificate được CA Service quản lý, kèm đầy đủ metadata để Admin Dashboard có thể list/filter/detail/revoke đúng phạm vi và để KDC/Bank Service tra cứu trạng thái, public key, issuer/chain.
- **Foreign Key**: Không — CA DB tách biệt, không có FK sang hệ thống khác. `owner_id` là user ID từ Bank system, được lưu như VARCHAR tham chiếu lỏng.
- **Constraint**:
  - `serial_number` UNIQUE: mỗi certificate có serial duy nhất theo chuẩn X.509
  - `fingerprint_sha256` UNIQUE: fingerprint dùng để tra cứu nhanh
  - `(owner_id) WHERE cert_type = 'client' AND status = 'active'` partial unique index: một user chỉ có một client certificate active tại một thời điểm
  - `revoked_at IS NOT NULL` khi `status = 'revoked'`: enforce tại app layer
  - `revocation_reason IS NOT NULL` khi `action = revoke`: enforce tại app layer
  - `cert_type = 'client'` thì `owner_id`, `subject_email` và `public_key_pem` bắt buộc có giá trị
  - `cert_type IN ('root_ca', 'intermediate_ca')` thì `is_ca = true`; leaf cert thì `is_ca = false`

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Internal primary key |
| `serial_number` | VARCHAR(128) | UNIQUE, NOT NULL | Hex-encoded X.509 serial number, ví dụ: `"1a2b3c4d..."` — là key chính để KDC/Bank lookup |
| `cert_type` | VARCHAR(20) | NOT NULL, CHECK IN ('root_ca', 'intermediate_ca', 'service_tls', 'client') | Phân loại cert để tránh nhầm CA cert, service TLS cert và user cert |
| `issuer_id` | VARCHAR(64) | NULL | Stable issuer ID: `root-ca`, `grpc-transport-ca`, `client-ca`; NULL với Root CA self-signed |
| `issuer_common_name` | VARCHAR(255) | NOT NULL | CN của issuer trong certificate |
| `issuer_serial_number` | VARCHAR(128) | NULL | Serial của issuer certificate; dùng audit/trace chain |
| `owner_id` | VARCHAR(255) | NULL | User ID từ hệ thống Bank; chỉ bắt buộc với `cert_type='client'` |
| `subject_cn` | VARCHAR(255) | NOT NULL | Common Name trong X.509 Subject |
| `subject_email` | VARCHAR(255) | NULL | Email trong X.509 Subject; cho phép Admin search user/client cert theo email |
| `public_key_pem` | TEXT | NULL | Public key của leaf/client cert; KDC/Bank dùng để verify chữ ký người dùng |
| `certificate_pem` | TEXT | NOT NULL | Toàn bộ X.509 certificate ở định dạng PEM; trả về trong GetCertificate response |
| `chain_pem` | TEXT | NULL | PEM chain từ issuer lên Root CA, không chứa private key |
| `chain_fingerprints` | JSONB | NULL | Danh sách fingerprint chain, ví dụ `["client-ca", "root-ca"]` hoặc fingerprint SHA-256 |
| `fingerprint_sha256` | VARCHAR(64) | UNIQUE, NOT NULL | SHA-256 fingerprint của certificate (hex); cho phép Admin search theo fingerprint |
| `is_ca` | BOOLEAN | NOT NULL, DEFAULT false | X.509 BasicConstraints CA flag |
| `key_usage` | TEXT[] | NULL | KeyUsage đã parse, ví dụ `digitalSignature`, `keyEncipherment`, `certSign` |
| `extended_key_usage` | TEXT[] | NULL | EKU đã parse, ví dụ `serverAuth`, `clientAuth` |
| `not_before` | TIMESTAMPTZ | NOT NULL | Thời điểm certificate có hiệu lực (X.509 notBefore) |
| `not_after` | TIMESTAMPTZ | NOT NULL | Thời điểm certificate hết hạn (X.509 notAfter) |
| `status` | VARCHAR(10) | NOT NULL, DEFAULT 'active', CHECK IN ('active', 'revoked', 'expired') | Trạng thái hiện tại; `expired` được cập nhật bằng background job hoặc tại thời điểm check |
| `issued_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm CA cấp certificate |
| `revoked_at` | TIMESTAMPTZ | NULL | Thời điểm bị thu hồi; NULL nếu chưa revoke |
| `revocation_reason` | TEXT | NULL | Lý do thu hồi, bắt buộc có giá trị khi `revoked_at IS NOT NULL` |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm record được tạo trong DB |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Cập nhật tự động qua trigger khi status thay đổi |

**Index:**
```sql
CREATE UNIQUE INDEX idx_certs_serial         ON certificates(serial_number);
CREATE UNIQUE INDEX idx_certs_fingerprint    ON certificates(fingerprint_sha256);
CREATE INDEX        idx_certs_owner_id       ON certificates(owner_id);
CREATE INDEX        idx_certs_subject_email  ON certificates(subject_email);
CREATE INDEX        idx_certs_type           ON certificates(cert_type);
CREATE INDEX        idx_certs_issuer_id      ON certificates(issuer_id);
CREATE INDEX        idx_certs_status         ON certificates(status);
CREATE INDEX        idx_certs_not_after      ON certificates(not_after);
-- Enforce tối đa 1 active client cert mỗi user
CREATE UNIQUE INDEX idx_certs_one_active_per_owner
    ON certificates(owner_id)
    WHERE cert_type = 'client' AND status = 'active';
```

**Mapping theo kiến trúc CA:**

| `cert_type` | Issuer | Có thể revoke từ Admin MVP? | Ghi chú |
|---|---|---|---|
| `root_ca` | Self-signed | Không | Trust anchor cao nhất; chỉ xem metadata |
| `intermediate_ca` | Root CA | Không trong MVP | Gồm `grpc-transport-ca`, `client-ca`; rotation cần quy trình riêng |
| `service_tls` | gRPC Transport CA | Không trong MVP | Cert TLS của CA/KDC/Bank; rotation bằng script/provisioning |
| `client` | Client CA | Có | User/client certificate; đối tượng chính của Admin revoke |

**Migration delta so với schema MVP hiện tại:**

1. Thêm bảng `ca_issuers`.
2. Thêm vào `certificates`: `cert_type`, `issuer_id`, `issuer_common_name`, `issuer_serial_number`, `chain_pem`, `chain_fingerprints`, `is_ca`, `key_usage`, `extended_key_usage`.
3. Nới `owner_id`, `subject_email`, `public_key_pem` thành nullable để lưu Root/Intermediate/service TLS cert metadata; enforce bắt buộc bằng app/check constraint khi `cert_type='client'`.
4. Đổi partial unique index một active cert mỗi user thành `WHERE cert_type = 'client' AND status = 'active'`.
5. Thêm vào `certificate_audit_log`: `cert_type`, `issuer_id`; mở rộng `action` với `issuer_provisioned`, `verify_certificate`, `chain_verified`.

---

### certificate_audit_log

- **Mục đích**: Ghi lại mọi thao tác nhạy cảm liên quan đến certificate lifecycle: provision issuer metadata (`issuer_provisioned`), cấp mới (`issued`), thu hồi (`revoked`), tra cứu bởi Admin (`looked_up`), verify certificate bởi KDC/Bank (`verify_certificate`). Phục vụ Admin Dashboard và truy vết tranh chấp.
- **Foreign Key**: `serial_number` tham chiếu lỏng đến `certificates.serial_number` — không dùng FK cứng để log vẫn được ghi kể cả khi lookup thất bại.
- **Constraint**: `reason` bắt buộc có giá trị khi `action = 'revoked'` — enforce tại app layer.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Internal primary key |
| `serial_number` | VARCHAR(128) | NOT NULL | Cert serial liên quan đến thao tác; tham chiếu lỏng (không FK) |
| `cert_type` | VARCHAR(20) | NULL | Snapshot loại cert tại thời điểm audit: `client`, `service_tls`, `intermediate_ca`, `root_ca` |
| `issuer_id` | VARCHAR(64) | NULL | Snapshot issuer liên quan |
| `action` | VARCHAR(30) | NOT NULL, CHECK IN ('issuer_provisioned', 'issued', 'revoked', 'looked_up', 'verify_certificate', 'chain_verified') | Loại thao tác được thực hiện |
| `performed_by` | VARCHAR(255) | NOT NULL | Định danh caller: `'admin:admin@bank.com'`, `'system:kdc-service'`, `'system:bank-service'` |
| `reason` | TEXT | NULL | Lý do; bắt buộc khi `action = 'revoked'`; NULL cho các action khác |
| `performed_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm thao tác được thực hiện |
| `metadata` | JSONB | NULL | Thông tin bổ sung: request_id, IP address (admin actions), cert_owner_id |

**Index:**
```sql
CREATE INDEX idx_audit_serial       ON certificate_audit_log(serial_number);
CREATE INDEX idx_audit_performed_at ON certificate_audit_log(performed_at DESC);
CREATE INDEX idx_audit_action       ON certificate_audit_log(action);
CREATE INDEX idx_audit_cert_type    ON certificate_audit_log(cert_type);
CREATE INDEX idx_audit_issuer_id    ON certificate_audit_log(issuer_id);
```

---

## BANK Database

Bank DB là MVP — chỉ đủ đáp ứng yêu cầu cơ bản của proposal: quản lý tài khoản, giao dịch ACID, audit log bảo mật, immutable ledger với hash chaining và chống replay. Không có bảng certificate (delegate CA qua gRPC).

### Bảng tóm tắt danh sách table

| Bảng | Mục đích |
|---|---|
| `users` | Thông tin cơ bản của khách hàng trong hệ thống Bank |
| `accounts` | Tài khoản ngân hàng với số dư và hạn mức |
| `transactions` | Immutable ledger các giao dịch chuyển khoản với hash chaining và chữ ký số |
| `used_nonces` | Persistent backup cho Redis nonce replay cache |
| `bank_audit_log` | Audit trail cho security event và lifecycle giao dịch quan trọng trong Bank Service |
| `ledger_state` | Trạng thái đầu chuỗi hash-chain, dùng để serialize append ledger |

---

### users

- **Mục đích**: Lưu thông tin khách hàng trong phạm vi Bank Service. Không lưu certificate — revocation check được gọi qua CA Service gRPC với `cert_serial` lấy từ Ticket_v.
- **Foreign Key**: Không.
- **Constraint**: `email` UNIQUE — là định danh chính của người dùng trong Bank system.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | User ID dùng làm `owner_id` trong CA DB và `ID_c` trong ticket |
| `email` | VARCHAR(255) | UNIQUE, NOT NULL | Email đăng ký; cũng là định danh trong PKI enrollment |
| `full_name` | VARCHAR(255) | NOT NULL | Tên đầy đủ |
| `status` | VARCHAR(10) | NOT NULL, DEFAULT 'active', CHECK IN ('active', 'locked') | Trạng thái tài khoản: active / locked |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm đăng ký |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Cập nhật tự động qua trigger |

---

### accounts

- **Mục đích**: Tài khoản ngân hàng, mỗi user có thể có nhiều tài khoản. `balance` lưu dạng cents (BIGINT) để tránh floating-point precision.
- **Foreign Key**: `user_id` → `users(id)` ON DELETE RESTRICT.
- **Constraint**: `balance >= 0`; `account_number` UNIQUE.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Account ID, dùng trong `from_account_id` / `to_account_id` của transaction |
| `user_id` | UUID | FK → users(id) ON DELETE RESTRICT, NOT NULL | Chủ sở hữu tài khoản |
| `account_number` | VARCHAR(30) | UNIQUE, NOT NULL | Số tài khoản hiển thị cho người dùng |
| `balance` | BIGINT | NOT NULL, DEFAULT 0, CHECK >= 0 | Số dư hiện tại tính bằng cents; 100.000 VND = 10.000.000 cents |
| `daily_transfer_limit` | BIGINT | NOT NULL, DEFAULT 50000000 | Hạn mức chuyển khoản trong ngày (cents); Bank Service kiểm tra trước khi ghi ledger |
| `currency` | VARCHAR(3) | NOT NULL, DEFAULT 'VND' | Mã tiền tệ ISO 4217 |
| `status` | VARCHAR(10) | NOT NULL, DEFAULT 'active', CHECK IN ('active', 'locked', 'frozen') | active: bình thường; locked: bị khóa; frozen: đóng băng do fraud |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Ngày mở tài khoản |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Cập nhật tự động qua trigger |

**Index:**
```sql
CREATE INDEX idx_accounts_user_id ON accounts(user_id);
```

---

### transactions

- **Mục đích**: Immutable ledger — chỉ INSERT, không UPDATE/DELETE. Mỗi record lưu chữ ký số của client (`client_signature`) để chống chối bỏ, và hash chain (`previous_hash`, `current_hash`) để chống sửa lịch sử.
- **Foreign Key**: `from_account_id` và `to_account_id` → `accounts(id)`; nullable để hỗ trợ giao dịch hệ thống (seed/credit) không có from.
- **Constraint**:
  - `amount > 0`: không cho chuyển 0 hoặc âm
  - `from_account_id != to_account_id`: không tự chuyển cho mình
  - `nonce` UNIQUE: chống replay attack tại DB layer (Redis là cache chính)
  - `idempotency_key` UNIQUE: chống double-spend khi client retry

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Transaction ID |
| `from_account_id` | UUID | FK → accounts(id), NULL | Tài khoản gửi; NULL cho giao dịch hệ thống |
| `to_account_id` | UUID | FK → accounts(id), NULL | Tài khoản nhận |
| `from_account_number` | VARCHAR(30) | NULL | Snapshot số tài khoản gửi tại thời điểm giao dịch (denormalized để hiển thị lịch sử) |
| `to_account_number` | VARCHAR(30) | NULL | Snapshot số tài khoản nhận |
| `amount` | BIGINT | NOT NULL, CHECK > 0 | Số tiền chuyển (cents) |
| `currency` | VARCHAR(3) | NOT NULL, DEFAULT 'VND' | Mã tiền tệ |
| `status` | VARCHAR(10) | NOT NULL, DEFAULT 'pending', CHECK IN ('pending', 'completed', 'failed') | Trạng thái; chỉ cập nhật 1 lần (pending → completed/failed) |
| `description` | TEXT | NULL | Nội dung chuyển khoản do client gửi |
| `payload_hash` | VARCHAR(64) | NOT NULL | SHA-256 hex của canonical payload gốc trước khi ký; Bank Service verify lại sau khi giải mã |
| `client_signature` | TEXT | NOT NULL | Chữ ký số RSA-PSS/ECDSA của client trên canonical payload; bằng chứng chống chối bỏ |
| `cert_serial` | VARCHAR(128) | NOT NULL | Serial của cert dùng để ký giao dịch (lấy từ Ticket_v); dùng cho audit sau này |
| `scope` | VARCHAR(50) | NOT NULL | Scope từ Ticket_v, ví dụ `transfer:create`; Bank Service verify trước khi xử lý |
| `nonce` | VARCHAR(255) | UNIQUE, NOT NULL | Nonce từ AP_REQ Authenticator; UNIQUE constraint là fallback khi Redis miss |
| `idempotency_key` | VARCHAR(255) | UNIQUE, NOT NULL | Client-generated key; nếu retry với cùng key thì trả kết quả cũ, không ghi mới |
| `previous_hash` | VARCHAR(64) | NULL | SHA-256 của `current_hash` của giao dịch trước trong ledger; NULL cho giao dịch đầu tiên |
| `current_hash` | VARCHAR(64) | NULL | `SHA-256(previous_hash \|\| payload_hash \|\| client_signature \|\| id \|\| created_at)` |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm record được tạo |
| `completed_at` | TIMESTAMPTZ | NULL | Thời điểm giao dịch kết thúc (completed hoặc failed) |

**Index:**
```sql
CREATE INDEX idx_txn_from_account    ON transactions(from_account_id);
CREATE INDEX idx_txn_to_account      ON transactions(to_account_id);
CREATE INDEX idx_txn_created_at      ON transactions(created_at DESC);
CREATE UNIQUE INDEX idx_txn_nonce    ON transactions(nonce);
CREATE UNIQUE INDEX idx_txn_idem_key ON transactions(idempotency_key);
```

---

### used_nonces

- **Mục đích**: Persistent backup cho Redis nonce replay cache. Dùng khi Redis restart để tránh replay window. Redis là primary (fast path); bảng này là safety net.
- **Foreign Key**: Không.
- **Constraint**: `nonce` là PK — đảm bảo unique tuyệt đối.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `nonce` | VARCHAR(255) | PK | Nonce value (hash của nonce + request context) |
| `used_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm nonce được ghi nhận |
| `expires_at` | TIMESTAMPTZ | NOT NULL | Thời điểm nonce hết hiệu lực (`used_at + 5 minutes`); dùng để cleanup job |

**Index:**
```sql
CREATE INDEX idx_used_nonces_expires_at ON used_nonces(expires_at);
```

---

### bank_audit_log

- **Mục đích**: Ghi các sự kiện bảo mật và nghiệp vụ quan trọng trong Bank Service: transfer completed/rejected, replay detected, invalid signature, revoked certificate, forbidden ownership, insufficient funds. Bảng này bổ sung audit cho các request bị reject trước khi có record trong `transactions`.
- **Foreign Key**: Không dùng FK cứng để audit vẫn ghi được kể cả khi request fail trước khi xác định đủ account/transaction.
- **Constraint**: `action` nằm trong tập giá trị chuẩn; `request_id` nên có giá trị khi caller cung cấp.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | UUID | PK, DEFAULT uuid_generate_v4() | Internal primary key |
| `action` | VARCHAR(40) | NOT NULL, CHECK IN ('transfer_completed', 'transfer_rejected', 'replay_detected', 'invalid_signature', 'certificate_rejected', 'forbidden_ownership', 'insufficient_funds') | Loại sự kiện |
| `user_id` | UUID | NULL | User ID nếu đã xác định được từ `Ticket_v` |
| `account_id` | UUID | NULL | Account liên quan nếu có |
| `transaction_id` | UUID | NULL | Transaction liên quan nếu đã tạo |
| `cert_serial` | VARCHAR(128) | NULL | Certificate serial liên quan đến request |
| `request_id` | VARCHAR(64) | NULL | Request ID từ authenticator/header |
| `reason` | TEXT | NULL | Lý do reject hoặc metadata ngắn |
| `metadata` | JSONB | NULL | Thông tin bổ sung không chứa key material hoặc raw ticket |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm ghi audit |

**Index:**
```sql
CREATE INDEX idx_bank_audit_created_at ON bank_audit_log(created_at DESC);
CREATE INDEX idx_bank_audit_action     ON bank_audit_log(action);
CREATE INDEX idx_bank_audit_user_id    ON bank_audit_log(user_id);
CREATE INDEX idx_bank_audit_request_id ON bank_audit_log(request_id);
```

---

### ledger_state

- **Mục đích**: Lưu `last_hash` hiện tại của immutable ledger để Bank Service có thể lock một row bằng `SELECT ... FOR UPDATE` trong DB transaction. Cơ chế này tránh hai giao dịch đồng thời cùng đọc một `previous_hash` và làm rẽ nhánh hash-chain.
- **Foreign Key**: Không.
- **Constraint**: MVP chỉ cần một row `id = 'main'`.

| Field | Kiểu | Ràng buộc chính | Ý nghĩa |
|---|---|---|---|
| `id` | VARCHAR(20) | PK | Tên ledger, mặc định `'main'` |
| `last_hash` | VARCHAR(64) | NOT NULL, DEFAULT 'genesis' | Hash cuối cùng đã commit trong chain |
| `last_transaction_id` | UUID | NULL | Transaction cuối cùng tương ứng với `last_hash` |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Thời điểm ledger state cập nhật |

**Khởi tạo:**
```sql
INSERT INTO ledger_state(id, last_hash)
VALUES ('main', 'genesis')
ON CONFLICT (id) DO NOTHING;
```

**Append rule:**
```sql
-- Trong cùng DB transaction với balance update và transaction insert:
SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE;
-- Tính current_hash từ last_hash + canonical payload metadata
UPDATE ledger_state
SET last_hash = $current_hash,
    last_transaction_id = $tx_id,
    updated_at = NOW()
WHERE id = 'main';
```

---

## ADMIN AUTH DATA MODEL

Trong MVP, Admin Auth dùng credential demo cấu hình tại API Gateway qua env/config và cấp JWT ngắn hạn cho Dashboard. Không tạo bảng admin trong CA DB hoặc Bank DB để tránh trộn dữ liệu quản trị ứng dụng với certificate lifecycle hoặc dữ liệu ngân hàng.

Admin Dashboard là control plane, không phải một CA hoặc Intermediate CA. Dashboard đọc metadata từ `ca_issuers` và `certificates`, revoke user/client cert khi có quyền phù hợp, nhưng không được truy cập private key, không ký certificate và không rotate Root/Intermediate CA trực tiếp trong MVP.

Nếu mở rộng production, tạo datastore riêng cho Admin Auth, tối thiểu gồm:

| Bảng | Mục đích |
|---|---|
| `admin_users` | Lưu admin identity, password hash, role/scope và trạng thái |
| `admin_sessions` | Lưu refresh/session metadata nếu không dùng JWT stateless hoàn toàn |

Các bảng production này thuộc auth/admin domain riêng, không thuộc CA DB hoặc Bank DB trong MVP.

---

## REDIS

Redis là in-memory store cho dữ liệu ngắn hạn. Dưới đây là các key pattern thực tế.

### OTP — xác minh email khi đăng ký

```
Key:   otp:{email}
Value: "482910"        (6-digit OTP)
TTL:   300 seconds     (5 phút, dùng 1 lần)

Ví dụ:
  SET otp:alice@example.com "482910" EX 300
  GET otp:alice@example.com   → "482910"
  DEL otp:alice@example.com   (sau khi verify thành công)
```

### Nonce Replay Cache — chống replay attack

```
Key:   replay:{SHA-256(ID_c + nonce + timestamp + service_id + request_id)}
Value: "1"
TTL:   300 seconds     (5 phút — đủ bao freshness window)

Sử dụng SET NX EX để atomic check-and-set:
  SET replay:a3f1... "1" NX EX 300
  → OK    (nonce chưa dùng — cho phép request)
  → nil   (nonce đã tồn tại — reject replay)
```

### Rate Limit Counter — chống spam OTP

```
Key:   rate:otp_request:{ip_address}
Value: "3"             (số lần gọi trong window hiện tại)
TTL:   60 seconds      (reset sau 1 phút)

Sử dụng INCR + EXPIRE:
  INCR rate:otp_request:203.0.113.1   → 1 (lần đầu)
  EXPIRE rate:otp_request:203.0.113.1 60
  INCR rate:otp_request:203.0.113.1   → 2
  INCR rate:otp_request:203.0.113.1   → 3
  → reject nếu value > 5
```

### Revocation Cache — cache trạng thái cert ngắn hạn

```
Key:   revocation:{serial_number}
Value: "active" | "revoked" | "expired"
TTL:   60 seconds      (TTL ngắn — chấp nhận 60s stale; CA DB là nguồn chính xác)

Ví dụ:
  SET revocation:1a2b3c4d "active"  EX 60
  SET revocation:5e6f7a8b "revoked" EX 60
```

Cache này áp dụng cho certificate leaf được KDC/Bank kiểm tra ở runtime, chủ yếu là `cert_type='client'`. Service TLS cert được kiểm tra bằng TLS trust bundle; Root/Intermediate CA status được quản lý bằng issuer metadata và quy trình rotation riêng, không dùng key `revocation:{serial}` trong đường nóng.
