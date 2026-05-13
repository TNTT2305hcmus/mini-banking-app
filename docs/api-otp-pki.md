# API DESIGN DOCUMENT — Mini App Banking
# OTP & PKI APIs (Mục 9–10)

---

## Mục lục

9. [OTP APIs](#9-otp-apis)
10. [PKI APIs](#10-pki-apis)

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

### 10.4 GET /v1/pki/certificate/:sn

**Purpose:** Tra cứu nội dung X.509 certificate theo serial number. Endpoint này được sử dụng nội bộ bởi KDC Service và Bank Service (qua gRPC `CAService.GetCertificate`) để lấy public key và trạng thái cert trong quá trình xác thực.

**Authentication:** Internal — chỉ được gọi bởi các internal services (KDC, Bank) qua mTLS gRPC. Không expose trực tiếp ra public internet.

**Rate Limit:** Không áp dụng (internal only)

**gRPC Call:** `CAService.GetCertificate(GetCertificateRequest)`

#### Path Parameters

| Param | Type   | Required | Mô tả                                          |
| ----- | ------ | -------- | ---------------------------------------------- |
| `sn`  | string | ✅       | Certificate serial number (hex string)         |

#### Processing Flow

```
1. Validate serial number format (hex string)
2. Lookup: SELECT * FROM user_certificates WHERE serial_number = :sn
3. Nếu không tìm thấy → 404 CERT_NOT_FOUND
4. Return certificate data bao gồm public_key_pem và status
```

#### DB Usage (CA Service)

```sql
SELECT serial_number, public_key_pem, status, not_after,
       revoked_at, revocation_reason, user_id
FROM user_certificates
WHERE serial_number = ;
```

| Bảng                | Thao tác | Columns                                                                              |
| ------------------- | -------- | ------------------------------------------------------------------------------------ |
| `user_certificates` | SELECT   | `serial_number`, `public_key_pem`, `status`, `not_after`, `revoked_at`, `user_id`   |

#### Success Response (200 OK)

```json
{
  "success": true,
  "message": "Certificate found",
  "data": {
    "cert_serial": "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A",
    "cert_pem": "-----BEGIN CERTIFICATE-----\nMIID...base64...\n-----END CERTIFICATE-----",
    "public_key_pem": "-----BEGIN PUBLIC KEY-----\nMIIB...base64...\n-----END PUBLIC KEY-----",
    "subject_cn": "alice",
    "status": "active",
    "issued_at": 1715500120,
    "expires_at": 1747036120,
    "revoked_at": null,
    "revocation_reason": null
  },
  "request_id": "ee0e8400-e29b-41d4-a716-446655440999",
  "timestamp": "2026-05-12T23:20:00+07:00"
}
```

#### Error Responses

| HTTP | Error Code               | Khi nào                      |
| ---- | ------------------------ | ---------------------------- |
| 400  | `MISSING_REQUIRED_FIELD` | serial number không cung cấp |
| 404  | `CERT_NOT_FOUND`         | Serial không tồn tại         |
| 502  | `CA_SERVICE_UNAVAILABLE` | gRPC call tới CA fail        |

#### Security Notes

- Endpoint này không expose ra public internet — chỉ internal services mới truy cập qua mTLS
- Public key trả về được các service dùng để verify chữ ký số của client
- Status field phải được caller kiểm tra trước khi sử dụng cert cho mục đích xác thực
