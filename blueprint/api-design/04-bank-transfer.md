# API Design: Bank Transfer

Nguồn nghiệp vụ chính:
- `blueprint/specs/04-bank-transfer.md`
- `blueprint/design.md` — Flow 3: Secure Banking Transaction
- `blueprint/database-design.md` — Bank DB: `accounts`, `transactions`, `used_nonces`

---

## 1. Mục tiêu

Cung cấp 1 endpoint để khách hàng thực hiện chuyển tiền. Bank Service xác thực ticket, chống replay, kiểm tra revocation, verify chữ ký số, kiểm tra authorization rồi ghi ACID transaction vào immutable ledger.

---

## 2. Resource và mapping database

| Resource | Bảng / Key | Vai trò |
|---|---|---|
| Tài khoản | Bank DB `accounts` | Đọc (kiểm tra), ghi (cập nhật balance) |
| Giao dịch | Bank DB `transactions` | INSERT (immutable ledger) |
| Nonce persistent | Bank DB `used_nonces` | INSERT (fallback khi Redis miss) |
| Bank audit | Bank DB `bank_audit_log` | INSERT cho transfer rejected/completed và lỗi security quan trọng |
| Ledger state | Bank DB `ledger_state` | Row lock khi append hash-chain để tránh race condition |
| Nonce cache | Redis `replay:{hash}` | SET NX EX (primary check) |
| Revocation cache | Redis `revocation:{serial}` | GET (nhanh) |
| Certificate | CA DB `certificates` | qua gRPC `VerifyCertificate`: status + validity + public key + issuer/chain |

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/bank/transfer` | `Ticket_v` scope `transfer:create` + Authenticator | Chuyển tiền giữa 2 tài khoản |

---

## 4. API chi tiết

### 4.1. POST /v1/bank/transfer

**Request:**

```json
{
  "ticket_v": "BASE64_ENCRYPTED_TICKET_V",
  "authenticator": "BASE64_ENCRYPTED_AUTHENTICATOR",
  "cipher_payload": "BASE64_ENCRYPTED_PAYLOAD",
  "iv": "BASE64_12_BYTES_IV"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `ticket_v` | string (Base64) | Có | `E_{K_v}[ID_c, cert_sn, K_{c,v}, scope, service_id, expires_at]`; `cert_sn` là serial của user/client cert do Client CA cấp |
| `authenticator` | string (Base64) | Có | `E_{K_{c,v}}[id_c, nonce, timestamp, request_id]` — AES-256-GCM |
| `cipher_payload` | string (Base64) | Có | `AES-256-GCM_{K_{c,v}}[canonical_payload ‖ client_signature]` |
| `iv` | string (Base64) | Có | IV 12 bytes (96-bit) dùng để mã hóa `cipher_payload` |

**Canonical payload** (client tạo, ký và mã hóa):

```json
{
  "from_account_id": "UUID",
  "to_account_id": "UUID",
  "amount": 5000000,
  "currency": "VND",
  "nonce": "BASE64_32_RANDOM_BYTES",
  "timestamp": 1735689600,
  "request_id": "UUID",
  "idempotency_key": "UUID",
  "scope": "transfer:create"
}
```

`client_signature` = RSA-PSS/ECDSA signature trên `SHA-256(canonical_payload_json)` bằng `privKeyRSA_c`.

**Response `200 OK`:**

```json
{
  "data": {
    "ap_rep": "BASE64_ENCRYPTED_AP_REP"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `ap_rep` | string (Base64) | `E_{K_{c,v}}[result, tx_id, nonce]` — client giải mã để xác nhận kết quả |

Sau khi giải mã AP_REP: `nonce` phải khớp nonce đã gửi, `result = "ok"`, `tx_id` là UUID của giao dịch.

**Idempotency**: nếu `idempotency_key` đã xử lý thành công trước đó, endpoint trả `200` với AP_REP của lần đầu — không ghi giao dịch mới.

Bank Service dùng CA gRPC `VerifyCertificate(cert_sn)` để lấy trạng thái certificate, validity window, issuer/chain và public key trong một response nhất quán trước khi verify chữ ký payload. Certificate chỉ được chấp nhận khi chain hợp lệ `Root CA -> Client CA -> user certificate`.

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field hoặc sai schema |
| `401` | `INVALID_TICKET` | `Ticket_v` hết hạn hoặc không giải mã được |
| `401` | `STALE_REQUEST` | Timestamp trong Authenticator ngoài ±5 phút |
| `401` | `REPLAY_DETECTED` | Nonce đã được dùng |
| `401` | `CERT_REVOKED` | Certificate đã bị thu hồi |
| `401` | `CERT_EXPIRED` | Certificate đã hết hạn |
| `401` | `INVALID_SIGNATURE` | Chữ ký số trên payload không hợp lệ hoặc public key không thuộc chain Client CA hợp lệ |
| `403` | `WRONG_SCOPE` | Scope trong Ticket_v không phải `transfer:create` |
| `403` | `FORBIDDEN` | `from_account.user_id` không khớp `ID_c` |
| `422` | `ACCOUNT_NOT_ACTIVE` | Tài khoản bị locked hoặc frozen |
| `422` | `INSUFFICIENT_FUNDS` | Số dư không đủ |
| `422` | `DAILY_LIMIT_EXCEEDED` | Vượt hạn mức chuyển khoản ngày |
| `503` | `SERVICE_UNAVAILABLE` | Bank Service hoặc CA Service không khả dụng |

---

## 6. Acceptance criteria

- Chuyển tiền hợp lệ → `200`, `ap_rep` chứa `result=ok`, `tx_id`; `from_account.balance` giảm, `to_account.balance` tăng (atomic).
- `transactions` có 1 record mới với `payload_hash`, `client_signature`, `cert_serial`, hash chain hợp lệ.
- Gọi lại với cùng `nonce` → `401 REPLAY_DETECTED`.
- Gọi lại với cùng `idempotency_key` → `200` kết quả cũ, không có record mới.
- Chuyển tiền khi số dư không đủ → `422 INSUFFICIENT_FUNDS`, balance không thay đổi.
- Chuyển tiền với cert đã revoke → `401 CERT_REVOKED`.
- Chuyển tiền sang tài khoản người khác dùng `from_account_id` không thuộc mình → `403 FORBIDDEN`.
