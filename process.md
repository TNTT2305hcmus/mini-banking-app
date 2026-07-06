# Đánh giá tiến độ và Phân công

## Kiểm tra chống lệch pha với code hiện tại

File này đã được đối chiếu lại với code hiện tại ở các phần chính: `frontend/src/pages/AdminCA.tsx`, `frontend/src/pages/AdminBank.tsx`, `frontend/src/app/routes.tsx`, API Gateway routes/services, CA proto/generated client, Bank proto/generated client, CA/Bank DB migrations và các service ghi audit.

Kết luận: các đầu việc bên dưới là đúng theo thiết kế và tiến độ hiện tại, nhưng cần lưu ý vài ranh giới để tránh false positive:

- Admin CA UI đã nối API thật cho list/detail/revoke certificate theo hướng layered CA; tab Audit Log vẫn chờ endpoint đọc audit.
- API Gateway đã mount route Admin CA dưới `/v1/admin-ca/*` và có admin auth demo cho role `admin-ca`; các route admin chung/Admin Bank vẫn cần thống nhất sau.
- CA Service đã có gRPC `ListCertificates`, `GetCertificateDetail`, `RevokeCertificate`, nhưng chưa có gRPC/API đọc `certificate_audit_log`.
- CA proto hiện dùng field camelCase ở TypeScript generated client như `serialNumber`, `ownerId`, `subjectEmail`, `fingerprintSha256`, `notBeforeUnix`, `notAfterUnix`; nếu REST trả snake_case thì Gateway phải map rõ ràng.
- Bank Service hiện chỉ có gRPC user flow `TransferMoney`, `GetBalance`, `GetHistory`; chưa có admin gRPC methods. Nếu chọn hướng admin gRPC thì phải sửa proto và regenerate client/server.
- Request id hiện có 2 lớp: HTTP `X-Request-ID` header do frontend API client gắn cho Gateway, và `request_id` trong body/authenticator của Bank AP flow (`transfer`, `profile`, `history`, `balance`). Bank audit lấy request id từ authenticator/body flow, không lấy trực tiếp từ HTTP header.
- Bank audit action enum trong DB hiện chỉ cho phép: `transfer_completed`, `transfer_rejected`, `replay_detected`, `invalid_signature`, `certificate_rejected`, `forbidden_ownership`, `insufficient_funds`.
- Các endpoint admin đề xuất trong file này là contract cần implement, chưa phải endpoint đã tồn tại.

## 0. Đánh giá mức hoàn thành hiện tại

| Hạng mục | Mức hoàn thành | Ghi chú |
|---|---:|---|
| CA Service | 85% | Đã có cấp/verify/list/detail/revoke cert, store JSON/Postgres, TLS gRPC. Audit CA đã ghi cho issue/verify/detail/revoke. |
| KDC Service | 80% | Đã có AS/TGS, replay Redis, key provisioning. Cần hardening env/Docker và test end-to-end. |
| Banking Service | 75% | Đã có profile/balance/history/transfer, Postgres + Redis, audit write vào `bank_audit_log`. Chưa có Dockerfile riêng và chưa có API admin đọc dữ liệu vận hành. |
| API Gateway | 70% | Đã nối OTP, PKI, KDC, Bank cho luồng user. Chưa mount `/v1/admin/*`, chưa có admin auth/role, chưa expose API CA admin hoặc Bank admin ra REST. |
| User Frontend | 70% | Đã có UI và service client crypto cho đăng ký/login/bank flow; cần chạy full backend để xác nhận end-to-end. |
| Admin CA UI | 85% | Đã có login, bảng certificates, filter status/type/issuer, search, detail drawer, revoke modal và gọi API thật. Audit tab vẫn pending. |
| Admin Bank UI | 20% | Có route `/admin-bank` và layout Overview/Users/Ledger/Security Audit, nhưng hiện chỉ là placeholder, chưa fetch API. |
| Admin CA API | 85% | Gateway đã expose `/v1/admin-ca/auth`, list/detail/revoke certificates, có auth demo và error mapping layered CA. Chưa có endpoint đọc audit log. |
| Admin Bank API | 15% | Bank DB có users/accounts/transactions/audit; service chưa có gRPC/REST admin list users, list ledger, list audit, overview metrics. |
| Audit Log | 60% | CA và Bank đều đã ghi audit nội bộ. Thiếu API đọc audit, filter/pagination, admin viewer, request-id/performed-by đầy đủ từ Gateway, và chính sách giữ log khi deploy. |
| Docker/DevOps | 45% | `docker-compose.yml` hiện mới cover CA + Gateway + Redis; chưa compose full KDC/Bank/Postgres. |
| Tài liệu chạy | 80% sau file này | Có hướng dẫn local chi tiết; compose full vẫn là việc còn lại. |

