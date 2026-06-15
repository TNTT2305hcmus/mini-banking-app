# BANK SERVICE — LUỒNG CHI TIẾT

Tài liệu tổng hợp mọi luồng có **Bank Service** tham gia, trích từ:
- `blueprint/specs/01-otp-pki-registration.md`, `04-bank-transfer.md`, `05-bank-balance-history.md`
- `blueprint/api-design/` (các file tương ứng + `base-api.md`)
- `blueprint/database-design.md`

---

## 0. Vai trò của Bank Service

Bank Service là **service đích (verifier `v`)** trong mô hình Kerberos-style. Đặc điểm:

- **Không lưu certificate** — delegate toàn bộ certificate lifecycle cho CA Service qua gRPC `VerifyCertificate`. Không có bảng cert trong Bank DB.
- Sở hữu **khóa dài hạn `K_v`** để giải mã `Ticket_v` do KDC cấp.
- Mọi request từ client đi qua **API Gateway** (REST → gRPC); client **không** gọi trực tiếp Bank Service.
- Cấp gRPC method: `CreateUser` (enrollment), `TransferMoney` (AP exchange), `QueryBalance`, `QueryTransactions`.

**DB / cache Bank Service đụng tới:**

| Thành phần | Bảng / Key | Vai trò |
|---|---|---|
| Bank DB | `users` | INSERT khi enrollment thành công |
| Bank DB | `accounts` | SELECT (check), UPDATE balance (ACID) |
| Bank DB | `transactions` | INSERT immutable ledger |
| Bank DB | `used_nonces` | INSERT (fallback replay khi Redis restart) |
| Bank DB | `bank_audit_log` | INSERT security/business event |
| Bank DB | `ledger_state` | SELECT ... FOR UPDATE (serialize hash-chain) |
| Redis | `replay:{nonce_hash}` | SET NX EX (primary replay check) |
| Redis | `revocation:{serial}` | GET (revocation cache, TTL 60s) |
| CA Service | `certificates` (qua gRPC) | `VerifyCertificate`: status + validity + public key |

---

## 1. Luồng đăng ký user — `Bank.CreateUser` (PKI Enrollment)

Bank Service tham gia ở **bước cuối** của luồng đăng ký. Đăng ký gồm 2 pha: OTP (do API Gateway xử lý) và PKI Enrollment (CA cấp cert, rồi Bank tạo user).

### 1.1. Endpoint liên quan

| Method | Endpoint | Bank tham gia? |
|---|---|---|
| `POST` | `/v1/otp/request` | Không |
| `POST` | `/v1/otp/verify` | Không |
| `POST` | `/v1/pki/register` | **Có** — bước tạo user record |

### 1.2. Luồng chi tiết

**Pha 1 — OTP (API Gateway + Redis, Bank chưa tham gia):**

1. Client `POST /otp/request {email}`.
2. API Gateway check rate limit Redis `rate:otp_request:{ip}` (≤ 5 lần/phút), sinh OTP 6 số, `SET otp:{email} <otp> EX 300`, gửi email.
3. Client `POST /otp/verify {email, otp}`.
4. API Gateway `GET otp:{email}` so khớp → `DEL otp:{email}`, cấp `registration_token` (JWT dùng 1 lần, TTL 10 phút).

**Pha 2 — PKI Enrollment (CA cấp cert → Bank tạo user):**

5. Client sinh RSA key pair (WebCrypto, `extractable: false`), tạo CSR ký proof-of-possession, lưu wrapped private key vào IndexedDB.
6. Client `POST /pki/register {csr_pem, registration_token}`.
7. API Gateway verify `registration_token` (chữ ký, chưa dùng, chưa hết hạn).
8. API Gateway gọi CA gRPC `RegisterUser(csr_pem, user_id)`.
9. CA verify CSR proof-of-possession → ký X.509 bằng `privKeyRSA_ca` → INSERT `certificates` + `certificate_audit_log` (action=`issued`) → trả `certificate_pem`, `serial_number`, `not_after_unix`.
10. **API Gateway gọi `Bank.CreateUser(user_id, email, full_name)`** (gRPC).
11. **Bank Service INSERT vào `users`** (`email` UNIQUE, `status='active'`); có thể đồng thời khởi tạo `accounts` tùy implementation.
12. Nếu `CreateUser` thành công → API Gateway trả `201 { certificate_pem, serial_number, not_after }`; client lưu cert vào IndexedDB cạnh wrapped key.

### 1.3. Bù trừ khi lỗi (compensating action)

- Nếu **`Bank.CreateUser` thất bại sau khi CA đã cấp cert** → API Gateway gọi CA revoke/mark cert vừa cấp với `reason = enrollment_failed`, rồi trả `503 SERVICE_UNAVAILABLE`.
- Mục đích: **không để tồn tại certificate `active` mà không có Bank user tương ứng**.

### 1.4. Ràng buộc

