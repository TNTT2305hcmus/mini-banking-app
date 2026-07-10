# Non-Functional And Security Demo Script

Mục tiêu: chứng minh phần Applied Cryptography và bảo mật của dự án, không chỉ CRUD banking.

Thời lượng gợi ý: 8-12 phút.

## 0. Mở đầu

Nói ngắn:

```text
Phần này tập trung vào thiết kế bảo mật: PKI hierarchy, private key trong browser, cert-based admin, Kerberos-like AS/TGS/AP, replay/idempotency, audit timeline và hash-chain.
```

## 1. PKI hierarchy

Hiển thị hoặc mô tả từ guide/cert files:

```text
ca-service/certs/root-ca/ca.crt
ca-service/certs/intermediate/client-ca.crt
ca-service/certs/intermediate/grpc-ca.crt
```

Nói khi quay:

```text
Root CA chỉ ký Intermediate CA. Client CA ký identity certificate cho customer, bank_admin và ca_admin. gRPC Transport CA tách riêng để ký TLS certificate cho service nội bộ.
```

Điểm cần nhấn:

- Root CA không ký trực tiếp identity cert.
- Client CA ký cert đăng nhập.
- gRPC Transport CA ký cert TLS service.
- Cert CA Admin là identity cert role `ca_admin`, không phải CA con.

Expected evidence:

- File cert tồn tại.
- UI/Admin CA detail cho thấy role/issuer/status nếu có.

## 2. Private key protection

Thao tác:

1. Mở register hoặc admin activate flow.
2. Chỉ ra bước đặt PIN.
3. Chỉ ra browser tạo keypair/CSR.

Nói khi quay:

```text
Private key được sinh trong browser bằng WebCrypto, được wrap bằng PIN rồi lưu local. Server chỉ nhận CSR, không nhận private key plaintext.
```

Điểm cần nhấn:

- CSR gửi lên server.
- Private key plaintext không rời browser.
- IndexedDB lưu credential theo scope: `customer`, `bank_admin`, `ca_admin`.

## 3. Cert-based admin access

Thao tác:

1. Mở `/admin-ca`.
2. Login bằng PIN/cert.
3. Mở `/admin-bank`.
4. Login bằng Bank Admin cert/PIN.

Nói khi quay:

```text
Admin CA và Admin Bank không dùng password tĩnh làm đường chính. Quyền admin đến từ certificate role và proof-of-possession của private key.
```

Expected:

- Admin CA session chỉ có sau cert proof.
- Admin Bank session/cookie chỉ có sau cert flow.
- Customer cert không dùng được cho admin endpoint.

Optional negative:

- Gọi SOC/Admin endpoint không token hoặc sai role, thấy `401/403`.

## 4. Kerberos-like AS/TGS/AP

Thao tác:

1. Login customer.
2. Mở Network tab nếu phù hợp.
3. Chỉ các call:
   - `/v1/auth/as-req`
   - `/v1/auth/tgs-req`
   - bank/profile/balance/transfer request.

Nói khi quay:

```text
Flow chia thành AS cấp TGT, TGS cấp service ticket, rồi AP request tới Banking Service. Service ticket có scope riêng cho profile, balance, history hoặc transfer.
```

Điểm cần nhấn:

- AS/TGS tách khỏi Bank.
- Scope enforcement.
- AP `request_id` dùng cho replay/idempotency.
- `operation_id`/`X-Request-ID` dùng để trace xuyên service, không thay thế AP `request_id`.

## 5. Replay and idempotency

Chọn một case dễ quay:

- Replay request nếu có công cụ/curl đã chuẩn bị.
- Hoặc idempotency transfer: gửi lại cùng idempotency key nếu flow hỗ trợ.

Nói khi quay:

```text
Replay/idempotency bảo đảm request lặp lại không tạo giao dịch mới hoặc bị reject có kiểm soát. Event này được ghi vào Bank audit như replay_detected hoặc trả lại kết quả cũ tùy case.
```

Expected:

- Không có transaction duplicate.
- Có audit/security evidence nếu case tạo event.

Nếu không quay trực tiếp:

```text
Case replay đầy đủ cần giữ nguyên AP request đã ký, nên được kiểm trong security testcase thay vì thao tác UI nhanh.
```

## 6. Registration consistency and rollback

Thao tác dễ quay:

1. Dùng email đã đăng ký.
2. Thử register lại.

Nói khi quay:

