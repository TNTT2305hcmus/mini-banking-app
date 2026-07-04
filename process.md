# Đánh giá tiến độ và Phân công

## 0. Đánh giá mức hoàn thành hiện tại

| Hạng mục | Mức hoàn thành | Ghi chú |
|---|---:|---|
| CA Service | 85% | Đã có cấp/verify/list/detail/revoke cert, store JSON/Postgres, TLS gRPC. Audit CA đã ghi cho issue/verify/detail/revoke. |
| KDC Service | 80% | Đã có AS/TGS, replay Redis, key provisioning. Cần hardening env/Docker và test end-to-end. |
| Banking Service | 75% | Đã có profile/balance/history/transfer, Postgres + Redis, audit write vào `bank_audit_log`. Chưa có Dockerfile riêng và chưa có API admin đọc dữ liệu vận hành. |
| API Gateway | 70% | Đã nối OTP, PKI, KDC, Bank cho luồng user. Chưa mount `/v1/admin/*`, chưa có admin auth/role, chưa expose API CA admin hoặc Bank admin ra REST. |
| User Frontend | 70% | Đã có UI và service client crypto cho đăng ký/login/bank flow; cần chạy full backend để xác nhận end-to-end. |
| Admin CA UI | 25% | Có route `/admin-ca` và layout tab Certificates/Audit Log, nhưng hiện chỉ là placeholder, chưa fetch API, chưa list/detail/revoke thật. |
| Admin Bank UI | 20% | Có route `/admin-bank` và layout Overview/Users/Ledger/Security Audit, nhưng hiện chỉ là placeholder, chưa fetch API. |
| Admin CA API | 45% | CA gRPC đã có list/detail/revoke; Gateway chưa wrap các method này thành REST, chưa có endpoint audit log. |
| Admin Bank API | 15% | Bank DB có users/accounts/transactions/audit; service chưa có gRPC/REST admin list users, list ledger, list audit, overview metrics. |
| Audit Log | 60% | CA và Bank đều đã ghi audit nội bộ. Thiếu API đọc audit, filter/pagination, admin viewer, request-id/performed-by đầy đủ từ Gateway, và chính sách giữ log khi deploy. |
| Docker/DevOps | 45% | `docker-compose.yml` hiện mới cover CA + Gateway + Redis; chưa compose full KDC/Bank/Postgres. |
| Tài liệu chạy | 80% sau file này | Có hướng dẫn local chi tiết; compose full vẫn là việc còn lại. |

### 0.1. Admin UI

| Module admin | Hiện có | Còn thiếu để demo được |
|---|---|---|
| Admin CA UI | Route `/admin-ca`, sidebar, tab Certificates/Audit Log, empty state. | Bảng certificates, search/filter, detail drawer, revoke modal, audit tab, loading/error state, gọi API thật. |
| Admin CA API | CA Service gRPC có `ListCertificates`, `GetCertificateDetail`, `RevokeCertificate`. | Gateway route `/v1/admin/ca/certificates`, `/detail`, `/revoke`; admin auth middleware; audit read endpoint. |
| Admin Bank UI | Route `/admin-bank`, sidebar, tab Overview/Users/Ledger/Security Audit, empty state. | Dashboard metrics, bảng users/accounts, bảng transactions/ledger hash, audit table, filter/search, gọi API thật. |
| Admin Bank API | DB đã có `users`, `accounts`, `transactions`, `bank_audit_log`; Bank Service ghi audit. | gRPC/REST admin queries để đọc users/accounts/transactions/audit/metrics; phân trang; filter theo user/action/time. |

### 0.2. Audit Log

Đã có:

- CA ghi audit cho issue, verify/revocation check, detail lookup, revoke.
- Bank ghi audit cho transfer completed/rejected, replay, invalid signature, certificate rejected, forbidden ownership, insufficient funds.
- DB schema có `certificate_audit_log` và `bank_audit_log`.

Còn thiếu để kiểm thử/deploy ổn:

- API đọc audit log cho admin, có pagination/filter theo action, serial/user, time range.
- Gateway truyền `X-Request-ID`, admin identity (`performed_by`) và IP/user-agent xuống CA/Bank nhất quán.
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
| P0 | Admin CA REST API. | List/detail/revoke certificates qua Gateway, có validation/error mapping. |
| P0 | Admin Bank REST API tối thiểu. | Overview metrics, users/accounts list, transactions list, audit list. |
| P1 | Admin CA UI nối API thật. | Table + filters + detail + revoke modal + audit tab. |
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
- Mọi việc liên quan audit/admin phải truyền được `X-Request-ID` và admin identity ở mức demo.
- AI dùng để scaffold code, sinh test/curl, rà lỗi TypeScript/Go, viết migration/query, nhưng người phụ trách vẫn phải đọc lại và chạy test.

### Thanh - Admin CA API + Frontend Admin CA

Mục tiêu: Admin CA xem được danh sách certificate, xem detail, revoke certificate, và xem audit CA nếu endpoint audit đã sẵn sàng.

**CA Backend**

- Tạo route admin CA trong API Gateway, ví dụ `api-gateway/src/routes/admin-ca.route.ts`.
- Mount route trong `server.ts` dưới prefix `/v1/admin/ca`.
- Tạo service wrapper trong `api-gateway/src/services/ca.service.ts` cho các gRPC method đã có:
  - `listCertificates`
  - `getCertificateDetail`
  - `revokeCertificate`
