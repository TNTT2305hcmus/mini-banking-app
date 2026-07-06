# Đặc tả: TGS Exchange (Ticket-Granting Service Exchange)

## 1. Mô tả

Khách hàng dùng TGT (lấy ở AS Exchange) để xin `Ticket_v` và `K_{c,v}` từ KDC cho một scope cụ thể. `Ticket_v` sau đó được dùng trong AP Exchange để thực hiện các thao tác tại Bank Service. Đây là Phase 3 trong luồng bảo mật.

## 2. Actor / Thành phần tham gia

- **Khách hàng** — yêu cầu ticket cho scope cụ thể (balance:read, transfer:create, history:read)
- **Customer Web App** — tạo Authenticator, giải mã TGS_REP
- **API Gateway** — nhận REST request, forward gRPC sang KDC
- **KDC Service** — verify TGT, verify Authenticator, cấp `Ticket_v` và `K_{c,v}`
- **Redis** — nonce replay cache

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `replay:{nonce_hash}` | Redis | SET NX EX (kiểm tra và ghi nonce2) |

TGT được giải mã bằng `K_tgs` — không cần DB lookup thêm. `cert_sn` bên trong TGT là serial của user/client certificate do Client CA cấp ở Phase 1.

## 4. Luồng chính

1. Customer Web App xác định scope cần thiết (ví dụ `transfer:create`).
2. Customer Web App sinh `nonce2`, `ts2`, `request_id2`.
3. Customer Web App tạo Authenticator: `E_{K_{c,tgs}}[ID_c, nonce2, ts2, request_id2]`.
4. Customer Web App gửi `POST /auth/tgs-req {TGT, Authenticator, scope}`.
5. API Gateway forward → KDC gRPC `RequestServiceTicket(...)`.
6. KDC giải mã TGT bằng `K_tgs` → lấy `ID_c`, `cert_sn` của user certificate do Client CA cấp, `K_{c,tgs}`, `expires_at`.
7. KDC kiểm tra TGT còn trong TTL (`expires_at > now`).
8. KDC giải mã Authenticator bằng `K_{c,tgs}` → lấy `ID_c`, `nonce2`, `ts2`.
9. KDC kiểm tra `ID_c` trong Authenticator khớp `ID_c` trong TGT.
10. KDC kiểm tra freshness window: `|now - ts2| ≤ 5 phút`.
11. KDC kiểm tra nonce2 replay: `SET replay:{SHA-256(ID_c+nonce2+ts2+request_id2)} "1" NX EX 300`.
12. KDC kiểm tra `scope` hợp lệ (thuộc tập: `balance:read`, `transfer:create`, `history:read`).
13. KDC sinh `K_{c,v}` (AES-256 random session key).
14. KDC tạo `Ticket_v`: `E_{K_v}[ID_c, cert_sn, K_{c,v}, scope, service_id="bank", issued_at, expires_at]`.
15. KDC tạo TGS_REP: `E_{K_{c,tgs}}[K_{c,v}, Ticket_v, nonce2, scope]`.
16. KDC trả TGS_REP → API Gateway → Customer Web App.
17. Customer Web App giải mã TGS_REP bằng `K_{c,tgs}`, xác nhận `nonce2` và `scope` khớp.
18. Customer Web App lưu `Ticket_v` + `K_{c,v}` trong session memory.

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| TGT không giải mã được (tampered hoặc key sai) | 401 Invalid TGT | |
| TGT đã hết hạn | 401 TGT Expired | Khách hàng cần làm lại AS Exchange |
| `ID_c` trong Authenticator không khớp TGT | 401 Unauthorized | |
| Timestamp ngoài freshness window | 401 Stale Request | |
| Nonce2 đã tồn tại trong Redis (replay) | 401 Replay Detected | |
| Scope không hợp lệ hoặc không được phép | 403 Forbidden Scope | |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- `Ticket_v` có TTL đề xuất 5–10 phút; client có thể dùng lại trong TTL mà không cần xin lại.
- Mỗi `Ticket_v` chứa đúng 1 scope — không có ticket đa scope.
- `K_{c,v}` chỉ lưu trong session memory, không persist.
- `Ticket_v` reusable trong TTL nhưng mỗi AP request phải có nonce/timestamp riêng để chống replay.
- Scope được ghi trong `Ticket_v` và Bank Service verify lại độc lập — không tin scope từ request.

## 7. Tiêu chí chấp nhận

- Sau khi hoàn thành, Customer Web App có `Ticket_v` + `K_{c,v}` đúng scope trong session memory.
- `nonce2` đã được ghi vào Redis replay cache (TTL 5 phút).
- Gọi lại `POST /auth/tgs-req` với TGT hết hạn → 401 TGT Expired.
- Gọi với scope không hợp lệ (`admin:write`) → 403 Forbidden Scope.
- Gọi lại với cùng `nonce2` → 401 Replay Detected.
