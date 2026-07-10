# docs/testcases.md — Bảng Testcase Mini Banking Demo

Tài liệu này liệt kê toàn bộ testcase cho demo end-to-end, được phân thành 5 nhóm
theo yêu cầu của Quang. Cột **Audit field/action** đối chiếu với `docs/audit-testcases.md`.

---

## Cột giải thích

| Cột | Ý nghĩa |
|---|---|
| **ID** | Mã định danh testcase |
| **Nhóm** | Nhóm chức năng |
| **Mô tả** | Nội dung test |
| **Expected** | Kết quả mong đợi |
| **Pass/Fail** | Điền sau khi chạy thực tế |
| **Owner** | Người phụ trách kiểm thử |
| **Note** | Ghi chú thêm, audit action tương ứng |

---

## Nhóm 1: User Flow

| ID | Nhóm | Mô tả | Expected | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| TC-U-01 | User Flow | `POST /v1/otp/request` với email hợp lệ | HTTP 200, `message="OTP sent"`, `expires_in=300`; Redis có key `otp:{email}` TTL 5 phút | | | Tiên quyết: SMTP_USER/SMTP_PASS hợp lệ |
| TC-U-02 | User Flow | `POST /v1/otp/request` vượt 5 lần/phút cùng IP | HTTP 429 `RATE_LIMITED` | | | Rate limit theo IP |
| TC-U-03 | User Flow | `POST /v1/otp/verify` với OTP đúng | HTTP 200, `registration_token` (JWT), `expires_in=600`; Redis key `otp:{email}` bị xóa | | | |
| TC-U-04 | User Flow | `POST /v1/otp/verify` lần 2 với cùng OTP (đã dùng) | HTTP 400 `INVALID_OTP` | | | OTP bị xóa sau verify lần 1 |
| TC-U-05 | User Flow | `POST /v1/otp/verify` với OTP sai | HTTP 400 `INVALID_OTP` | | | |
| TC-U-06 | User Flow | `POST /v1/pki/register` với CSR hợp lệ + registration_token hợp lệ | HTTP 201, trả `certificate_pem`, `serial_number`, `issuer`, `chain`, `not_after`; CA DB có cert `cert_type=client` do Client CA cấp; Bank DB có user record; CA audit `issued` | | | **CA audit action: `issued`** (xem audit-testcases.md #1) |
| TC-U-07 | User Flow | `POST /v1/pki/register` với registration_token đã dùng | HTTP 401 `INVALID_REGISTRATION_TOKEN` | | | Token 1 lần |
| TC-U-08 | User Flow | `POST /v1/pki/register` với user đã có active cert | HTTP 409 `ACTIVE_CERT_EXISTS` | | | DB unique constraint |
| TC-U-09 | User Flow | `POST /v1/auth/as-req` với payload hợp lệ (cert active, sig đúng, nonce mới) | HTTP 200, `as_rep` (Base64); giải mã bằng private key → chứa TGT + `K_{c,tgs}`, nonce khớp; CA audit `verify_certificate` (KDC tra cert active + chain) | | | **CA audit action: `verify_certificate`** (xem audit-testcases.md #4 — KDC là `performed_by`) |
| TC-U-10 | User Flow | `POST /v1/auth/as-req` gọi lại với cùng nonce | HTTP 401 `REPLAY_DETECTED` | | | Redis nonce cache |
| TC-U-11 | User Flow | `POST /v1/auth/tgs-req` với TGT hợp lệ, scope `transfer:create` | HTTP 200, `tgs_rep`; giải mã bằng `K_{c,tgs}` → `Ticket_v` với scope đúng, nonce khớp | | | |
| TC-U-12 | User Flow | `POST /v1/auth/tgs-req` với scope không hợp lệ (`admin:write`) | HTTP 403 `WRONG_SCOPE` | | | |
| TC-U-13 | User Flow | `GET /v1/profile/me` với JWT/ticket hợp lệ | HTTP 200, trả thông tin user (email, name, status) | | | Endpoint profile/me |
| TC-U-14 | User Flow | `POST /v1/bank/accounts/{id}/balance/query` với Ticket_v scope `balance:read` | HTTP 200, trả `account_id`, `account_number`, `balance`, `currency`, `status` | | | Dùng seed account alice |
| TC-U-15 | User Flow | `POST /v1/bank/accounts/{id}/transactions/query` với Ticket_v scope `history:read` | HTTP 200, danh sách transaction có phân trang; không có `client_signature` trong response | | | Dùng seed alice account |
| TC-U-16 | User Flow | `POST /v1/bank/transfer` chuyển tiền hợp lệ | HTTP 200, `ap_rep`; giải mã → `result=ok`, `tx_id`; balance thay đổi atomic; DB có transaction record; Bank audit `transfer_completed` | | | **Bank audit action: `transfer_completed`** (xem audit-testcases.md #5) |
| TC-U-17 | User Flow | `POST /v1/bank/transfer` gọi lại với cùng `idempotency_key` | HTTP 200 kết quả cũ, không có record mới trong DB | | | Idempotency |

---

## Nhóm 2: Admin CA

| ID | Nhóm | Mô tả | Expected | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| TC-CA-01 | Admin CA | Provision pending CA Admin rồi `POST /v1/admin-ca/activate` với activation token + CSR hợp lệ | HTTP 201, trả cert role `ca_admin`, activation token bị xoá | | | Cert-based activation |
| TC-CA-02 | Admin CA | `POST /v1/admin-ca/session` với cert `ca_admin` active và chữ ký challenge hợp lệ | HTTP 200, Bearer session `role=admin-ca`, có `cert_serial` | | | Cert proof-of-possession |
| TC-CA-03 | Admin CA | `GET /v1/admin-ca/certificates` không có filter | HTTP 200, danh sách chứa tất cả cert, có `total/limit/offset` | | | |
| TC-CA-04 | Admin CA | `GET /v1/admin-ca/certificates?status=active` | HTTP 200, chỉ cert `status=active` trong list | | | Filter active |
| TC-CA-05 | Admin CA | `GET /v1/admin-ca/certificates?status=revoked` | HTTP 200, chỉ cert `status=revoked`; hoặc list rỗng nếu chưa revoke | | | Filter revoked |
| TC-CA-06 | Admin CA | `GET /v1/admin-ca/certificates?cert_type=client` | HTTP 200, chỉ `cert_type=client` | | | Filter theo type |
| TC-CA-07 | Admin CA | `GET /v1/admin-ca/certificates/{serial}` của cert đang active | HTTP 200, metadata đầy đủ: `cert_type`, `issuer_id`, `issuer_common_name`, `chain_fingerprints`, `key_usage`, `extended_key_usage`; CA DB có audit `looked_up` | | | **CA audit action: `looked_up`** (xem audit-testcases.md #2) |
| TC-CA-08 | Admin CA | `GET /v1/admin-ca/certificates/{serial}` serial không tồn tại | HTTP 404 `NOT_FOUND` | | | |
| TC-CA-09 | Admin CA | `POST /v1/admin-ca/certificates/{serial}/revoke` cert active với `reason` hợp lệ | HTTP 200, cert `status=revoked`, `revoked_at` có giá trị; CA DB audit `revoked`; Redis `revocation:{serial}="revoked"` | | | **CA audit action: `revoked`** (xem audit-testcases.md #3) |
| TC-CA-10 | Admin CA | `POST /v1/admin-ca/certificates/{serial}/revoke` lần 2 cùng cert đã revoke | HTTP 409 `ALREADY_REVOKED` | | | Idempotency revoke |
| TC-CA-11 | Admin CA | Dùng cert đã revoke để gọi `POST /v1/auth/as-req` | HTTP 401 `CERT_REVOKED` (hoặc `UNAUTHORIZED`); CA audit `verify_certificate` ghi lại (xem audit-testcases.md #4) | | | **CA audit action: `verify_certificate`** |
| TC-CA-12 | Admin CA | `POST /v1/admin-ca/certificates/{serial}/revoke` thiếu `reason` | HTTP 400 `MISSING_REASON` | | | |
| TC-CA-13 | Admin CA | `POST /v1/admin-ca/certificates/{serial}/revoke` cho Root CA serial | HTTP 422 `CERT_TYPE_NOT_REVOKABLE` | | | Chỉ revoke `cert_type=client` |

---

## Nhóm 3: Admin Bank

| ID | Nhóm | Mô tả | Expected | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| TC-AB-01 | Admin Bank | Admin Bank activate + session | HTTP 200, session cookie set | | | Thái phụ trách |
| TC-AB-02 | Admin Bank | `GET /v1/admin/bank/overview` (hoặc endpoint tương đương) | HTTP 200, số liệu tổng hợp: tổng user, tổng tài khoản, tổng giao dịch, tổng balance | | | Thái phụ trách |
| TC-AB-03 | Admin Bank | `GET /v1/admin/bank/users` (list users) | HTTP 200, danh sách users từ seed: alice, bob, charlie | | | Thái phụ trách |
| TC-AB-04 | Admin Bank | `GET /v1/admin/bank/users/{user_id}/accounts` (list accounts của user) | HTTP 200, danh sách accounts của user tương ứng | | | Thái phụ trách |
| TC-AB-05 | Admin Bank | `GET /v1/admin/bank/transactions` hoặc query transactions | HTTP 200, danh sách giao dịch từ seed (≥3 records) | | | Thái phụ trách |
| TC-AB-06 | Admin Bank | `POST /v1/admin/bank/audit/query` không có filter | HTTP 200, danh sách audit log; có `transfer_completed` từ seed | | | Thái phụ trách |
| TC-AB-07 | Admin Bank | `POST /v1/admin/bank/audit/query` với `{"action":"transfer_completed"}` | HTTP 200, chỉ event `transfer_completed` | | | Filter theo action |
| TC-AB-08 | Admin Bank | `POST /v1/admin/bank/audit/query` với `{"limit":101}` | HTTP 400 (limit max 100) | | | Thái phụ trách |
| TC-AB-09 | Admin Bank | Gọi Admin Bank endpoints không có session cookie | HTTP 401 hoặc redirect | | | Thái phụ trách |

---

## Nhóm 4: Audit Log

Đối chiếu với `docs/audit-testcases.md` — bảng này là superset bổ sung cột Pass/Fail/Owner/Note.

| ID | Nhóm | Mô tả | Expected | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| TC-AUD-01 | Audit | Đăng ký user mới (OTP → PKI register thành công) | CA `certificate_audit_log` có record: `action=issued`, `cert_type=client`, `performed_by=system:pki_register` | | | Xem audit-testcases.md #1 |
| TC-AUD-02 | Audit | Admin CA mở detail cert | CA audit: `action=looked_up`, `performed_by=admin:<email>`, `serial_number` khớp cert đã xem | | | Xem audit-testcases.md #2 |
| TC-AUD-03 | Audit | Revoke cert (có reason) | CA audit: `action=revoked`, `reason` không null | | | Xem audit-testcases.md #3 |
| TC-AUD-04 | Audit | AS/TGS/bank flow với cert đã revoke | CA audit: `action=verify_certificate`, `performed_by=kdc-service` hoặc `banking-service`; flow bị reject | | | Xem audit-testcases.md #4 |
| TC-AUD-05 | Audit | Transfer thành công | Bank `bank_audit_log`: `action=transfer_completed`, `transaction_id` có giá trị, `request_id` từ authenticator | | | Xem audit-testcases.md #5 |
| TC-AUD-06 | Audit | Gửi lại request với cùng nonce/request_id | Bank audit: `action=replay_detected`, `reason=redis_replay` hoặc `db_replay` | | | Xem audit-testcases.md #6 |
| TC-AUD-07 | Audit | Query balance/history của account không thuộc user | Bank audit: `action=forbidden_ownership` | | | Xem audit-testcases.md #7 |
| TC-AUD-08 | Audit | Transfer từ account không thuộc user | Bank audit: `action=forbidden_ownership`, `reason=from_account_owner_mismatch` | | | Xem audit-testcases.md #8 |
| TC-AUD-09 | Audit | Payload transfer chữ ký sai | Bank audit: `action=invalid_signature` | | | Xem audit-testcases.md #9 |
| TC-AUD-10 | Audit | Transfer vượt số dư | Bank audit: `action=insufficient_funds` | | | Xem audit-testcases.md #10 |
| TC-AUD-11 | Audit | Bank flow với cert revoked/expired | Bank audit: `action=certificate_rejected` | | | Xem audit-testcases.md #11 |
| TC-AUD-12 | Audit | Balance request với ticket scope sai | Bank audit: `action=transfer_rejected`, `reason=wrong_scope`, `metadata.scope=balance:read` | | | Xem audit-testcases.md #12 |
| TC-AUD-13 | Audit | `GET /v1/admin-ca/audit?action=issued&limit=5` | HTTP 200, danh sách event `issued`, field đầy đủ | | | curl mẫu khớp audit-testcases.md §4 curl #2 |
| TC-AUD-14 | Audit | `GET /v1/admin-ca/audit?action=hack` (action rác) | HTTP 400 `INVALID_REQUEST` | | | Xem audit-testcases.md #13 |
| TC-AUD-15 | Audit | `GET /v1/admin-ca/audit?limit=101` | HTTP 400 (limit max 100) | | | Xem audit-testcases.md #14 |
| TC-AUD-16 | Audit | Audit endpoint không có token | HTTP 401 | | | Xem audit-testcases.md #15 |
| TC-AUD-17 | Audit | Tắt Postgres rồi gọi request chính | Request chính vẫn OK (HTTP 200); service log có `warning: cannot append/insert audit` | | | Audit ghi best-effort — xem audit-testcases.md #16 |
| TC-AUD-18 | Audit | Filter CA audit theo serial: `GET /v1/admin-ca/audit?serial=<serial>` | HTTP 200; response `data.items[]` chỉ chứa event có đúng `serial_number` = serial đã truyền | | Thuận | curl mẫu: audit-testcases.md §4 curl #3 |
| TC-AUD-19 | Audit | Filter CA audit theo performed_by: `GET /v1/admin-ca/audit?performed_by=admin-ca` | HTTP 200; tất cả item trả về có `performed_by` khớp giá trị filter; dùng `admin:<email>` khi filter thao tác của Admin | | Thuận | audit-testcases.md §4 curl #3: `performed_by=admin-ca` |
| TC-AUD-20 | Audit | Filter CA audit theo khoảng thời gian: `GET /v1/admin-ca/audit?from=<ISO>&to=<ISO>` | HTTP 200; chỉ trả event trong `[from, to)` (nửa mở); `from`/`to` là **ISO 8601 UTC string** (khác Bank dùng Unix) | | Thuận | **CA dùng ISO string**, không phải Unix epoch — xem audit-testcases.md §4 curl #4 |
| TC-AUD-21 | Audit | Filter Bank audit theo request_id: `POST /v1/admin/bank/audit/query` body `{"request_id":"<id>"}` | HTTP 200; chỉ trả event có `request_id` khớp (AP flow request_id trong authenticator, không phải X-Request-ID gateway) | | Thuận | audit-testcases.md §4 curl #6; phân biệt 2 loại request_id xem audit-testcases.md §1.3 |
| TC-AUD-22 | Audit | Filter Bank audit theo cert_serial: body `{"cert_serial":"<serial>"}` | HTTP 200; chỉ trả event có `cert_serial` khớp; **field tên là `cert_serial`**, không phải `serial` hay `serial_number` | | Thuận | Tên field Bank audit là `cert_serial` (cột DB `bank_audit_log.cert_serial`) — không nhầm với CA audit dùng `serial` |
| TC-AUD-23 | Audit | Filter Bank audit theo khoảng thời gian: body `{"from_unix":<epoch>,"to_unix":<epoch>}` | HTTP 200; chỉ trả event trong khoảng `[from_unix, to_unix)`; `from_unix`/`to_unix` là **Unix timestamp (giây, int64)** | | Thuận | **Bank dùng Unix epoch**, không phải ISO string như CA — copy nhầm format giữa 2 service sẽ lỗi; xem audit-testcases.md §2 |
| TC-AUD-24 | Audit | Transfer với cert đã revoke → `verify_certificate` do **banking-service** gọi (không chỉ kdc-service) | CA audit: `action=verify_certificate`, `performed_by=banking-service`; Bank audit: `action=certificate_rejected` | | Thuận | Phân biệt với TC-AUD-04 (KDC gọi). Banking-service cũng gọi CA `VerifyCertificate` trước khi xử lý bank flow — xem audit-testcases.md #4 |
| *(Ghi chú)* | Audit | **Các thao tác chủ đích KHÔNG ghi audit** (theo audit-testcases.md §5) — không báo FAIL khi không thấy audit event | `ListCertificates` thành công; `balance`/`history`/`profile` thành công; `CreateUser` Bank. Đây là thiết kế cố ý, không phải bug. | N/A | | audit-testcases.md §5 |

---

## Nhóm 5: Negative Tests

| ID | Nhóm | Mô tả | Expected | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| TC-NEG-01 | Negative | Thiếu header `X-Request-ID` cho Admin CA endpoint | HTTP 400 `BAD_REQUEST` hoặc Gateway auto-generate (tùy implementation) | | | Xem base-api.md §1.3 |
| TC-NEG-02 | Negative | `Authorization: Bearer <token>` sai role (role: customer dùng Admin CA endpoint) | HTTP 403 `FORBIDDEN` | | | |
| TC-NEG-03 | Negative | Token admin hết hạn | HTTP 401 `UNAUTHORIZED` | | | |
| TC-NEG-04 | Negative | Cert serial không tồn tại khi gọi `GET /v1/admin-ca/certificates/{serial}` | HTTP 404 `NOT_FOUND` | | | |
| TC-NEG-05 | Negative | Gọi `POST /v1/auth/as-req` với timestamp cách server >5 phút | HTTP 401 `STALE_REQUEST` | | | Freshness window ±5 phút |
| TC-NEG-06 | Negative | Gọi `POST /v1/bank/transfer` với số tiền âm hoặc 0 | HTTP 400 `BAD_REQUEST` | | | amount CHECK constraint |
| TC-NEG-07 | Negative | Gọi bank endpoint khi DB Postgres down | HTTP 503 `SERVICE_UNAVAILABLE`; không crash, không lộ stack trace | | | Resilience — xem audit-testcases.md #16 |
| TC-NEG-08 | Negative | Gọi bất kỳ endpoint khi Redis down | HTTP 503 `SERVICE_UNAVAILABLE` hoặc degraded mode tuỳ thiết kế; không crash | | | |
| TC-NEG-09 | Negative | `POST /v1/pki/register` với CSR proof-of-possession sai | HTTP 400 `INVALID_CSR` | | | |
| TC-NEG-10 | Negative | `POST /v1/bank/transfer` dùng `Ticket_v` scope `balance:read` | HTTP 403 `WRONG_SCOPE` | | | Scope enforcement tại Bank Service |
| TC-NEG-11 | Negative | Gọi `GET /v1/admin-ca/certificates` mà không có Authorization header | HTTP 401 `UNAUTHORIZED` | | | |
| TC-NEG-12 | Negative | Gọi `POST /v1/bank/accounts/{id}/balance/query` với `account_id` không tồn tại | HTTP 404 `NOT_FOUND` | | | |
| TC-NEG-13 | Negative | Gọi `POST /v1/bank/accounts/{id}/balance/query` với account thuộc người khác | HTTP 403 `FORBIDDEN`; Bank audit `forbidden_ownership` | | | |
| TC-NEG-14 | Negative | `POST /v1/bank/transfer` khi số dư không đủ | HTTP 422 `INSUFFICIENT_FUNDS`; balance không thay đổi; Bank audit `insufficient_funds` | | | |
| TC-NEG-15 | Negative | Body request không phải JSON hợp lệ | HTTP 400 `BAD_REQUEST`; không lộ nội dung stack trace | | | |

---

## Ghi chú chung

1. **Seed data**: Các testcase dùng seed users (alice/bob/charlie) từ `db/bank/seed_demo.sql`.
   Account IDs cố định — xem file seed để tra UUID.

2. **Audit cross-check**: Sau khi chạy mỗi testcase nhóm User/Admin CA/Bank, kiểm tra
   audit log tương ứng theo hướng dẫn trong `docs/audit-testcases.md` mục 4 (curl mẫu).

3. **Idempotency**: Testcase TC-U-17, TC-CA-10, TC-NEG-06 kiểm tra idempotency —
   chạy 2 lần liên tiếp, lần 2 không được tạo record mới hoặc thay đổi state.

4. **Ownership của Pass/Fail**:
   - User Flow: người chạy demo cuối
   - Admin CA: Quang
   - Admin Bank: Thái
   - Audit: Thuận
   - Negative: Quang

5. **Cột Pass/Fail** điền sau khi chạy thực tế: ✓ PASS / ✗ FAIL / ⊘ SKIP