- Tạo controller admin CA, ví dụ `api-gateway/src/controller/admin-ca.controller.ts`.
- API tối thiểu:
  - `GET /v1/admin/ca/certificates?status&owner_id&email&serial&limit&offset`
  - `GET /v1/admin/ca/certificates/:serial`
  - `POST /v1/admin/ca/certificates/:serial/revoke`
- Request revoke tối thiểu:
  - `reason`: bắt buộc, không rỗng.
- Response list cần có:
  - `items`: mảng certificate metadata.
  - `total`, `limit`, `offset`.
- Response detail cần có:
  - serial, owner id, CN, email, fingerprint, status, not_before, not_after, issued_at, revoked_at, revocation_reason.
- Map lỗi gRPC sang HTTP:
  - not found -> 404.
  - invalid argument -> 400.
  - already revoked/already exists -> 409.
  - còn lại -> 502 hoặc 500 tùy lỗi Gateway/CA.
- Thêm middleware admin demo nếu chưa có:
  - đọc `Authorization: Bearer <admin-token>` hoặc dùng JWT demo.
  - check role `ca_admin` hoặc `admin`.
  - set `performed_by=admin:<email-or-name>` khi gọi CA gRPC.
- Đảm bảo mọi request admin có `X-Request-ID`; nếu client không gửi thì Gateway tự sinh.

**Frontend**

- Thay placeholder trong `frontend/src/pages/AdminCA.tsx` bằng UI có dữ liệu thật.
- Tạo client API frontend, ví dụ `frontend/src/services/admin/ca-admin.api.ts`.
- Màn Certificates:
  - Bảng certificate: serial, email, owner id, status, issued_at, expires_at.
  - Filter status: all/active/revoked/expired.
  - Search theo email hoặc serial.
  - Pagination limit/offset đơn giản.
  - Nút xem detail.
  - Nút revoke chỉ bật khi cert đang active.
- Detail drawer/modal:
  - Hiển thị fingerprint, CN, owner id, email, validity, revoke info.
  - Có copy serial/fingerprint.
- Revoke modal:
  - Nhập reason.
  - Confirm rõ đây là thao tác không hoàn tác.
  - Sau revoke refresh list/detail.
- Audit tab:
  - Nếu thành viên 3 đã có endpoint CA audit thì nối API.
  - Nếu chưa kịp, hiển thị message "Audit endpoint pending" nhưng không để trang gãy.
- UI state:
  - loading skeleton hoặc text gọn.
  - empty state khi chưa có cert.
  - error state có nút retry.
  - toast hoặc message khi revoke thành công/thất bại.

**Tự test trước**

- `npx.cmd tsc --noEmit` trong `api-gateway`.
- `npm run build` hoặc `npx.cmd vite build` trong `frontend`.
- Curl list cert.
- Curl detail cert tồn tại và không tồn tại.
- Curl revoke cert active.
- Revoke lại cert đã revoked phải ra 409.
- UI Admin CA load được, filter/search không crash.

**Deliveriable**

- Admin CA API chạy được qua Gateway.
- Admin CA UI dùng API thật cho list/detail/revoke.
- Ghi lại 5-7 curl mẫu cho thành viên 4 đưa vào demo script/testcase list.

### Thái - Admin Bank API + Frontend Admin Bank

Mục tiêu: Admin Bank xem được overview, user/account list, ledger/transaction list, và audit bank nếu endpoint audit đã sẵn sàng.

**Bank Backend**

- Thêm các query read-only trong Bank repository/service; không thêm nghiệp vụ chỉnh sửa tiền hoặc khóa user trong scope 3 ngày.
- Endpoint tối thiểu nên expose qua Gateway dưới `/v1/admin/bank`.
- Triển khai thêm gRPC admin methods vào Bank Service rồi Gateway gọi gRPC.

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
- `GET /v1/admin/bank/audit?action&user_id&cert_serial&from&to&limit&offset`
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

### Quag - Demo end-to-end, Docker Compose, seed/test data, testcase list

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

| Ngày | Thành viên 1 - Admin CA API + UI | Thành viên 2 - Admin Bank API + UI | Thành viên 3 - Audit log | Thành viên 4 - Demo/Compose/Test |
|---|---|---|---|---|
| Ngày 1 | Chốt contract Admin CA; implement Gateway list/detail/revoke; dựng API client frontend. | Chốt contract Admin Bank; implement overview/users hoặc query DB/service đầu tiên. | Rà audit schema/code; chốt API audit contract; viết testcase audit. | Chạy stack theo guide; tạo compose/demo skeleton; lập bug/env list. |
| Ngày 2 | Hoàn thành Admin CA UI table/detail/revoke; test curl + UI. | Hoàn thành Admin Bank API overview/users/transactions; dựng UI overview/users/ledger. | Implement/read audit CA/Bank hoặc phối hợp endpoint với TV1/TV2; tạo seed tình huống audit. | Hoàn thiện seed data; smoke script bản đầu; compose đủ service quan trọng. |
| Ngày 3 | Fix lỗi Admin CA; nối audit tab nếu endpoint sẵn; bàn giao curl/testcase. | Fix lỗi Admin Bank; nối audit tab; bàn giao curl/testcase. | Chạy audit regression; xác nhận event xuất hiện trong UI/API; ghi note còn thiếu. | Deploy rehearsal; chạy full testcase list; gom bug cuối; chuẩn bị demo script cuối. |