```text
Gateway pre-check email trước khi yêu cầu CA issue cert. Email trùng trả 409 EMAIL_ALREADY_REGISTERED, tránh tạo active orphan certificate.
```

Expected:

- `409 EMAIL_ALREADY_REGISTERED`.
- Không có cert active mới cho email đó.

Nếu muốn nói thêm:

```text
Nếu Bank fail sau khi CA issue, Gateway revoke best-effort với reason registration_rollback.
```

## 7. Revoked cert rejected

Thao tác:

1. Dùng Admin CA revoke cert phụ.
2. Thử dùng cert đó cho flow login/admin/bank nếu có môi trường phụ.

Nói khi quay:

```text
Cert revoked không được tiếp tục dùng cho privileged action. CA/KDC/Bank verify certificate status trước khi cấp vé hoặc xử lý request.
```

Expected:

- Request bị reject.
- Audit có `verify_certificate` hoặc `certificate_rejected`.

Lưu ý:

- Không dùng cert chính đang cần cho demo.

## 8. SOC timeline by operation_id

Thao tác:

1. Lấy `operation_id` từ một flow register/login/transfer.
2. Mở `/admin-soc`.
3. Search timeline theo `operation_id`.

Nói khi quay:

```text
operation_id là X-Request-ID được frontend tái sử dụng trong một flow lớn. SOC dùng nó để nối sự kiện CA, KDC và Bank theo cùng một phiên nghiệp vụ.
```

Expected:

- Timeline có CA/KDC events.
- Bank events xuất hiện nếu có bank admin session cookie.

Nhấn mạnh:

- `operation_id` khác AP `request_id`.
- AP `request_id` phục vụ replay/idempotency trong Bank protocol.

## 9. Hash-chain verify

Thao tác:

1. Trong SOC, bấm verify.
2. Hiển thị kết quả CA/KDC/Bank.

Nói khi quay:

```text
Mỗi audit table có hash-chain. Verify replay chuỗi hash để phát hiện sửa/xóa/đảo event ở giữa chuỗi.
```

Expected:

- DB sạch: source checked trả ok.
- Bank có thể `checked:false` nếu không có cookie `bank_admin_session`.

Optional tamper demo:

- Chỉ làm trên DB disposable.
- Sửa một dòng audit giữa chuỗi.
- Bấm verify để thấy `ok:false` và `broken_seq`.

## 10. Summary and export

Thao tác:

1. Mở SOC summary.
2. Export CSV/JSON.

Nói khi quay:

```text
SOC summary gom event theo severity/category/outcome và export CSV/JSON để làm bằng chứng báo cáo.
```

Expected:

- Summary có số liệu.
- File export tải được.

## 11. Rate limit and demo mode

Nói ngắn:

```text
Gateway có rate-limit cho OTP, AS/TGS và Bank API. Khi rehearsal hoặc quay video nhiều lần, có thể bật RATE_LIMIT_DISABLED=1 để tránh tự khóa demo. Đây là demo mode, không phải production mode.
```

Nếu muốn demo thật:

- Set `RATE_LIMIT_DISABLED=0`.
- Gọi OTP/AS nhiều lần để thấy 429.
- Chỉ làm sau khi đã quay flow chính.

## 12. Limitations

Nói trung thực:

```text
Hash-chain hiện chưa có external anchor tự động, nên không chống tail truncation tuyệt đối. Timestamp/metadata không nằm trong field hash cốt lõi vì vấn đề round-trip byte stability. Audit insert là best-effort để không làm fail nghiệp vụ chính.
```

Các limitation cần nhắc:

- Chưa có external anchor tự động.
- Không phát hiện xóa dòng cuối nếu không có checkpoint ngoài DB.
- Timestamp/metadata không được hash.
- Audit insert best-effort.
- Admin CA cert-login verify trực tiếp ở Gateway, chưa đi AS/TGS/AP như Bank Admin.

## 13. Kết thúc security demo

Nói ngắn:

```text
Phần security demo đã thể hiện PKI, cert-based access, AS/TGS/AP, replay/idempotency, audit timeline và hash-chain. Các giới hạn đã được ghi rõ để không overclaim mức bảo mật của demo.
```

Sau khi quay:

- Cập nhật `runtime-results.md`.
- Lưu SOC export CSV/JSON.
- Ghi lại `operation_id`.
- Ghi lại cert serial đã revoke hoặc dùng cho negative case.
