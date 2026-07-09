# Đánh giá hiện trạng dự án sau scan lại

Cập nhật: 09/07/2026.  
Nguồn đối chiếu: `process.md`, `temp-docs/thai-report.md`, `temp-docs/thuan-report.md`, `temp-docs/problem2.md`, static scan code hiện tại và các lệnh build/test local.

## 1. Phạm vi scan và kết quả kiểm chứng

### 1.1. Trạng thái Git/worktree

Nhánh hiện tại: `thanh`, đang đồng bộ với `origin/thanh`.

Worktree có thay đổi chưa commit trước/sau scan:

- `regard.md`: file đánh giá hiện trạng đang được cập nhật.
- `temp-docs/problem2.md`: untracked, đã được dùng làm nguồn tóm tắt góp ý của Thái.
- `temp-docs/PROBLEM.md`: đang bị xóa trong worktree.
- `temp-docs/kệ-file-này-đi-ô.md`: đang bị xóa trong worktree.

Lưu ý: các thay đổi trong `temp-docs` không được sửa hoặc revert trong lần scan này.

### 1.2. Test/build đã chạy

Các kiểm tra local đã pass:

- Frontend: `npm.cmd run build -- --outDir ..\..\.tmp\frontend-build --emptyOutDir`
- API Gateway: `tsc --noEmit`
- CA Service: `go test ./...`
- Banking Service: `go test ./...`
- KDC Service: `go test ./...`

Ghi chú frontend build: Vite cảnh báo bundle JS lớn hơn 500 kB sau minify. Đây là warning tối ưu hiệu năng, không phải lỗi build.

### 1.3. Docker/database runtime

Docker daemon truy cập được sau khi chạy ngoài sandbox, nhưng `docker ps` không có container nào đang chạy. Vì vậy:

- Chưa validate được Postgres thật.
- Chưa validate được seed `seed_demo.sql` trên DB thật.
- Chưa validate được các endpoint qua Gateway đang chạy.
- Chưa validate được KDC audit ghi thật vào `kdc_audit_log`.
- Chưa chạy được smoke test runtime.

Kết luận kiểm chứng hiện tại: code build/test sạch, nhưng trạng thái demo end-to-end và database runtime vẫn chưa được xác nhận.

## 2. Đánh giá tổng quan theo `process.md`

Dự án đã có nền tảng chức năng khá rộng:

- User flow: OTP/register, AS/TGS, AP exchange, profile/balance/history/transfer.
- Admin CA: login demo, list/detail/revoke certificate, audit tab/API.
- Admin Bank: cert-based activation/login/session, dashboard overview/users/accounts/transactions/audit.
- Admin SOC: security-admin auth, KDC audit, timeline, verify, summary, export.
- Audit: CA/Bank/KDC đều có hướng hash-chain và read API.
- Demo assets: compose, Dockerfile, seed, smoke scripts, docs/testcases.

Tuy nhiên trạng thái hiện tại vẫn chưa đạt “demo final chắc tay” vì các điểm P0 trong `process.md` còn vướng ở tích hợp runtime, env/smoke, register rollback, Admin CA cert-based và regression audit/database thật.

## 3. Hiện trạng theo module

| Module | Đánh giá hiện tại | Bằng chứng đã thấy | Rủi ro còn lại |
|---|---|---|---|
| CA Service | Khá tốt về core CA/audit, test pass | `go test ./...` pass; có issue/verify/list/detail/revoke/audit/hash-chain | Chưa có role `ca_admin`; Admin CA cert-based chưa làm; cần regression DB thật |
| KDC Service | Code audit/trace đã tốt hơn `process.md` cũ | Gateway `kdc.service.ts` đã gửi gRPC metadata `x-request-id`; KDC có `DATABASE_URL` optional; test pass | Compose local/demo chưa set `DATABASE_URL` cho KDC, nên Docker demo có thể no-op audit |
| Banking Service | Core Bank/Admin Bank khá chắc, test pass | User/account/transfer/history/admin session/audit đều có code; Banking test pass | Cần E2E với DB thật; Admin Bank report chưa có Postman/curl cụ thể |
| API Gateway | Nhiều route đã đủ, nhưng Thanh P0 còn hở | Admin CA, Admin Bank, KDC, SOC routes tồn tại; typecheck pass | Register rollback chưa xong; rate-limit hardcode; Admin CA env default chưa fail-closed |
| Frontend User | Có cải thiện UI 50M so với report cũ | Home hiển thị daily used/remaining, cảnh báo vượt balance/limit, refresh sau fail | `api.service.ts` vẫn sinh `X-Request-ID` mới mỗi HTTP call, chưa có `operation_id` xuyên cả flow |
| Admin CA UI/API | Có list/detail/revoke/audit | Route chính là `/v1/admin-ca/*`; middleware Bearer/JWT/static token | Auth chính vẫn password/JWT/static token, chưa cert-based |
| Admin Bank UI/API | Khá hoàn chỉnh | `/admin-bank/activate`, `/admin-bank/login`, dashboard, 5 API query, cookie session | Cần chạy full flow thật; thiếu curl/Postman mẫu trong report |
| Admin SOC | Có API/UI nền | `/admin-soc`, `/v1/admin-sec/auth`, timeline/verify/summary/export | Bank audit vẫn cookie-gated đúng thiết kế; cần demo rõ và test có/không cookie |
| Audit | Nền code tốt, test local pass | CA/KDC/Bank audit read, semantics, hash-chain verify | `docs/audit-testcases.md` chưa điền pass/fail; external anchor chưa có |
| Docker/DevOps | Chưa đạt yêu cầu P0 | Có compose/env/smoke/docs | Docker hiện không có container chạy; `.env.demo.example` và smoke vẫn lệch route/env; KDC audit DB chưa wired |

