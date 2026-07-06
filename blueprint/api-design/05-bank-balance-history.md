# API Design: Bank Balance & Transaction History

Nguồn nghiệp vụ chính:
- `blueprint/specs/05-bank-balance-history.md`
- `blueprint/design.md` — Flow 3: Secure Banking Transaction (read paths)
- `blueprint/database-design.md` — Bank DB: `accounts`, `transactions`

---

## 1. Mục tiêu

Cung cấp 2 endpoint để khách hàng xem số dư và lịch sử giao dịch của tài khoản thuộc sở hữu của mình, dùng `Ticket_v` với scope tương ứng.

---

## 2. Resource và mapping database

| Resource | Bảng / Key | Vai trò |
|---|---|---|
| Tài khoản | Bank DB `accounts` | SELECT balance, status |
| Giao dịch | Bank DB `transactions` | SELECT với phân trang |
| Nonce cache | Redis `replay:{hash}` | SET NX EX |
| Revocation cache | Redis `revocation:{serial}` | GET |
| Certificate | CA DB `certificates` | qua gRPC `VerifyCertificate`: status + validity + issuer/chain |

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/bank/accounts/{account_id}/balance/query` | `Ticket_v` scope `balance:read` + Authenticator | Xem số dư tài khoản |
| `POST` | `/v1/bank/accounts/{account_id}/transactions/query` | `Ticket_v` scope `history:read` + Authenticator | Xem lịch sử giao dịch |

Hai endpoint đọc dùng `POST` read-action để gửi `Ticket_v` và `Authenticator` trong request body một cách tương thích với browser/proxy. Endpoint không thay đổi trạng thái nghiệp vụ và vẫn luôn `Cache-Control: no-store`.

---

## 4. API chi tiết

### 4.1. POST /v1/bank/accounts/{account_id}/balance/query

**Path param:**

| Param | Kiểu | Mô tả |
|---|---|---|
| `account_id` | string (UUID) | ID tài khoản cần xem số dư |

**Request body:**

```json
{
  "ticket_v": "BASE64_ENCRYPTED_TICKET_V",
  "authenticator": "BASE64_ENCRYPTED_AUTHENTICATOR"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `ticket_v` | string (Base64) | Có | Ticket_v scope `balance:read` |
| `authenticator` | string (Base64) | Có | `E_{K_{c,v}}[id_c, nonce, timestamp, request_id]` |

Bank Service kiểm tra `cert_sn` trong `Ticket_v` qua CA Service. Certificate chỉ được chấp nhận khi trạng thái active, còn hiệu lực và chain hợp lệ `Root CA -> Client CA -> user certificate`.

**Response `200 OK`:**

```json
{
  "data": {
    "account_id": "550e8400-e29b-41d4-a716-446655440000",
    "account_number": "0123456789",
    "balance": 10000000,
    "currency": "VND",
    "status": "active"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `account_id` | string | UUID tài khoản |
| `account_number` | string | Số tài khoản hiển thị |
| `balance` | int64 | Số dư hiện tại (cents) |
| `currency` | string | Mã tiền tệ (VND) |
| `status` | string | Trạng thái tài khoản |

---

### 4.2. POST /v1/bank/accounts/{account_id}/transactions/query

**Path param:**

| Param | Kiểu | Mô tả |
|---|---|---|
| `account_id` | string (UUID) | ID tài khoản cần xem lịch sử |

**Query params:**

| Param | Kiểu | Default | Mô tả |
|---|---|---|---|
| `limit` | int | 20 | Số record mỗi trang (tối đa 100) |
| `offset` | int | 0 | Vị trí bắt đầu |

**Request body:**

```json
{
  "ticket_v": "BASE64_ENCRYPTED_TICKET_V",
  "authenticator": "BASE64_ENCRYPTED_AUTHENTICATOR"
}
```

**Response `200 OK`:**

```json
{
  "data": [
    {
      "tx_id": "UUID",
      "from_account_number": "0123456789",
      "to_account_number": "9876543210",
      "amount": 5000000,
      "currency": "VND",
      "status": "completed",
      "description": "Chuyển tiền học phí",
      "scope": "transfer:create",
      "created_at": "2025-01-01T10:00:00Z",
      "completed_at": "2025-01-01T10:00:01Z"
    }
  ],
  "pagination": {
    "total": 42,
    "limit": 20,
    "offset": 0
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

**Lưu ý**: `client_signature` và `payload_hash` không được trả về trong response (dữ liệu nội bộ, chỉ dùng cho audit).

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field bắt buộc |
| `401` | `INVALID_TICKET` | `Ticket_v` hết hạn hoặc không hợp lệ |
| `401` | `STALE_REQUEST` | Timestamp trong Authenticator ngoài ±5 phút |
| `401` | `REPLAY_DETECTED` | Nonce đã được dùng |
| `401` | `CERT_REVOKED` | Certificate đã bị thu hồi |
| `403` | `WRONG_SCOPE` | Dùng sai scope (ví dụ `transfer:create` cho balance endpoint) |
| `403` | `FORBIDDEN` | Tài khoản không thuộc sở hữu của khách hàng |
| `404` | `NOT_FOUND` | `account_id` không tồn tại |
| `503` | `SERVICE_UNAVAILABLE` | Bank Service hoặc CA Service không khả dụng |

---

## 6. Acceptance criteria

- Xem balance tài khoản của mình với `scope=balance:read` → `200` với dữ liệu đúng.
- Xem history tài khoản của mình với `scope=history:read` → `200` với danh sách phân trang.
- Dùng `Ticket_v` `scope=balance:read` gọi `/transactions/query` → `403 WRONG_SCOPE`.
- Truyền `account_id` thuộc người khác → `403 FORBIDDEN`.
- Nonce đã dùng ở request trước → `401 REPLAY_DETECTED`.
- Response history không chứa `client_signature`.
