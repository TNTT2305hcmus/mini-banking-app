# API Design: TGS Exchange

Nguồn nghiệp vụ chính:
- `blueprint/specs/04-tgs-exchange.md`
- `blueprint/design.md` — Flow 2: Kerberos-like Authentication (TGS Exchange)

---

## 1. Mục tiêu

Cung cấp 1 endpoint để khách hàng dùng TGT xin `Ticket_v` và `K_{c,v}` cho một scope cụ thể. `Ticket_v` sẽ được dùng để gọi các API của Bank Service.

---

## 2. Resource và mapping database

| Resource | Bảng / Key | Vai trò |
|---|---|---|
| Nonce cache | Redis `replay:{hash}` | SET NX EX — chống replay |

TGT được giải mã bằng `K_tgs` (secret của KDC) — không cần DB lookup.

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/auth/tgs-req` | TGT + Authenticator | Xin `Ticket_v` + `K_{c,v}` theo scope |

---

## 4. API chi tiết

### 4.1. POST /v1/auth/tgs-req

Khách hàng gửi TGT và Authenticator (mã hóa bằng `K_{c,tgs}`) để xin ticket cho Bank Service.

**Request:**

```json
{
  "tgt": "BASE64_ENCRYPTED_TGT",
  "authenticator": "BASE64_ENCRYPTED_AUTHENTICATOR",
  "scope": "transfer:create"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `tgt` | string (Base64) | Có | TGT opaque nhận từ AS Exchange — `E_{K_tgs}[ID_c, cert_sn, K_{c,tgs}, expires_at]` |
| `authenticator` | string (Base64) | Có | `E_{K_{c,tgs}}[id_c, nonce, timestamp, request_id]` |
| `scope` | string | Có | Scope yêu cầu; một trong: `balance:read`, `transfer:create`, `history:read` |

**Response `200 OK`:**

```json
{
  "data": {
    "tgs_rep": "BASE64_ENCRYPTED_TGS_REP"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `tgs_rep` | string (Base64) | `E_{K_{c,tgs}}[K_{c,v}, Ticket_v, nonce, scope]` — client giải mã bằng `K_{c,tgs}` |

Client verify: `nonce` và `scope` trong TGS_REP phải khớp request.

**Nội dung Authenticator** (client tạo, mã hóa bằng `K_{c,tgs}`, AES-256-GCM):

```json
{
  "id_c": "550e8400-e29b-41d4-a716-446655440000",
  "nonce": "BASE64_32_RANDOM_BYTES",
  "timestamp": 1735689600,
  "request_id": "UUID"
}
```

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field hoặc scope không hợp lệ |
| `401` | `INVALID_TICKET` | TGT không giải mã được hoặc đã hết hạn |
| `401` | `UNAUTHORIZED` | `id_c` trong Authenticator không khớp TGT |
| `401` | `STALE_REQUEST` | Timestamp trong Authenticator ngoài ±5 phút |
| `401` | `REPLAY_DETECTED` | Nonce đã được dùng trước đó |
| `403` | `WRONG_SCOPE` | Scope không thuộc tập hợp lệ |
| `503` | `SERVICE_UNAVAILABLE` | KDC không khả dụng |

---

## 6. Acceptance criteria

- Request hợp lệ với `scope=transfer:create` → `200`, `tgs_rep` giải mã được bằng `K_{c,tgs}`, chứa `Ticket_v` với scope đúng.
- Gọi lại với cùng nonce trong Authenticator → `401 REPLAY_DETECTED`.
- TGT đã hết hạn → `401 INVALID_TICKET`.
- `scope=admin:write` → `403 WRONG_SCOPE`.
- Dùng `Ticket_v` scope `balance:read` để gọi transfer endpoint → `403 WRONG_SCOPE` (enforce tại Bank Service).