### 0.1. Admin UI

| Module admin | Hiện có | Còn thiếu để demo được |
|---|---|---|
| Admin CA UI | Route `/admin-ca`, login, table certificates, filter/search/pagination, detail drawer, revoke modal, loading/error state, gọi API thật. | Nối Audit Log khi có endpoint đọc audit; chạy lại UI build và demo test. |
| Admin CA API | Gateway route hiện tại `/v1/admin-ca/*` đã wrap CA gRPC list/detail/revoke và có admin auth demo. | Chốt prefix route/role với nhóm; thêm hoặc nối endpoint audit read khi Thuận hoàn thành. |
| Admin Bank UI | Route `/admin-bank`, sidebar, tab Overview/Users/Ledger/Security Audit, empty state. | Dashboard metrics, bảng users/accounts, bảng transactions/ledger hash, audit table, filter/search, gọi API thật. |
| Admin Bank API | DB đã có `users`, `accounts`, `transactions`, `bank_audit_log`; Bank Service ghi audit. | gRPC/REST admin queries để đọc users/accounts/transactions/audit/metrics; phân trang; filter theo user/action/time. |

### 0.2. Audit Log

Đã có:

- CA ghi audit cho issue, verify/revocation check, detail lookup, revoke.
- Bank ghi audit cho transfer completed/rejected, replay, invalid signature, certificate rejected, forbidden ownership, insufficient funds.
- DB schema có `certificate_audit_log` và `bank_audit_log`.

Còn thiếu để kiểm thử/deploy ổn:

- API đọc audit log cho admin, có pagination/filter theo action, serial/user, time range.
- Gateway truyền `X-Request-ID`, admin identity (`performed_by`) và IP/user-agent xuống các admin API nhất quán. Riêng CA hiện chưa có `request_id` trong admin proto, nên muốn ghi request id vào CA audit phải bổ sung proto field hoặc đọc gRPC metadata. Riêng Bank user flow hiện dùng `request_id` trong body/authenticator để ghi audit.
- Admin UI hiển thị audit log thật, không chỉ placeholder.
- Test case audit: issue cert, lookup detail, revoke cert, transfer success, replay, invalid signature, ownership denied.
- Quy định log retention/export khi deploy: ít nhất backup DB hoặc export CSV/JSON cho demo.
- Không để audit fail làm request chính crash; hiện Bank đã có hướng này, cần xác nhận CA/Gateway route mới cũng theo nguyên tắc đó.

### 0.3. Nhiệm vụ

Ưu tiên P0 là demo được end-to-end; P1 là admin đủ dùng; P2 là polish.