## 4. Các điểm đã tiến triển so với nội dung `process.md`

Một số rủi ro ghi trong `process.md` đã được code hiện tại xử lý một phần:

- Gateway đã forward `X-Request-ID` xuống KDC:
  - `api-gateway/src/services/kdc.service.ts` tạo gRPC `Metadata` và set `x-request-id`.
  - `requestTgt(...)`, `requestServiceTicket(...)`, `listKdcAuditEvents(...)`, `verifyKdcAuditChain(...)` đều nhận `requestId`.
- Gateway đã forward `X-Request-ID` xuống Bank:
  - `api-gateway/src/services/bank.service.ts` set `x-request-id` cho `transferMoney`, `getBalance`, `getHistory`.
- User Home đã cải thiện vấn đề 50M:
  - có `dailyTransferUsed`;
  - hiển thị hạn mức còn lại;
  - cảnh báo trước khi amount vượt balance hoặc daily limit;
  - gọi refresh sau transfer fail để cập nhật balance/history.
- Admin Bank API/UI đã có đường chính cho dashboard:
  - overview/users/accounts/transactions/audit;
  - status `pending/completed/failed`;
  - audit enrichment qua `AuditTimeline`.

Tuy nhiên, tiến triển này chưa thay thế được regression runtime. Các testcase audit và luồng demo vẫn cần chạy thật.

## 5. Các điểm P0 còn chưa đạt

### 5.1. Register rollback của Thanh chưa đạt

Trong `api-gateway/src/controller/ca.controller.ts`, flow hiện tại vẫn:

- kiểm tra `jti`;
- set `jtiKey` thành `"1"` trước khi flow chắc chắn thành công;
- gọi CA `registerUser`;
- sau đó mới gọi Bank `createUserBankAccount`.

Chưa thấy:

- Bank RPC/read path `CheckUserEmail(email)`;
- pre-check email trước khi cấp cert;
- rollback `jti` khi Bank fail;
- compensation revoke cert với reason `registration_rollback`;
- map lỗi email tồn tại thành `409 EMAIL_ALREADY_REGISTERED` riêng.

Đây là P0 lớn nhất của Thanh vì có thể tạo lệch dữ liệu CA/Bank: cert đã cấp nhưng Bank user/account chưa tạo được.

### 5.2. Admin CA cert-based chưa đạt

Trong `proto/ca.proto` và `ca-service/internal/ca/identity_role.go`, role hiện chỉ có:

- `customer`;
- `bank_admin`.

Chưa có role/cert type `ca_admin`. Admin CA hiện vẫn dùng:

- `POST /v1/admin-ca/auth`;
- email/password demo;
- JWT role `admin-ca`;
- static token `ADMIN_CA_DEMO_TOKEN`.

Như vậy yêu cầu trong `process.md` “Admin CA cert-based bằng role/cert `ca_admin` hoặc cơ chế tương đương” vẫn chưa hoàn thành. Password/JWT/static token hiện chỉ nên xem là fallback hoặc demo shortcut, chưa phải cơ chế chính để trình bày final.

### 5.3. Admin CA env chưa fail-closed đúng yêu cầu

Trong `api-gateway/src/config/env.ts`, các biến Admin CA vẫn có default dạng chuỗi:

- `ADMIN_CA_DEMO_EMAIL` default `"ADMIN_CA_DEMO_EMAIL is required"`;
- `ADMIN_CA_DEMO_PASSWORD` default `"ADMIN_CA_DEMO_PASSWORD is required"`;
- `ADMIN_CA_DEMO_TOKEN` default `"ADMIN_CA_DEMO_TOKEN is required"`.

Điểm này trái với phân công Thanh trong `process.md`: không default thành chuỗi đoán được/chuỗi placeholder, cần fail-closed hoặc bắt buộc cấu hình rõ.

### 5.4. Rate-limit demo chưa đạt

`api-gateway/src/middleware/rateLimiter.ts` vẫn hardcode:

- AS/IP: 10 request / 5 phút;
- TGS/cert serial: 10 request / 5 phút;
- Bank API/IP: 20 request / 1 phút;
- OTP/email: 3 request / 10 phút.

Chưa thấy:

- env để tăng ngưỡng;
- `RATE_LIMIT_DISABLED=1`;
- chỉ đếm fail thay vì đếm cả success;
- `Retry-After` trong response 429.

Rủi ro demo: login đúng hoặc rehearsal nhiều lần vẫn có thể bị 429.

### 5.5. Compose/smoke/env vẫn lệch

Các lệch đã ghi trong `process.md` vẫn còn:

- `.env.demo.example` vẫn dùng `CA_DEMO_EMAIL` và `CA_DEMO_PASSWORD`, trong khi Gateway đọc `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN`.
- Smoke scripts vẫn gọi `/v1/admin/certificates`, trong khi route code chính thức là `/v1/admin-ca/certificates`.
- `docker-compose.local.yml` và `docker-compose.demo.yml` chưa set `DATABASE_URL` cho `kdc-service`.
- `docker-compose.local.yml` vẫn publish gRPC ports 50051/50052/50053 để debug local; điều này chấp nhận được cho local nhưng cần ghi rõ, còn compose demo production-like nên tránh expose nếu không cần.

### 5.6. Audit testcase chưa được chốt pass/fail

`mini-banking-app/docs/audit-testcases.md` có bảng 27 testcase nhưng cột `Pass/Fail`, `Owner`, `Note` vẫn trống. Đây là deliverable trực tiếp của Thuận trong `process.md`.

Code audit có nền tốt, nhưng chưa có bằng chứng rehearsal final cho:

- CA audit;
- KDC audit với DB thật;
- Bank audit;
- SOC timeline;
- verify hash-chain;
- summary/export;
- negative cases.

## 6. Đánh giá theo từng thành viên

### 6.1. Thanh

Phạm vi trong `process.md`: Admin CA, đăng ký PKI, auth CA.

Hiện trạng:

- Admin CA UI/API đã có đường password/JWT/static token.
- Admin CA list/detail/revoke/audit đã có route `/v1/admin-ca/*`.
- Register user vẫn còn rủi ro cấp cert trước khi Bank user/account tạo thành công.
- Chưa có `ca_admin` cert role.
- Chưa có Admin CA activation/session cert-based.
- Chưa sửa rate-limit demo.
- Chưa sửa Admin CA env fail-closed.

Đánh giá: phần nền đã có, nhưng các P0 cốt lõi của Thanh trong `process.md` chưa hoàn tất. Nhiệm vụ Thanh nên được ưu tiên tiếp theo vì đang ảnh hưởng trực tiếp tới tính nhất quán CA/Bank và câu chuyện demo Admin CA cert-based.

### 6.2. Thái

Phạm vi trong `process.md`: Admin Bank API/UI và dữ liệu dashboard.

Hiện trạng code:

- Admin Bank cert-based flow đã có: provision/activate/login/session/dashboard.
- 5 API dashboard đã có route/service/UI:
  - overview;
  - users;
  - accounts;
  - transactions;
  - audit.
- UI ledger có status `completed/failed`.
- User Home đã cải thiện đáng kể vấn đề 50M.
- Seed report của Thái ghi đã có 20 customer, 20 account, 50 transaction và 55 audit event trong `seed_demo.sql`.

Khoảng trống:

- `thai-report.md` chưa có Postman/curl cụ thể cho 5 endpoint.
- Report nói đã test tay một số case, nhưng chưa có checklist pass/fail đầy đủ.
- Vì Docker không có container chạy, chưa xác nhận seed đã nạp vào DB thật.

Đánh giá: phần Thái có chất lượng code/UI tốt hơn mức report thể hiện. Cần bổ sung bằng chứng test/curl và validate runtime để biến thành deliverable final.

### 6.3. Thuận

Phạm vi trong `process.md`: Audit, SOC, trace-id.

