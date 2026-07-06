# Flow: Bank Transfer (AP Exchange — Phase 4)

Sơ đồ luồng cho:
- `blueprint/specs/04-bank-transfer.md`
- `blueprint/api-design/04-bank-transfer.md`
- `blueprint/design.md` — Flow 3: Secure Banking Transaction + Bank transaction authorization pipeline

Endpoint duy nhất: `POST /v1/bank/transfer` — auth bằng `Ticket_v` (scope `transfer:create`) + Authenticator.

---

## 1. Sequence Diagram (đầy đủ pipeline + nhánh lỗi)

```mermaid
sequenceDiagram
    autonumber
    actor U as Khach hang
    participant W as Customer Web App
    participant GW as API Gateway
    participant B as Bank Service
    participant CA as CA Service
    participant R as Redis
    participant DB as Bank DB

    rect rgb(235, 245, 255)
    note over U,W: CHUAN BI PHIA CLIENT (da co Ticket_v + Kcv tu TGS Exchange)
    U->>W: Nhap from/to account, amount, description, PIN
    W->>W: Sinh nonce3, ts3, request_id3, idempotency_key
    W->>W: Tao canonical_payload (from, to, amount, currency, nonce3, ts3, request_id3, idempotency_key, scope)
    W->>W: Unwrap privKeyRSA_c tu IndexedDB bang PIN
    W->>W: client_signature = Sign(payload, privKeyRSA_c), payload_hash = SHA256(payload)
    W->>W: Authenticator = E_Kcv[ID_c, nonce3, ts3, request_id3]
    W->>W: CipherPayload = AES256GCM_Kcv[canonical_payload + client_signature] (IV 12 bytes random)
    end

    W->>GW: POST /v1/bank/transfer (ticket_v, authenticator, cipher_payload, iv)
    GW->>B: gRPC TransferMoney(...)

    rect rgb(255, 248, 235)
    note over B,DB: PIPELINE XAC THUC - fail-closed, khong mo DB tx neu bat ky buoc nao fail

    B->>B: 1) Decrypt Ticket_v bang K_v (ID_c, cert_sn, Kcv, scope, service_id, expires_at)
    alt Ticket_v khong giai ma duoc hoac het han
        B-->>GW: 401 INVALID_TICKET
    end
    B->>B: 2) Check scope == transfer:create va expires_at > now
    alt scope sai
        B-->>GW: 403 WRONG_SCOPE
    end
    B->>B: 3) Decrypt Authenticator bang Kcv (nonce3, ts3, request_id3)
    B->>B: 4) Freshness abs(now - ts3) <= 5 phut
    alt ngoai window
        B-->>GW: 401 STALE_REQUEST
    end

    B->>R: 5) SET replay:hash NX EX 300
    alt key da ton tai (replay)
        B->>DB: INSERT bank_audit_log (replay_detected)
        B-->>GW: 401 REPLAY_DETECTED
    else OK
        B->>DB: INSERT used_nonces (fallback khi Redis restart)
    end

    B->>DB: 6) Check idempotency_key trong transactions
    alt da xu ly thanh cong
        B-->>GW: 200 (AP_REP cua lan dau, khong ghi moi)
    else da xu ly nhung failed
        B-->>GW: 422 (cung loi ban dau)
    end

    B->>R: 7a) GET revocation:cert_sn (cache TTL 60s)
    B->>CA: 7b) gRPC VerifyCertificate(cert_sn) [neu cache miss]
    CA-->>B: status, not_before, not_after, pubKeyRSA_c, issuer/chain
    alt status khac active
        B->>DB: INSERT bank_audit_log (certificate_rejected)
        B-->>GW: 401 CERT_REVOKED hoac CERT_EXPIRED
    else CA Service down
        B-->>GW: 503 SERVICE_UNAVAILABLE (fail-closed)
    end

    B->>B: 8) Decrypt CipherPayload bang Kcv (canonical_payload, client_signature)
    B->>B: 9) Verify client_signature tren payload bang pubKeyRSA_c
    alt chu ky sai
        B->>DB: INSERT bank_audit_log (invalid_signature)
        B-->>GW: 401 INVALID_SIGNATURE
    end

    B->>DB: 10) SELECT from_account, check ownership from_account.user_id == ID_c
    alt khong khop
        B->>DB: INSERT bank_audit_log (forbidden_ownership)
        B-->>GW: 403 FORBIDDEN
    end
    B->>B: 11) Business rules status=active, balance >= amount, daily_used + amount <= limit
    alt account locked hoac frozen
        B-->>GW: 422 ACCOUNT_NOT_ACTIVE
    else so du khong du
        B->>DB: INSERT bank_audit_log (insufficient_funds)
        B-->>GW: 422 INSUFFICIENT_FUNDS
    else vuot han muc
        B-->>GW: 422 DAILY_LIMIT_EXCEEDED
    end
    end

    rect rgb(235, 255, 240)
    note over B,DB: GHI LEDGER - DB transaction ACID (chi khi moi buoc pass)
    B->>DB: BEGIN
    B->>DB: UPDATE accounts SET balance = balance - amount WHERE id = from
    B->>DB: UPDATE accounts SET balance = balance + amount WHERE id = to
    B->>DB: SELECT last_hash FROM ledger_state WHERE id=main FOR UPDATE
    B->>B: previous_hash = last_hash, current_hash = SHA256(prev + payload_hash + signature + tx_id + created_at)
    B->>DB: INSERT INTO transactions (immutable)
    B->>DB: UPDATE ledger_state SET last_hash = current_hash, last_transaction_id = tx_id
    B->>DB: INSERT bank_audit_log (transfer_completed)
    B->>DB: COMMIT
    end

    rect rgb(245, 240, 255)
    note over B,W: PHAN HOI - Bank chung minh danh tinh (chi Bank co K_v lay duoc Kcv tu Ticket_v)
    B->>B: AP_REP = E_Kcv[result=ok, tx_id, nonce3]
    B-->>GW: 200 (data.ap_rep, meta)
    GW-->>W: 200 (ap_rep)
    W->>W: Decrypt AP_REP bang Kcv, xac nhan nonce3 khop va result=ok
    W->>W: Xoa PIN + plaintext privKeyRSA_c khoi RAM
    W-->>U: Hien thi Chuyen tien thanh cong + tx_id
    end
```

