# API DESIGN DOCUMENT — Mini App Banking
# Standard API Specifications (Mục 1–8)

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