Hiện trạng code:

- Gateway đã forward trace-id xuống KDC/Bank qua gRPC metadata, đây là điểm tốt hơn nội dung process cũ.
- KDC audit có code, migration, list/verify, hash-chain.
- Bank/CA audit có read path và semantic enrichment.
- SOC có timeline/summary/export/verify.

Khoảng trống:

- Frontend vẫn sinh `X-Request-ID` mới mỗi HTTP call trong `api.service.ts`, nên chưa có `operation_id` xuyên suốt cả flow register/login/transfer.
- `docs/audit-testcases.md` chưa điền pass/fail.
- KDC audit phụ thuộc `DATABASE_URL`; compose chưa set nên Docker demo có thể không có KDC audit thật.
- Hash-chain vẫn chưa cover timestamp/metadata và chưa chống tail truncation nếu không có external anchor.

Đánh giá: phần Thuận có nền kỹ thuật tốt và đã xử lý được trace metadata ở Gateway, nhưng deliverable final vẫn thiếu bằng chứng chạy thật và bảng testcase. Cần ghi giới hạn hash-chain trung thực khi báo cáo.

### 6.4. Quang

Phạm vi trong `process.md`: full stack, compose, seed, smoke test.

Hiện trạng:

- Có bộ compose local/demo, Dockerfile, seed, smoke scripts, docs/testcases.
- Nhưng các lệch route/env/smoke/KDC DB trong `process.md` vẫn còn.
- Không có `temp-docs/quang-report.md` trong scan hiện tại.
- Docker daemon hiện không chạy container nào, nên stack demo chưa được xác nhận.

Đánh giá: phần vận hành có nền tài liệu và script, nhưng chưa đạt tiêu chí “máy sạch chạy được lặp lại”. Đây là rủi ro demo độc lập với code compile.

## 7. Đánh giá checklist demo P0 trong `process.md`

| P0 demo item | Trạng thái hiện tại | Ghi chú |
|---|---|---|
| Stack chạy được từ hướng dẫn mới | Chưa xác nhận | Docker không có container chạy |
| `.env.demo.example`, compose, smoke khớp Gateway | Chưa đạt | Còn `CA_DEMO_*`, route `/v1/admin/certificates` sai |
| Smoke script không gọi sai Admin CA route | Chưa đạt | Smoke vẫn gọi `/v1/admin/certificates` |
| Đăng ký user mới không tạo cert rác khi Bank fail | Chưa đạt | Register rollback/CheckUserEmail chưa có |
| AS/TGS không bị rate-limit trong demo | Chưa đạt | Rate-limit hardcoded, không env disable |
| Transfer thành công thấy balance/history đúng | Chưa xác nhận runtime | Code có flow, cần E2E |
| Transfer failed/daily-limit không hiện như success | Đạt một phần | UI đã cải thiện, cần runtime proof |
| Admin CA cert-based `ca_admin` | Chưa đạt | Chưa có role/cert/session cert-based |
| Admin CA xem certificate/detail/revoke/audit | Đạt về code, chưa xác nhận runtime | Route/UI có |
| Admin Bank activate/login/dashboard | Đạt về code, chưa xác nhận runtime | Cần chạy full flow thật |
| KDC audit có `DATABASE_URL` trong demo | Chưa đạt | Compose chưa set |
| SOC xem KDC audit/summary/export/verify/timeline | Đạt về code, chưa xác nhận runtime | Audit testcase chưa pass/fail |

## 8. Ưu tiên đề xuất trước khi implement nhiệm vụ Thanh

Để bước tiếp theo tập trung và không vỡ demo, nên xử lý nhiệm vụ Thanh theo thứ tự:

1. Sửa register consistency:
   - thêm Bank `CheckUserEmail(email)`;
   - pre-check trước CA issue;
   - không mark `jti` used quá sớm, hoặc rollback khi fail;
   - nếu Bank fail sau khi CA issue thì revoke cert best-effort với `registration_rollback`;
   - map email trùng thành `409 EMAIL_ALREADY_REGISTERED`.

2. Sửa rate-limit demo:
   - thêm env `RATE_LIMIT_DISABLED=1` hoặc các env limit/window;
   - thêm `Retry-After` khi 429;
   - ưu tiên không chặn rehearsal bình thường.

3. Sửa Admin CA env fail-closed:
   - bỏ default placeholder cho `ADMIN_CA_DEMO_*`;
   - đồng bộ `.env.demo.example`;
   - tránh token/credential đoán được.

