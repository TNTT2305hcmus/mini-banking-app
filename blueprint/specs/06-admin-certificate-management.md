# Đặc tả: Admin Certificate Management

## 1. Mô tả

Admin dùng Admin Web App Dashboard để quản lý certificate X.509 trong hệ thống PKI: list/search certificate, xem chi tiết issuer/chain và revoke user/client certificate. Mọi thao tác đi qua API Gateway với Admin Auth và được ghi vào audit log.

Dashboard là **control plane**, không phải Root CA hoặc Intermediate CA. Dashboard không giữ private key và không ký certificate. Root CA, gRPC Transport CA và Client CA chỉ xuất hiện trên Dashboard dưới dạng metadata/trust-chain để Admin hiểu certificate được cấp bởi issuer nào.

## 2. Actor / Thành phần tham gia

- **Admin** — đăng nhập Dashboard, tra cứu và revoke certificate
- **Admin Web App Dashboard** — giao diện quản trị PKI
- **API Gateway** — xác thực Admin Auth, forward gRPC sang CA Service, ghi audit
- **CA Service** — thực hiện lookup, verify chain metadata, revoke client cert, cập nhật CA DB và Redis
- **CA PostgreSQL DB** — lưu ca_issuers, certificates, certificate_audit_log
- **Redis** — invalidate revocation cache khi revoke
- **Gateway env/config** — lưu Admin credential demo trong MVP

## 3. Bảng dữ liệu liên quan

| Bảng / Key | DB | Thao tác |
|---|---|---|
| `ca_issuers` | CA DB | SELECT issuer metadata: Root CA, gRPC Transport CA, Client CA |
| `certificates` | CA DB | SELECT (list/detail/revoke check); UPDATE status, revoked_at, revocation_reason cho `cert_type='client'` |
| `certificate_audit_log` | CA DB | INSERT mỗi thao tác (looked_up, revoked, chain_verified nếu cần) |
| `revocation:{serial}` | Redis | SET EX cho client cert sau khi revoke |

## 4. Luồng chính

**Đăng nhập Admin:**

1. Admin truy cập Dashboard, gửi credentials.
2. API Gateway xác thực bằng Admin credential cấu hình qua env/config demo, trả Admin session token (JWT với role `pki_admin`).

**List / Search Certificate:**

1. Admin nhập filter (email, serial, status, cert_type, issuer, ...) và gửi `GET /admin/certificates?filter=...`.
2. API Gateway verify Admin session token và role `pki:read`.
3. API Gateway forward → CA gRPC `ListCertificates(filter, page, limit)`.
4. CA query `certificates` theo filter, trả metadata: `serial_number`, `cert_type`, `issuer_id`, `issuer_common_name`, `subject_email`, `subject_cn`, `status`, `not_after`.
5. Dashboard hiển thị danh sách với phân trang.

**Certificate Detail:**

1. Admin click xem chi tiết → `GET /admin/certificates/{serial}`.
2. API Gateway verify Admin session.
3. CA query `certificates` theo `serial_number`, trả đầy đủ: serial, cert_type, issuer_id, issuer_common_name, issuer_serial_number, chain_fingerprints, is_ca, key_usage, extended_key_usage, subject_cn, subject_email, fingerprint_sha256, not_before, not_after, status, issued_at, revoked_at, revocation_reason.
4. CA INSERT `certificate_audit_log` với `action='looked_up'`, `performed_by='admin:{email}'`.
5. Dashboard hiển thị chi tiết (không hiển thị private key — CA không lưu private key khách hàng).

**Revoke Certificate:**