| Ưu tiên | Việc cần làm | Output mong muốn |
|---|---|---|
| P0 | Chạy full local stack từ guide, fix lỗi env/cert/DB phát sinh. | 5 terminal chạy ổn: CA, KDC, Bank, Gateway, Frontend. |
| P0 | End-to-end user flow: OTP -> PKI register -> AS -> TGS -> profile/balance/history/transfer. | Checklist test tay + bug list rõ ràng. |
| P0 | Gateway admin auth tối thiểu. | `POST /v1/admin/auth`, JWT role `ca_admin`/`bank_admin` hoặc demo admin. |
| P0 | Admin CA REST API. | Đã có list/detail/revoke qua Gateway; cần chạy lại regression và chốt contract route/role cho demo. |
| P0 | Admin Bank REST API tối thiểu. | Overview metrics, users/accounts list, transactions list, audit list. |
| P1 | Admin CA UI nối API thật. | Đã có table + filters + detail + revoke modal; còn audit tab phụ thuộc endpoint audit. |
| P1 | Admin Bank UI nối API thật. | Overview cards + users table + ledger table + audit table. |
| P1 | Audit log test suite. | Test hoặc script chứng minh audit được ghi và đọc lại. |
| P1 | Docker/deploy package. | Compose full hoặc deploy docs rõ: DB/Redis/env/certs/ports. |
| P1 | Smoke test script. | Một file checklist/lệnh curl để xác nhận deploy sống. |
| P2 | Security cleanup. | Không commit secret, đổi dev secrets, CORS/env production, README demo account. |

### 0.4. Phân công

#### Nguyên tắc

- Mỗi thành viên tự tạo checklist ngắn đầu ngày, cuối ngày báo: đã xong, đang lỗi, cần người khác unblock.
- Mọi API mới phải có contract rõ: method, path, query/body, response success, response error.
- Mọi UI mới phải có đủ loading, empty, error, success state tối thiểu.
- Mọi việc liên quan audit/admin phải xác định rõ nguồn request id: Gateway trace dùng HTTP `X-Request-ID`; Bank AP flow dùng `request_id` trong body/authenticator; admin identity dùng `performed_by` hoặc JWT claim ở mức demo. Nếu service/proto chưa nhận được field này thì ghi rõ phần cần bổ sung, không giả định đã có sẵn.
- AI dùng để scaffold code, sinh test/curl, rà lỗi TypeScript/Go, viết migration/query, nhưng người phụ trách vẫn phải đọc lại và chạy test.

### Thanh - Hoàn thiện Admin CA sau nâng cấp layered CA

Mục tiêu mới: phần Admin CA không còn là placeholder. Nhiệm vụ của Thanh chuyển sang ổn định và bàn giao Admin CA theo kiến trúc CA mới: Root CA chỉ ký Intermediate CA, `grpc-ca` ký service TLS, `client-ca` ký user/client cert, và mọi màn/API phải hiểu `cert_type`, issuer và chain metadata.

**Trạng thái đã có**

- API Gateway đã mount Admin CA dưới `/v1/admin-ca/*`.
- Đã có `POST /v1/admin-ca/auth` với admin auth demo, token demo/JWT role `admin-ca`.
- Đã có REST list/detail/revoke certificates:
  - `GET /v1/admin-ca/certificates`
  - `GET /v1/admin-ca/certificates/:serial`
  - `POST /v1/admin-ca/certificates/:serial/revoke`
- Gateway đã map CA proto sang JSON cho frontend, gồm metadata mới:
  - `cert_type`
  - `issuer_id`
  - `issuer_common_name`
  - `issuer_serial_number`
  - `chain_pem`
  - `chain_fingerprints`
  - `is_ca`
  - `key_usage`
  - `extended_key_usage`
- Revoke đã có guard layered CA:
  - Chỉ revoke `cert_type = client`.
  - Root CA, Intermediate CA và service TLS cert trả `422 CERT_TYPE_NOT_REVOKABLE`.
- Frontend `AdminCA.tsx` đã gọi API thật:
  - login admin
  - bảng certificates
  - filter status/type/issuer
  - search email/serial
  - pagination
  - detail drawer
  - copy serial/fingerprint/chain
  - revoke modal
  - chỉ bật revoke với client cert đang active
- Đã có curl mẫu tại `mini-banking-app/scripts/admin-ca-curl-examples.md`.

**Việc Thanh cần làm tiếp**

- Chạy regression cho Admin CA sau layered CA:
  - list certificates trả được Root CA, Intermediate CA, service TLS và client cert.
  - filter `cert_type=client`, `cert_type=service_tls`, `issuer_id=client-ca` hoạt động.
  - detail hiển thị issuer/chain metadata đúng.
  - revoke client cert active thành công.
  - revoke lại cert đã revoked trả 409.
  - revoke Root CA, Intermediate CA hoặc service TLS trả 422 và không đổi trạng thái cert.