4. Quyết định cách làm Admin CA cert-based:
   - phương án đầy đủ: thêm `ca_admin` role vào CA proto/service, activation/session tương tự Bank Admin;
   - phương án demo an toàn hơn nếu thiếu thời gian: ghi rõ JWT/static chỉ là fallback, nhưng cần có lý do và giới hạn trong report.

5. Sau khi sửa Thanh, chạy lại:
   - API Gateway typecheck;
   - CA/Banking/KDC tests;
   - frontend build;
   - nếu stack Docker được bật: smoke route Admin CA, register duplicate/fail rollback, KDC audit DB.

## 9. Đề xuất giai đoạn triển khai phần Thanh và Thuận

Phạm vi dưới đây cố ý không bao gồm phần Quang vì compose/smoke/full-stack đang được xử lý riêng. Các giai đoạn tập trung vào phần còn thiếu của Thanh và Thuận, theo hướng làm đến đâu có test và điểm chốt đến đó.

### Giai đoạn 0 - Chốt baseline và phạm vi sửa

Mục tiêu:

- Không trộn phần Quang vào nhánh việc của Thanh/Thuận.
- Giữ baseline compile/test hiện tại đang xanh.
- Xác định rõ file nào sẽ bị chạm để tránh lan phạm vi.

Việc cần làm:

- Giữ nguyên các thay đổi hiện có trong `temp-docs` nếu không được yêu cầu xử lý.
- Tạo checklist ngắn cho hai luồng:
  - Thanh: register consistency, rate-limit demo, Admin CA env, Admin CA cert-based.
  - Thuận: operation/trace id, audit testcase, SOC timeline, hash-chain limitation.
- Trước khi sửa code, ghi nhận lại trạng thái test đang pass.

Tiêu chí hoàn tất:

- Có danh sách file dự kiến sửa.
- Có thứ tự ưu tiên rõ.
- Chưa có thay đổi logic ngoài phạm vi Thanh/Thuận.

Kiểm tra:

- `git status --short --branch`
- `npm.cmd run build -- --outDir ..\..\.tmp\frontend-build --emptyOutDir`
- `.\node_modules\.bin\tsc.cmd --noEmit`
- `go test ./...` cho `ca-service`, `banking-service`, `kdc-service`

### Giai đoạn 1 - Thanh: sửa register consistency CA/Bank

Mục tiêu:

- Đăng ký user không còn tạo cert rác khi Bank create user/account fail.
- Email đã tồn tại được chặn trước khi CA cấp cert.
- Registration token/JTI không bị đánh dấu used quá sớm.

Việc cần làm:

- Thêm Bank RPC/read path `CheckUserEmail(email)` hoặc cơ chế tương đương:
  - cập nhật `proto/bank.proto`;
  - regenerate Go/TS proto;
  - implement trong Banking Service repository/service/gRPC handler;
  - expose Gateway service wrapper.
- Sửa `api-gateway/src/controller/ca.controller.ts`:
  - gọi `CheckUserEmail` trước `registerUser`;
  - nếu email đã tồn tại, trả `409 EMAIL_ALREADY_REGISTERED`;
  - chỉ mark `jti` used khi flow đã thành công, hoặc rollback `jti` khi fail;
  - nếu CA đã cấp cert nhưng Bank fail, gọi revoke best-effort với reason `registration_rollback`;
  - audit `ra_registration_rejected` với reason rõ ràng.

Tiêu chí hoàn tất:

- Email trùng không gọi CA issue.
- Bank fail sau CA issue không để cert active không chủ.
- JTI không khóa vĩnh viễn khi lỗi hạ tầng giữa chừng.
- Lỗi trả về có mã ổn định để frontend/demo giải thích được.

Kiểm tra:

- Unit test Banking Service cho `CheckUserEmail`.
- API Gateway typecheck.
- CA/Banking tests.
- Nếu có stack runtime: thử đăng ký email trùng và thử giả lập Bank fail để xác nhận rollback/revoke.

### Giai đoạn 2 - Thanh: rate-limit demo và Admin CA env fail-closed

Mục tiêu:

- Demo/rehearsal không bị chặn bởi AS/TGS rate-limit quá thấp.
- Admin CA credential/token demo không có default placeholder đoán được.

Việc cần làm:

- Sửa `api-gateway/src/middleware/rateLimiter.ts`:
  - thêm `RATE_LIMIT_DISABLED=1` cho rehearsal/demo; hoặc
  - thêm env cấu hình window/max cho AS/TGS/Bank/OTP;
  - response 429 nên có `Retry-After` và message rõ.
