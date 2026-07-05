# Đặc tả: Bank Transfer (AP Exchange)

## 1. Mô tả

Khách hàng gửi yêu cầu chuyển tiền. Bank Service xác thực `Ticket_v`, chống replay, kiểm tra revocation qua CA, xác minh chữ ký số, kiểm tra authorization rồi thực hiện ACID transaction và ghi vào immutable ledger với hash chaining. Đây là Phase 4 trong luồng bảo mật.

## 2. Actor / Thành phần tham gia

- **Khách hàng** — nhập thông tin chuyển tiền và PIN
- **Customer Web App** — unwrap private key, ký payload, mã hóa request, verify AP_REP
- **API Gateway** — nhận REST, forward gRPC sang Bank Service
- **Bank Service** — xác thực toàn bộ pipeline, thực hiện ACID transfer, ghi ledger
- **CA Service** — cung cấp trạng thái revocation và public key
- **Redis** — nonce replay cache
- **Bank PostgreSQL DB** — ghi transaction, cập nhật balance

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `accounts` | Bank DB | SELECT (kiểm tra balance, limit, status); UPDATE balance (trong ACID tx) |
| `transactions` | Bank DB | INSERT (immutable ledger) |
| `used_nonces` | Bank DB | INSERT (persistent fallback cho nonce) |
| `bank_audit_log` | Bank DB | INSERT cho transfer completed/rejected và lỗi security quan trọng |
| `ledger_state` | Bank DB | SELECT FOR UPDATE khi append hash-chain |
| `certificates` | CA DB | SELECT qua gRPC `VerifyCertificate` (public key, status, validity, issuer/chain metadata) |
| `replay:{nonce_hash}` | Redis | SET NX EX |
| `revocation:{serial}` | Redis | GET (revocation cache) |

## 4. Luồng chính

**Chuẩn bị phía client:**

1. Khách hàng nhập `from_account_id`, `to_account_id`, `amount`, `description` và PIN.
2. Customer Web App sinh `nonce3`, `ts3`, `request_id3`, `idempotency_key` (UUID).
3. Customer Web App tạo canonical payload: `{from_account_id, to_account_id, amount, currency, nonce3, ts3, request_id3, idempotency_key, scope}`.
4. Customer Web App unwrap private key từ IndexedDB bằng PIN.
5. Customer Web App ký canonical payload → `client_signature`; tính `payload_hash = SHA-256(canonical_payload)`.
6. Customer Web App tạo Authenticator: `E_{K_{c,v}}[ID_c, nonce3, ts3, request_id3]`.
7. Customer Web App mã hóa `{canonical_payload, client_signature}` bằng AES-256-GCM với `K_{c,v}` và random IV → `CipherPayload`.
8. Customer Web App gửi `POST /bank/transfer {Ticket_v, Authenticator, CipherPayload}`.

**Xử lý tại Bank Service:**

9. API Gateway forward → Bank Service gRPC `TransferMoney(...)`.
10. Bank Service giải mã `Ticket_v` bằng `K_v` → lấy `ID_c`, `cert_sn` của user certificate do Client CA cấp, `K_{c,v}`, `scope`, `expires_at`.
11. Bank Service kiểm tra `scope = 'transfer:create'` và `expires_at > now`.
12. Bank Service giải mã Authenticator bằng `K_{c,v}` → lấy `nonce3`, `ts3`.
13. Bank Service kiểm tra freshness: `|now - ts3| ≤ 5 phút`.
14. Bank Service kiểm tra nonce3 replay: Redis `SET NX EX` + INSERT `used_nonces` (persistent fallback).
15. Bank Service kiểm tra idempotency: nếu `idempotency_key` đã tồn tại trong `transactions` → trả kết quả cũ, không xử lý tiếp.
16. Bank Service gọi CA gRPC `VerifyCertificate(cert_sn)` → nhận `status`, validity window, `pubKeyRSA_c` và issuer/chain metadata; từ chối nếu chain Root CA → Client CA → user cert không hợp lệ, `status ≠ active` hoặc đã hết hạn.
17. Bank Service giải mã `CipherPayload` bằng `K_{c,v}` → lấy `canonical_payload`, `client_signature`.
18. Bank Service verify `client_signature` trên `canonical_payload` bằng `pubKeyRSA_c`.
19. Bank Service kiểm tra ownership: `from_account.user_id == ID_c`.
20. Bank Service kiểm tra business rules: `account.status = 'active'`, `balance ≥ amount`, `daily_used + amount ≤ daily_transfer_limit`.

