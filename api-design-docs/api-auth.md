# API DESIGN DOCUMENT — Mini App Banking
# Authentication APIs (Mục 11)

---

## Mục lục

11. [Authentication APIs](#11-authentication-apis)

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