- User record Bank DB **chỉ được tạo sau khi CA xác nhận cấp cert thành công** (không tạo trước).
- `email` là UNIQUE — định danh chính của user trong Bank system; `users.id` chính là `owner_id` trong CA DB và `ID_c` trong ticket.

### 1.5. Kịch bản lỗi

| Tình huống | HTTP |
|---|---|
| User đã có active certificate | 409 `ACTIVE_CERT_EXISTS` (CA reject) |
| CA Service không khả dụng | 503 (không tạo user) |
| Bank Service không khả dụng sau khi cert đã cấp | 503 + revoke cert (`enrollment_failed`) |

---

## 2. Tiền đề chung cho mọi luồng nghiệp vụ — `Ticket_v` từ KDC

Bank không tham gia bước này nhưng đây là nguồn tin cậy của mọi request nghiệp vụ:

- Trong **TGS Exchange** (`POST /auth/tgs-req`), KDC tạo:
  `Ticket_v = E_{K_v}[ID_c, cert_sn, K_{c,v}, scope, service_id="bank", issued_at, expires_at]`.
- Mỗi `Ticket_v` chứa **đúng 1 scope** (`transfer:create` / `balance:read` / `history:read`), TTL 5–10 phút, reusable trong TTL.
- **Bank verify scope độc lập từ trong ticket**, không tin scope gửi kèm request.
- Mỗi AP request phải có **nonce + timestamp riêng** dù dùng lại cùng ticket.

---

## 3. Luồng chuyển tiền — `Bank.TransferMoney` (AP Exchange)

`POST /v1/bank/transfer` — luồng nghiệp vụ chính. Nguyên tắc **fail-closed**: bất kỳ bước nào trước khi ghi ledger fail → reject, **không** mở DB transaction.

### 3.1. Chuẩn bị phía client

1. Client nhập `from_account_id`, `to_account_id`, `amount`, `description`, PIN.
2. Sinh `nonce3`, `ts3`, `request_id3`, `idempotency_key` (UUID).
3. Tạo canonical payload `{from_account_id, to_account_id, amount, currency, nonce3, ts3, request_id3, idempotency_key, scope}`.
4. Unwrap private key từ IndexedDB bằng PIN → ký payload → `client_signature`; `payload_hash = SHA-256(canonical_payload)`.
5. Tạo Authenticator `E_{K_{c,v}}[ID_c, nonce3, ts3, request_id3]`.
6. Mã hóa `{canonical_payload, client_signature}` bằng AES-256-GCM với `K_{c,v}` + IV ngẫu nhiên → `CipherPayload`.
7. Gửi `{ticket_v, authenticator, cipher_payload, iv}`.

### 3.2. Pipeline xác thực tại Bank Service (theo thứ tự, fail-closed)

| # | Bước | Fail → |
|---|---|---|
| 1 | Giải mã `Ticket_v` bằng `K_v` → `ID_c, cert_sn, K_{c,v}, scope, expires_at` | 401 `INVALID_TICKET` |
| 2 | Check `scope = 'transfer:create'` và `expires_at > now` | 403 `WRONG_SCOPE` / 401 |
| 3 | Giải mã Authenticator bằng `K_{c,v}` → `nonce3, ts3` | 401 |
| 4 | Freshness: `\|now − ts3\| ≤ 5 phút` | 401 `STALE_REQUEST` |
| 5 | Replay: Redis `SET NX EX` trên `replay:{hash}` + INSERT `used_nonces` | 401 `REPLAY_DETECTED` |
| 6 | Idempotency: nếu `idempotency_key` đã có trong `transactions` → trả kết quả cũ | 200/422 (kết quả cũ) |
| 7 | Revocation: CA gRPC `VerifyCertificate(cert_sn)` → `status`, validity, `pubKeyRSA_c` | 401 `CERT_REVOKED`/`CERT_EXPIRED`, CA down → 503 |
| 8 | Giải mã `CipherPayload` bằng `K_{c,v}` → `canonical_payload`, `client_signature` | 401 |
| 9 | Verify `client_signature` trên payload bằng `pubKeyRSA_c` | 401 `INVALID_SIGNATURE` |
| 10 | Ownership: `from_account.user_id == ID_c` | 403 `FORBIDDEN` |
| 11 | Business rules: account active; `balance ≥ amount`; `daily_used + amount ≤ daily_transfer_limit` | 422 `ACCOUNT_NOT_ACTIVE` / `INSUFFICIENT_FUNDS` / `DAILY_LIMIT_EXCEEDED` |

### 3.3. Ghi ledger (ACID transaction — chỉ khi mọi bước pass)

```
BEGIN
  UPDATE accounts SET balance = balance - amount WHERE id = from_account_id
  UPDATE accounts SET balance = balance + amount WHERE id = to_account_id
  SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE   -- serialize hash-chain
  previous_hash := last_hash (hoặc 'genesis')
  current_hash := SHA-256(previous_hash || payload_hash || client_signature || tx_id || created_at)
  INSERT INTO transactions (...)                                    -- immutable, chỉ INSERT
  UPDATE ledger_state SET last_hash = current_hash, last_transaction_id = tx_id
  INSERT INTO bank_audit_log (action='transfer_completed', ...)
COMMIT
```