1. Admin chọn revoke user/client certificate, nhập reason bắt buộc → `POST /admin/certificates/{serial}/revoke {reason}`.
2. API Gateway verify Admin session và role `pki:revoke`.
3. API Gateway forward → CA gRPC `RevokeCertificate(serial, reason)`.
4. CA kiểm tra certificate tồn tại trong `certificates`.
5. CA kiểm tra `cert_type = 'client'`. Root CA, Intermediate CA và service TLS cert không được revoke qua endpoint này trong MVP; trả 409/422 với code `CERT_TYPE_NOT_REVOKABLE`.
6. CA kiểm tra `status = 'active'` — nếu đã revoked → trả 409 (idempotent no-op).
7. CA UPDATE `certificates SET status='revoked', revoked_at=NOW(), revocation_reason=reason` WHERE `serial_number=serial`.
8. CA `SET revocation:{serial} "revoked" EX 60` — invalidate cache ngay lập tức cho KDC/Bank.
9. CA INSERT `certificate_audit_log` với `action='revoked'`, `serial`, `cert_type='client'`, `issuer_id='client-ca'`, `reason`, `performed_by='admin:{email}'`.
10. CA trả kết quả → API Gateway → Dashboard hiển thị xác nhận.

## 5. Kịch bản lỗi

| Tình huống | Kết quả | Ghi chú |
|---|---|---|
| Admin Auth không hợp lệ hoặc hết hạn | 401 Unauthorized | |
| Thiếu role `pki:revoke` | 403 Forbidden | |
| Revoke certificate không tồn tại | 404 Not Found | |
| Revoke certificate đã revoked | 409 Already Revoked | CA trả idempotent no-op |
| Revoke Root/Intermediate/service TLS cert qua endpoint client revoke | 409/422 Cert Type Not Revocable | Rotation/retire issuer là workflow riêng |
| Revoke thiếu `reason` | 400 Bad Request | Reason bắt buộc |
| CA Service không khả dụng | 503 | |

## 6. Ràng buộc nghiệp vụ và kỹ thuật

- Admin không gọi trực tiếp CA Service — mọi thao tác đi qua API Gateway + Admin Auth.
- Admin credential MVP nằm ở API Gateway env/config; không lưu trong CA DB hoặc Bank DB. Nếu mở rộng production, tạo datastore riêng cho `admin_users`/`admin_sessions`.
- `reason` bắt buộc khi revoke; API Gateway reject trước khi forward nếu thiếu.
- Mọi thao tác admin (detail view, revoke) được ghi vào `certificate_audit_log`.
- Revoke phải invalidate revocation cache ngay lập tức (`SET revocation:{serial} "revoked" EX 60`) để KDC và Bank Service không tiếp tục dùng cached `active`.
- Dashboard không hiển thị `public_key_pem` raw hay `certificate_pem` đầy đủ trong list view — chỉ metadata.
- Dashboard được hiển thị issuer/chain metadata để Admin phân biệt `Root CA`, `gRPC Transport CA`, `Client CA`, `service_tls` và `client` cert.
- Trong MVP, action revoke chỉ áp dụng cho `cert_type='client'`. Service TLS certificate rotate bằng provisioning/script; Intermediate CA retire/rotate bằng quy trình CA riêng.

## 7. Tiêu chí chấp nhận

- Admin có thể search certificate theo email, serial, status, cert_type hoặc issuer_id.
- Admin detail view hiển thị issuer/chain để biết client cert được ký bởi Client CA và chain về Root CA.
- Sau revoke: `certificates.status = 'revoked'`, `revoked_at` có giá trị, `revocation_reason` khớp input.
- Redis key `revocation:{serial}` có giá trị `"revoked"`.
- `certificate_audit_log` có record `action='revoked'` với đầy đủ `cert_type`, `issuer_id`, `performed_by`, `reason`.
- Khách hàng dùng certificate đã revoke ở AS Exchange tiếp theo → 401 Unauthorized.
- Gọi revoke lần 2 với cùng serial → 409 Already Revoked, không INSERT thêm audit log.
- Gọi revoke với Root CA, Intermediate CA hoặc service TLS cert → bị từ chối, không đổi trạng thái cert.