- Sửa `api-gateway/src/config/env.ts`:
  - bỏ default `"ADMIN_CA_DEMO_* is required"`;
  - chuyển `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN` sang required hoặc optional fail-closed giống `ADMIN_SEC_DEMO_*`;
  - đảm bảo thiếu config thì login Admin CA không mở bằng placeholder.
- Đồng bộ tài liệu liên quan trong phạm vi Thanh nếu cần, không đụng phần compose/smoke của Quang trừ khi chỉ ghi chú.

Tiêu chí hoàn tất:

- Có thể disable hoặc nới rate-limit bằng env.
- 429 có thông tin chờ rõ ràng.
- Không có credential/token placeholder hoạt động như secret thật.

Kiểm tra:

- API Gateway typecheck.
- Test tay rate-limit với env disable/nới ngưỡng nếu có server.
- Login Admin CA khi thiếu env phải fail closed, không dùng chuỗi placeholder.

### Giai đoạn 3 - Thanh: Admin CA cert-based hoặc quyết định fallback có kiểm soát

Mục tiêu:

- Đạt yêu cầu `process.md`: Admin CA dùng cert-based role `ca_admin`

Phương án đầy đủ:

- Thêm role `ca_admin` vào CA identity role:
  - proto CA;
  - Go identity role normalization;
  - generated TS/Go proto;
  - migration/metadata nếu cần.
- Tạo flow Admin CA activation/session tương tự Bank Admin:
  - provision pending Admin CA;
  - issue cert role `ca_admin`;
  - login bằng AS/TGS/AP hoặc cơ chế cert proof tương đương;
  - Gateway set session/cookie hoặc token scoped riêng cho Admin CA.
- Cập nhật UI `/admin-ca` để cert-based là đường chính, bỏ password/JWT chỉ fallback.

Tiêu chí hoàn tất:

- Nếu làm đầy đủ: Admin CA auth không còn phụ thuộc password/JWT là đường chính.

Kiểm tra:

- CA tests.
- API Gateway typecheck.
- Frontend build nếu sửa UI.
- Manual Admin CA login/list/detail/revoke/audit khi có runtime.

### Giai đoạn 4 - Thuận: operation_id xuyên flow và audit correlation

Mục tiêu:

- Timeline SOC có ý nghĩa theo một nghiệp vụ, không chỉ theo từng HTTP request rời rạc.
- CA/KDC/Bank có thể cùng xuất hiện trong timeline theo một id chung.

Việc cần làm:

- Thiết kế `operation_id` ở frontend:
  - tạo một id cho từng flow lớn: register, login, transfer;
  - tái sử dụng id đó cho các HTTP call thuộc cùng flow;
  - vẫn giữ AP `request_id` riêng nếu protocol cần.
- Sửa `frontend/src/services/api.service.ts` hoặc wrapper liên quan:
  - cho phép caller truyền `X-Request-ID` thay vì luôn `crypto.randomUUID()`;
  - tránh phá các call hiện có.
- Đảm bảo Gateway tiếp tục forward id xuống:
  - CA qua gRPC metadata hoặc audit metadata;
  - KDC qua metadata `x-request-id`;
  - Bank qua metadata `x-request-id`.
- Cập nhật `docs/audit-testcases.md` case timeline theo `operation_id`.

Tiêu chí hoàn tất:

- Một flow register/login/transfer có thể truy lại bằng một id chung.
- Timeline SOC ít nhất có CA + KDC; nếu có Bank cookie thì gộp thêm Bank.
- Không làm sai AP request_id/idempotency_key của Bank.

Kiểm tra:

- Frontend build.
- API Gateway typecheck.
- KDC/Bank/CA tests nếu có sửa backend.
- Manual hoặc curl runtime: query `/v1/admin/audit/timeline?request_id=<operation_id>`.

### Giai đoạn 5 - Thuận: audit testcase pass/fail và hash-chain limitation

Mục tiêu:

- Biến audit từ “có code” thành “có bằng chứng demo”.
- Ghi rõ giới hạn bảo mật, tránh claim quá mức.

Việc cần làm:

- Điền `mini-banking-app/docs/audit-testcases.md`:
  - `Pass/Fail`;
  - owner;
  - note;
  - endpoint hoặc màn hình kiểm chứng.
- Tập trung trước các case P0:
  - CA `issued`, `looked_up`, `revoked`, `ra_*`;
  - KDC `as_ticket_issued`, `as_rejected`, `tgs_ticket_issued`, `tgs_rejected`;
  - Bank `transfer_completed`, `transfer_rejected`, `replay_detected`, `forbidden_ownership`, `certificate_rejected`;
  - SOC timeline/summary/export/verify.
