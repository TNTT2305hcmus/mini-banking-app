# Đặc tả: OTP & PKI Registration

## 1. Mô tả

Luồng đăng ký khách hàng mới gồm 2 bước nối tiếp: xác minh email bằng OTP để nhận registration token, sau đó sinh cặp khóa ở browser, tạo CSR và gửi lên CA để nhận X.509 certificate. Kết thúc luồng, khách hàng có wrapped private key + certificate lưu ở browser và có user record trong Bank DB.

## 2. Actor / Thành phần tham gia

- **Khách hàng** — nhập email, nhận OTP, nhập OTP, trigger sinh key pair
- **Customer Web App** — sinh RSA key pair bằng WebCrypto API, tạo CSR, lưu wrapped key vào IndexedDB
- **API Gateway** — rate limit OTP, sinh OTP, gửi email, verify OTP, cấp registration token, forward gRPC sang CA và Bank Service
- **CA Service** — verify CSR proof-of-possession, ký X.509, lưu certificate vào CA DB
- **Bank Service** — tạo user record trong Bank DB sau khi CA cấp certificate thành công
- **Redis** — lưu OTP TTL, rate limit counter
- **CA PostgreSQL DB** — lưu certificate metadata
- **Bank PostgreSQL DB** — lưu user record sau khi enrollment thành công
- **Email/OTP Provider** — gửi email OTP

## 3. Bảng dữ liệu liên quan

| Bảng | DB | Thao tác |
|---|---|---|
| `certificates` | CA DB | INSERT khi CA cấp cert thành công |
| `certificate_audit_log` | CA DB | INSERT action='issued' |
| `users` | Bank DB | INSERT qua Bank Service gRPC sau khi PKI enrollment hoàn thành |
| `otp:{email}` | Redis | SET EX (request), GET+DEL (verify) |
| `rate:otp_request:{ip}` | Redis | INCR + EXPIRE |

## 4. Luồng chính

**Bước 1 — OTP:**

1. Khách hàng nhập email, Customer Web App gửi `POST /otp/request {email}`.
2. API Gateway kiểm tra rate limit (Redis `rate:otp_request:{ip}`); nếu vượt 5 lần/phút → reject.
3. API Gateway sinh OTP 6 chữ số, lưu `SET otp:{email} <otp> EX 300`, gọi Email Provider gửi email.
4. Khách hàng nhập OTP, Customer Web App gửi `POST /otp/verify {email, otp}`.
5. API Gateway `GET otp:{email}` và so sánh; nếu khớp thì `DEL otp:{email}`, tạo `registration_token` (JWT ngắn hạn, dùng 1 lần).
6. API Gateway trả `registration_token` cho Customer Web App.

**Bước 2 — PKI Enrollment:**

7. Customer Web App sinh RSA key pair bằng WebCrypto API (`extractable: false` cho private key).
8. Customer Web App tạo CSR với public key và ký CSR bằng private key (proof-of-possession).
9. Customer Web App lưu wrapped private key vào IndexedDB.
10. Customer Web App gửi `POST /pki/register {csr_pem, registration_token}`.
11. API Gateway verify `registration_token` (chữ ký, chưa dùng, chưa hết hạn).
12. API Gateway gọi CA Service qua gRPC `RegisterUser(csr_pem, user_id)`.
13. CA Service verify CSR proof-of-possession (chữ ký CSR khớp public key trong CSR).
14. CA Service ký X.509 certificate bằng `privKeyRSA_ca`, lưu vào CA DB (`certificates`, `certificate_audit_log` action='issued').
15. CA Service trả `certificate_pem`, `serial_number`, `not_after_unix`.
16. API Gateway gọi Bank Service gRPC `CreateUser(user_id, email, full_name)` để tạo user record trong Bank DB.
17. Nếu `CreateUser` thất bại, API Gateway gọi CA Service revoke/mark certificate với reason `enrollment_failed` và trả `503`.
18. API Gateway trả `certificate_pem` cho Customer Web App.
19. Customer Web App lưu `certificate_pem` vào IndexedDB cùng wrapped private key.

## 5. Kịch bản lỗi

| Tình huống | HTTP | Ghi chú |
|---|---|---|
| OTP request vượt rate limit | 429 | Không gửi email, trả lỗi ngay |
| OTP sai hoặc hết hạn (> 5 phút) | 400 | Không tiết lộ "OTP sai" hay "hết hạn" — chỉ "Invalid OTP" |
| `registration_token` không hợp lệ hoặc đã dùng | 401 | |
| CSR proof-of-possession không hợp lệ | 400 | CA reject |
| User đã có active certificate | 409 | CA trả `ErrActiveCertificateExists` |
| CA Service không khả dụng | 503 | API Gateway trả lỗi, không tạo user record |
| Bank Service không khả dụng sau khi CA đã cấp cert | 503 | Gateway revoke/mark certificate vừa cấp với reason `enrollment_failed` |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- Private key sinh ở browser, không gửi lên server ở bất kỳ bước nào.
- `registration_token` dùng 1 lần: bị vô hiệu hóa ngay sau khi PKI register thành công.
- OTP TTL: 5 phút, xóa khỏi Redis ngay sau khi verify thành công.
- Một user chỉ có tối đa 1 certificate `status = 'active'` tại một thời điểm (partial unique index trong CA DB).
- User record Bank DB chỉ được tạo qua Bank Service sau khi CA đã xác nhận thành công.
- Nếu tạo user thất bại sau khi cert đã được cấp, certificate vừa cấp phải bị revoke/mark failed để tránh active cert không có Bank user tương ứng.

## 7. Tiêu chí chấp nhận

- Sau khi hoàn thành luồng, browser có wrapped private key + `certificate_pem` trong IndexedDB.
- CA DB có 1 record trong `certificates` với `status = 'active'`, `owner_id = user_id`.
- CA DB có 1 record trong `certificate_audit_log` với `action = 'issued'`.
- Bank DB có 1 record trong `users` với `email` và `status = 'active'`, tạo qua `Bank.CreateUser`.
- Gọi lại `POST /otp/verify` với OTP đã dùng → 400 (OTP đã bị xóa khỏi Redis).
- Gửi lại `POST /pki/register` với cùng `registration_token` → 401 (token đã dùng).
