# API DESIGN DOCUMENT — Mini App Banking

---

## Mục lục

1. [System Overview](#1-system-overview)
2. [Global API Standards](#2-global-api-standards)
3. [Response Envelope Standard](#3-response-envelope-standard)
4. [Error Handling Standard](#4-error-handling-standard)
5. [Authentication & Authorization](#5-authentication--authorization)
6. [Rate Limiting](#6-rate-limiting)
7. [Idempotency](#7-idempotency)
8. [Audit Logging](#8-audit-logging)
9. [OTP APIs](#9-otp-apis)
10. [PKI APIs](#10-pki-apis)
11. [Authentication APIs](#11-authentication-apis)
12. [Banking APIs](#12-banking-apis)
13. [Certificate Management APIs](#13-certificate-management-apis)
14. [API → Table Mapping](#14-api--table-mapping)
15. [Proto → REST Mapping](#15-proto--rest-mapping)
16. [API → Actors Mapping](#16-api--actors-mapping)

---

## 1. System Overview

### 1.1 Mục tiêu hệ thống

Mini App Banking triển khai mô hình xác thực bảo mật nhiều lớp:

| Cơ chế            | Mô tả                                           |
| ----------------- | ----------------------------------------------- |
| OTP Email         | Xác thực danh tính ban đầu                      |
| PKI / X.509       | Định danh dài hạn bằng chứng chỉ số             |
| Kerberos-like     | Phân phối session key an toàn (AS/TGS Exchange) |
| Digital Signature | Non-repudiation cho mọi giao dịch               |
| AES-256-GCM       | Mã hóa đối xứng session                         |
| Hash Chaining     | Immutable ledger chống giả mạo                  |

### 1.2 Kiến trúc hệ thống

```
Client (React/TS)
    │ HTTPS/TLS 1.3
    ▼
API Gateway (Node.js/TS)  ← DMZ Layer
    │ gRPC + mTLS
    ├─→ CA Service (Go)     ← PKI Management
    ├─→ KDC Service (Go)    ← Kerberos Auth
    └─→ Bank Service (Go)   ← Banking Operations
             │
             ▼
        PostgreSQL + Redis
```

### 1.3 Domain Boundaries

| Domain           | Resources                      | Endpoints |
| ---------------- | ------------------------------ | --------- |
| Identity         | OTP, Registration Token        | `/otp/*`  |
| PKI              | Certificate, CSR, CRL          | `/pki/*`  |
| Authentication   | TGT, Ticket_v, Session         | `/auth/*` |
| Banking          | Account, Transaction, Transfer | `/bank/*` |
| Certificate Mgmt | Status, Revocation, OCSP       | `/cert/*` |

### 1.4 Authentication Flow (4 Phases)

```
Phase 1: OTP → PKI Registration     → X.509 Certificate
Phase 2: AS Exchange (PKINIT)       → TGT + K_{c,tgs}
Phase 3: TGS Exchange               → Ticket_v + K_{c,v}
Phase 4: AP Exchange (Transaction)  → AP_REP (mutual auth)
```

### 1.5 State Machine — Token/Session Lifecycle

```
[No State]
    │ POST /otp/request
    ▼
[OTP Pending] TTL=5min
    │ POST /otp/verify (success)
    ▼
[Registration Token] TTL=10min, single-use
    │ POST /pki/register
    ▼
[Certificate Issued] TTL=1 year
    │ POST /auth/as-req
    ▼
[TGT Active] TTL=30min (RAM only)
    │ POST /auth/tgs-req
    ▼
[Ticket_v Active] TTL=5min (RAM only)
    │ POST /bank/transfer
    ▼
[AP_REP] → Session zeroed from RAM
```

---

## 2. Global API Standards

### 2.1 Base URL

```
https://api.banking.local/v1
```

Versioning: URI versioning (`/v1`). Khi breaking change → `/v2` song song.

### 2.2 HTTP Headers

#### Required Headers (tất cả endpoint)

| Header             | Type    | Required | Mô tả                                                                 |
| ------------------ | ------- | -------- | --------------------------------------------------------------------- |
| `Content-Type`     | string  | ✅       | `application/json`                                                    |
| `X-Request-ID`     | UUID v4 | ✅       | Idempotency & distributed tracing. Gateway log và echo trong response |
| `X-Timestamp`      | number  | ✅       | Unix epoch (seconds). Gateway validate ±5 phút clock skew             |
| `X-Client-Version` | string  | ✅       | Client build version. Dùng audit log và deprecation tracking          |

#### Security Headers (endpoint yêu cầu auth)

| Header              | Type    | Required    | Mô tả                                                         |
| ------------------- | ------- | ----------- | ------------------------------------------------------------- |
| `Authorization`     | string  | Conditional | `Bearer {token}` — dùng cho Registration Token và Admin JWT   |
| `X-Idempotency-Key` | UUID v4 | Conditional | Bắt buộc cho write operations (transfer). Chống double-submit |

#### Response Headers (từ server)

| Header                  | Mô tả                           |
| ----------------------- | ------------------------------- |
| `X-Request-ID`          | Echo lại request ID             |
| `X-RateLimit-Limit`     | Limit của tier hiện tại         |
| `X-RateLimit-Remaining` | Số request còn lại trong window |
| `X-RateLimit-Reset`     | Unix timestamp khi window reset |

### 2.3 HTTP Verbs

| Verb     | Dùng cho                                              | Idempotent                      |
| -------- | ----------------------------------------------------- | ------------------------------- |
| `GET`    | Read-only queries                                     | ✅                              |
| `POST`   | Create, trigger operations, complex queries with body | ❌ (trừ khi có Idempotency-Key) |
| `PUT`    | Full replacement                                      | ✅                              |
| `PATCH`  | Partial update                                        | ❌                              |
| `DELETE` | Soft delete                                           | ✅                              |

> **Lưu ý:** Banking read operations (`/bank/balance`, `/bank/transactions`) dùng `POST` vì cần body chứa encrypted auth credentials (ticket_v, authenticator). GET không thể mang body an toàn.

### 2.4 HTTP Status Codes

| Code | Ý nghĩa                                               |
| ---- | ----------------------------------------------------- |
| 200  | OK — thành công, có data trả về                       |
| 201  | Created — resource được tạo                           |
| 204  | No Content — thành công, không có data                |
| 400  | Bad Request — validation failed, malformed            |
| 401  | Unauthorized — chưa auth hoặc auth sai                |
| 403  | Forbidden — đã auth nhưng không có quyền              |
| 404  | Not Found — resource không tồn tại                    |
| 408  | Request Timeout — timestamp out of window             |
| 409  | Conflict — replay detected, duplicate, already exists |
| 422  | Unprocessable Entity — business rule violation        |
| 429  | Too Many Requests — rate limit exceeded               |
| 500  | Internal Server Error                                 |
| 502  | Bad Gateway — upstream service (gRPC) failure         |
| 503  | Service Unavailable — maintenance                     |

---

## 3. Response Envelope Standard

### 3.1 Success Response

```json
{
  "success": true,
  "message": "Transfer completed successfully",
  "data": {},
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "has_more": true,
    "next_cursor": "txn-uuid-xyz"
  },
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-05-12T23:15:40+07:00"
}
```

| Field        | Type             | Mô tả                                            |
| ------------ | ---------------- | ------------------------------------------------ |
| `success`    | boolean          | `true` khi xử lý thành công                      |
| `message`    | string           | Mô tả kết quả ngắn gọn, human-readable           |
| `data`       | object/null      | Payload chính. `null` khi không có data          |
| `meta`       | object/null      | Pagination metadata. `null` khi không phân trang |
| `request_id` | string (UUID)    | Echo từ `X-Request-ID` header. Dùng debug/trace  |
| `timestamp`  | string (ISO8601) | Server timestamp khi response được tạo           |

### 3.2 Error Response

```json
{
  "success": false,
  "message": "Validation failed",
  "errors": [
    {
      "field": "amount",
      "code": "INVALID_AMOUNT",
      "message": "Amount must be greater than 0"
    }
  ],
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-05-12T23:15:40+07:00"
}
```

---

## 4. Error Handling Standard

### 4.1 Error Code

#### Validation Errors (400)

| Code                     | HTTP | Mô tả                       |
| ------------------------ | ---- | --------------------------- |
| `INVALID_EMAIL`          | 400  | Email format RFC 5322 sai   |
| `INVALID_OTP_FORMAT`     | 400  | OTP không phải 6 chữ số     |
| `INVALID_CSR_FORMAT`     | 400  | CSR không phải PEM hợp lệ   |
| `INVALID_NONCE`          | 400  | Nonce thiếu hoặc sai format |
| `INVALID_TIMESTAMP`      | 400  | Timestamp không hợp lệ      |
| `INVALID_AMOUNT`         | 400  | Amount ≤ 0 hoặc sai format  |
| `INVALID_ACCOUNT`        | 400  | Account ID sai format       |
| `INVALID_PAGINATION`     | 400  | page/limit out of bounds    |
| `MISSING_REQUIRED_FIELD` | 400  | Field bắt buộc thiếu        |

#### Authentication Errors (401)

| Code                | HTTP | Mô tả                                      |
| ------------------- | ---- | ------------------------------------------ |
| `OTP_MISMATCH`      | 401  | OTP không khớp Redis value                 |
| `OTP_EXPIRED`       | 401  | Redis TTL đã hết                           |
| `OTP_NOT_FOUND`     | 404  | Không có OTP pending cho email này         |
| `REG_TOKEN_INVALID` | 401  | JWT signature sai                          |
| `REG_TOKEN_EXPIRED` | 401  | JWT exp đã qua                             |
| `REG_TOKEN_USED`    | 401  | JWT jti đã consumed — single-use violation |
| `CERT_NOT_FOUND`    | 401  | Không tìm thấy cert theo serial            |
| `CERT_REVOKED`      | 403  | Cert có trong CRL                          |
| `CERT_EXPIRED`      | 401  | Cert đã quá not_after                      |
| `IDENTITY_MISMATCH` | 401  | ID_c không khớp cert Subject CN            |
| `SIGNATURE_INVALID` | 401  | RSA signature verify failed                |
| `TGT_EXPIRED`       | 401  | TGT đã hết lifetime                        |
| `TICKET_EXPIRED`    | 401  | Ticket_v đã hết lifetime                   |
| `AUTH_INVALID`      | 401  | Authenticator decrypt failed               |
| `SESSION_EXPIRED`   | 401  | Session K\_{c,tgs} hết hạn                 |

#### Authorization Errors (403)

| Code                    | HTTP | Mô tả                                     |
| ----------------------- | ---- | ----------------------------------------- |
| `FORBIDDEN`             | 403  | Không có quyền thực hiện action           |
| `SCOPE_DENIED`          | 403  | Ticket_v.scope không chứa scope cần thiết |
| `NOT_OWNER`             | 403  | ID_c không phải owner của account         |
| `ACCOUNT_ACCESS_DENIED` | 403  | Account không thuộc identity đang auth    |

#### Security/Replay Errors (408/409)

| Code                          | HTTP | Mô tả                                        |
| ----------------------------- | ---- | -------------------------------------------- |
| `REQUEST_EXPIRED`             | 408  | Timestamp ngoài ±5 phút window               |
| `REPLAY_DETECTED`             | 409  | Nonce đã tồn tại trong Redis/DB              |
| `IDEMPOTENT_DUPLICATE`        | 409  | Idempotency-Key đã xử lý — trả cached result |
| `IDENTITY_ALREADY_REGISTERED` | 409  | Cert cho id_c đã active                      |
| `ALREADY_REVOKED`             | 409  | Cert đã bị revoke trước đó                   |

#### Business Rule Errors (422)

| Code                   | HTTP | Mô tả                              |
| ---------------------- | ---- | ---------------------------------- |
| `INSUFFICIENT_FUNDS`   | 422  | Balance < amount                   |
| `DAILY_LIMIT_EXCEEDED` | 422  | Vượt hạn mức chuyển khoản ngày     |
| `ACCOUNT_FROZEN`       | 422  | Tài khoản bị đóng băng             |
| `ACCOUNT_LOCKED`       | 422  | Tài khoản bị khóa                  |
| `ACCOUNT_NOT_FOUND`    | 422  | Tài khoản nguồn/đích không tồn tại |
| `SELF_TRANSFER`        | 422  | from_account == to_account         |

#### Rate Limit Errors (429)

| Code                    | HTTP | Mô tả                      |
| ----------------------- | ---- | -------------------------- |
| `OTP_RATE_LIMITED`      | 429  | >3 OTP request/email/10min |
| `VERIFY_RATE_LIMITED`   | 429  | >5 verify attempt/email    |
| `AUTH_RATE_LIMITED`     | 429  | >10 AS_REQ/IP/5min         |
| `TRANSFER_RATE_LIMITED` | 429  | >20 transfer/session/min   |

#### System Errors (5xx)

| Code                       | HTTP | Mô tả                               |
| -------------------------- | ---- | ----------------------------------- |
| `INTERNAL_ERROR`           | 500  | Lỗi nội bộ không xác định           |
| `CA_SERVICE_UNAVAILABLE`   | 502  | gRPC call tới CA Service thất bại   |
| `KDC_UNAVAILABLE`          | 502  | gRPC call tới KDC thất bại          |
| `BANK_SERVICE_UNAVAILABLE` | 502  | gRPC call tới Bank Service thất bại |
| `EMAIL_DISPATCH_FAILED`    | 500  | SMTP/email service lỗi              |
| `CRL_UNAVAILABLE`          | 503  | CA CRL endpoint không đạt được      |

---

## 5. Authentication & Authorization

### 5.1 Authentication Levels

| Level              | Dùng cho                      | Cơ chế                                                  |
| ------------------ | ----------------------------- | ------------------------------------------------------- |
| Public             | `/otp/request` `/cert/status` | Không cần auth                                          |
| Registration Token | `/pki/register`               | JWT (HS256) single-use, 10min                           |
| Certificate-bound  | `/auth/as-req`                | X.509 cert_sn + PKINIT Pre-auth Signature               |
| TGT Session        | `/auth/tgs-req`               | TGT + Authenticator (E*{K*{c,tgs}})                     |
| Ticket Session     | `/bank/*`                     | Ticket*v + Authenticator (E*{K\_{c,v}}) + RSA Signature |
| Admin JWT          | `/cert/revoke` (admin)        | Separate admin JWT (HS256/RS256)                        |

### 5.2 Permission Model

| Scope               | Cho phép                      |
| ------------------- | ----------------------------- |
| `transfer:internal` | Chuyển khoản nội bộ           |
| `account:read`      | Đọc số dư và lịch sử          |
| `cert:revoke`       | Thu hồi chứng chỉ (chỉ admin) |

### 5.3 Security Checks

Áp dụng cho **Phase 4: AP Exchange**

```
1. Decrypt Ticket_v bằng K_v → extract K_{c,v}, pub_c, scope, cert_sn, expiry
2. Kiểm tra Ticket_v.expiry ≤ now
3. Decrypt Auth_v bằng K_{c,v} → extract ID_c, TS_5, Nonce_3
4. Verify |now - TS_5| < 5 phút
5. Verify ID_c trong Auth_v == ID_c trong Ticket_v
6. Check Nonce_3 chưa dùng (Redis NX → DB backup)
7. Decrypt cipher_payload bằng K_{c,v} → extract Payload + Signature
8. Verify RSA Signature(Payload, pub_c) — non-repudiation
9. Check cert_sn revocation (Redis cache 3min → CA gRPC fallback)
10. Check Ticket_v.scope ⊇ required_scope
11. Check ID_c == owner of from_account
12. Check account status (active, not frozen/locked)
13. Check balance ≥ amount
14. Check daily_transfer_limit không vượt
15. ACID transaction + Hash Chaining + Audit Log
```

---

## 6. Rate Limiting

| Tier         | Limit        | Window       | Scope       | Storage                          |
| ------------ | ------------ | ------------ | ----------- | -------------------------------- |
| OTP Request  | 3 requests   | 10 min       | per email   | Redis: `rate:otp:{email}`        |
| OTP Verify   | 5 attempts   | 15 min       | per email   | Redis: `rate:verify:{email}`     |
| PKI Register | 1 requests   | JWT lifetime | per JWT jti | Redis: `jwt_used:{jti}`          |
| AS Exchange  | 10 requests  | 5 min        | per IP      | Redis: `rate:as:{ip}`            |
| TGS Exchange | 10 requests  | 5 min        | per cert_sn | Redis: `rate:tgs:{cert_sn}`      |
| Banking      | 20 requests  | 1 min        | per session | Redis: `rate:bank:{ticket_hash}` |
| Cert Status  | 100 requests | 1 min        | per IP      | Redis: `rate:cert:{ip}`          |

**Response khi vượt limit:**

```json
{
  "success": false,
  "message": "Rate limit exceeded",
  "errors": [
    {
      "code": "OTP_RATE_LIMITED",
      "message": "Max 3 OTP requests per 10 minutes"
    }
  ],
  "request_id": "uuid",
  "timestamp": "ISO8601"
}
```

Headers: `Retry-After: 540` (seconds until reset)

---

## 7. Idempotency

### 7.1 Endpoints cần Idempotency-Key

| Endpoint              | Bắt buộc | TTL     | Storage                  |
| --------------------- | -------- | ------- | ------------------------ |
| `POST /bank/transfer` | ✅       | 24h     | Redis `idempotent:{key}` |
| `POST /otp/request`   | ❌       | —       | (handled by rate limit)  |
| `POST /pki/register`  | Implicit | JWT TTL | Redis `jwt_used:{jti}`   |

### 7.2 Idempotency Flow

```
1. Client sinh UUID làm Idempotency-Key
2. Server: SET idempotent:{key} {status:processing} EX 86400 NX
   - NX fail → key tồn tại → check cached result
   - NX ok   → process normally
3. Khi process xong: UPDATE idempotent:{key} = {status:done, result: {...}}
4. Lần retry: trả cached result, HTTP 200, không xử lý lại
```

---

## 8. Audit Logging

### 8.1 Audit Events

| Event                | Khi nào             | Bảng DB               |
| -------------------- | ------------------- | --------------------- |
| `OTP_REQUESTED`      | POST /otp/request   | — (Redis only)        |
| `OTP_VERIFIED`       | POST /otp/verify    | — (Redis only)        |
| `CERT_ISSUED`        | POST /pki/register  | `user_certificates`   |
| `AUTH_ATTEMPT`       | POST /auth/as-req   | audit log             |
| `TICKET_ISSUED`      | POST /auth/tgs-req  | audit log             |
| `TRANSFER_INITIATED` | POST /bank/transfer | `transactions`        |
| `TRANSFER_COMPLETED` | Bank Service commit | `transaction_details` |
| `CERT_REVOKED`       | POST /cert/revoke   | `user_certificates`   |

### 8.2 Audit Log Schema (Gateway)

```sql
INSERT INTO gateway_audit_log (
  request_id, endpoint, method, id_c, cert_sn,
  ip_address, user_agent, status_code,
  error_code, created_at
) VALUES (...);
```

### 8.3 Transaction Immutable Ledger

Mỗi transaction trong bảng `transactions` có:

- `payload_hash`: SHA-256 của plaintext payload
- `client_signature`: Sign(payload_hash, priv_c) — non-repudiation
- `previous_hash`: Hash của tx trước → hash chaining
- `current_hash = SHA256(previous_hash || payload_hash || client_signature)`

Không có UPDATE/DELETE trên bảng `transactions`.

---

## 9. OTP APIs

---

### 9.1 POST /v1/otp/request

**Purpose:** Khởi tạo luồng xác thực OTP. Gateway sinh OTP ngẫu nhiên 6 chữ số, lưu HMAC vào Redis với TTL=5 phút, gửi email cho user. Đây là entry point của Phase 1 PKI Registration.

**Authentication:** Public — không cần token

**Rate Limit:** 3 req / email / 10 phút

**Audit:** Log request (IP, email masked, timestamp)

#### Request Headers

| Header             | Required | Mô tả              |
| ------------------ | -------- | ------------------ |
| `Content-Type`     | ✅       | `application/json` |
| `X-Request-ID`     | ✅       | UUID v4            |
| `X-Timestamp`      | ✅       | Unix epoch         |
| `X-Client-Version` | ✅       | Client build       |

#### Request Body

| Field   | Type   | Required | Validation                         | Mô tả                             |
| ------- | ------ | -------- | ---------------------------------- | --------------------------------- |
| `email` | string | ✅       | RFC 5322, max 254 chars, lowercase | Email đăng ký. Dùng làm Redis key |

#### Example Request

```json
POST /v1/otp/request
Content-Type: application/json
X-Request-ID: 550e8400-e29b-41d4-a716-446655440000
X-Timestamp: 1715500000

{
  "email": "alice@example.com"
}
```

#### Business Rules

- Email chỉ cần tồn tại theo RFC 5322, hệ thống không kiểm tra email đã tồn tại trong DB (tránh user enumeration)
- OTP 6 chữ số, HMAC-SHA256 trước khi lưu Redis (raw OTP không bao giờ persist)
- Re-request trong TTL: reset TTL, sinh OTP mới
- Response không tiết lộ email có tồn tại trong hệ thống hay không

#### Processing Flow

```
1. Validate Content-Type, X-Request-ID, X-Timestamp (format)
2. Validate email format (RFC 5322)
3. Check rate limit: INCR rate:otp:{email} → nếu >3 → 429
4. Gen OTP = crypto.randomInt(100000, 999999).toString()
5. hmac_otp = HMAC_SHA256(OTP, GATEWAY_OTP_SECRET)
6. Redis: SET otp:{email} {hmac_otp} EX 300
7. Redis: SET rate:otp:{email} {count} EX 600
8. Dispatch email (async) → log result
9. Return 200 (kể cả nếu email gửi fail — tránh timing attack)
```

#### Redis Usage

| Key                | Value            | TTL  |
| ------------------ | ---------------- | ---- |
| `otp:{email}`      | HMAC-SHA256(OTP) | 300s |
| `rate:otp:{email}` | request count    | 600s |

#### DB Usage

Không truy cập DB.

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "OTP dispatched",
  "data": {
    "email_masked": "a***@example.com",
    "expires_in": 300
  },
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-05-12T23:15:40+07:00"
}
```

#### Error Responses

| HTTP | Error Code               | Khi nào                               |
| ---- | ------------------------ | ------------------------------------- |
| 400  | `INVALID_EMAIL`          | Email format RFC 5322 sai             |
| 400  | `MISSING_REQUIRED_FIELD` | email field thiếu                     |
| 429  | `OTP_RATE_LIMITED`       | >3 request/email/10min                |
| 500  | `EMAIL_DISPATCH_FAILED`  | SMTP lỗi (internal log, không expose) |

#### Security Notes

- HMAC OTP trước khi lưu: attacker access Redis không lấy được OTP gốc
- Enumeration prevention: response luôn 200 dù email valid hay không
- Rate limit per email, không per IP (tránh false positive từ shared IP)

---

### 9.2 POST /v1/otp/verify

**Purpose:** Xác minh OTP → cấp Registration Token (JWT) single-use. Client dùng token này cho `/pki/register`.

**Authentication:** Public

**Rate Limit:** 5 failed attempts / email → lock 15 phút

**Audit:** Log verify result (success/fail), masked email, IP

#### Request Body

| Field   | Type   | Required | Validation                  | Mô tả              |
| ------- | ------ | -------- | --------------------------- | ------------------ |
| `email` | string | ✅       | RFC 5322, max 254           | Email đã gửi OTP   |
| `otp`   | string | ✅       | Exactly 6 digits `[0-9]{6}` | OTP nhận qua email |

#### Example Request

```json
POST /v1/otp/verify
Content-Type: application/json
X-Request-ID: 660e8400-e29b-41d4-a716-446655440111
X-Timestamp: 1715500060

{
  "email": "alice@example.com",
  "otp": "482901"
}
```

#### Processing Flow

```
1. Validate fields
2. Check Redis: GET otp:{email} → nếu không có → 404 OTP_NOT_FOUND
3. Check fail counter: GET rate:verify:{email} → nếu >=5 → 429 VERIFY_RATE_LIMITED
4. Compute: expected = HMAC_SHA256(otp, GATEWAY_OTP_SECRET)
5. Compare expected == Redis value (constant-time comparison)
   - Fail: INCR rate:verify:{email} EX 900 → 401 OTP_MISMATCH
   - OK: DEL otp:{email} (single-use)
6. Generate JWT:
   payload = { sub: email, purpose: "pki_registration", jti: uuidv4(), iat, exp: iat+600 }
   signed = HS256(payload, GATEWAY_JWT_SECRET)
7. Redis: SET jwt_used:{jti} 0 EX 600 (0=unused, 1=used)
8. Return 200 + reg_token
```

#### Redis Usage

| Key                   | Value                             | TTL  |
| --------------------- | --------------------------------- | ---- |
| `otp:{email}`         | HMAC(OTP) — DEL sau khi verify OK | 300s |
| `rate:verify:{email}` | fail count                        | 900s |
| `jwt_used:{jti}`      | 0 (unused)                        | 600s |

#### JWT Payload Structure

```json
{
  "sub": "alice@example.com",
  "purpose": "pki_registration",
  "jti": "a1b2c3d4-...",
  "iat": 1715500060,
  "exp": 1715500660
}
```

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "OTP verified",
  "data": {
    "reg_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 600
  },
  "request_id": "660e8400-e29b-41d4-a716-446655440111",
  "timestamp": "2026-05-12T23:16:40+07:00"
}
```

| Field        | Type         | Mô tả                                                                 |
| ------------ | ------------ | --------------------------------------------------------------------- |
| `reg_token`  | string (JWT) | Registration Token. Single-use. 10 phút. Lưu RAM only — không persist |
| `expires_in` | number       | Giây còn lại đến khi token hết hạn                                    |

#### Error Responses

| HTTP | Error Code               | Khi nào                        |
| ---- | ------------------------ | ------------------------------ |
| 400  | `INVALID_OTP_FORMAT`     | OTP không phải 6 chữ số        |
| 400  | `MISSING_REQUIRED_FIELD` | email hoặc otp thiếu           |
| 401  | `OTP_MISMATCH`           | HMAC không khớp                |
| 401  | `OTP_EXPIRED`            | Redis TTL đã hết               |
| 404  | `OTP_NOT_FOUND`          | Không có OTP pending cho email |
| 429  | `VERIFY_RATE_LIMITED`    | >=5 lần fail/email             |

#### Security Notes

- Constant-time comparison tránh timing attack
- OTP DEL ngay sau verify thành công (single-use)
- JWT jti pre-registered in Redis, consumed in `/pki/register`
- Client phải lưu `reg_token` trong RAM only, không vào LocalStorage

---

## 10. PKI APIs

---

### 10.1 POST /v1/pki/register

**Purpose:** Nhận CSR từ client → forward CA Service (gRPC) → CA verify Proof-of-Possession → cấp X.509 cert. Đây là bước cuối Phase 1.

**Authentication:** Registration Token (Bearer JWT từ `/otp/verify`)

**Rate Limit:** 1 request / JWT (enforced bởi single-use jti)

**Audit:** Log cert serial, user_id, issued_at → `user_certificates`

**gRPC Call:** `CAService.RegisterUser(RegisterUserRequest)`

#### Request Headers

| Header          | Required | Mô tả                |
| --------------- | -------- | -------------------- |
| `Content-Type`  | ✅       | `application/json`   |
| `Authorization` | ✅       | `Bearer {reg_token}` |
| `X-Request-ID`  | ✅       | UUID v4              |
| `X-Timestamp`   | ✅       | Unix epoch           |

#### Request Body

| Field     | Type   | Required | Validation                                                | Mô tả                                                           |
| --------- | ------ | -------- | --------------------------------------------------------- | --------------------------------------------------------------- |
| `csr_pem` | string | ✅       | PEM format, bắt đầu `-----BEGIN CERTIFICATE REQUEST-----` | PKCS#10 CSR. Phải self-signed bằng priv_c (Proof of Possession) |
| `id_c`    | string | ✅       | 3–64 chars, `[a-zA-Z0-9._-]+`                             | Client identity. Embedded vào X.509 Subject CN                  |
| `email`   | string | ✅       | RFC 5322                                                  | Phải khớp với `sub` claim trong JWT                             |

#### Example Request

```json
POST /v1/pki/register
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Request-ID: 770e8400-e29b-41d4-a716-446655440222
X-Timestamp: 1715500120

{
  "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----\nMIIC...base64...\n-----END CERTIFICATE REQUEST-----",
  "id_c": "alice",
  "email": "alice@example.com"
}
```

#### Processing Flow

```
1. Validate headers và body fields
2. Verify JWT: signature, exp, purpose=="pki_registration"
3. Check jti: GET jwt_used:{jti} → nếu == "1" → 401 REG_TOKEN_USED
4. Verify email == JWT.sub
5. Mark JWT consumed: SET jwt_used:{jti} 1 (không xóa — giữ để block replay trong TTL)
6. gRPC: CAService.RegisterUser({ csr_pem, user_id: JWT.sub })
   - CA: Truy vấn CSDL kiểm tra định danh (`SELECT cert_serial FROM users WHERE email = sub`)
   - CA: Nếu user đã có `cert_serial`, kiểm tra tiếp trạng thái (`SELECT status FROM user_certificates WHERE serial_number = cert_serial`)
   - CA: Nếu `status == 'active'` → Abort luồng, trả về lỗi gRPC ALREADY_EXISTS (HTTP 409 IDENTITY_ALREADY_REGISTERED) để chặn chiếm quyền.
   - CA: parse CSR, extract pub_c
   - CA: verify CSR self-signature (Proof of Possession)
   - CA: sign CSR bằng privKeyRSA_ca → X.509
   - CA: Bắt đầu DB Transaction (BEGIN):
      + INSERT user_certificates(user_id, serial_number, public_key_pem, status='active', not_after)
      + UPDATE users SET cert_serial = serial_number WHERE email = sub
   - CA: Đóng DB Transaction (COMMIT)
7. Return 201 + cert_pem
```

#### DB Usage (CA Service)

```sql
INSERT INTO user_certificates (
  id, user_id, serial_number, public_key_pem, status, not_after, created_at
) VALUES (uuid(), , , , 'active', , NOW());

UPDATE users SET cert_serial =  WHERE email = ;
```

| Bảng                | Thao tác | Columns                                                             |
| ------------------- | -------- | ------------------------------------------------------------------- |
| `user_certificates` | INSERT   | `user_id`, `serial_number`, `public_key_pem`, `status`, `not_after` |
| `users`             | UPDATE   | `cert_serial`                                                       |

#### Success Response (201 Created)

```json
{
  "success": true,
  "message": "X.509 certificate issued",
  "data": {
    "cert_pem": "-----BEGIN CERTIFICATE-----\nMIID...base64...\n-----END CERTIFICATE-----",
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "issued_at": 1715500120,
    "expires_at": 1747036120,
    "subject_cn": "alice",
    "ca_fingerprint": "sha256:AABBCCDDEEFF..."
  },
  "request_id": "770e8400-e29b-41d4-a716-446655440222",
  "timestamp": "2026-05-12T23:18:40+07:00"
}
```

#### Error Responses

| HTTP | Error Code                    | Khi nào                                 |
| ---- | ----------------------------- | --------------------------------------- |
| 400  | `INVALID_CSR_FORMAT`          | CSR không phải PEM hợp lệ               |
| 400  | `MISSING_REQUIRED_FIELD`      | Thiếu field bắt buộc                    |
| 401  | `REG_TOKEN_INVALID`           | JWT signature fail                      |
| 401  | `REG_TOKEN_EXPIRED`           | JWT exp qua                             |
| 401  | `REG_TOKEN_USED`              | JWT jti đã dùng                         |
| 400  | `CSR_SIGNATURE_INVALID`       | CA không verify được CSR self-signature |
| 409  | `IDENTITY_ALREADY_REGISTERED` | id_c đã có cert active                  |
| 502  | `CA_SERVICE_UNAVAILABLE`      | gRPC call fail                          |

#### Security Notes

- JWT consumed TRƯỚC khi gRPC call → tránh double-submission race condition
- CA thực hiện Proof of Possession: client phải sở hữu priv_c tương ứng csr_pem
- privKeyRSA_c không bao giờ rời client device
- cert_pem là public data — an toàn lưu IndexedDB/LocalStorage

---

### 10.2 POST /v1/pki/renew

**Purpose:** Gia hạn certificate sắp hết hạn. Điều kiện: trong 30 ngày trước hết hạn hoặc 7 ngày grace period sau hết hạn.

**Authentication:** Ticket_v + Auth_v (Phase 4 session)

#### Request Body

| Field         | Type            | Required | Mô tả                                           |
| ------------- | --------------- | -------- | ----------------------------------------------- |
| `cert_serial` | string          | ✅       | Cert cần gia hạn                                |
| `csr_pem`     | string          | ✅       | CSR mới, phải self-signed (Proof of Possession) |
| `ticket_v`    | string (base64) | ✅       | Service ticket từ Phase 3                       |
| `auth_v`      | string (base64) | ✅       | Authenticator `E_{K_{c,v}}[ID_c, TS, Nonce]`    |

#### Success Response (201 Created)

```json
{
  "success": true,
  "message": "Certificate renewed",
  "data": {
    "new_cert_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "new_cert_serial": "3F5B7D9E0A2C4E6F8A0B2C4D6E8F0A2B",
    "issued_at": 1715500420,
    "expires_at": 1747036420,
    "previous_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A"
  },
  "request_id": "aa0e8400-e29b-41d4-a716-446655440555",
  "timestamp": "2026-05-12T23:23:40+07:00"
}
```

#### Error Responses

| HTTP | Error Code              | Khi nào                          |
| ---- | ----------------------- | -------------------------------- |
| 400  | `CSR_SIGNATURE_INVALID` | CSR self-signature không hợp lệ  |
| 401  | `TICKET_EXPIRED`        | Ticket_v hết hạn                 |
| 403  | `CERT_NOT_OWNED`        | cert_serial không thuộc ID_c     |
| 409  | `CERT_ALREADY_REVOKED`  | Không thể gia hạn cert đã revoke |
| 422  | `RENEWAL_NOT_ALLOWED`   | Cert còn >30 ngày hạn (quá sớm)  |

---

### 10.3 POST /v1/pki/revoke (User Self-Revoke)

**Purpose:** User tự thu hồi cert của mình (key compromise). Cần chứng minh possession bằng digital signature.

**Authentication:** Ticket_v + Auth_v + RSA Signature

**gRPC Call:** `CAService.RevokeCertificate(RevokeCertificateRequest)`

#### Request Body

| Field         | Type            | Required | Validation                                                     | Mô tả                                     |
| ------------- | --------------- | -------- | -------------------------------------------------------------- | ----------------------------------------- |
| `cert_serial` | string          | ✅       | —                                                              | Cert cần revoke                           |
| `reason`      | string          | ✅       | Enum: `KEY_COMPROMISE`, `SUPERSEDED`, `CESSATION_OF_OPERATION` | Lý do revoke                              |
| `ticket_v`    | string (base64) | ✅       | —                                                              | Service ticket                            |
| `auth_v`      | string (base64) | ✅       | —                                                              | Authenticator                             |
| `signature`   | string (base64) | ✅       | RSA-PSS-SHA256                                                 | `Sign({cert_serial, reason, ts}, priv_c)` |

#### DB Usage (CA Service)

```sql
UPDATE user_certificates
SET status = 'revoked', revoked_at = NOW(), revocation_reason =
WHERE serial_number = ;
-- Redis: SET revocation:{cert_sn} REVOKED EX 180
```

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "Certificate revoked",
  "data": {
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "revoked_at": 1715500520,
    "reason": "KEY_COMPROMISE"
  },
  "request_id": "bb0e8400-e29b-41d4-a716-446655440666",
  "timestamp": "2026-05-12T23:25:20+07:00"
}
```

#### Error Responses

| HTTP | Error Code               | Khi nào                   |
| ---- | ------------------------ | ------------------------- |
| 400  | `INVALID_REASON`         | reason không trong enum   |
| 401  | `SIGNATURE_INVALID`      | RSA signature verify fail |
| 401  | `TICKET_EXPIRED`         | Ticket_v hết hạn          |
| 404  | `CERT_NOT_FOUND`         | Serial không tồn tại      |
| 409  | `ALREADY_REVOKED`        | Cert đã bị revoke         |
| 502  | `CA_SERVICE_UNAVAILABLE` | gRPC fail                 |

---

## 11. Authentication APIs

---

### 11.1 POST /v1/auth/as-req

**Purpose:** Phase 2 AS Exchange. Client gửi PKINIT Pre-auth request → KDC xác thực PKI → cấp TGT + K\_{c,tgs}.

**Authentication:** Certificate-bound (cert_sn + Pre-auth RSA Signature)

**Rate Limit:** 10 request / IP / 5 phút | **gRPC Call:** `KDCService.RequestTGT(ASRequest)`

#### Request Body

| Field                | Type            | Required | Validation          | Mô tả                                                  |
| -------------------- | --------------- | -------- | ------------------- | ------------------------------------------------------ |
| `client_id`          | string          | ✅       | 3–64 chars          | ID_c — phải khớp cert Subject CN                       |
| `tgs_id`             | string          | ✅       | `krbtgt/MINIBANK`   | Target TGS identifier                                  |
| `nonce1`             | string (base64) | ✅       | 16 bytes random     | Chống replay. Echo trong AS_REP                        |
| `timestamp`          | number          | ✅       | Unix epoch, ±5 phút | TS_1 — freshness proof                                 |
| `cert_sn`            | string          | ✅       | hex string          | Serial của X.509 cert client                           |
| `pre_auth_signature` | string (base64) | ✅       | RSA-PSS-SHA256      | `Sign({client_id, tgs_id, nonce1, timestamp}, priv_c)` |

#### Example Request

```json
POST /v1/auth/as-req
Content-Type: application/json
X-Request-ID: cc0e8400-e29b-41d4-a716-446655440777
X-Timestamp: 1715500600

{
  "client_id": "alice",
  "tgs_id": "krbtgt/MINIBANK",
  "nonce1": "dGhpcyBpcyAxNiBieXRl",
  "timestamp": 1715500600,
  "cert_sn": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
  "pre_auth_signature": "base64RsaSignatureOfPayload..."
}
```

#### Processing Flow

```
1. Validate fields format
2. Check |now - timestamp| < 300s → nếu không → 408
3. Check rate limit: rate:as:{ip}
4. gRPC: KDCService.RequestTGT(...)

   KDC xử lý:
   a. Lookup cert: CAService.GetCertificate(cert_sn) → lấy pub_c, status
   b. Verify cert status == VALID (not revoked, not expired)
   c. Verify client_id == cert.subject_cn
   d. Verify pre_auth_signature dùng pub_c
   e. Anti-replay: SET nonce:as:{SHA256(nonce1+client_id)} 1 EX 300 NX → nếu fail → 409
   f. Gen K_{c,tgs} = 32 random bytes
   g. TGT = E_{K_tgs}[{client_id, K_{c,tgs}, exp: now+1800}]
   h. Payload = Sign_{priv_KDC}({K_{c,tgs}, TGT, nonce1, ts})
   i. AS_REP = E_{pub_c}[Payload]
```

#### Redis Usage (KDC)

| Key                                   | Value | TTL  |
| ------------------------------------- | ----- | ---- |
| `nonce:as:{SHA256(nonce1+client_id)}` | `1`   | 300s |

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "AS_REP generated",
  "data": {
    "encrypted_payload": "base64EncodedE_pub_c_Data...",
    "tgt_expiry": 1715502400
  },
  "request_id": "cc0e8400-e29b-41d4-a716-446655440777",
  "timestamp": "2026-05-12T23:30:00+07:00"
}
```

**AS_REP Plaintext (sau khi client decrypt + verify KDC signature):**

```json
{
  "K_c_tgs": "<32-byte-base64-session-key>",
  "TGT": "<base64-opaque-E_K_tgs-blob>",
  "nonce1": "dGhpcyBpcyAxNiBieXRl",
  "kdc_signature": "<base64-RSA-sig>"
}
```

#### Error Responses

| HTTP | Error Code          | Khi nào                        |
| ---- | ------------------- | ------------------------------ |
| 400  | `INVALID_NONCE`     | nonce1 sai format/length       |
| 401  | `CERT_NOT_FOUND`    | cert_sn không tồn tại          |
| 401  | `CERT_REVOKED`      | Cert trong CRL                 |
| 401  | `CERT_EXPIRED`      | Cert quá not_after             |
| 401  | `IDENTITY_MISMATCH` | client_id ≠ cert Subject CN    |
| 401  | `SIGNATURE_INVALID` | pre_auth_signature verify fail |
| 408  | `REQUEST_EXPIRED`   | timestamp ngoài ±5 phút        |
| 409  | `REPLAY_DETECTED`   | nonce1 đã dùng                 |
| 429  | `AUTH_RATE_LIMITED` | >10 req/IP/5min                |
| 502  | `KDC_UNAVAILABLE`   | gRPC fail                      |

#### Security Notes

- PKINIT Pre-auth: KDC verify signature TRƯỚC khi cấp bất kỳ resource nào
- TGT opaque với client — chỉ KDC có K_tgs để decrypt
- Client verify KDC signature bằng hardcoded pubKeyRSA_KDC
- Client zeroing PIN + priv_c RAM sau khi decrypt AS_REP

---

### 11.2 POST /v1/auth/tgs-req

**Purpose:** Phase 3 TGS Exchange. Client dùng TGT xin Ticket*v + K*{c,v} để giao tiếp với Bank Service.

**Authentication:** TGT + Authenticator `E_{K_{c,tgs}}[...]`

**Rate Limit:** 10 request / cert_sn / 5 phút | **gRPC Call:** `KDCService.RequestServiceTicket(TGSRequest)`

#### Request Body

| Field             | Type            | Required | Validation                                | Mô tả                                                                |
| ----------------- | --------------- | -------- | ----------------------------------------- | -------------------------------------------------------------------- |
| `service_id`      | string          | ✅       | `bank-service`                            | Target service identifier                                            |
| `tgt_ciphertext`  | string (base64) | ✅       | base64                                    | TGT opaque blob từ AS_REP                                            |
| `authenticator`   | string (base64) | ✅       | base64                                    | `E_{K_{c,tgs}}[{ID_c, TS_3, Nonce2, requested_service}]` AES-256-GCM |
| `cert_sn`         | string          | ✅       | hex                                       | Serial X.509 cert                                                    |
| `nonce2`          | string (base64) | ✅       | 16 bytes                                  | Random nonce chống replay                                            |
| `requested_scope` | string          | ✅       | Enum: `transfer:internal`, `account:read` | Scope yêu cầu                                                        |

#### Example Request

```json
POST /v1/auth/tgs-req
Content-Type: application/json
X-Request-ID: dd0e8400-e29b-41d4-a716-446655440888
X-Timestamp: 1715500700

{
  "service_id": "bank-service",
  "tgt_ciphertext": "base64TgtBlob...",
  "authenticator": "base64AES256GCM_Auth_c...",
  "cert_sn": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
  "nonce2": "cmFuZG9tMTZieXRlcw==",
  "requested_scope": "transfer:internal"
}
```

**Authenticator Plaintext (trước khi encrypt):**

```json
{
  "ID_c": "alice",
  "TS_3": 1715500700,
  "nonce_req": "cmFuZG9tMTZieXRlcw==",
  "requested_service": "bank-service"
}
```

#### Processing Flow (KDC)

```
1. Decrypt TGT bằng K_tgs → lấy {ID_c, K_{c,tgs}, exp}
2. Kiểm tra TGT exp > now
3. Decrypt authenticator bằng K_{c,tgs} → lấy {ID_c_auth, TS_3, nonce_req, requested_service}
4. Verify ID_c_auth == ID_c (từ TGT)
5. Verify requested_service == service_id
6. Check |now - TS_3| < 300s
7. Anti-replay: SET nonce:tgs:{SHA256(nonce_req+ID_c+TS_3)} 1 EX 300 NX
8. Check cert_sn revocation (Redis cache → CA gRPC)
9. Authorize scope: kiểm tra requested_scope với user permissions
10. Gen K_{c,v} = 32 random bytes
11. Ticket_v = E_{K_v}[{ID_c, sname=service_id, TS_4, exp=now+300, K_{c,v}, pub_c, cert_sn, scope}]
12. TGS_REP = E_{K_{c,tgs}}[{K_{c,v}, ID_v, TS_4, nonce2_echo, nonce_req_echo, Ticket_v}]
```

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "TGS_REP generated",
  "data": {
    "encrypted_payload": "base64EncodedE_K_c_tgs_Data...",
    "ticket_expiry": 1715501000
  },
  "request_id": "dd0e8400-e29b-41d4-a716-446655440888",
  "timestamp": "2026-05-12T23:31:40+07:00"
}
```

**TGS*REP Plaintext (client decrypt bằng K*{c,tgs}):**

```json
{
  "K_c_v": "<32-byte-base64-service-session-key>",
  "ID_v": "bank-service",
  "TS_4": 1715500720,
  "nonce2_echo": "cmFuZG9tMTZieXRlcw==",
  "nonce_req_echo": "cmFuZG9tMTZieXRlcw==",
  "Ticket_v": "<base64-opaque-E_K_v-blob>"
}
```

#### Error Responses

| HTTP | Error Code           | Khi nào                         |
| ---- | -------------------- | ------------------------------- |
| 400  | `INVALID_TGT_FORMAT` | TGT không phải base64 hợp lệ    |
| 400  | `INVALID_AUTH_C`     | Authenticator decrypt fail      |
| 401  | `TGT_EXPIRED`        | TGT lifetime vượt               |
| 401  | `IDENTITY_MISMATCH`  | ID_c trong Auth_c ≠ TGT         |
| 401  | `CERT_REVOKED`       | Cert bị revoke sau khi TGT cấp  |
| 403  | `SCOPE_DENIED`       | requested_scope không được phép |
| 408  | `AUTH_C_EXPIRED`     | TS_3 ngoài ±5 phút              |
| 409  | `REPLAY_DETECTED`    | nonce_req đã tồn tại            |
| 502  | `KDC_UNAVAILABLE`    | gRPC fail                       |

---

## 12. Banking APIs

---

### 12.1 POST /v1/bank/transfer

**Purpose:** Phase 4 AP Exchange. Chuyển tiền giữa 2 tài khoản với full Kerberos mutual auth + PKI digital signature. Mọi giao dịch được ghi vào immutable ledger với hash chaining.

**Authentication:** Ticket_v + Auth_v + Digital Signature (RSA-PSS-SHA256)

**Rate Limit:** 20 requests / session / 1 phút | **Idempotency:** `X-Idempotency-Key` bắt buộc

**gRPC Call:** `BankService.TransferMoney(TransferRequest)`

#### Request Headers

| Header              | Required | Mô tả                                         |
| ------------------- | -------- | --------------------------------------------- |
| `Content-Type`      | ✅       | `application/json`                            |
| `X-Request-ID`      | ✅       | UUID v4                                       |
| `X-Timestamp`       | ✅       | Unix epoch — phải align với TS_5 trong Auth_v |
| `X-Idempotency-Key` | ✅       | UUID v4. Chống double-submit. TTL 24h         |

#### Request Body

| Field             | Type            | Required | Mô tả                                          |
| ----------------- | --------------- | -------- | ---------------------------------------------- |
| `ticket_v`        | string (base64) | ✅       | `E_{K_v}[ID_c, K_{c,v}, pub_c, scope, exp]`    |
| `authenticator`   | string (base64) | ✅       | `E_{K_{c,v}}[ID_c, TS_5, Nonce_3]` AES-256-GCM |
| `cipher`          | string (base64) | ✅       | `E_{K_{c,v}}[TransferPayload + RSA_Signature]` |
| `cert_sn`         | string          | ✅       | Certificate serial                             |
| `idempotency_key` | string          | ✅       | Phải khớp header `X-Idempotency-Key`           |

**Transfer Payload (plaintext bên trong cipher, sau khi Bank decrypt):**

| Field          | Type            | Required | Validation    | Mô tả                                         |
| -------------- | --------------- | -------- | ------------- | --------------------------------------------- |
| `from_account` | string          | ✅       | 10 digits     | Tài khoản nguồn. Phải thuộc ID_c              |
| `to_account`   | string          | ✅       | 10 digits     | Tài khoản đích. Phải khác from_account        |
| `amount`       | string          | ✅       | Numeric, >0   | Số tiền (cents). String tránh float precision |
| `currency`     | string          | ✅       | ISO 4217      | VND, USD, EUR                                 |
| `memo`         | string          | ❌       | max 255 chars | Ghi chú giao dịch                             |
| `ts_5`         | number          | ✅       | Unix epoch    | Phải khớp TS_5 trong Auth_v                   |
| `nonce_3`      | string (base64) | ✅       | 16 bytes      | Anti-replay nonce                             |

#### Security Checks (Bank Service — theo thứ tự bắt buộc)

```
| # | Check | Fail / Match → |
| :--- | :--- | :--- |
| 1 | Giải mã `Ticket_v` bằng `K_v` → kiểm tra `exp` | 401 `TICKET_EXPIRED` |
| 2 | Giải mã `Auth_v` bằng `K_{c,v}` → xác thực `ID_c`, check `|now - TS_5| < 300s` | 401 `AUTH_INVALID` / 408 `REQUEST_EXPIRED` |
| 3 | So khớp `ID_c` trong `Auth_v` == `ID_c` trong `Ticket_v` | 401 `IDENTITY_MISMATCH` |
| 4 | **Idempotency:** Kiểm tra `X-Idempotency-Key` | Trả về Cached Result (200) hoặc 409 `IDEMPOTENT_DUPLICATE` |
| 5 | **Anti-replay:** Check `Nonce_3` (Redis NX) | 409 `REPLAY_DETECTED` |
| 6 | Giải mã `cipher` bằng `K_{c,v}` → Payload + Signature | 400 `INVALID_CIPHER_PAYLOAD` |
| 7 | Xác thực Chữ ký số RSA (Payload, `pub_c` từ `Ticket_v`) | 401 `SIGNATURE_INVALID` |
| 8 | Kiểm tra thu hồi chứng chỉ (`cert_sn`) | 403 `CERT_REVOKED` |
| 9 | Kiểm tra Quyền (Scope) và Sở hữu (Ownership) | 403 `SCOPE_DENIED` / `NOT_OWNER` |
| 10 | Kiểm tra Số dư và Hạn mức | 422 `INSUFFICIENT_FUNDS` |
```

#### DB Usage

| Bảng                  | Thao tác          | Columns                                                                  |
| --------------------- | ----------------- | ------------------------------------------------------------------------ |
| `accounts`            | SELECT FOR UPDATE | `balance`, `daily_transfer_limit`                                        |
| `accounts`            | UPDATE            | `balance`, `updated_at`                                                  |
| `transactions`        | INSERT            | All fields                                                               |
| `transaction_details` | INSERT            | `transaction_id`, `status_before`, `status_after`, `changed_by`, `notes` |
| `used_nonces`         | INSERT            | `nonce`, `used_at`, `expires_at`                                         |

#### Redis Usage

| Key                            | Value         | TTL                        |
| ------------------------------ | ------------- | -------------------------- |
| `nonce:ap:{nonce_3}`           | `1`           | 300s (primary anti-replay) |
| `revocation:{cert_sn}`         | status        | 180s                       |
| `idempotent:{idempotency_key}` | cached result | 86400s                     |

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "Transfer completed",
  "data": {
    "ap_rep": "base64EncodedE_K_c_v_Result...",
    "transaction_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
  },
  "request_id": "ff0e8400-e29b-41d4-a716-446655441000",
  "timestamp": "2026-05-12T23:40:00+07:00"
}
```

**AP*REP Plaintext (client decrypt bằng K*{c,v}, verify TS_5+1):**

```json
{
  "result": "SUCCESS",
  "transaction_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "amount": "5000000",
  "currency": "VND",
  "ts_5_plus_1": 1715500801
}
```

#### Error Responses

| HTTP | Error Code                 | Khi nào                       |
| ---- | -------------------------- | ----------------------------- |
| 400  | `INVALID_TICKET_FORMAT`    | ticket_v malformed            |
| 401  | `TICKET_EXPIRED`           | Ticket_v hết hạn              |
| 401  | `IDENTITY_MISMATCH`        | ID_c mismatch                 |
| 401  | `SIGNATURE_INVALID`        | RSA signature fail            |
| 403  | `CERT_REVOKED`             | Cert bị revoke                |
| 403  | `SCOPE_DENIED`             | Scope không đủ                |
| 403  | `NOT_OWNER`                | from_account không thuộc ID_c |
| 408  | `REQUEST_EXPIRED`          | TS_5 ngoài ±5 phút            |
| 409  | `REPLAY_DETECTED`          | Nonce_3 đã dùng               |
| 409  | `IDEMPOTENT_DUPLICATE`     | Idempotency-Key đã xử lý      |
| 422  | `INSUFFICIENT_FUNDS`       | balance < amount              |
| 422  | `DAILY_LIMIT_EXCEEDED`     | Vượt hạn mức ngày             |
| 422  | `ACCOUNT_FROZEN`           | Tài khoản bị đóng băng        |
| 422  | `ACCOUNT_LOCKED`           | Tài khoản bị khóa             |
| 422  | `ACCOUNT_NOT_FOUND`        | from/to account không tồn tại |
| 422  | `SELF_TRANSFER`            | from == to                    |
| 429  | `TRANSFER_RATE_LIMITED`    | Rate limit vượt               |
| 502  | `BANK_SERVICE_UNAVAILABLE` | gRPC fail                     |

---

### 12.2 POST /v1/bank/balance

**Purpose:** Truy vấn số dư tài khoản. Read-only, không ghi ledger. Vẫn yêu cầu full authentication chain Phase 4.

**Authentication:** Ticket_v + Auth_v (scope: `account:read`)

**gRPC Call:** `BankService.GetBalance(BalanceRequest)`

#### Request Body

| Field           | Type            | Required | Mô tả                          |
| --------------- | --------------- | -------- | ------------------------------ |
| `ticket_v`      | string (base64) | ✅       | Service ticket                 |
| `authenticator` | string (base64) | ✅       | `E_{K_{c,v}}[ID_c, TS, Nonce]` |
| `account_id`    | string          | ✅       | Account number 10 digits       |
| `cert_sn`       | string          | ✅       | Certificate serial             |

#### DB Usage: `accounts` `users`

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "ap_rep": "base64EncodedBytes...",
    "balance": "100000000",
    "currency": "VND"
  },
  "request_id": "110e8400-e29b-41d4-a716-446655441111",
  "timestamp": "2026-05-12T23:41:40+07:00"
}
```

#### Error Responses

| HTTP | Error Code                 | Khi nào                           |
| ---- | -------------------------- | --------------------------------- |
| 401  | `TICKET_EXPIRED`           | Ticket_v hết hạn                  |
| 403  | `ACCOUNT_ACCESS_DENIED`    | account_id không thuộc ID_c       |
| 403  | `SCOPE_DENIED`             | Ticket scope thiếu `account:read` |
| 404  | `ACCOUNT_NOT_FOUND`        | account_id không tồn tại          |
| 502  | `BANK_SERVICE_UNAVAILABLE` | gRPC fail                         |

---

### 12.3 POST /v1/bank/transactions

**Purpose:** Lịch sử giao dịch với cursor-based pagination. Response mã hóa bằng K\_{c,v}.

**Authentication:** Ticket_v + Auth_v (scope: `account:read`)

**gRPC Call:** `BankService.GetTransactions(TransactionHistoryRequest)`

#### Request Body

| Field               | Type            | Required | Validation        | Mô tả                                 |
| ------------------- | --------------- | -------- | ----------------- | ------------------------------------- |
| `ticket_v`          | string (base64) | ✅       | base64            | Service ticket                        |
| `authenticator`     | string (base64) | ✅       | base64            | Authenticator                         |
| `account_id`        | string          | ✅       | account_number    | Tài khoản cần xem lịch sử             |
| `cert_sn`           | string          | ✅       | hex               | Certificate serial                    |
| `cursor_last_tx_id` | string          | ❌       | UUID              | ID của tx cuối cùng. Rỗng = trang đầu |
| `limit`             | number          | ❌       | 1–100, default 20 | Số records mỗi trang                  |
| `from_ts`           | number          | ❌       | Unix epoch        | Filter từ timestamp                   |
| `to_ts`             | number          | ❌       | Unix epoch        | Filter đến timestamp                  |

#### DB Usage: `transactions` `transaction_details`

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "ap_rep": "base64EncodedBytes...",
    "records": [
      {
        "transaction_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "from_account": "1000000001",
        "to_account": "1000000002",
        "amount": "5000000",
        "currency": "VND",
        "memo": "Thanh toan hoa don thang 5",
        "created_at": 1715500800,
        "status": "completed",
        "status_trail": [
          {
            "status_before": null,
            "status_after": "completed",
            "changed_by": "system",
            "changed_at": 1715500802,
            "notes": "Transfer completed"
          }
        ]
      }
    ],
    "next_cursor": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "has_more": true
  },
  "request_id": "220e8400-e29b-41d4-a716-446655441222",
  "timestamp": "2026-05-12T23:43:20+07:00"
}
```

#### Error Responses

| HTTP | Error Code                 | Khi nào                     |
| ---- | -------------------------- | --------------------------- |
| 400  | `INVALID_PAGINATION`       | limit ngoài 1-100           |
| 401  | `TICKET_EXPIRED`           | Ticket hết hạn              |
| 403  | `ACCOUNT_ACCESS_DENIED`    | account_id không thuộc ID_c |
| 502  | `BANK_SERVICE_UNAVAILABLE` | gRPC fail                   |

---

## 13. Certificate Management APIs

---

### 13.1 GET /v1/cert/status/:cert_serial

**Purpose:** Real-time certificate status check — CRL, expiry, chain trust.

**Authentication:** Public | **Rate Limit:** 100 req / min / IP

**gRPC Call:** `CAService.CheckRevocation(CheckRevocationRequest)`

**Redis Cache:** `revocation:{cert_sn}` TTL=180s

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "status": "VALID",
    "subject_cn": "alice",
    "issued_at": 1715500120,
    "expires_at": 1747036120,
    "days_remaining": 364,
    "revocation_reason": null,
    "revoked_at": null,
    "ca_chain_valid": true,
    "crl_last_checked": 1715501090
  },
  "request_id": "330e8400-e29b-41d4-a716-446655441333",
  "timestamp": "2026-05-12T23:45:00+07:00"
}
```

**Status Enum:** `VALID` | `EXPIRED` | `REVOKED` | `UNKNOWN`

#### Error Responses

| HTTP | Error Code        | Khi nào                        |
| ---- | ----------------- | ------------------------------ |
| 404  | `CERT_NOT_FOUND`  | Serial không tồn tại           |
| 503  | `CRL_UNAVAILABLE` | CA CRL endpoint không đạt được |

---

### 13.2 POST /v1/cert/revoke (Admin Only)

**Purpose:** Admin thu hồi certificate khẩn cấp (phát hiện compromise từ phía server).

**Authentication:** Admin JWT

**gRPC Call:** `CAService.RevokeCertificate(RevokeCertificateRequest)`

**Audit:** ✅ Critical security event

#### Request Body

| Field           | Type   | Required | Validation                                                                                             | Mô tả                      |
| --------------- | ------ | -------- | ------------------------------------------------------------------------------------------------------ | -------------------------- |
| `serial_number` | string | ✅       | hex string                                                                                             | Serial của cert cần revoke |
| `reason`        | string | ✅       | Enum: `KEY_COMPROMISE`, `CA_COMPROMISE`, `CESSATION_OF_OPERATION`, `SUPERSEDED`, `PRIVILEGE_WITHDRAWN` | Lý do revoke               |
| `admin_note`    | string | ❌       | max 500 chars                                                                                          | Ghi chú nội bộ admin       |

#### Processing Flow

```
1. Verify Admin JWT (RS256, audience="admin-api")
2. gRPC: CAService.RevokeCertificate({serial_number, reason})
3. CA: UPDATE user_certificates SET status='revoked', revocation_reason= WHERE serial_number=
4. CA: UPDATE users SET cert_serial=NULL WHERE cert_serial=
5. Redis: SET revocation:{sn} REVOKED EX 180
6. Audit log: INSERT gateway_audit_log
7. Return 200
```

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "Certificate revoked successfully",
  "data": {
    "serial_number": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "revoked_at": 1715501220,
    "reason": "KEY_COMPROMISE"
  },
  "request_id": "440e8400-e29b-41d4-a716-446655441444",
  "timestamp": "2026-05-12T23:46:40+07:00"
}
```

#### Error Responses

| HTTP | Error Code               | gRPC Code          | Khi nào                   |
| ---- | ------------------------ | ------------------ | ------------------------- |
| 401  | `UNAUTHORIZED`           | —                  | Admin JWT invalid/expired |
| 403  | `FORBIDDEN`              | `PermissionDenied` | Không có quyền admin      |
| 404  | `CERT_NOT_FOUND`         | `NotFound`         | Serial không tồn tại      |
| 409  | `ALREADY_REVOKED`        | `AlreadyExists`    | Cert đã bị revoke         |
| 502  | `CA_SERVICE_UNAVAILABLE` | —                  | gRPC fail                 |

---

### 13.3 GET /v1/cert/crl

**Purpose:** Trả Certificate Revocation List (CRL) đầy đủ. Gateway cache 5 phút. Hỗ trợ conditional GET (ETag).

**Authentication:** Public | **Cache:** Redis `crl:current` TTL=300s

#### Request Headers

| Header          | Required | Mô tả                                                |
| --------------- | -------- | ---------------------------------------------------- |
| `X-Request-ID`  | ✅       | UUID v4                                              |
| `If-None-Match` | ❌       | ETag từ response trước. Nếu match → 304 Not Modified |

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "crl_version": 42,
    "issued_at": 1715500000,
    "next_update": 1715500300,
    "revoked_serials": [
      {
        "cert_serial": "AABBCCDDEEFF00112233445566778899",
        "revoked_at": 1715490000,
        "reason": "KEY_COMPROMISE"
      }
    ],
    "ca_signature": "base64CASignatureOfCRLPayload..."
  },
  "request_id": "550e8400-e29b-41d4-a716-446655441555",
  "timestamp": "2026-05-12T23:48:00+07:00"
}
```

**Response 304 Not Modified:** Khi `If-None-Match` khớp ETag hiện tại.

#### Security Notes

- CRL signed bởi CA (privKeyRSA_ca) — client nên verify ca_signature
- ETag = `"crl-v{crl_version}"` — deterministic
- `next_update`: client nên re-fetch trước thời điểm này

---

### 13.4 POST /v1/cert/ocsp

**Purpose:** OCSP Stapling — truy vấn trạng thái 1 cert cụ thể mà không cần tải toàn bộ CRL. Response được CA ký số.

**Authentication:** Public | **gRPC Call:** `CAService.CheckRevocation(CheckRevocationRequest)`

#### Request Body

| Field              | Type            | Required | Validation | Mô tả                                               |
| ------------------ | --------------- | -------- | ---------- | --------------------------------------------------- |
| `cert_serial`      | string          | ✅       | hex        | Cert cần check                                      |
| `issuer_name_hash` | string (base64) | ✅       | SHA-256    | SHA-256 của issuer DN — bind response với CA cụ thể |
| `issuer_key_hash`  | string (base64) | ✅       | SHA-256    | SHA-256 của issuer public key                       |
| `nonce`            | string (base64) | ✅       | 16 bytes   | Anti-replay — phải được echo trong signed response  |

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "status": "GOOD",
    "this_update": 1715501290,
    "next_update": 1715501590,
    "nonce_echo": "cmFuZG9tMTZieXRlcw==",
    "ocsp_signature": "base64SignedOCSPResponse..."
  },
  "request_id": "660e8400-e29b-41d4-a716-446655441666",
  "timestamp": "2026-05-12T23:49:40+07:00"
}
```

**OCSP Status:** `GOOD` | `REVOKED` (kèm revocation_time, reason) | `UNKNOWN`

#### Error Responses

| HTTP | Error Code            | Khi nào                                             |
| ---- | --------------------- | --------------------------------------------------- |
| 400  | `INVALID_NONCE`       | Nonce thiếu hoặc sai length                         |
| 400  | `ISSUER_MISMATCH`     | issuer_name_hash hoặc issuer_key_hash không khớp CA |
| 502  | `CA_OCSP_UNAVAILABLE` | CA OCSP endpoint không đạt được                     |

---

### 13.5 GET /v1/cert/details/:cert_serial

**Purpose:** Tra cứu nội dung chi tiết của X.509 certificate theo serial. Phục vụ cho nghiệp vụ kiểm tra, đối soát hoặc hỗ trợ khách hàng nội bộ của bộ phận Admin.

**Authentication:** Strictly Admin JWT

**Rate Limit:** 20 requests / min / IP

#### Path Parameters

| Param         | Type   | Required | Mô tả                     |
| ------------- | ------ | -------- | ------------------------- |
| `cert_serial` | string | ✅       | Certificate serial number |

#### DB Usage: `user_certificates` `users`

#### Success Response (200 OK)

```json
{
  "success": true,
  "data": {
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "cert_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
    "subject_cn": "alice",
    "issued_at": 1715500120,
    "expires_at": 1747036120,
    "status": "active"
  },
  "request_id": "880e8400-e29b-41d4-a716-446655440333",
  "timestamp": "2026-05-12T23:20:00+07:00"
}
```

#### Error Responses

| HTTP | Error Code       | Khi nào                               |
| ---- | ---------------- | ------------------------------------- |
| 401  | `UNAUTHORIZED`   | Admin JWT thiếu hoặc invalid          |
| 403  | `FORBIDDEN`      | JWT hợp lệ nhưng không có quyền admin |
| 404  | `CERT_NOT_FOUND` | Serial không tồn tại                  |

---

## 14. API → Table Mapping

| API Endpoint                 | DB Tables                                                        | Thao tác                     | Ghi chú              |
| ---------------------------- | ---------------------------------------------------------------- | ---------------------------- | -------------------- |
| `POST /otp/request`          | —                                                                | —                            | Redis only           |
| `POST /otp/verify`           | —                                                                | —                            | Redis only           |
| `POST /pki/register`         | `users`, `user_certificates`                                     | INSERT, UPDATE               | CA Service thực hiện |
| `POST /pki/renew`            | `user_certificates`, `users`                                     | INSERT, UPDATE               | CA Service           |
| `POST /pki/revoke`           | `user_certificates`                                              | UPDATE                       | CA Service           |
| `POST /auth/as-req`          | `user_certificates`                                              | SELECT                       | KDC via CA gRPC      |
| `POST /auth/tgs-req`         | `user_certificates`                                              | SELECT                       | KDC via CA gRPC      |
| `POST /bank/transfer`        | `accounts`, `transactions`, `transaction_details`, `used_nonces` | SELECT, LOCK, UPDATE, INSERT | ACID transaction     |
| `POST /bank/balance`         | `accounts`, `users`                                              | SELECT                       |                      |
| `POST /bank/transactions`    | `transactions`, `transaction_details`                            | SELECT                       | Cursor pagination    |
| `GET /cert/status/:sn`       | `user_certificates`                                              | SELECT                       | Via CA gRPC          |
| `POST /cert/revoke`          | `user_certificates`, `users`                                     | UPDATE                       | Admin via CA gRPC    |
| `GET /cert/crl`              | `user_certificates`                                              | SELECT (revoked)             | Cached Redis         |
| `POST /cert/ocsp`            | `user_certificates`                                              | SELECT                       | Via CA gRPC          |
| `GET /cert/details/:cert_sn` | `user_certificates`, `users`                                     | SELECT                       |                      |

---

## 15. Proto → REST Mapping

### 15.1 CAService (ca.proto)

| gRPC Method         | REST Endpoint                             | Direction            |
| ------------------- | ----------------------------------------- | -------------------- |
| `RegisterUser`      | `POST /pki/register`                      | Gateway → CA         |
| `GetCertificate`    | `GET /pki/certificate/:sn`                | Internal (KDC, Bank) |
| `CheckRevocation`   | `GET /cert/status/:sn`, `POST /cert/ocsp` | Internal + Public    |
| `RevokeCertificate` | `POST /cert/revoke`, `POST /pki/revoke`   | Gateway → CA         |

**Request/Response Mapping — RegisterUser:**

```
REST body.csr_pem → gRPC RegisterUserRequest.csr_pem
JWT.sub           → gRPC RegisterUserRequest.user_id
gRPC RegisterUserResponse.certificate_pem → REST data.cert_pem
gRPC RegisterUserResponse.serial_number   → REST data.cert_serial
gRPC RegisterUserResponse.not_after_unix  → REST data.expires_at
```

**gRPC Error → HTTP:**

```
INVALID_ARGUMENT  → 400
NOT_FOUND         → 404
ALREADY_EXISTS    → 409
PERMISSION_DENIED → 403
INTERNAL          → 502
```

### 15.2 KDCService (kdc.proto)

| gRPC Method                        | REST Endpoint        |
| ---------------------------------- | -------------------- |
| `RequestTGT(ASRequest)`            | `POST /auth/as-req`  |
| `RequestServiceTicket(TGSRequest)` | `POST /auth/tgs-req` |

**Mapping — RequestTGT:**

```
REST body.client_id          → gRPC ASRequest.client_id
REST body.nonce1 (base64)    → gRPC ASRequest.nonce1 (bytes)
REST body.timestamp          → gRPC ASRequest.timestamp
REST body.cert_sn            → gRPC ASRequest.cert_sn
REST body.pre_auth_signature → gRPC ASRequest.pre_auth_signature (bytes)
gRPC ASResponse.encrypted_payload → REST data.encrypted_payload
gRPC ASResponse.tgt_expiry_unix   → REST data.tgt_expiry
```

**Mapping — RequestServiceTicket:**

```
REST body.service_id      → gRPC TGSRequest.service_id
REST body.tgt_ciphertext  → gRPC TGSRequest.tgt_ciphertext (bytes)
REST body.authenticator   → gRPC TGSRequest.authenticator (bytes)
REST body.cert_sn         → gRPC TGSRequest.cert_sn
REST body.nonce2          → gRPC TGSRequest.nonce2 (bytes)
REST body.requested_scope → gRPC TGSRequest.requested_scope
gRPC TGSResponse.encrypted_payload  → REST data.encrypted_payload
gRPC TGSResponse.ticket_expiry_unix → REST data.ticket_expiry
```

### 15.3 BankService (bank.proto)

| gRPC Method                                  | REST Endpoint             |
| -------------------------------------------- | ------------------------- |
| `TransferMoney(TransferRequest)`             | `POST /bank/transfer`     |
| `GetBalance(BalanceRequest)`                 | `POST /bank/balance`      |
| `GetTransactions(TransactionHistoryRequest)` | `POST /bank/transactions` |

**Mapping — TransferMoney:**

```
REST body.ticket_v      → gRPC TransferRequest.ticket_v (bytes)
REST body.authenticator → gRPC TransferRequest.authenticator (bytes)
REST body.cipher        → gRPC TransferRequest.cipher (bytes)
REST body.cert_sn       → gRPC TransferRequest.cert_sn
REST header Idempotency-Key → gRPC TransferRequest.idempotency_key
gRPC TransferResponse.ap_rep         → REST data.ap_rep
gRPC TransferResponse.transaction_id → REST data.transaction_id
```

**Mapping — GetBalance:**

```
REST body.ticket_v      → gRPC BalanceRequest.ticket_v (bytes)
REST body.authenticator → gRPC BalanceRequest.authenticator (bytes)
REST body.account_id    → gRPC BalanceRequest.account_id
REST body.cert_sn       → gRPC BalanceRequest.cert_sn
gRPC BalanceResponse.ap_rep   → REST data.ap_rep (base64)
gRPC BalanceResponse.balance  → REST data.balance (int64 → string)
gRPC BalanceResponse.currency → REST data.currency
```

**Mapping — GetTransactions:**

```
REST body.ticket_v           → gRPC TransactionHistoryRequest.ticket_v (bytes)
REST body.authenticator      → gRPC TransactionHistoryRequest.authenticator (bytes)
REST body.account_id         → gRPC TransactionHistoryRequest.account_id
REST body.cert_sn            → gRPC TransactionHistoryRequest.cert_sn
REST body.cursor_last_tx_id  → gRPC TransactionHistoryRequest.cursor_last_tx_id
REST body.limit              → gRPC TransactionHistoryRequest.limit
gRPC TransactionHistoryResponse.ap_rep      → REST data.ap_rep (base64)
gRPC TransactionHistoryResponse.records     → REST data.records[]
gRPC TransactionRecord.transaction_id       → REST data.records[].transaction_id
gRPC TransactionRecord.from_account         → REST data.records[].from_account
gRPC TransactionRecord.to_account           → REST data.records[].to_account
gRPC TransactionRecord.amount               → REST data.records[].amount (int64 → string)
gRPC TransactionRecord.currency             → REST data.records[].currency
gRPC TransactionRecord.memo                 → REST data.records[].memo
gRPC TransactionRecord.created_at           → REST data.records[].created_at
gRPC TransactionRecord.status               → REST data.records[].status
gRPC TransactionRecord.status_trail         → REST data.records[].status_trail
gRPC TransactionHistoryResponse.next_cursor → REST data.next_cursor
gRPC TransactionHistoryResponse.has_more    → REST data.has_more
```

---

## 16. API → Actors Mapping

#### Khách hàng

- **Định danh:** `otp/request`, `otp/verify`, `pki/register`.
- **Phiên làm việc:** `auth/as-req`, `auth/tgs-req`.
- **Nghiệp vụ:** `bank/balance`, `bank/transactions`, `bank/transfer`.
- **Tự quản trị:** `pki/renew`, `pki/revoke`.

#### Quản trị viên

- **Tra cứu:** `cert/details/:cert_sn`
- **Thu hồi:** `cert/revoke`

#### Hệ thống

- **Kiểm tra trạng thái:** `cert/status/:sn`
- **Tra cứu OSCP:** `cert/ocsp`
- **Danh sách thu hồi:** `cert/crl`