- Ghi rõ limitation:
  - hash-chain không cover timestamp/metadata;
  - chưa phát hiện tail truncation nếu không có external anchor;
  - audit insert là best-effort, không làm fail request chính.

Tiêu chí hoàn tất:

- Bảng audit testcase không còn trống ở các case P0/P1.
- SOC demo có checklist rõ.
- Báo cáo dùng ngôn ngữ “đạt một phần” cho hash-chain nếu chưa có external anchor.

Kiểm tra:

- API Gateway typecheck nếu sửa route/semantics.
- Go tests nếu sửa verify/hash-chain.
- Runtime: query audit endpoint và export/verify nếu stack đang chạy.

### Giai đoạn 6 - Regression tích hợp Thanh + Thuận

Mục tiêu:

- Đảm bảo các sửa của Thanh không phá audit của Thuận và ngược lại.
- Chốt nhánh ở trạng thái có thể bàn giao cho phần vận hành của Quang.

Việc cần làm:

- Chạy toàn bộ kiểm tra local:
  - frontend build;
  - API Gateway typecheck;
  - CA tests;
  - Banking tests;
  - KDC tests.
- Kiểm tra các luồng logic chính:
  - register thành công;
  - register email trùng;
  - Bank fail sau CA issue được rollback/revoke;
  - AS/TGS không bị rate-limit trong rehearsal;
  - Admin CA auth theo phương án đã chốt;
  - audit timeline theo operation id.
- Cập nhật `regard.md` hoặc report cá nhân nếu trạng thái thay đổi.

Tiêu chí hoàn tất:

- Không có test local đỏ.
- Không còn P0 Thanh chưa ghi hướng xử lý.
- Thuận có pass/fail audit testcase hoặc ghi rõ case nào chưa chạy do phụ thuộc stack Quang.
- Phần cần Quang xử lý được tách thành dependency rõ: compose/env/smoke/DB runtime.

Thứ tự khuyến nghị:

1. Giai đoạn 1 trước, vì register consistency là lỗi dữ liệu thật.
2. Giai đoạn 2 ngay sau đó, vì rate-limit/env ảnh hưởng rehearsal.
3. Giai đoạn 4 song song được nếu không đụng cùng file nhiều với Thanh.
4. Giai đoạn 3 tùy thời gian: làm cert-based đầy đủ hoặc chốt fallback có kiểm soát.
5. Giai đoạn 5 và 6 là phần khóa sổ trước khi phối hợp lại với Quang.

## 10. Kết luận ngắn

Code hiện tại compile/test local tốt, và các phần Admin Bank/Audit đã tiến triển rõ. Nhưng dự án chưa ở trạng thái demo final vì các P0 của Thanh và Quang còn trực tiếp ảnh hưởng tới luồng chạy thật: register rollback, Admin CA cert-based, rate-limit demo, env/smoke route và KDC audit DB.

Điểm nên làm tiếp theo cùng Thanh là chốt và implement register consistency trước, vì đây là lỗi có tác động dữ liệu thật lớn nhất và là nền cho demo đăng ký Gmail/OTP/PKI.

## 11. Ghi chú trạng thái sau khi hoàn thành giai đoạn 0 và 1

Cập nhật sau khi implement giai đoạn 1:

- Giai đoạn 0 đã hoàn thành:
  - đã chốt baseline và phạm vi sửa cho Thanh/Thuận;
  - đã tách rõ phần không đụng tới Quang trong nhánh việc này;
  - đã có danh sách file/phạm vi ưu tiên trong `temp-docs/phase0-thanh-thuan.md`.
- Giai đoạn 1 đã hoàn thành phần register consistency chính:
  - đã thêm Bank RPC/read path `CheckUserEmail(email)`;
  - Gateway đã pre-check email trước khi gọi CA `registerUser`;
  - email trùng trả `409 EMAIL_ALREADY_REGISTERED` và không gọi CA issue;
  - JTI chỉ mark used sau khi CA issue và Bank create user/account đều thành công;
  - nếu CA đã cấp cert nhưng Bank create fail, Gateway gọi revoke best-effort với reason `registration_rollback`;
  - đã có unit test Banking Service cho `CheckUserEmail`;
  - API Gateway typecheck và các Go tests liên quan đã pass.
- Phần chưa implement trong giai đoạn 1:
  - flow cấp lại cert cho tài khoản/cert đã bị revoked mới dừng ở mức đề xuất thiết kế, chưa có route/service/UI/test runtime.

