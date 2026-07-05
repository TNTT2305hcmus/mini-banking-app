# API Design: AS Exchange

Nguồn nghiệp vụ chính:
- `blueprint/specs/02-as-exchange.md`
- `blueprint/design.md` — Flow 2: Kerberos-like Authentication (AS Exchange)

---

## 1. Mục tiêu

Cung cấp 1 endpoint để khách hàng thực hiện AS Exchange: xác thực với KDC bằng chữ ký số, nhận TGT và `K_{c,tgs}` được mã hóa.

---

## 2. Resource và mapping database

| Resource | Bảng / Key | Vai trò |
|---|---|---|
| Certificate lookup | CA DB `certificates` | KDC lấy public key, issuer/chain và trạng thái để verify signature |
| Audit | CA DB `certificate_audit_log` | action='looked_up' |
| Nonce cache | Redis `replay:{hash}` | SET NX EX — chống replay |

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/auth/as-req` | Chữ ký số AS_REQ | Xác thực với KDC, nhận TGT + `K_{c,tgs}` |

---

## 4. API chi tiết

### 4.1. POST /v1/auth/as-req

Khách hàng ký AS_REQ payload bằng private key, gửi lên KDC để xác thực và nhận TGT.

**Request:**

```json
{
  "id_c": "550e8400-e29b-41d4-a716-446655440000",
  "cert_sn": "1a2b3c4d5e6f",
  "nonce": "dGhpcyBpcyBhIG5vbmNl",
  "timestamp": 1735689600,
  "request_id": "7f3e8b20-1c4d-4a5e-9f2a-123456789abc",
  "signature": "MEYCIQDx..."
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `id_c` | string (UUID) | Có | User ID của khách hàng |
| `cert_sn` | string | Có | Serial number (hex) của X.509 client certificate do Client CA cấp |
| `nonce` | string (Base64) | Có | Random 32 bytes, Base64 encoded |
| `timestamp` | int64 | Có | Unix timestamp (giây) lúc tạo request; phải trong ±5 phút so với server time |
| `request_id` | string (UUID) | Có | UUID duy nhất cho request này |
| `signature` | string (Base64) | Có | RSA-PSS/ECDSA signature trên canonical payload `{id_c, cert_sn, nonce, timestamp, request_id}` |

**Canonical payload để ký**: JSON serialized, keys theo thứ tự alphabet, không có space.
```
{"cert_sn":"...","id_c":"...","nonce":"...","request_id":"...","timestamp":...}
```

**Response `200 OK`:**

```json
{
  "data": {
    "as_rep": "BASE64_ENCRYPTED_AS_REP"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `as_rep` | string (Base64) | `E_{pubKeyRSA_c}[K_{c,tgs}, TGT, nonce]` — client giải mã bằng private key; chứa TGT (opaque) và `K_{c,tgs}` |

Client verify response: sau khi giải mã, `nonce` trong AS_REP phải khớp `nonce` đã gửi.

KDC chỉ chấp nhận certificate nếu CA Service trả về trạng thái active, còn hiệu lực và chain hợp lệ `Root CA -> Client CA -> user certificate`. Public key dùng để verify `signature` phải lấy từ certificate đã qua kiểm tra chain này.

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field hoặc sai schema |
| `401` | `UNAUTHORIZED` | Certificate không tồn tại, revoked, expired |
| `401` | `INVALID_SIGNATURE` | Signature AS_REQ không hợp lệ |
| `401` | `STALE_REQUEST` | Timestamp ngoài ±5 phút |
| `401` | `REPLAY_DETECTED` | Nonce đã được dùng trước đó |
| `503` | `SERVICE_UNAVAILABLE` | KDC hoặc CA Service không khả dụng |

---

## 6. Acceptance criteria

- Request hợp lệ → `200`, `as_rep` giải mã được bằng private key, chứa TGT + `K_{c,tgs}`, nonce khớp.
- Gọi lại với cùng `nonce` → `401 REPLAY_DETECTED`.
- Timestamp cách server time > 5 phút → `401 STALE_REQUEST`.
- `cert_sn` của cert đã revoke hoặc chain không hợp lệ → `401 UNAUTHORIZED`.
- Signature sai (thay đổi 1 byte) → `401 INVALID_SIGNATURE`.
