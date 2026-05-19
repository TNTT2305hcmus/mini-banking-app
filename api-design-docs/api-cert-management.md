# API DESIGN DOCUMENT — Mini App Banking
# Certificate Management APIs (Mục 13)

---

## Mục lục

13. [Certificate Management APIs](#13-certificate-management-apis)

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