- Nối Audit tab khi Thuận có endpoint đọc CA audit:
  - Nếu endpoint chưa sẵn sàng, giữ trạng thái "Audit endpoint pending" nhưng không làm gãy trang.
  - Khi endpoint sẵn sàng, hiển thị action, serial, cert_type, issuer_id, performed_by, reason, timestamp.
- Cập nhật tài liệu/curl mẫu theo contract cuối:
  - login admin
  - list all certs
  - filter theo `cert_type`
  - filter theo `issuer_id`
  - detail cert
  - revoke client cert
  - revoke non-client cert expected 422
- Bàn giao cho Quang các case Admin CA để đưa vào smoke test/demo script.

**Tự test trước khi bàn giao**

- `npx.cmd tsc --noEmit` trong `api-gateway`.
- `npm run build` hoặc `npx.cmd vite build` trong `frontend`.
- Curl list/detail/revoke trên stack đang chạy.
- UI Admin CA load được, filter/search/detail/revoke không crash.
- Xác nhận non-client cert không revoke được qua Admin CA UI/API.

**Deliverable**

- Admin CA API/UI ổn định theo layered CA.
- Contract route/role đã chốt và được ghi trong docs/curl.
- Audit tab nối API nếu endpoint đã có; nếu chưa, có ghi chú rõ dependency với Thuận.
- 6-8 curl/testcase Admin CA bàn giao cho Quang.

### Thái - Admin Bank API + Frontend Admin Bank

Mục tiêu: Admin Bank xem được overview, user/account list, ledger/transaction list, và audit bank nếu endpoint audit đã sẵn sàng.

**Bank Backend**

- Thêm các query read-only trong Bank repository/service; không thêm nghiệp vụ chỉnh sửa tiền hoặc khóa user trong scope 3 ngày.
- Endpoint tối thiểu nên expose qua Gateway dưới `/v1/admin/bank`.
- Triển khai admin Bank theo một trong hai hướng, phải chọn rõ ngay từ đầu:
  - Hướng đúng kiến trúc: thêm gRPC admin methods vào Bank proto/service rồi Gateway gọi gRPC. Cần sửa proto, regenerate code Go/TS, implement handler.
  - Hướng demo nhanh: Gateway query trực tiếp Bank Postgres bằng connection read-only cho các endpoint admin. Cần ghi rõ đây là đường tắt demo, không phải kiến trúc dài hạn.

API tối thiểu:

- `GET /v1/admin/bank/overview`
  - total_users
  - active_users
  - total_accounts
  - total_balance
  - total_transactions
  - completed_transactions
  - failed_transactions
  - audit_events_24h
- `GET /v1/admin/bank/users?email&status&limit&offset`
  - user id, email, full_name, status, account_count, total_balance, created_at.
- `GET /v1/admin/bank/users/:userId/accounts`
  - account id, account_number, balance, currency, status, created_at.
- `GET /v1/admin/bank/transactions?account_id&status&from&to&limit&offset`
  - transaction id, from/to account number, amount, status, description, cert_serial, current_hash, created_at.
- `GET /v1/admin/bank/audit?action&user_id&cert_serial&request_id&from&to&limit&offset`
  - id, action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata, created_at.

**Gateway**

- Tạo route `api-gateway/src/routes/admin-bank.route.ts`.
- Tạo controller `admin-bank.controller.ts`.
- Tạo middleware admin role:
  - `bank_admin` hoặc `admin`.
  - reuse được với thành viên 1 nếu cả hai thống nhất.
- Validate query:
  - `limit` max 100.
  - `offset` >= 0.
  - date range parse được.
  - status/action thuộc enum cho phép.
- Enum action Bank audit phải khớp DB: `transfer_completed`, `transfer_rejected`, `replay_detected`, `invalid_signature`, `certificate_rejected`, `forbidden_ownership`, `insufficient_funds`.
- Chuẩn hóa response:
  - `{ success: true, data, request_id, timestamp }`.
  - lỗi có `success: false`, `error_code`, `message`.

