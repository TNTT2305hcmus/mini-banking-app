# Mini Banking — Base API Design

Tài liệu này mô tả quy ước chung cho toàn bộ REST API của Mini Banking.

---

## 1. Quy ước chung cho tất cả API

### 1.1. Base URL và versioning

| Môi trường | Base URL | Ghi chú |
|---|---|---|
| Local development | `http://localhost:3000/v1` | API Gateway chạy port 3000 |
| Docker Compose | `http://api-gateway:3000/v1` | Tên service trong Docker network |

Versioning nằm trong path (`/v1`). Khi có breaking change, tạo prefix mới (`/v2`).

---

### 1.2. Giao thức và định dạng dữ liệu

- **Giao thức**: HTTPS (production) / HTTP (local demo).
- **Content-Type**: `application/json` cho tất cả request/response.
- **Encoding binary**: dữ liệu nhị phân (ticket, encrypted payload, signature) dùng **Base64 standard encoding** (không URL-safe).
- **Timestamp**: Unix timestamp (giây, `int64`) cho các trường trong crypto payload; ISO 8601 UTC (`"2025-01-01T00:00:00Z"`) cho response hiển thị.
- **Số tiền**: `int64` tính bằng cents (1 VND = 1 cent).

---

### 1.3. Header chuẩn

| Header | Bắt buộc | Mô tả |
|---|---|---|
| `Content-Type: application/json` | Có (với body) | Tất cả POST/PUT request |
| `Accept: application/json` | Khuyến nghị | |
| `X-Request-ID` | Khuyến nghị | UUID client sinh, dùng để trace log; nếu không có, Gateway sinh tự động |
| `Authorization: Bearer <token>` | Tùy endpoint | Admin session JWT |

---

### 1.4. Authentication và Authorization

| Nhóm endpoint | Cơ chế auth | Ghi chú |
|---|---|---|
| `/v1/otp/*` | Không cần auth | Rate limit theo IP |
| `/v1/pki/register` | `registration_token` trong body | JWT dùng 1 lần, hết hạn sau 10 phút |
| `/v1/auth/as-req` | Chữ ký số trên AS_REQ payload | Client ký bằng `privKeyRSA_c` |
| `/v1/auth/tgs-req` | TGT + Authenticator trong body | Authenticator mã hóa bằng `K_{c,tgs}` |
| `/v1/bank/*` | `Ticket_v` + Authenticator trong body | Authenticator mã hóa bằng `K_{c,v}` |
| `/v1/admin/*` | `Authorization: Bearer <admin_jwt>` | JWT với claim `role: pki_admin` |

Mọi request đến các internal services (CA, KDC, Bank) đi qua API Gateway — client không gọi trực tiếp internal services.

---

### 1.5. Response envelope

**Response thành công cho object đơn:**

```json
{
  "data": {
    "field": "value"
  },
  "meta": {
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": "2025-01-01T00:00:00Z"
  }
}
```

**Response thành công cho collection:**

```json
{
  "data": [
    { "field": "value" }
  ],
  "pagination": {
    "total": 150,
    "limit": 20,
    "offset": 0
  },
  "meta": {
    "request_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": "2025-01-01T00:00:00Z"
  }
}
```

---

### 1.6. Pagination, filter và sort

- **Pagination**: query params `limit` (default `20`, max `100`) và `offset` (default `0`).
- **Filter**: query params tùy endpoint, ví dụ `?status=active&email=foo@bar.com`.
- **Sort**: không hỗ trợ custom sort trong MVP; mặc định `created_at DESC`.

---

### 1.7. Lỗi chuẩn RFC 7807

```json
{
  "type": "https://minibanking.local/errors/INVALID_OTP",
  "title": "Invalid OTP",
  "status": 400,
  "detail": "OTP is incorrect or has expired.",
  "instance": "/v1/otp/verify",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Mã lỗi dùng chung:**

| HTTP | Code | Mô tả |
|---|---|---|
| `400` | `BAD_REQUEST` | Request body sai schema hoặc thiếu field bắt buộc |
| `400` | `INVALID_OTP` | OTP sai hoặc hết hạn |
| `400` | `INVALID_CSR` | CSR proof-of-possession thất bại |
| `400` | `MISSING_REASON` | Revoke thiếu trường `reason` |
| `401` | `UNAUTHORIZED` | Không xác thực được (token, signature, ticket) |
| `401` | `INVALID_SIGNATURE` | Chữ ký số không hợp lệ |
| `401` | `REPLAY_DETECTED` | Nonce đã được dùng trước đó |
| `401` | `STALE_REQUEST` | Timestamp ngoài freshness window |
| `401` | `INVALID_TICKET` | Ticket_v hết hạn hoặc không giải mã được |
| `401` | `CERT_REVOKED` | Certificate đã bị thu hồi |
| `401` | `CERT_EXPIRED` | Certificate đã hết hạn |
| `401` | `INVALID_REGISTRATION_TOKEN` | Registration token không hợp lệ hoặc đã dùng |
| `403` | `FORBIDDEN` | Có auth nhưng không có quyền (ownership, scope) |
| `403` | `WRONG_SCOPE` | Ticket_v scope không phù hợp endpoint |
| `404` | `NOT_FOUND` | Resource không tồn tại |
| `409` | `ACTIVE_CERT_EXISTS` | User đã có active certificate |
| `409` | `ALREADY_REVOKED` | Certificate đã ở trạng thái revoked |
| `422` | `INSUFFICIENT_FUNDS` | Số dư không đủ |
| `422` | `DAILY_LIMIT_EXCEEDED` | Vượt hạn mức chuyển khoản ngày |
| `422` | `ACCOUNT_NOT_ACTIVE` | Tài khoản bị locked hoặc frozen |
| `429` | `RATE_LIMITED` | Vượt rate limit |
| `503` | `SERVICE_UNAVAILABLE` | Internal service (CA, KDC, Bank) không khả dụng |

Response lỗi **không bao giờ** trả key material, nội dung ticket hay lý do nội bộ chi tiết.

---

### 1.8. Idempotency

Endpoint `POST /v1/bank/transfer` yêu cầu `idempotency_key` trong request body (UUID, client sinh).

- Nếu request với cùng `idempotency_key` đã xử lý thành công → trả `200` với kết quả cũ, không ghi mới.
- Nếu đã xử lý nhưng thất bại → trả lại mã lỗi ban đầu.
- `idempotency_key` phải duy nhất cho mỗi intent chuyển tiền; client retry (network timeout) dùng lại cùng key.

---

### 1.9. Caching

| Endpoint | Cache strategy | Header |
|---|---|---|
| Tất cả POST (auth, transfer) | No cache | `Cache-Control: no-store` |
| `GET /v1/bank/accounts/*/balance` | No cache | `Cache-Control: no-store` |
| `GET /v1/bank/accounts/*/transactions` | No cache | `Cache-Control: no-store` |
| `GET /v1/admin/certificates` | No cache | `Cache-Control: no-store` |

Tất cả endpoint đều no-cache — dữ liệu nhạy cảm, không cache phía client.

---

### 1.10. Trạng thái và audit

- Mọi thay đổi trạng thái quan trọng (issue cert, revoke cert, transfer) được ghi vào audit table tương ứng trong DB.
- `X-Request-ID` được forward vào internal service log để trace end-to-end.
- Response lỗi cho auth failure không phân biệt nguyên nhân cụ thể (tránh information leakage).

---