---

## 2. Chú thích Key / Ticket / Payload — chứa gì

### 2.1. Khóa dài hạn của service (không rời server)

| Khóa | Owner | Nội dung / Mục đích |
|---|---|---|
| `K_v` | Bank Service | Khóa đối xứng dài hạn (AES-256) **chỉ Bank Service giữ**. Dùng để **giải mã `Ticket_v`**. Đây là lý do chỉ Bank mới lấy được `K_{c,v}` bên trong ticket → nền tảng để Bank "chứng minh danh tính" qua AP_REP. Lưu env/file secret (production: KMS/HSM, có key version) |
| `privKeyRSA_c` | Khách hàng | Private key client, **wrapped trong IndexedDB**, unwrap bằng PIN trong RAM. Dùng ký `canonical_payload`. Không bao giờ rời browser; xóa khỏi RAM sau khi dùng |
| `pubKeyRSA_c` | Khách hàng | Public key client — Bank **không** nhận từ request mà lấy từ user certificate do Client CA ký qua `VerifyCertificate(cert_sn)` của CA Service. Dùng verify `client_signature` |

### 2.2. `Ticket_v` (do KDC cấp ở TGS Exchange, Bank giải mã)

`Ticket_v = E_{K_v}[ ... ]` — mã hóa bằng `K_v`, client **không đọc được**, chỉ chuyển tiếp. Bên trong chứa:

| Trường | Ý nghĩa |
|---|---|
| `ID_c` | User ID khách hàng (= `users.id`); dùng check ownership |
| `cert_sn` | Serial user/client certificate do Client CA cấp, dùng để chain/status/revocation check qua CA |
| `K_{c,v}` | **Session key** giữa client ↔ Bank (AES-256), KDC sinh; là bí mật chia sẻ cốt lõi |
| `scope` | Phải = `transfer:create`; Bank verify độc lập, không tin scope từ request |
| `service_id` | `"bank"` |
| `issued_at` / `expires_at` | TTL 5–10 phút; reusable trong TTL |

### 2.3. `Authenticator` (client tạo mỗi request)

`Authenticator = E_{K_{c,v}}[ID_c, nonce3, ts3, request_id3]` — mã hóa AES-256-GCM bằng `K_{c,v}`.

| Trường | Vai trò |
|---|---|
| `ID_c` | Phải khớp `ID_c` trong ticket — chứng minh client thực sự nắm `K_{c,v}` |
| `nonce3` | 32 random bytes — chống replay (`SET NX EX` trên Redis) |
| `ts3` | Timestamp — freshness window ±5 phút |
| `request_id3` | UUID — trace + thành phần của replay cache key |

