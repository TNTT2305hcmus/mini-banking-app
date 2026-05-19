# API DESIGN DOCUMENT — Mini App Banking
# Mapping Tables (Mục 14–16)

---

## Mục lục

14. [API → Table Mapping](#14-api--table-mapping)
15. [Proto → REST Mapping](#15-proto--rest-mapping)
16. [API → Actors Mapping](#16-api--actors-mapping)

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
| `GetCertificateDetails` | `GET /cert/details/:cert_serial`      | Gateway → CA (Admin) |

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