### 3.4. Phản hồi (mutual auth)

- Bank tạo **AP_REP** `= E_{K_{c,v}}[result="ok", tx_id, nonce3]`.
- Trả `200 { data: { ap_rep }, meta }`.
- Client giải mã AP_REP, xác nhận `nonce3` khớp + `result = "ok"`, rồi xóa PIN & plaintext private key khỏi RAM.

### 3.5. Error catalog

| HTTP | Code |
|---|---|
| 400 | `BAD_REQUEST` |
| 401 | `INVALID_TICKET`, `STALE_REQUEST`, `REPLAY_DETECTED`, `CERT_REVOKED`, `CERT_EXPIRED`, `INVALID_SIGNATURE` |
| 403 | `WRONG_SCOPE`, `FORBIDDEN` |
| 422 | `ACCOUNT_NOT_ACTIVE`, `INSUFFICIENT_FUNDS`, `DAILY_LIMIT_EXCEEDED` |
| 503 | `SERVICE_UNAVAILABLE` (Bank hoặc CA down — fail-closed) |

---

## 4. Luồng xem số dư — `Bank.QueryBalance`

`POST /v1/bank/accounts/{account_id}/balance/query` — read-only, scope `balance:read`.

1. Client đã có `Ticket_v` scope `balance:read` + `K_{c,v}`; sinh `nonce, ts, request_id`.
2. Tạo Authenticator `E_{K_{c,v}}[ID_c, nonce, ts, request_id]`; gửi `{ticket_v, authenticator}`.
3. Bank giải mã `Ticket_v` → check `scope = 'balance:read'` + TTL.
4. Giải mã Authenticator → freshness + replay.
5. CA `VerifyCertificate(cert_sn)` (revocation/validity).
6. **Ownership**: `account.user_id == ID_c`.
7. Trả `200 { account_id, account_number, balance, currency, status }`.

Không ghi ledger, không AP_REP mã hóa.

**Lỗi:** `INVALID_TICKET` (401), `WRONG_SCOPE` (403), `CERT_REVOKED` (401), `STALE_REQUEST`/`REPLAY_DETECTED` (401), `FORBIDDEN` (403, account người khác), `NOT_FOUND` (404), `SERVICE_UNAVAILABLE` (503).

---

## 5. Luồng xem lịch sử giao dịch — `Bank.QueryTransactions`

`POST /v1/bank/accounts/{account_id}/transactions/query` — read-only, scope `history:read`.

1–5. Giống luồng số dư nhưng check `scope = 'history:read'`.
6. **Ownership**: `account.user_id == ID_c`.
7. Query:
   `SELECT ... FROM transactions WHERE from_account_id = account_id OR to_account_id = account_id ORDER BY created_at DESC LIMIT $limit OFFSET $offset`
   (phân trang: `limit` default 20, max 100; `offset` default 0).
8. Trả danh sách transaction metadata + `pagination { total, limit, offset }`.

**Quan trọng:** response **không** trả `client_signature` và `payload_hash` (dữ liệu audit nội bộ).

**Lỗi:** tương tự luồng số dư; dùng sai scope (vd `balance:read` gọi endpoint history) → `403 WRONG_SCOPE`.

---

## 6. Đặc tính bảo mật chung của Bank Service

- **Scope kiểm tra chặt, không hoán đổi**: `balance:read` không gọi được `/transactions/query` (→ `WRONG_SCOPE`).
- **Ownership bắt buộc**: ticket hợp lệ vẫn không xem/chuyển được tài khoản người khác (`account.user_id == ID_c`).
- **Immutable ledger + hash chaining**: `transactions` chỉ INSERT; sửa giao dịch cũ làm vỡ chain.
- **Ledger concurrency**: `SELECT ... FOR UPDATE` trên `ledger_state` chống hai transfer cùng dùng một `previous_hash`.
- **Idempotency**: chỉ áp dụng cho `transfer` (UNIQUE `idempotency_key`); retry cùng key → kết quả cũ, không ghi mới.
- **Replay**: Redis là primary cache; `used_nonces` (UNIQUE) là fallback khi Redis restart.
- **Delegate cert cho CA**: Bank không có bảng cert; revocation cache Redis TTL 60s (chấp nhận stale ngắn, CA là nguồn sự thật).
- **Fail-closed**: CA down hoặc bất kỳ check nào fail trước ghi ledger → reject.
- **Không information leakage**: response lỗi không trả key material, nội dung ticket, hay lý do nội bộ chi tiết; auth failure không phân biệt nguyên nhân.
- **Security audit**: replay, invalid signature, revoked/expired cert, forbidden ownership, insufficient funds, transfer completed/rejected → ghi `bank_audit_log`.
