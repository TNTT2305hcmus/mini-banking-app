# API Design: OTP & PKI Registration

Nguồn nghiệp vụ chính:
- `blueprint/specs/01-otp-pki-registration.md`
- `blueprint/design.md` — Flow 1: Customer Registration & PKI Enrollment
- `blueprint/database-design.md` — CA DB: `certificates`; Bank DB: `users`

---

## 1. Mục tiêu

Cung cấp 3 endpoint để khách hàng mới hoàn thành đăng ký:
1. Yêu cầu OTP gửi về email.
2. Xác minh OTP, nhận `registration_token`.
3. Gửi CSR, nhận X.509 certificate từ CA và tạo user record qua Bank Service.

---

## 2. Resource và mapping database

| Resource | Bảng | Vai trò |
|---|---|---|
| OTP | Redis `otp:{email}` | Lưu OTP TTL 5 phút |
| Rate limit | Redis `rate:otp_request:{ip}` | Chống spam |
| Certificate | CA DB `certificates` | Ghi khi CA cấp cert |
| Audit | CA DB `certificate_audit_log` | Ghi action='issued' |
| User | Bank DB `users` | Tạo qua Bank Service gRPC sau khi enrollment thành công |

---

## 3. Endpoint tổng hợp

| Method | Endpoint | Auth | Mục đích |
|---|---|---|---|
| `POST` | `/v1/otp/request` | Không | Yêu cầu gửi OTP về email |
| `POST` | `/v1/otp/verify` | Không | Xác minh OTP, nhận registration_token |
| `POST` | `/v1/pki/register` | `registration_token` | Gửi CSR, nhận X.509 certificate |

---

## 4. API chi tiết

### 4.1. POST /v1/otp/request

Yêu cầu gửi OTP 6 chữ số về email đăng ký.

**Request:**

```json
{
  "email": "alice@example.com"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `email` | string | Có | Email hợp lệ, tối đa 255 ký tự |

**Response `200 OK`:**

```json
{
  "data": {
    "message": "OTP sent",
    "expires_in": 300
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `message` | string | Xác nhận đã gửi |
| `expires_in` | int | Số giây OTP còn hiệu lực (300) |

**Rate limit**: tối đa 5 request/phút/IP. Vượt → `429 RATE_LIMITED`.

---

### 4.2. POST /v1/otp/verify

Xác minh OTP, nhận `registration_token` dùng 1 lần để tiếp tục PKI enrollment.

**Request:**

```json
{
  "email": "alice@example.com",
  "otp": "482910"
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `email` | string | Có | Phải khớp email đã request OTP |
| `otp` | string | Có | 6 chữ số |

**Response `200 OK`:**

```json
{
  "data": {
    "registration_token": "eyJhbGci...",
    "expires_in": 600
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `registration_token` | string | JWT dùng 1 lần, TTL 10 phút |
| `expires_in` | int | Số giây token còn hiệu lực (600) |

**Lưu ý**: OTP bị xóa khỏi Redis ngay sau khi verify thành công — không thể dùng lại.

---

### 4.3. POST /v1/pki/register

Gửi CSR kèm `registration_token`, nhận X.509 certificate từ CA.

API Gateway không ghi trực tiếp Bank DB. Sau khi CA Service cấp certificate thành công, Gateway gọi Bank Service gRPC `CreateUser` để tạo user record. Nếu bước tạo user thất bại, Gateway gọi CA Service revoke/mark certificate với reason `enrollment_failed` để tránh certificate active không có Bank user tương ứng, rồi trả `503 SERVICE_UNAVAILABLE`.

**Request:**

```json
{
  "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----\nMIICv...\n-----END CERTIFICATE REQUEST-----",
  "registration_token": "eyJhbGci..."
}
```

| Field | Kiểu | Bắt buộc | Mô tả |
|---|---|---|---|
| `csr_pem` | string | Có | X.509 CSR ở định dạng PEM; phải chứa chữ ký proof-of-possession hợp lệ |
| `registration_token` | string | Có | JWT nhận từ `/v1/otp/verify`, dùng 1 lần |

**Response `201 Created`:**

```json
{
  "data": {
    "certificate_pem": "-----BEGIN CERTIFICATE-----\nMIIDX...\n-----END CERTIFICATE-----",
    "serial_number": "1a2b3c4d5e6f",
    "not_after": "2026-01-01T00:00:00Z"
  },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

| Field | Kiểu | Mô tả |
|---|---|---|
| `certificate_pem` | string | X.509 certificate đã được CA ký, định dạng PEM |
| `serial_number` | string | Hex-encoded serial number do CA cấp |
| `not_after` | string | Thời điểm hết hạn certificate (ISO 8601 UTC) |

---

## 5. Error catalog

| HTTP | Code | Khi nào xảy ra |
|---|---|---|
| `400` | `BAD_REQUEST` | Thiếu field bắt buộc hoặc sai schema |
| `400` | `INVALID_OTP` | OTP sai hoặc đã hết hạn |
| `400` | `INVALID_CSR` | CSR proof-of-possession thất bại |
| `401` | `INVALID_REGISTRATION_TOKEN` | Token không hợp lệ, đã dùng, hoặc hết hạn |
| `409` | `ACTIVE_CERT_EXISTS` | User đã có active certificate |
| `429` | `RATE_LIMITED` | Vượt rate limit OTP request |
| `503` | `SERVICE_UNAVAILABLE` | CA Service hoặc Bank Service không khả dụng |

---

## 6. Acceptance criteria

- `POST /v1/otp/request` với email hợp lệ → `200`, email được gửi, Redis có `otp:{email}` TTL 5 phút.
- `POST /v1/otp/verify` với OTP đúng → `200`, trả `registration_token`, Redis key bị xóa.
- `POST /v1/otp/verify` lần 2 với cùng OTP → `400 INVALID_OTP` (đã xóa).
- `POST /v1/pki/register` với CSR hợp lệ và token hợp lệ → `201`, CA DB có cert record, Bank DB có user record tạo qua `Bank.CreateUser`.
- `POST /v1/pki/register` với token đã dùng → `401 INVALID_REGISTRATION_TOKEN`.
- `POST /v1/otp/request` vượt 5 lần/phút → `429 RATE_LIMITED`.
