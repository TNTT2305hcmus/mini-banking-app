# API Design: Admin Certificate Management

Nguồn nghiệp vụ chính:
- `blueprint/specs/07-admin-certificate-management.md`
- `blueprint/design.md` — Flow 4: PKI Admin Certificate Management
- `blueprint/database-design.md` — CA DB: `certificates`, `certificate_audit_log`

---

## 1. Mục tiêu

Cung cấp các endpoint để Admin quản lý certificate X.509: đăng nhập, list/search, xem chi tiết và revoke. Mọi thao tác đi qua API Gateway với Admin Auth và được ghi vào audit log.

---

## 2. Resource và mapping database

| Resource | Bảng / Key | Vai trò |
|---|---|---|
| Certificate | CA DB `certificates` | SELECT (list/detail), UPDATE (revoke) |
| Audit | CA DB `certificate_audit_log` | INSERT khi looked_up, revoked |
| Revocation cache | Redis `revocation:{serial}` | SET EX 60 sau khi revoke |

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/admin/auth` | Credentials | Đăng nhập, nhận Admin JWT |
| `GET` | `/v1/admin/certificates` | Admin JWT | List/search certificate với filter |
| `GET` | `/v1/admin/certificates/{serial}` | Admin JWT | Xem chi tiết certificate |
| `POST` | `/v1/admin/certificates/{serial}/revoke` | Admin JWT | Thu hồi certificate |

---

## 4. API chi tiết

### 4.1. POST /v1/admin/auth

Đăng nhập Admin, nhận JWT session token.

**Request:**

```json
{
  "email": "admin@minibanking.local",
  "password": "secret"
}
```

**Response `200 OK`:**

```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "expires_in": 3600
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `access_token` | string | JWT với claim `role: pki_admin`, TTL 1 giờ |
| `expires_in` | int | Giây còn lại (3600) |

---

### 4.2. GET /v1/admin/certificates

List/search certificate với filter. Hỗ trợ phân trang.

**Header**: `Authorization: Bearer <access_token>`

**Query params:**

| Param | Kiểu | Mô tả |
|---|---|---|
| `status` | string | Filter theo status: `active`, `revoked`, `expired` |
| `email` | string | Filter theo `subject_email` (partial match) |
| `serial` | string | Filter theo `serial_number` (exact match) |
| `limit` | int | Default 20, max 100 |
| `offset` | int | Default 0 |

**Response `200 OK`:**

```json
{
  "data": [
    {
      "serial_number": "1a2b3c4d5e6f",
      "subject_email": "alice@example.com",
      "subject_cn": "Alice Nguyen",
      "status": "active",
      "not_after": "2026-01-01T00:00:00Z",
      "issued_at": "2025-01-01T00:00:00Z"
    }
  ],
  "pagination": {
    "total": 50,
    "limit": 20,
    "offset": 0
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

---

### 4.3. GET /v1/admin/certificates/{serial}

Xem toàn bộ metadata của một certificate. Ghi audit log `action='looked_up'`.

**Header**: `Authorization: Bearer <access_token>`

**Path param**: `serial` — hex serial number.

**Response `200 OK`:**

```json
{
  "data": {
    "serial_number": "1a2b3c4d5e6f",
    "subject_cn": "Alice Nguyen",
    "subject_email": "alice@example.com",
    "fingerprint_sha256": "a3f1b2...",
    "not_before": "2025-01-01T00:00:00Z",
    "not_after": "2026-01-01T00:00:00Z",
    "status": "active",
    "issued_at": "2025-01-01T00:00:00Z",
    "revoked_at": null,
    "revocation_reason": null
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

**Lưu ý**: `public_key_pem` và `certificate_pem` không trả về trong API này (không cần thiết cho Admin Dashboard MVP).

---

### 4.4. POST /v1/admin/certificates/{serial}/revoke

Thu hồi một certificate đang active. Yêu cầu `reason` bắt buộc.

**Header**: `Authorization: Bearer <access_token>`

**Path param**: `serial` — hex serial number của certificate cần revoke.

**Request:**

```json
{
  "reason": "Private key compromised"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `reason` | string | Có | Lý do thu hồi, tối thiểu 5 ký tự |

**Response `200 OK`:**

```json
{
  "data": {
    "serial_number": "1a2b3c4d5e6f",
    "status": "revoked",
    "revoked_at": "2025-06-01T10:00:00Z",
    "revocation_reason": "Private key compromised"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

**Idempotency**: nếu certificate đã ở trạng thái `revoked` → trả `409 ALREADY_REVOKED`.

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field `reason` hoặc sai schema |
| `400` | `MISSING_REASON` | `reason` rỗng hoặc quá ngắn |
| `401` | `UNAUTHORIZED` | Admin JWT không hợp lệ hoặc hết hạn |
| `403` | `FORBIDDEN` | JWT hợp lệ nhưng thiếu role `pki_admin` |
| `404` | `NOT_FOUND` | Serial không tồn tại trong CA DB |
| `409` | `ALREADY_REVOKED` | Certificate đã ở trạng thái revoked |
| `503` | `SERVICE_UNAVAILABLE` | CA Service không khả dụng |

---

## 6. Acceptance criteria

- `POST /v1/admin/auth` với credentials hợp lệ → `200`, trả JWT với `role: pki_admin`.
- `GET /v1/admin/certificates?status=active` → danh sách chỉ chứa cert `status=active`.
- `GET /v1/admin/certificates/{serial}` → đầy đủ metadata; CA DB có audit record `action='looked_up'`.
- `POST /v1/admin/certificates/{serial}/revoke` hợp lệ → `200`; CA DB `status=revoked`; Redis `revocation:{serial}="revoked"`; audit record `action='revoked'`.
- Revoke lần 2 với cùng serial → `409 ALREADY_REVOKED`.
- Revoke thiếu `reason` → `400 MISSING_REASON`.
- Gọi bất kỳ endpoint `/v1/admin/*` không có JWT → `401 UNAUTHORIZED`.
- Khách hàng dùng cert vừa revoke ở AS Exchange tiếp theo → `401 CERT_REVOKED`.
