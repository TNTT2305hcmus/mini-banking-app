# Đặc tả: Admin Certificate Management

## 1. Mô tả

Admin dùng Admin Web App Dashboard để quản lý certificate X.509 trong hệ thống PKI: list/search certificate, xem chi tiết và revoke. Mọi thao tác đi qua API Gateway với Admin Auth và được ghi vào audit log.

## 2. Actor / Thành phần tham gia

- **Admin** — đăng nhập Dashboard, tra cứu và revoke certificate
- **Admin Web App Dashboard** — giao diện quản trị PKI
- **API Gateway** — xác thực Admin Auth, forward gRPC sang CA Service, ghi audit
- **CA Service** — thực hiện lookup, revoke, cập nhật CA DB và Redis
- **CA PostgreSQL DB** — lưu certificates, certificate_audit_log
- **Redis** — invalidate revocation cache khi revoke

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `certificates` | CA DB | SELECT (list/detail/revoke check); UPDATE status, revoked_at, revocation_reason |
| `certificate_audit_log` | CA DB | INSERT mỗi thao tác (looked_up, revoked) |
| `revocation:{serial}` | Redis | SET EX (invalidate/update sau khi revoke) |

## 4. Luồng chính

**Đăng nhập Admin:**

1. Admin truy cập Dashboard, gửi credentials.
2. API Gateway xác thực, trả Admin session token (JWT với role `pki_admin`).

**List / Search Certificate:**

1. Admin nhập filter (email, serial, status, ...) và gửi `GET /admin/certificates?filter=...`.
2. API Gateway verify Admin session token và role `pki:read`.
3. API Gateway forward → CA gRPC `ListCertificates(filter, page, limit)`.
4. CA query `certificates` theo filter, trả metadata: `serial_number`, `subject_email`, `subject_cn`, `status`, `not_after`.
5. Dashboard hiển thị danh sách với phân trang.

**Certificate Detail:**

1. Admin click xem chi tiết → `GET /admin/certificates/{serial}`.
2. API Gateway verify Admin session.
3. CA query `certificates` theo `serial_number`, trả đầy đủ: serial, subject_cn, subject_email, fingerprint_sha256, not_before, not_after, status, issued_at, revoked_at, revocation_reason.
4. CA INSERT `certificate_audit_log` với `action='looked_up'`, `performed_by='admin:{email}'`.
5. Dashboard hiển thị chi tiết (không hiển thị private key — CA không lưu private key khách hàng).

**Revoke Certificate:**

1. Admin chọn revoke, nhập reason bắt buộc → `POST /admin/certificates/{serial}/revoke {reason}`.
2. API Gateway verify Admin session và role `pki:revoke`.
3. API Gateway forward → CA gRPC `RevokeCertificate(serial, reason)`.
4. CA kiểm tra certificate tồn tại trong `certificates`.
5. CA kiểm tra `status = 'active'` — nếu đã revoked → trả 409 (idempotent no-op).
6. CA UPDATE `certificates SET status='revoked', revoked_at=NOW(), revocation_reason=reason` WHERE `serial_number=serial`.
7. CA `SET revocation:{serial} "revoked" EX 60` — invalidate cache ngay lập tức.
8. CA INSERT `certificate_audit_log` với `action='revoked'`, `serial`, `reason`, `performed_by='admin:{email}'`.
9. CA trả kết quả → API Gateway → Dashboard hiển thị xác nhận.

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| Admin Auth không hợp lệ hoặc hết hạn | 401 Unauthorized | |
| Thiếu role `pki:revoke` | 403 Forbidden | |
| Revoke certificate không tồn tại | 404 Not Found | |
| Revoke certificate đã revoked | 409 Already Revoked | CA trả idempotent no-op |
| Revoke thiếu `reason` | 400 Bad Request | Reason bắt buộc |
| CA Service không khả dụng | 503 | |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- Admin không gọi trực tiếp CA Service — mọi thao tác đi qua API Gateway + Admin Auth.
- `reason` bắt buộc khi revoke; API Gateway reject trước khi forward nếu thiếu.
- Mọi thao tác admin (detail view, revoke) được ghi vào `certificate_audit_log`.
- Revoke phải invalidate revocation cache ngay lập tức (`SET revocation:{serial} "revoked" EX 60`) để KDC và Bank Service không tiếp tục dùng cached `active`.
- Dashboard không hiển thị `public_key_pem` raw hay `certificate_pem` đầy đủ trong list view — chỉ metadata.

## 7. Tiêu chí chấp nhận

- Admin có thể search certificate theo email, serial hoặc status.
- Sau revoke: `certificates.status = 'revoked'`, `revoked_at` có giá trị, `revocation_reason` khớp input.
- Redis key `revocation:{serial}` có giá trị `"revoked"`.
- `certificate_audit_log` có record `action='revoked'` với đầy đủ thông tin.
- Khách hàng dùng certificate đã revoke ở AS Exchange tiếp theo → 401 Unauthorized.
- Gọi revoke lần 2 với cùng serial → 409 Already Revoked, không INSERT thêm audit log.
