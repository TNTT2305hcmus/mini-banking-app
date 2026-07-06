# Đặc tả: Bank Balance & Transaction History

## 1. Mô tả

Khách hàng xem số dư tài khoản hoặc lịch sử giao dịch của tài khoản thuộc sở hữu của mình. Cả hai thao tác đều yêu cầu `Ticket_v` với scope tương ứng và đi qua pipeline xác thực của Bank Service (ticket, replay, revocation, ownership).

## 2. Actor / Thành phần tham gia

- **Khách hàng** — yêu cầu xem số dư hoặc lịch sử
- **Customer Web App** — tạo Authenticator, gửi request
- **API Gateway** — forward gRPC sang Bank Service
- **Bank Service** — xác thực ticket, kiểm tra ownership, trả dữ liệu
- **CA Service** — revocation check
- **Redis** — nonce replay cache
- **Bank PostgreSQL DB** — đọc accounts, transactions

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `accounts` | Bank DB | SELECT (balance, status, owner) |
| `transactions` | Bank DB | SELECT với phân trang (history) |
| `certificates` | CA DB | SELECT qua gRPC `VerifyCertificate` để lấy status, validity và issuer/chain metadata |
| `replay:{nonce_hash}` | Redis | SET NX EX |
| `revocation:{serial}` | Redis | GET (revocation cache) |

## 4. Luồng chính

**Xem số dư — `POST /bank/accounts/{account_id}/balance/query`:**

1. Customer Web App đã có `Ticket_v` với `scope = 'balance:read'` và `K_{c,v}`.
2. Customer Web App sinh `nonce`, `ts`, `request_id`.
3. Customer Web App tạo Authenticator: `E_{K_{c,v}}[ID_c, nonce, ts, request_id]`.
4. Customer Web App gửi `POST /bank/accounts/{account_id}/balance/query {Ticket_v, Authenticator}`.
5. Bank Service giải mã `Ticket_v` → kiểm tra `scope = 'balance:read'` và TTL.
6. Bank Service giải mã Authenticator → kiểm tra freshness và nonce replay.
7. Bank Service gọi CA `VerifyCertificate(cert_sn)` để kiểm tra chain Root CA → Client CA → user cert, status và validity.
8. Bank Service kiểm tra ownership: `account.user_id == ID_c`.
9. Bank Service trả `{account_number, balance, currency, status}`.

**Xem lịch sử — `POST /bank/accounts/{account_id}/transactions/query`:**

1–7. Tương tự với `scope = 'history:read'`.
8. Bank Service kiểm tra ownership.
9. Bank Service query `transactions` WHERE `from_account_id = account_id OR to_account_id = account_id` ORDER BY `created_at DESC` với phân trang (`limit`, `offset` từ query param).
10. Bank Service trả danh sách transaction metadata (không trả `client_signature`).

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| `Ticket_v` hết hạn | 401 | Cần TGS Exchange lại |
| Scope sai (dùng `transfer:create` để xem balance) | 403 Forbidden | Scope được kiểm tra chặt |
| Certificate revoked | 401 | |
| Timestamp ngoài freshness window | 401 Stale Request | |
| Nonce replay | 401 Replay Detected | |
| Ownership không khớp (xem tài khoản người khác) | 403 Forbidden | |
| `account_id` không tồn tại | 404 Not Found | |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- **Scope kiểm tra chặt**: `balance:read` chỉ dùng cho endpoint `/balance/query`; `history:read` chỉ dùng cho endpoint `/transactions/query` — không hoán đổi được.
- **Ownership bắt buộc**: `account.user_id` phải khớp `ID_c` trong `Ticket_v` — không trả dữ liệu tài khoản người khác dù có ticket hợp lệ.
- History response không trả `client_signature` (dữ liệu nhạy cảm, chỉ dùng cho audit nội bộ).
- Phân trang lịch sử: mặc định `limit = 20`, tối đa `limit = 100`.

## 7. Tiêu chí chấp nhận

- Xem balance tài khoản của mình với `scope = 'balance:read'` → 200 với dữ liệu đúng.
- Dùng `Ticket_v` scope `balance:read` để gọi endpoint history → 403 Forbidden.
- Cố xem tài khoản của người khác (account_id không thuộc mình) → 403 Forbidden.
- Nonce đã dùng ở request trước → 401 Replay Detected.
