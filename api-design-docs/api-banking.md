# API DESIGN DOCUMENT — Mini App Banking
# Banking APIs (Mục 12)

---

## Mục lục

12. [Banking APIs](#12-banking-apis)

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