**Bank Frontend**

- Thay placeholder trong `frontend/src/pages/AdminBank.tsx` bằng UI thật.
- Tạo client API frontend, ví dụ `frontend/src/services/admin/bank-admin.api.ts`.
- Overview tab:
  - Cards: users, accounts, total balance, transactions, audit events 24h.
  - Không cần chart phức tạp nếu thiếu thời gian.
- Users tab:
  - Bảng users.
  - Search email.
  - Filter status.
  - Click user để xem accounts.
- Ledger tab:
  - Bảng transactions.
  - Filter status/time range/account id.
  - Hiển thị hash chain field `current_hash` dạng rút gọn.
- Security Audit tab:
  - Nếu thành viên 3 phụ trách endpoint audit chung, phối hợp contract.
  - Bảng action, user/account/transaction, cert serial, reason, created_at.
- UI state:
  - loading/empty/error/retry.
  - format tiền VND và thời gian.
  - pagination đơn giản.

**Tự test trước**

- Query overview trên DB có data.
- Query users khi chưa có user phải trả empty list, không lỗi.
- Query transactions sau transfer thành công phải thấy transaction.
- Query audit sau replay/ownership denied phải thấy event.
- `go test ./...` cho Bank nếu sửa Go.
- `npx.cmd tsc --noEmit` cho Gateway nếu sửa TS.
- `npm run build` hoặc `npx.cmd vite build` cho frontend.

**Deliverable**

- Admin Bank API read-only chạy được.
- Admin Bank UI dùng API thật cho overview/users/ledger/audit.
- Có sample response và curl mẫu cho thành viên 4.

### Thuận - Audit log còn thiếu

Mục tiêu: audit có thể ghi, đọc, filter, chứng minh được trong demo cho cả CA và Bank.

**Việc cần làm ở tầng dữ liệu/service:**

- Rà lại CA audit:
  - issue cert phải ghi `issued`.
  - admin detail phải ghi `looked_up`.
  - verify/check revocation phải ghi `verify_certificate` hoặc `revocation_checked`.
  - revoke phải ghi `revoked` và có reason.
- Rà lại Bank audit:
  - transfer success ghi `transfer_completed`.
  - transfer rejected ghi action phù hợp.
  - replay ghi `replay_detected`.
  - invalid signature ghi `invalid_signature`.
  - revoked/expired cert ghi `certificate_rejected`.
  - ownership sai ghi `forbidden_ownership`.
  - thiếu tiền ghi `insufficient_funds`.
- Đảm bảo audit không làm request chính crash nếu insert audit lỗi không nghiêm trọng.
- Chuẩn hóa metadata:
  - request_id.
  - actor/performed_by nếu là admin.
  - ip/user_agent nếu lấy được từ Gateway.
  - route/action.
- Nếu CA chưa có API đọc audit:
  - thêm repository method list audit hoặc endpoint phù hợp.
  - hiện repository interface CA chỉ có `AppendAudit`, chưa có `ListAudit`; Postgres query cần đọc bảng `certificate_audit_log`, JSON store cần đọc `AuditEvents()`.
  - filter theo serial, action, performed_by, from/to, limit/offset.
- Nếu Bank chưa có API đọc audit:
  - phối hợp thành viên 2 để query `bank_audit_log`.
  - thống nhất response dùng cho UI Admin Bank.

**API audit đề xuất (thêm nếu thấy cần thiết):**

- `GET /v1/admin/audit/ca?action&serial&performed_by&from&to&limit&offset`
- `GET /v1/admin/audit/bank?action&user_id&cert_serial&request_id&from&to&limit&offset`

**Testcase audit phải chuẩn bị:**

- Đăng ký user mới -> CA có `issued`.
- Mở detail cert trong Admin CA -> CA có `looked_up`.
- Revoke cert -> CA có `revoked`.
- Login/AS/TGS với cert revoked -> CA có check/verify event và flow bị reject.
- Transfer thành công -> Bank có `transfer_completed`.
- Gửi lại cùng request/idempotency hoặc nonce -> Bank có `replay_detected` hoặc response idempotency đúng.
- Gửi transfer từ account không thuộc user -> Bank có `forbidden_ownership`.
- Gửi chữ ký sai nếu tạo được payload test -> Bank có `invalid_signature`.