**Ghi ledger (chỉ khi toàn bộ kiểm tra pass):**

21. Bank Service mở DB transaction ACID:
    - `UPDATE accounts SET balance = balance - amount WHERE id = from_account_id`
    - `UPDATE accounts SET balance = balance + amount WHERE id = to_account_id`
    - Lock `ledger_state` bằng `SELECT ... FOR UPDATE` để serialize thao tác append ledger
    - `previous_hash` = `last_hash` trong `ledger_state` (hoặc `"genesis"` nếu chưa có)
    - `current_hash = SHA-256(previous_hash || payload_hash || client_signature || tx_id || created_at)`
    - `INSERT INTO transactions ...`
    - `UPDATE ledger_state SET last_hash = current_hash`
    - `INSERT INTO bank_audit_log ... action='transfer_completed'`
22. Bank Service tạo AP_REP: `E_{K_{c,v}}[result="ok", tx_id, nonce3]`.
23. Bank Service trả AP_REP → API Gateway → Customer Web App.
24. Customer Web App giải mã AP_REP, xác nhận `nonce3` khớp.
25. Customer Web App xóa PIN, plaintext private key khỏi RAM.

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| `Ticket_v` hết hạn hoặc scope sai | 401 | Khách hàng cần TGS Exchange lại |
| Timestamp ngoài freshness window | 401 Stale Request | |
| Nonce replay | 401 Replay Detected | |
| `idempotency_key` đã xử lý thành công | 200 (kết quả cũ) | Idempotent — không ghi mới |
| `idempotency_key` đã xử lý nhưng failed | 422 (kết quả cũ) | Trả cùng lỗi ban đầu |
| Certificate revoked hoặc expired | 401 | |
| Chữ ký không hợp lệ | 401 Invalid Signature | |
| Ownership không khớp | 403 Forbidden | |
| Account bị locked hoặc frozen | 403 | |
| Số dư không đủ | 422 Insufficient Funds | |
| Daily limit vượt | 422 Daily Limit Exceeded | |
| CA Service không khả dụng | 503 | Fail closed — không xử lý giao dịch khi không verify được |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- **Fail closed**: nếu bất kỳ bước kiểm tra nào trước bước ghi ledger thất bại → reject, không mở DB transaction.
- **Immutable ledger**: bảng `transactions` chỉ INSERT; không UPDATE/DELETE.
- **Hash chaining**: `current_hash` phụ thuộc `previous_hash` của giao dịch ngay trước. Mọi chỉnh sửa giao dịch cũ sẽ làm vỡ chain.
- **Ledger concurrency**: thao tác append hash-chain phải lock `ledger_state` trong cùng DB transaction để tránh hai transfer cùng dùng một `previous_hash`.
- **Security audit**: lỗi replay, invalid signature, revoked/expired certificate, forbidden ownership và transfer completed/rejected quan trọng được ghi vào `bank_audit_log`.
- **Idempotency**: `idempotency_key` UNIQUE trong `transactions` — client retry với cùng key nhận kết quả cũ, không tạo giao dịch mới.
- Nonce primary cache là Redis; `used_nonces` là fallback khi Redis restart.
- Response lỗi không trả key material, nội dung ticket hay lý do nội bộ chi tiết.

## 7. Tiêu chí chấp nhận

- Chuyển tiền thành công: `from_account.balance` giảm đúng amount, `to_account.balance` tăng đúng amount (atomic).
- `transactions` có 1 record mới với `payload_hash`, `client_signature`, `cert_serial`, `current_hash` hợp lệ.
- `nonce3` đã lưu trong Redis và `used_nonces`.
- Gọi lại với cùng `nonce3` → 401 Replay Detected.
- Gọi lại với cùng `idempotency_key` → 200 kết quả cũ, không có record mới trong `transactions`.
- Chuyển tiền với certificate đã revoke → 401.
- Chuyển tiền khi số dư không đủ → 422, không thay đổi balance.