### Ghi chú riêng: cấp lại cert cho tài khoản/cert đã bị revoked

Vấn đề này không nên xử lý bằng cách nới register flow hiện tại, vì register flow đã được sửa để chặn email đã tồn tại và tránh tạo thêm Bank user/account. Hướng xử lý nên tách thành một flow riêng tên tạm là `reissue certificate`.

Đề xuất hướng xử lý:

- Không “un-revoke” cert cũ. Cert đã revoked phải giữ nguyên trạng thái để bảo toàn audit, revocation semantics và bằng chứng bảo mật.
- Cấp cert mới với serial mới, cùng `owner_id`, email và role hợp lệ của tài khoản cũ.
- Người dùng phải gửi CSR/keypair mới; không tái sử dụng private key/cert cũ.
- Trước khi cấp lại phải re-verify identity:
  - tối thiểu bằng OTP email;
  - hoặc bằng phiên đăng nhập hợp lệ nếu cert cũ chưa phải do key compromise;
  - nếu revocation reason là `key_compromise`, `fraud`, hoặc lý do nhạy cảm, nên yêu cầu Admin CA duyệt.
- Bank không tạo user/account mới trong reissue. Gateway/CA chỉ xác nhận Bank user tồn tại và đang usable; nếu Bank user/account bị locked thì từ chối hoặc yêu cầu Admin Bank xử lý trước.
- Metadata/audit nên ghi:
  - `reissued_from_serial`;
  - `reissue_reason`;
  - `owner_id`;
  - `request_id`;
  - actor phê duyệt nếu có Admin CA duyệt.
- KDC/Bank verify theo từng cert cụ thể:
  - cert cũ revoked phải tiếp tục fail AS/TGS/AP;
  - cert mới active thì được dùng bình thường với Bank account cũ.

Tiêu chí khi implement reissue sau này:

- Register email trùng vẫn trả `409 EMAIL_ALREADY_REGISTERED`.
- Reissue không tạo thêm Bank account.
- Cert cũ revoked không dùng được.
- Cert mới dùng được cho AS/TGS và Bank flow.
- Audit thể hiện được chuỗi: cert cũ `revoked` → reissue approved/rejected → cert mới `issued`.

### Hướng tiếp theo

Tiếp tục implement giai đoạn 2 theo định hướng ban đầu:

- sửa rate-limit demo để có thể disable hoặc nới ngưỡng bằng env;
- thêm `Retry-After`/message rõ khi 429;
- sửa Admin CA env fail-closed, không còn default placeholder cho `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN`;
- chưa xử lý reissue cert trong giai đoạn 2, chỉ giữ như backlog/thiết kế riêng để tránh làm loãng phạm vi.

## 12. Ghi chú trạng thái sau khi hoàn thành giai đoạn 2

Cập nhật sau khi implement giai đoạn 2:

- Giai đoạn 2 đã hoàn thành phần rate-limit demo:
  - thêm `RATE_LIMIT_DISABLED` để tắt rate-limit trong rehearsal/demo khi cần;
  - thêm env cấu hình window/max cho AS, TGS, Bank API và OTP;
  - giữ default bằng hành vi cũ: AS/TGS 10 request/300s, Bank 20 request/60s, OTP 3 request/600s;
  - response 429 có header `Retry-After` và message nêu số giây cần chờ.
- Giai đoạn 2 đã hoàn thành phần Admin CA env fail-closed:
  - bỏ default placeholder cho `ADMIN_CA_DEMO_EMAIL`, `ADMIN_CA_DEMO_PASSWORD`, `ADMIN_CA_DEMO_TOKEN`;
  - Admin CA password login trả `503 ADMIN_CA_NOT_CONFIGURED` nếu thiếu email/password demo;
  - Admin CA static token chỉ hoạt động khi `ADMIN_CA_DEMO_TOKEN` được cấu hình rõ, không còn mở bằng placeholder.
- Kiểm tra đã chạy:
  - API Gateway `tsc --noEmit`: pass.
- Phần chưa xử lý trong giai đoạn 2:
  - chưa cập nhật `.env.demo.example`, compose hoặc smoke script vì đây là phần phụ thuộc Quang;
  - chưa implement flow reissue cert cho cert/tài khoản revoked, vẫn giữ như backlog/thiết kế riêng.

Hướng tiếp theo sau giai đoạn 2:

- Có thể chuyển sang giai đoạn 3: Admin CA cert-based với role/cert `ca_admin`;
- hoặc chạy regression nhanh giai đoạn 1 + 2 nếu muốn khóa lại nhánh trước khi làm Admin CA cert-based.