**Việc cần làm cho tài liệu:**

- Tạo bảng mapping `event -> cách kích hoạt -> nơi kiểm tra`.
- Ghi rõ field audit quan trọng:
  - CA: serial_number, action, performed_by, reason, performed_at, metadata.
  - Bank: action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata, created_at.
- Thêm phần "Audit demo script" để thành viên 4 đưa vào kịch bản tổng.

**AI hỗ trợ nên dùng vào:**

- Sinh SQL query list/filter audit.
- Sinh unit/integration test case.
- Sinh curl/Postman examples.
- Rà xem action enum trong DB schema có khớp code không.

**Deliverable**

- Audit read API chạy được hoặc ít nhất Bank/CA audit có thể đọc qua endpoint admin tương ứng.
- Audit test checklist có kết quả pass/fail.
- UI của thành viên 1/2 có data audit để hiển thị.

### Quang - Demo end-to-end, Docker Compose, seed/test data, testcase list

Mục tiêu: cả nhóm có một đường chạy demo lặp lại được, càng ít thao tác tay càng tốt.

**Script demo end-to-end cần soạn:**

- Tạo file hướng dẫn/script, ví dụ:
  - `scripts/demo/README.md`
  - `scripts/demo/smoke-test.ps1`
  - `scripts/demo/smoke-test.sh`
- Nội dung smoke test tối thiểu:
  - kiểm tra Docker đang chạy.
  - kiểm tra port 3000, 50051, 50052, 50053, 6379, 5432.
  - kiểm tra Redis `PING`.
  - kiểm tra Gateway không crash.
  - chạy curl OTP request nếu SMTP đã cấu hình.
  - chạy flow PKI/register nếu có cách bypass/mock OTP cho demo.
  - chạy AS/TGS/profile/balance/history/transfer bằng payload mẫu nếu đã có test user/cert.
  - gọi Admin CA list/detail.
  - gọi Admin Bank overview/users/transactions/audit.

**Docker Compose cần chuẩn bị:**

- Hoàn thiện hoặc tạo file compose riêng để không phá file hiện tại:
  - `docker-compose.local.yml` cho local demo.
  - `docker-compose.demo.yml` cho deploy demo nếu cần.
- Services cần có:
  - `ca-service`
  - `kdc-service`
  - `banking-service`
  - `api-gateway`
  - `frontend` nếu muốn chạy bằng Docker.
  - `redis`
  - `bank-postgres`
  - `ca-postgres` nếu CA dùng Postgres thay vì JSON.
- Nếu `banking-service` chưa có Dockerfile:
  - tạo Dockerfile tương tự CA Service nhưng đúng module `banking-service`.
- Sửa/kiểm tra `kdc-service/Dockerfile`:
  - port expose phải là 50052, không phải 50051.
  - build context phải thấy được module `pkg` do `replace ../pkg`.
- Compose phải mount/copy cert đúng:
  - root CA key/cert.
  - gRPC CA bundle.
  - CA server cert/key.
  - Bank server cert/key.
  - KDC key/certs.
- Compose env phải dùng DNS service name:
  - `CA_SERVICE_ADDRESS=ca-service:50051`.
  - `CA_HOST=ca-service`.
  - `KDC_GRPC_ADDR=kdc-service:50052`.
  - `BANK_GRPC_ADDR=banking-service:50053`.
  - Redis URL dùng `redis://redis:6379/0`.
  - Postgres URL dùng host service name trong compose.

**Seed/test data cần chuẩn bị:**

- SQL seed Bank:
  - 2-3 users demo.
  - mỗi user có ít nhất 1 account.
  - balance đủ để transfer.
  - vài transactions mẫu nếu muốn Admin Bank có data ngay.
- CA seed:
  - ưu tiên tạo cert qua flow thật để CA audit có `issued`.
  - nếu seed trực tiếp DB, phải ghi rõ không đại diện flow thật.