> `Ticket_v` reusable trong TTL nhưng **mỗi request phải có Authenticator mới** (nonce/ts/request_id riêng) → chống replay dù dùng lại ticket.

### 2.4. `canonical_payload` + `client_signature` (trong `CipherPayload`)

`CipherPayload = AES-256-GCM_{K_{c,v}}[canonical_payload ‖ client_signature]` (kèm IV 12 bytes random).

**canonical_payload** (nội dung giao dịch, được ký):

| Trường | Ý nghĩa |
|---|---|
| `from_account_number`, `to_account_number` | Số tài khoản gửi/nhận (chuỗi số, `accounts.account_number`). Bank resolve sang `accounts.id` (UUID) qua `LoadAccountForUpdateByNumber` rồi check ownership `from.user_id == ID_c` |
| `amount` | Số tiền (int64, cents) |
| `currency` | VND |
| `nonce`, `timestamp`, `request_id` | Freshness/replay ở mức payload |
| `idempotency_key` | UUID — chống double-spend khi retry (UNIQUE trong `transactions`) |
| `scope` | `transfer:create` |

**client_signature** = `Sign(SHA-256(canonical_payload_json), privKeyRSA_c)` (RSA-PSS/ECDSA) — bằng chứng **chống chối bỏ (non-repudiation)**, lưu vĩnh viễn trong `transactions`.

### 2.5. `AP_REP` (Bank trả về — mutual auth)

`AP_REP = E_{K_{c,v}}[result, tx_id, nonce3]` — mã hóa bằng `K_{c,v}`.

| Trường | Vai trò |
|---|---|
| `result` | `"ok"` |
| `tx_id` | UUID giao dịch vừa ghi |
| `nonce3` | Echo lại nonce client gửi — client xác nhận khớp để chống MITM/replay response |

Chỉ Bank Service (có `K_v`) mới lấy được `K_{c,v}` để tạo AP_REP hợp lệ → client tin chắc đang nói chuyện với Bank thật.

---

## 3. Bảng dữ liệu & cache liên quan

| Bảng / Key | Store | Thao tác |
|---|---|---|
| `accounts` | Bank DB | SELECT (balance/limit/status); UPDATE balance trong ACID tx |
| `transactions` | Bank DB | INSERT (immutable ledger; UNIQUE `nonce`, `idempotency_key`) |
| `used_nonces` | Bank DB | INSERT (fallback replay khi Redis restart) |
| `bank_audit_log` | Bank DB | INSERT transfer_completed/rejected + lỗi security |
| `ledger_state` | Bank DB | SELECT ... FOR UPDATE (serialize hash-chain) |
| `replay:{nonce_hash}` | Redis | SET NX EX (primary replay check) |
| `revocation:{serial}` | Redis | GET (cache TTL 60s) |
| `certificates` | CA DB (qua gRPC) | `VerifyCertificate`: status + validity + public key + issuer/chain metadata |

---

## 4. Bất biến bảo mật cốt lõi

- **Fail-closed**: bất kỳ check nào fail trước ghi ledger → reject, không mở DB transaction (CA down → 503).
- **Immutable ledger**: `transactions` chỉ INSERT; hash chaining (`current_hash` phụ thuộc `previous_hash`) phát hiện sửa lịch sử.
- **Ledger concurrency**: `SELECT ... FOR UPDATE` trên `ledger_state` chống hai transfer cùng `previous_hash` (rẽ nhánh chain).
- **Replay 2 lớp**: Redis (primary) + `used_nonces` (persistent fallback); UNIQUE `nonce` ở DB là chốt chặn cuối.
- **Idempotency**: UNIQUE `idempotency_key` → retry trả kết quả cũ, không double-spend.
- **Ownership bắt buộc**: `from_account.user_id == ID_c` — ticket hợp lệ vẫn không chuyển hộ tài khoản người khác.
- **Trust model**: public key luôn lấy từ user certificate do Client CA ký và chain về Root CA (không tin raw key từ request) → chống public-key substitution.
- **Non-repudiation**: `client_signature` lưu vĩnh viễn — bằng chứng client đã uỷ quyền giao dịch.
- **Không information leakage**: response lỗi không trả key material, nội dung ticket, hay lý do nội bộ chi tiết.
- **Zero-knowledge / cleanup**: PIN + plaintext private key bị xóa khỏi RAM sau khi hoàn tất.
