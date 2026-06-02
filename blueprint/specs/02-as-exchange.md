# Đặc tả: AS Exchange (Authentication Service Exchange)

## 1. Mô tả

Khách hàng đã có X.509 certificate thực hiện AS Exchange để xác thực với KDC. Kết quả là nhận được TGT và `K_{c,tgs}` — session key dùng trong bước TGS Exchange tiếp theo. Đây là Phase 2 trong luồng bảo mật của hệ thống.

## 2. Actor / Thành phần tham gia

- **Khách hàng** — kích hoạt đăng nhập / bắt đầu phiên giao dịch
- **Customer Web App** — lấy private key từ IndexedDB, ký AS_REQ, giải mã AS_REP
- **API Gateway** — nhận REST request, forward gRPC sang KDC
- **KDC Service** — verify chữ ký, kiểm tra replay, cấp TGT và `K_{c,tgs}`
- **CA Service** — cung cấp certificate + public key khi KDC lookup
- **Redis** — lưu nonce replay cache

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `certificates` | CA DB | SELECT theo `serial_number` (KDC lookup) |
| `certificate_audit_log` | CA DB | INSERT action='looked_up' |
| `replay:{nonce_hash}` | Redis | SET NX EX (kiểm tra và ghi nonce) |

## 4. Luồng chính

1. Customer Web App sinh `nonce1` (random 32 bytes), `ts1` (timestamp hiện tại), `request_id1` (UUID).
2. Customer Web App tạo AS_REQ payload: `{ID_c, cert_sn, nonce1, ts1, request_id1}`.
3. Customer Web App unwrap private key từ IndexedDB, ký canonical AS_REQ payload → `signature`.
4. Customer Web App gửi `POST /auth/as-req {ID_c, cert_sn, nonce1, ts1, request_id1, signature}`.
5. API Gateway forward → KDC gRPC `RequestTGT(...)`.
6. KDC gọi CA gRPC `GetCertificate(cert_sn)` → nhận `certificate_pem`, `status`, `not_after_unix`.
7. KDC kiểm tra certificate: `status = active` và `not_after > now`.
8. KDC kiểm tra freshness window: `|now - ts1| ≤ 5 phút`; nếu ngoài window → reject.
9. KDC kiểm tra nonce replay: `SET replay:{SHA-256(ID_c+nonce1+ts1+request_id1)} "1" NX EX 300`; nếu key đã tồn tại → reject.
10. KDC verify `signature` trên canonical AS_REQ payload bằng `pubKeyRSA_c` lấy từ certificate.
11. KDC sinh `K_{c,tgs}` (AES-256 random session key).
12. KDC tạo TGT: `E_{K_tgs}[ID_c, cert_sn, K_{c,tgs}, issued_at, expires_at]`.
13. KDC tạo AS_REP: `E_{pubKeyRSA_c}[K_{c,tgs}, TGT, nonce1]` (hybrid: session key wrap bằng RSA-OAEP).
14. KDC trả AS_REP → API Gateway → Customer Web App.
15. Customer Web App giải mã AS_REP bằng private key, xác nhận `nonce1` trong response khớp.
16. Customer Web App lưu TGT và `K_{c,tgs}` trong session memory (RAM, không persist).
17. Customer Web App xóa plaintext private key khỏi RAM.

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| Certificate không tồn tại trong CA DB | 401 Unauthorized | |
| Certificate status = revoked hoặc expired | 401 Unauthorized | |
| Timestamp ngoài freshness window (> 5 phút) | 401 Stale Request | Chống clock-skew attack |
| Nonce đã tồn tại trong Redis (replay) | 401 Replay Detected | |
| Chữ ký AS_REQ không hợp lệ | 401 Invalid Signature | |
| CA Service không khả dụng | 503 | KDC không thể verify certificate |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- KDC không nhận raw public key từ request làm nguồn tin cậy — luôn lấy từ CA Service.
- Private key không rời browser; KDC chỉ nhận signature và verify.
- Nonce replay cache TTL = 5 phút (khớp freshness window để không có khoảng trống).
- TGT TTL đề xuất: 15–30 phút; sau khi hết hạn, khách hàng phải thực hiện lại AS Exchange.
- `K_{c,tgs}` chỉ lưu trong session memory (RAM), không persist, không gửi qua network plaintext.
- Response lỗi không tiết lộ lý do nội bộ (không trả chi tiết "certificate not found" vs "invalid signature").

## 7. Tiêu chí chấp nhận

- Sau khi hoàn thành, Customer Web App có TGT + `K_{c,tgs}` trong session memory.
- Nonce1 đã được ghi vào Redis replay cache (TTL 5 phút).
- Gọi lại `POST /auth/as-req` với cùng `nonce1` → 401 Replay Detected.
- Gọi `POST /auth/as-req` với certificate đã revoke → 401 Unauthorized.
- Gọi `POST /auth/as-req` với signature sai → 401 Unauthorized.