- Admin seed/env:
  - demo admin email/password hoặc token.
  - role `ca_admin`, `bank_admin`, `admin`.
- Redis seed:
  - thường không cần seed, nhưng cần clear Redis trước demo để tránh replay/idempotency cũ.

**Testcase list cần soạn:**

- User flow:
  - OTP request success.
  - OTP verify success.
  - PKI register success.
  - AS request success.
  - TGS request success.
  - profile/me success.
  - balance query success.
  - history query success.
  - transfer success.
- Admin CA:
  - list certificates.
  - filter active/revoked.
  - detail certificate.
  - revoke active certificate.
  - revoke same certificate again -> expected 409.
  - revoked cert không dùng được cho auth/bank flow.
- Admin Bank:
  - overview có số liệu.
  - list users.
  - list accounts của user.
  - list transactions.
  - list audit log.
- Audit:
  - CA issued/looked_up/revoked xuất hiện.
  - Bank transfer_completed xuất hiện.
  - lỗi replay/ownership/signature nếu kích hoạt được.
- Negative tests:
  - thiếu header `X-Request-ID`.
  - token admin sai role.
  - cert serial không tồn tại.
  - DB/Redis down thì Gateway trả lỗi dễ hiểu.

**Tài liệu deploy cần có:**

- File `.env.demo.example`.
- Danh sách secret cần đổi:
  - JWT secret.
  - OTP secret.
  - root CA key password.
  - SMTP user/pass.
  - DB password.
- Lệnh chạy:
  - sinh cert/key.
  - seed DB.
  - `docker compose -f docker-compose.demo.yml up --build`.
  - smoke test.
- Checklist trước demo:
  - xóa data cũ nếu cần.
  - chạy seed.
  - kiểm tra ports.
  - đăng nhập admin.
  - chạy một transfer mẫu.
  - chụp backup DB/certs nếu demo quan trọng.

**AI hỗ trợ nên dùng vào:**

- Sinh Dockerfile/compose dựa trên service hiện có.
- Sinh PowerShell smoke test.
- Sinh SQL seed idempotent.
- Sinh testcase table cho báo cáo.
- Rà lỗi env/cert path giữa local và Docker.

**Deliverable**
- Một lệnh hoặc một chuỗi lệnh rõ ràng để dựng demo.
- Compose file chạy được hoặc ít nhất chạy được backend critical path.
- Seed/test data có thể lặp lại.
- Testcase list có cột pass/fail/owner/note.

#### Timeline

| Ngày | Thanh - Admin CA sau layered CA | Thái - Admin Bank API + UI | Thuận - Audit log | Quang - Demo/Compose/Test |
|---|---|---|---|---|
| Ngày 1 | Chốt prefix `/v1/admin-ca/*` hoặc đổi đồng bộ; chạy typecheck/build; rà list/detail/revoke theo cert_type/issuer. | Chốt contract Admin Bank; implement overview/users hoặc query DB/service đầu tiên. | Rà audit schema/code; chốt API audit contract; viết testcase audit. | Chạy stack theo guide; tạo compose/demo skeleton; lập bug/env list. |
| Ngày 2 | Bổ sung curl/testcase layered CA: filter type/issuer, revoke client, reject non-client revoke; fix lỗi UI/API nếu có. | Hoàn thành Admin Bank API overview/users/transactions; dựng UI overview/users/ledger. | Implement/read audit CA/Bank hoặc phối hợp endpoint với TV1/TV2; tạo seed tình huống audit. | Hoàn thiện seed data; smoke script bản đầu; compose đủ service quan trọng. |
| Ngày 3 | Nối Audit tab nếu endpoint sẵn; bàn giao contract, curl mẫu và kết quả regression cho Quang. | Fix lỗi Admin Bank; nối audit tab; bàn giao curl/testcase. | Chạy audit regression; xác nhận event xuất hiện trong UI/API; ghi note còn thiếu. | Deploy rehearsal; chạy full testcase list; gom bug cuối; chuẩn bị demo script cuối. |
