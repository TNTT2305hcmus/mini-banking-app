# Đánh giá tiến độ và phân công

Cập nhật: 08/07/2026 sau khi merge `origin/quang` vào nhánh `thanh`. File này được scan lại theo code hiện tại, các report đã chuyển vào `temp-docs/`, bộ tài liệu demo mới của Quang, và các lỗi trong `temp-docs/PROBLEM.md`.

## 1. Kết luận nhanh

Dự án đã vượt xa PROCESS cũ ở các phần admin/audit:

- Admin CA đã có UI thật cho certificate list/detail/revoke và tab audit đọc `GET /v1/admin-ca/audit`.
- Admin Bank đã có flow cert-based: provision -> activate cert role `bank_admin` -> AS/TGS/AP -> cookie `bank_admin_session` -> dashboard overview/users/accounts/ledger/audit.
- KDC audit đã có code, proto, migration và route đọc `GET /v1/admin-kdc/audit`. Tuy nhiên Gateway user-flow AS/TGS chưa forward `X-Request-ID` xuống KDC, nên KDC audit có thể thiếu trace id.
- SOC console đã có route/UI: `/admin-soc`, `POST /v1/admin-sec/auth`, `GET /v1/admin/audit/timeline|verify|summary|export`.
- Audit hash-chain đã có cho CA/Bank/KDC, nhưng vẫn có hạn chế trong `PROBLEM.md`: timestamp/metadata không nằm trong hash và xóa dòng cuối cần external anchor/checkpoint mới phát hiện được.
- Quang đã bổ sung bộ demo vận hành: `DEMO_OVERVIEW.md`, `.env.demo.example`, `docker-compose.local.yml`, `docker-compose.demo.yml`, Bank Dockerfile, `db/bank/seed_demo.sql`, `scripts/demo/README.md`, `smoke-test.sh`, `smoke-test.ps1`, và `docs/testcases.md` với 78 testcase.

Trọng tâm còn lại không còn là "viết từ đầu" mà là sửa lệch tích hợp, chạy rehearsal trên stack thật, chốt endpoint/env, và điền pass/fail. Các lỗi mới sau merge Quang cần xử lý ngay: smoke script đang gọi sai route Admin CA (`/v1/admin/certificates` thay vì `/v1/admin-ca/certificates`), `.env.demo.example` dùng `CA_DEMO_*` trong khi Gateway đọc `ADMIN_CA_DEMO_*`, và compose chưa cấp `DATABASE_URL` cho KDC audit nên audit KDC vẫn có thể no-op trong Docker.

## 2. Hiện trạng theo module

| Module | Mức | Đã có trong code | Còn thiếu/rủi ro |
|---|---:|---|---|
| CA Service | 90% | Layered CA, issue/verify/list/detail/revoke, role `customer`/`bank_admin`, CA audit read, RA audit append, verify hash-chain. | Chưa có role `ca_admin`; nhóm đã chốt làm cert-based Admin CA P0. Cần regression DB thật. |
| KDC Service | 85% | AS/TGS, replay Redis, role-scope, `kdc_audit_log`, `ListAuditEvents`, `VerifyAuditChain`, hash-chain. | Audit optional nếu `DATABASE_URL` unset. Gateway AS/TGS chưa truyền metadata `x-request-id`, nên timeline theo request_id còn rời rạc. |
| Banking Service | 90% | User/account/transfer/history, AP auth, audit write/read, admin session, admin overview/users/accounts/transactions/audit, verify hash-chain. | Bank user-flow chưa nhận trace-id HTTP làm fallback; auth fail sớm vẫn thiếu AP `request_id` theo bản chất protocol. |
| API Gateway | 85% | User OTP/register/AS/TGS/Bank routes, Admin CA, Admin Bank, Admin KDC, SOC timeline/summary/export/verify. | Register còn cấp cert trước khi tạo Bank user; rate-limit AS/TGS thấp và đếm cả request thành công; env/smoke mới chưa khớp tên biến và route Admin CA. |
| User Frontend | 75% | Register/login/home/bank flow, IndexedDB private key wrapped by PIN, AS/TGS in RAM. | Cần full E2E; frontend API client sinh `X-Request-ID` mới mỗi HTTP call nên timeline một phiên nghiệp vụ chưa liền mạch. |
| Admin CA UI | 90% | `/admin-ca`, login, certificates table, filters, detail drawer, revoke modal, audit log tab. | Auth chưa cert-based; cần build/regression trên stack thật. |
| Admin Bank UI | 90% | `/admin-bank/activate`, `/admin-bank/login`, `/admin-bank`, dashboard overview/users/accounts/ledger/audit. | Cần chạy flow thật với cookie/session; cần seed data đủ đẹp. |
| Admin SOC UI | 85% | `/admin-soc`, KDC audit, cross-service summary, timeline, verify, export CSV/JSON. | Bank chỉ được gộp vào SOC khi có thêm cookie `bank_admin_session`; cần ghi rõ trong demo. |
| Audit | 85% | CA/Bank/KDC event read, semantic enrichment, summary/export/verify, hash-chain. | Chưa có external anchor; trace-id xuyên service chưa ổn; cần điền pass/fail vào `docs/audit-testcases.md`. |
| Docker/DevOps | 80% | Quang đã thêm compose local/demo full hơn, Bank Postgres + seed, KDC/Bank/Gateway/Frontend wiring, Bank Dockerfile, env template, smoke scripts và README demo. | Cần chạy thật và sửa lệch: KDC audit thiếu `DATABASE_URL`, Admin CA env/route trong smoke chưa khớp code, smoke chưa tự động AS/TGS/transfer/Admin Bank session. |

## 3. API và UI hiện có

### Bộ demo/vận hành mới sau merge Quang

- `DEMO_OVERVIEW.md`: tài liệu tổng hợp một file cho kiến trúc, compose, env, seed, testcase, smoke test và giới hạn demo.
- `mini-banking-app/docker-compose.local.yml`: local demo có Bank Postgres, CA, Redis, KDC, Banking Service, API Gateway và Frontend Vite.
- `mini-banking-app/docker-compose.demo.yml`: production-like demo, chỉ expose API Gateway ra host, các gRPC service chạy trong Docker network.
- `mini-banking-app/.env.demo.example`: template secret/env cho demo.
- `mini-banking-app/db/bank/seed_demo.sql`: seed Alice/Bob/Charlie, accounts, transactions mẫu và audit mẫu.
- `mini-banking-app/scripts/demo/smoke-test.sh` và `.ps1`: kiểm tra Docker, port, Redis, Gateway, OTP optional, Admin CA auth/list/detail, Admin Bank negative.
- `mini-banking-app/docs/testcases.md`: 78 testcase chia User Flow, Admin CA, Admin Bank, Audit Log, Negative Tests.

Lưu ý tích hợp ngay sau scan:

- Smoke script hiện gọi `GET /v1/admin/certificates...`, nhưng route code hiện tại là `GET /v1/admin-ca/certificates...`.
- `.env.demo.example` và smoke dùng `CA_DEMO_EMAIL/CA_DEMO_PASSWORD`, nhưng Gateway đọc `ADMIN_CA_DEMO_EMAIL/ADMIN_CA_DEMO_PASSWORD/ADMIN_CA_DEMO_TOKEN`.
- Compose local/demo chưa set `DATABASE_URL` cho `kdc-service`; theo code KDC, không có biến này thì `kdc_audit_log` thành no-op.
- Smoke test mới là smoke hạ tầng + một phần Admin CA/Admin Bank negative; chưa tự động hóa PKI register đầy đủ, AS/TGS, transfer thật, Admin Bank activate/session/query.

### User flow

- `POST /v1/otp/request`
- `POST /v1/otp/verify`
- `POST /v1/auth/register`
- `POST /v1/auth/as-req`
- `POST /v1/auth/tgs-req`
- `POST /v1/auth/me`
- `POST /v1/bank/transfer`
- `POST /v1/bank/accounts/:id/balance/query`
- `POST /v1/bank/accounts/:id/transactions/query`

### Admin CA

- UI: `/admin-ca`
- Auth: `POST /v1/admin-ca/auth`
- Certificates:
  - `GET /v1/admin-ca/certificates`
  - `GET /v1/admin-ca/certificates/:serial`
  - `POST /v1/admin-ca/certificates/:serial/revoke`
- Audit:
  - `GET /v1/admin-ca/audit?action&serial&performed_by&request_id&from&to&limit&offset`

### Admin Bank

- UI:
  - `/admin-bank/activate`
  - `/admin-bank/login`
  - `/admin-bank`
- API:
  - `POST /v1/admin/bank/activate`
  - `POST /v1/admin/bank/session`
  - `POST /v1/admin/bank/overview/query`
  - `POST /v1/admin/bank/users/query`
  - `POST /v1/admin/bank/users/:userId/accounts/query`
  - `POST /v1/admin/bank/transactions/query`
  - `POST /v1/admin/bank/audit/query`

### Security Operations

- UI: `/admin-soc`
- Auth: `POST /v1/admin-sec/auth`
- KDC audit: `GET /v1/admin-kdc/audit?action&client_id&cert_serial&request_id&from&to&limit&offset`
- Cross-service:
  - `GET /v1/admin/audit/timeline?request_id=...`
  - `GET /v1/admin/audit/verify`
  - `GET /v1/admin/audit/summary?window=24h`
  - `GET /v1/admin/audit/export?source=all|ca|kdc|bank&format=csv|json&from&to`

Ghi chú quan trọng: SOC luôn đọc CA + KDC bằng credential `security-admin`; Bank chỉ được gộp vào timeline/verify/summary/export khi request có thêm cookie `bank_admin_session`.

## 4. Lỗi/tồn đọng từ PROBLEM.md

| # | Vấn đề | Trạng thái sau scan | Ưu tiên | Owner chính |
|---|---|---|---|---|
| 1 | Cấp cert trước khi check email đã đăng ký tạo cert rác | Còn tồn tại. `handleRegister` vẫn set jti used, gọi CA `registerUser`, rồi mới gọi Bank `createUserBankAccount`; chưa có `CheckUserEmail`, chưa có compensation revoke. | P0 | Thanh |
| 2 | Cert có nhưng Bank user chưa có, hoặc Bank user có nhưng user không nhận cert | Còn tồn tại như hệ quả #1. Chưa có script đối soát CA/Bank hoặc rollback/retry idempotent. | P0 | Thanh |
| 3 | Lưu trữ AS/TGS | Thiết kế ổn: frontend giữ AS/TGS key trong RAM, private key wrapped bằng PIN trong IndexedDB. Cần ghi invariant và chấp nhận refresh mất session. | P2 | Quang |
| 4 | Chuyển tiền bị hiểu nhầm do "tối đa 50m" | Core Bank không có cap số dư; rủi ro là UI/seed/status failed. Admin Bank đã hiển thị `pending/completed/failed`, nhưng user flow/Home vẫn cần kiểm tra refetch balance và hiển thị lỗi daily limit. Kịch bản final chốt tài khoản mới có 10,000,000 VND, chuyển các khoản nhỏ dưới daily limit. | P1 | Thái |
| 5 | Login đúng vẫn bị rate-limit | Còn tồn tại. AS/TGS rate-limit là 10/5 phút và đếm cả success; Bank 20/phút; chưa có env disable hoặc chỉ đếm fail. | P0 | Thanh |
| 6 | Admin route/cert admin | Bank Admin đạt hướng cert-based. Admin CA vẫn password/JWT/static token, chưa có `ca_admin` cert role. Nhóm đã chốt làm cert-based Admin CA cho demo cuối. | P0 | Thanh |
| 7 | Audit enterprise thiếu KDC | Đã có code KDC audit + SOC. Còn rủi ro: compose local/demo chưa set `DATABASE_URL` cho KDC, nên chạy Docker có thể không ghi `kdc_audit_log`; Gateway chưa forward trace id ở AS/TGS; cần chạy migration/DB thật. | P0/P1 | Quang |
| 8 | Hash-chain không bắt timestamp/metadata hoặc xóa dòng cuối | Còn đúng. CA/Bank/KDC hash-chain chỉ cover field định danh ổn định, không cover timestamp/metadata và không có external anchor. | P2 | Thuận |
| 9 | Bank audit không gộp SOC nếu thiếu cookie bank | Đã chốt là isolation cố ý: SOC chỉ gộp Bank khi có thêm `bank_admin_session`; không thêm trusted read path cho security-admin trong demo. | P1 | Thái |
| 10 | Bank auth-layer fail sớm không có request_id | Còn đúng một phần. Nếu ticket/authenticator chưa giải mã được thì AP `request_id` không tồn tại; Gateway/Bank chưa có trace-id HTTP fallback. | P1 | Thuận |
| 11 | Timeline chỉ hiện 1 sự kiện | Còn rủi ro. Frontend sinh `X-Request-ID` mới mỗi call; Gateway AS/TGS/Bank user calls chưa forward metadata. | P0/P1 | Thuận |
| 12 | Lệch tài liệu/smoke/env sau merge Quang | Mới phát hiện. Smoke script gọi `/v1/admin/certificates`, code mount `/v1/admin-ca/certificates`; env demo dùng `CA_DEMO_*`, code đọc `ADMIN_CA_DEMO_*`; smoke chưa cover AS/TGS/transfer/Admin Bank session. | P0 | Quang |

## 5. Những điểm thống nhất

| # | Điểm | Quyết định đã chốt | Ghi chú thực hiện | Owner | Hạn |
|---|---|---|---|---|---|
| 1 | Admin CA | Làm cert-based Admin CA cho demo cuối. | Thanh thêm role/cert `ca_admin`, activation/session và UI/login tương ứng; password/JWT demo chỉ là fallback cứu demo. | Thanh | 08/07 |
| 2 | Trace-id | Dùng một `operation_id` cho cả flow nghiệp vụ register/login/transfer. | Quang chốt convention; Thuận/Gateway forward `operation_id` xuống KDC/Bank để timeline có CA -> KDC -> Bank chung trace. | Quang | 08/07 |
| 3 | SOC đọc Bank audit | Giữ cookie-gated Bank audit. | SOC chỉ gộp Bank khi operator có thêm `bank_admin_session`; báo cáo/demo ghi rõ đây là thiết kế có chủ đích. | Thái | 09/07 |
| 4 | Register rollback | Làm cả pre-check email và compensation revoke. | Thêm `CheckUserEmail` trước khi cấp cert; nếu Bank fail sau khi CA cấp cert thì revoke rollback và xử lý `jti` đúng. | Thanh | 08/07 |
| 5 | Rate-limit demo | Nâng ngưỡng qua env và thêm `RATE_LIMIT_DISABLED=1` cho demo. | Ưu tiên không để login đúng bị 429 trong rehearsal/demo; hướng chỉ đếm fail để sau nếu còn thời gian. | Thanh | 08/07 |
| 6 | Audit hash-chain | Chỉ ghi limitation, chưa làm external anchor. | Thuận ghi rõ hash-chain chưa phát hiện sửa timestamp/metadata hoặc xóa tail nếu không có external anchor. | Thuận | 09/07 |
| 7 | Compose demo chính | Dùng `docker-compose.local.yml` làm đường rehearsal/demo chính. | File này có frontend dev và dễ debug; `docker-compose.demo.yml` giữ làm phương án deploy/backup. | Quang | 08/07 |
| 8 | KDC audit DB | Dùng chung Postgres demo với DB/schema riêng cho KDC audit. | Quang cấu hình `DATABASE_URL` cho `kdc-service`, không để KDC audit no-op trong Docker. | Quang | 08/07 |
| 9 | Route Admin CA | Giữ route chính thức `/v1/admin-ca/*`. | Quang sửa smoke/docs/testcase theo code hiện tại, không đổi route Gateway phút cuối. | Quang | 08/07 |
| 10 | Env Admin CA demo | Giữ tên biến `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN`. | Quang sửa `.env.demo.example`, compose và smoke; Thanh bỏ default credential/token đoán được trong Gateway. | Quang | 08/07 |

## 6. Phân công chi tiết

Lưu ý chung cho tất cả thành viên: sau khi hoàn thành hoặc cập nhật nhiệm vụ, tạo mới hoặc cập nhật file report cá nhân trong `temp-docs/` theo dạng `name-report.md` (ví dụ `thanh-report.md`, `thai-report.md`, `thuan-report.md`, `quang-report.md`). Report cần ghi rõ việc đã làm, file/code đã sửa, cách test, kết quả pass/fail, blocker còn lại và ảnh hưởng tới demo.

### Thanh - Admin CA, đăng ký PKI, auth CA

Mục tiêu: Admin CA chạy ổn bằng cert-based admin, register không tạo dữ liệu lệch CA/Bank, và auth CA không còn secret mặc định đoán được.

Việc P0:

- Làm cert-based Admin CA theo chốt mục 5.1:
  - thêm role/cert type `ca_admin` hoặc cơ chế tương đương trong CA metadata.
  - tạo activation/session cho Admin CA theo mẫu Bank Admin nếu tái dùng được.
  - cập nhật UI/login Admin CA để dùng cert admin thay cho password/JWT là đường chính.
  - giữ password/JWT demo chỉ làm fallback cứu demo, không trình bày là cơ chế chính.
- Sửa register flow trong `api-gateway/src/controller/ca.controller.ts`:
  - Không set jti `"1"` trước khi chắc chắn flow thành công, hoặc rollback jti khi fail.
  - Nếu `createUserBankAccount` fail sau khi CA cấp cert, gọi revoke best-effort với reason `registration_rollback`.
  - Map lỗi email đã tồn tại thành `409 EMAIL_ALREADY_REGISTERED`, không đi qua `caGrpcError`.
- Thêm Bank RPC/read path `CheckUserEmail(email)` và gọi trước `registerUser`.
- Sửa rate-limit AS/TGS cho demo trong `api-gateway/src/middleware/rateLimiter.ts`:
  - nâng ngưỡng qua env hoặc thêm `RATE_LIMIT_DISABLED=1` cho rehearsal.
  - nếu vẫn trả 429 thì thêm `Retry-After`/message rõ thời gian chờ.
- Sửa `api-gateway/src/config/env.ts` cho Admin CA demo credential:
  - Không default `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN` thành chuỗi `"X is required"`.
  - Fail closed như `ADMIN_SEC_DEMO_*`, hoặc bắt buộc cấu hình rõ trong `.env.demo.example`.
  - Giữ tên `ADMIN_CA_DEMO_*` theo chốt mục 5.10; Quang sẽ đồng bộ `.env.demo.example`, compose và smoke.
  - Cấp/ghi rõ `ADMIN_CA_DEMO_TOKEN`; hiện `.env.demo.example` chưa có token này trong khi middleware có đường Bearer static token.
- Bổ sung vào báo cáo/demo các điểm CA/PKI còn thiếu:
  - Admin CA cert-based đã chốt làm; nếu có blocker kỹ thuật phải ghi rõ phần nào đã đạt/chưa đạt.
  - Key rotation hiện mới ở mức thủ công/dev script, chưa có quy trình thay khóa định kỳ tự động.
  - Hash-chain audit chưa có external anchor để phát hiện xóa dòng cuối.
- Viết runbook demo final bằng Gmail thật:
  - Không dùng bypass OTP cho kịch bản nộp bài chính; cấu hình SMTP Gmail thật bằng Gmail App Password trong `.env`.
  - Chuẩn bị tối thiểu 2 email thật của thành viên để tạo tài khoản mới ngay trong buổi demo.
  - Luồng chính: thành viên A đăng ký -> nhận OTP qua Gmail -> nhập OTP -> tạo cert/tài khoản; thành viên B làm tương tự.
  - Mỗi tài khoản tạo mới phải được cấp số dư khởi tạo 10,000,000 VND để hai bên chuyển tiền qua lại dễ kiểm thử.
  - Sau khi A/B đăng ký xong: A đăng nhập, xem số dư, chuyển tiền cho B; B đăng nhập, kiểm tra số dư/lịch sử và chuyển lại một giao dịch nhỏ.
  - Một thành viên đóng vai Admin CA: đăng nhập Admin CA, xem cert mới của A/B, xem audit `issued/looked_up`, revoke một tài khoản phụ đã chuẩn bị trước, chứng minh cert revoked không dùng được.
  - Một thành viên đóng vai Admin Bank: đăng nhập Admin Bank, xem overview/users/accounts/transactions/audit, chỉ ra giao dịch A -> B và B -> A.
  - Chuẩn bị trước 2-3 tài khoản phụ để test revoke, cert expired/revoked, ownership denied hoặc các negative test mà không làm hỏng tài khoản demo chính A/B.
  - Ghi rõ vai diễn trong demo: ai là customer A, customer B, Admin CA, Admin Bank, người điều phối terminal/compose.
  - Có checklist dự phòng nếu Gmail/SMTP chậm: chờ OTP, gửi lại OTP, đổi sang email phụ, hoặc dùng screenshot/video backup; nhưng flow chính vẫn là Gmail thật.
- Regression Admin CA:
  - cert-based login/activation
  - list/filter status/type/issuer/email/serial đúng route `/v1/admin-ca/certificates`
  - detail
  - revoke client active
  - revoke lại trả 409
  - revoke root/intermediate/service_tls trả 422
  - audit tab có `issued`, `looked_up`, `revoked`, `ra_*`, `admin_ca_login_*`.

Deliverable:

- PR/commit sửa register rollback và env Admin CA.
- PR/commit cert-based Admin CA hoặc ghi rõ blocker kỹ thuật nếu không hoàn tất.
- Curl mẫu Admin CA cập nhật, gồm `/v1/admin-ca/audit`.
- Kết quả pass/fail cho Admin CA trong smoke checklist.
- Runbook demo final bằng Gmail thật, có vai diễn từng thành viên và checklist OTP/transfer/admin-ca/admin-bank.
- Cập nhật `temp-docs/thanh-report.md`.

### Thái - Admin Bank API/UI và dữ liệu dashboard

Mục tiêu: Bank Admin cert-based demo được từ đầu đến cuối và UI phản ánh đúng ledger/audit.

Việc P0/P1:

- Chạy lại flow trong `thai-bank-admin-regist.md`:
  - provision bằng `npm.cmd run provision:bank-admin`
  - activate cert
  - login `/admin-bank/login`
  - dashboard `/admin-bank`
- Regression API:
  - overview query
  - users query + empty state
  - user accounts query
  - transactions query với `completed` và `failed`
  - audit query filter action/request_id/date
- Kiểm tra cookie:
  - thiếu cookie -> `ADMIN_SESSION_REQUIRED`
  - cookie sai/hết hạn -> lỗi đúng
  - customer cert xin scope `bank-admin:read` -> bị từ chối.
- Kiểm tra UI user flow/Home với vấn đề 50M:
  - transaction failed phải hiện failed/reason, không như thành công.
  - sau transfer thành công cần refetch balance hoặc hướng dẫn demo logout/login lại.
- SOC/Bank audit theo chốt mục 5.3:
  - giữ cookie-gated Bank audit.
  - đảm bảo demo giải thích rõ SOC chỉ gộp Bank khi có thêm `bank_admin_session`.

Deliverable:

- Checklist Bank Admin pass/fail.
- Postman/curl mẫu cho 5 endpoint `/v1/admin/bank/*/query`.
- Ghi rõ seed data nào cần cho dashboard đẹp.
- Cập nhật `temp-docs/thai-report.md`.

### Thuận - Audit, SOC, trace-id

Mục tiêu: audit chứng minh được bằng API/UI, timeline có ý nghĩa, và các lỗi bảo mật P0 không phá demo.

Việc P0:

- Forward trace id ở Gateway:
  - `requestTgt(grpcReq, requestId)` và `requestServiceTicket(grpcReq, requestId)` phải gửi gRPC metadata `x-request-id`.
  - Bank user calls `transferMoney/getBalance/getHistory` cần nhận `requestId` và gửi metadata hoặc ghi trace id vào audit metadata nếu proto chưa có field.
- Làm rõ frontend trace theo chốt mục 5.2:
  - Quang chốt convention `operation_id`.
  - Thuận đảm bảo Gateway forward `operation_id`/`X-Request-ID` xuống KDC/Bank để audit timeline dùng được.
- Chạy và điền `docs/audit-testcases.md`:
  - CA: `ra_otp_requested`, `ra_otp_verified`, `issued`, `looked_up`, `revoked`.
  - KDC: `as_ticket_issued`, `as_rejected`, `tgs_ticket_issued`, `tgs_rejected`.
  - Bank: `transfer_completed`, `transfer_rejected`, `replay_detected`, `forbidden_ownership`, `certificate_rejected`.
  - SOC: timeline/summary/export/verify.
- Hỗ trợ Thanh kiểm chứng rate-limit sau khi sửa:
  - AS/TGS login đúng không bị 429 trong rehearsal.
  - Nếu còn giữ 429, response có `Retry-After` hoặc thông báo đủ rõ cho demo.

Việc P1/P2:

- Ghi rõ limitation hash-chain: không cover timestamp/metadata, không phát hiện tail truncation nếu không có anchor.
- Nếu kịp, thêm checkpoint anchor tối thiểu cho audit verify hoặc export `last_seq,last_hash` sau rehearsal.

Deliverable:

- `docs/audit-testcases.md` có cột pass/fail/note.
- SOC screenshot/checklist cho KDC audit, timeline, summary, export, verify.
- Ghi rõ trong demo: Bank source cần cookie Bank Admin theo chốt cookie-gated.
- Cập nhật `temp-docs/thuan-report.md`.

### Quang - Full stack, compose, seed, smoke test

Mục tiêu: biến bộ compose/docs/smoke vừa merge thành đường chạy demo thật sự lặp lại được, không phụ thuộc trí nhớ từng người.

Việc P0:

- Chạy thực tế `docker-compose.local.yml` từ clean state:
  - copy `.env.demo.example` -> `.env`
  - provision CA
  - gen certs
  - `docker compose -f docker-compose.local.yml up --build -d`
  - ghi lại service nào fail, env nào thiếu, port nào xung đột.
- Sửa lệch smoke/docs mới phát hiện:
  - đổi `/v1/admin/certificates` thành `/v1/admin-ca/certificates` trong `smoke-test.sh`, `smoke-test.ps1` và docs/testcase liên quan.
  - đổi `CA_DEMO_EMAIL/PASSWORD` thành `ADMIN_CA_DEMO_EMAIL/PASSWORD` trong `.env.demo.example` và smoke.
  - bổ sung `ADMIN_CA_DEMO_TOKEN`, `ADMIN_SEC_DEMO_EMAIL/PASSWORD/TOKEN` vào env template nếu demo SOC/Admin CA cần dùng.
- Bổ sung KDC audit DB trong compose:
  - cấu hình `DATABASE_URL` cho `kdc-service` dùng chung Postgres demo theo chốt mục 5.8.
  - mount migration KDC nếu dùng Postgres local.
  - xác nhận `GET /v1/admin-kdc/audit` có event thật sau AS/TGS.
- Chốt compose dùng cho demo:
  - dùng `docker-compose.local.yml` làm đường rehearsal/demo chính theo chốt mục 5.7.
  - giữ `docker-compose.demo.yml` làm phương án deploy/backup.
- Chốt và ghi convention `operation_id` theo mục 5.2:
  - frontend/smoke dùng lại một `operation_id` cho register/login/transfer demo.
  - tài liệu chỉ rõ khi nào tạo mới operation và khi nào tái dùng.
- Không publish gRPC 50051/50052/50053 trong compose demo nếu không cần; local có thể expose để debug.
- Seed/demo data:
  - 2 customer tạo mới trong demo, mỗi account khởi tạo 10,000,000 VND.
  - 2-3 customer phụ tạo sẵn để revoke/negative test.
  - 1 bank admin.
  - transfer dưới daily limit; nếu cần, seed limit cao hơn để demo không bị chặn nhầm.
  - data audit đủ cho CA/KDC/Bank/SOC.
- Smoke test:
  - Giữ smoke hạ tầng hiện có.
  - Mở rộng tối thiểu: Admin CA auth/list/detail/audit đúng route; Admin Bank negative; SOC login/summary/verify nếu có env security-admin.
  - Nếu đủ thời gian, thêm một script/manual checklist cho PKI register -> AS/TGS -> balance/transfer vì smoke hiện chưa ký RSA/tạo AP tự động.

Deliverable:

- `scripts/demo/README.md` chạy được từ máy sạch.
- Smoke script chạy không fail vì sai route/env.
- Checklist pass/fail/note sau khi chạy compose thật.
- Danh sách email/tài khoản demo chính và tài khoản phụ cho revoke/negative test.
- Bug list cuối ngày gom theo Thanh/Thái/Thuận/Quang.
- Cập nhật `temp-docs/quang-report.md`.

## 7. Checklist hoàn thành demo

P0 bắt buộc:

- Stack chạy được từ hướng dẫn mới.
- `.env.demo.example`, compose và smoke script khớp với code Gateway hiện tại.
- Smoke script không gọi sai `/v1/admin/certificates`; Admin CA dùng `/v1/admin-ca/*`.
- Đăng ký user mới không tạo cert rác khi Bank create user fail.
- AS/TGS không bị rate-limit trong demo bình thường.
- Transfer thành công thấy balance/history đúng.
- Transfer failed/daily-limit không bị UI trình bày như success.
- Admin CA cert-based bằng role/cert `ca_admin` hoặc cơ chế tương đương theo chốt mục 5.1.
- Admin CA xem certificate, detail, revoke, audit.
- Admin Bank activate/login và xem overview/users/accounts/transactions/audit.
- KDC audit có `DATABASE_URL` thật trong đường demo; `GET /v1/admin-kdc/audit` trả event sau AS/TGS.
- SOC xem KDC audit, summary, export, verify; timeline có ít nhất CA+KDC chung trace id trong smoke/manual script.

P1 nên có:

- Bank audit gộp vào SOC timeline khi có cookie Bank Admin.
- Negative tests: sai role admin, thiếu cookie, cert revoked, replay, forbidden ownership.
- `docs/audit-testcases.md` có pass/fail đầy đủ.

P2 nếu còn thời gian:

- External anchor cho audit hash-chain.
- Compose demo production-like sạch cho toàn bộ hệ thống.
- Production hardening CORS/secret/port/internal network.

## 8. Đánh giá dự án theo tiêu chí báo cáo

### 8.1. Thông tin nhóm

| Thành viên | Vai trò chính | Tỷ lệ đóng góp ước tính | Ghi chú |
|---|---|---:|---|
| Thanh | Admin CA, layered CA UI/API, register flow, PKI lifecycle | 25% | Cần hoàn tất cert-based Admin CA, register rollback và rate-limit demo. |
| Thái | Admin Bank cert-based, activation/session, dashboard Bank | 25% | Đã có flow bank admin bằng certificate role `bank_admin`. |
| Thuận | Audit log CA/Bank/KDC, SOC, hash-chain, trace/security review | 25% | Cần hoàn tất regression audit và trace-id xuyên service. |
| Quang | KDC/Bank integration, demo stack, compose, seed, smoke test | 25% | Đã merge bộ compose/docs/smoke; cần verify stack thật và sửa lệch route/env/KDC audit DB. |

Tỷ lệ trên là ước tính theo phạm vi module hiện có trong repo; nhóm có thể chỉnh lại theo commit/thực tế báo cáo.

### 8.2. Mô tả đề tài

Đề tài là mini banking app áp dụng các cơ chế mật mã ứng dụng: PKI/X.509 để định danh người dùng và admin, KDC kiểu Kerberos để cấp TGT/service ticket, session key cho các luồng Bank, chữ ký số và AP authenticator để xác thực yêu cầu, audit log có hash-chain cho CA/KDC/Bank, cùng các giao diện quản trị CA, Bank và SOC.

Kiến trúc chính:

- Frontend giữ private key client trong IndexedDB, wrap bằng PIN; AS/TGS/session key giữ trong RAM.
- API Gateway làm Registration Authority cho OTP/register và điều phối REST -> gRPC.
- CA Service cấp, kiểm tra, thu hồi certificate; lưu audit vòng đời cert.
- KDC Service cấp AS/TGS ticket; ghi audit key issuance nếu có `DATABASE_URL`.
- Banking Service xử lý tài khoản/giao dịch, AP auth, audit tài nguyên và dashboard admin.
- Admin SOC hợp nhất audit CA/KDC và có thể gộp Bank nếu có thêm cookie Bank Admin.

### 8.3. Chức năng đã hoàn thành

- Đăng ký user bằng OTP -> CSR -> CA cấp X.509 client certificate.
- Login Kerberos-like: AS_REQ lấy TGT, TGS_REQ lấy service ticket theo scope.
- Bank flow: profile, balance, history, transfer, replay/idempotency/audit.
- Admin CA: login demo, list/filter/detail/revoke certificate, audit tab.
- Admin Bank: provision activation, activate certificate role `bank_admin`, login bằng AS/TGS/AP, dashboard overview/users/accounts/transactions/audit.
- Admin SOC: security-admin login, KDC audit, timeline theo request_id, verify hash-chain, summary, export CSV/JSON.
- Audit log: CA/Bank/KDC có read API, semantic enrichment và hash-chain verification.
- Layered CA: root/intermediate/service TLS/client cert metadata, guard không revoke non-client cert.
- Demo vận hành: compose local/demo, Bank Dockerfile, env template, seed demo, smoke test Bash/PowerShell và bảng 78 testcase đã có sau merge Quang.

### 8.4. Checklist cơ bản

| Tiêu chí | Trạng thái | Bằng chứng/ghi chú |
|---|---|---|
| Mã hóa dữ liệu bằng symmetric encryption | Đạt | AS/TGS/AP dùng AES-GCM/session key; ticket và payload nhạy cảm được mã hóa đối xứng. |
| Dùng hybrid encryption để phân phối khóa phiên hoặc KDC mức cơ bản | Đạt | AS_REP dùng hybrid encryption: AES payload + RSA-OAEP wrap key; KDC cấp `K_c_tgs` và `K_c_v`. |
| Có key lifecycle: sinh khóa, phân phối, thời hạn, thay khóa | Đạt một phần | Có sinh khóa/session key, phân phối qua AS/TGS, TTL ticket/cert. Thay khóa/rotation còn thủ công, cần Thanh ghi limitation nếu chưa có quy trình. |
| Có xác thực người dùng: identification + verification | Đạt | OTP xác minh email, cert owner_id, chữ ký pre-auth, AS/TGS/AP authenticator. |
| Có chống replay bằng nonce/timestamp/challenge-response | Đạt | AS/TGS dùng nonce/timestamp và Redis replay marker; Bank dùng AP authenticator request_id/nonce và Redis/DB replay fallback. |
| Có xác thực nguồn khóa công khai qua trusted public key/certificate | Đạt | CA là nguồn tin cậy, KDC/Bank verify cert qua CA; gRPC dùng CA trust bundle. |

### 8.5. Checklist mức khá

| Tiêu chí | Trạng thái | Bằng chứng/ghi chú |
|---|---|---|
| Tách rõ master key và session key | Đạt | KDC có `K_tgs`, service key/Bank key và sinh session key `K_c_tgs`, `K_c_v`. |
| Có KDC/KMS hoặc dịch vụ quản lý khóa tập trung | Đạt | KDC Service cấp TGT và service ticket cho nhiều scope/service. |
| Có mutual authentication client-server | Đạt một phần | Client chứng minh private key qua pre-auth/AP; server trả AS_REP/TGS_REP/AP_REP được mã hóa/ký để client kiểm tra. Chưa phải mTLS client cert ở HTTP layer. |
| Có phân quyền truy cập dựa trên identity đã xác thực | Đạt | Scope `balance:read`, `history:read`, `transfer:write`, `bank-admin:read`; Bank Admin yêu cầu role `bank_admin`. |
| Dùng X.509 certificate | Đạt | CA cấp client cert, bank admin cert, service TLS cert. |
| Có revocation: CRL hoặc cơ chế tương đương | Đạt | CA revoke cert, KDC/Bank verify status qua CA; Admin CA revoke client cert. |
| Có cơ chế bảo vệ khỏi MITM khi trao đổi khóa công khai | Đạt | Public key đi trong cert do CA ký; gRPC TLS trust bundle; chain metadata. |

### 8.6. Checklist nâng cao

| Tiêu chí | Trạng thái | Bằng chứng/ghi chú |
|---|---|---|
| PKI tương đối đầy đủ: CA, RA, repository, đăng ký/cấp/thu hồi cert | Đạt | API Gateway làm RA cho OTP/register; CA Service cấp/revoke/list/detail/audit; repository Postgres/JSON. |
| Certificate chain validation | Đạt | Layered CA có root/intermediate/service/client metadata, chain fingerprints; verify cert dùng CA-authoritative metadata. |
| Kerberos-like ticketing hoặc SSO cho nhiều dịch vụ nội bộ | Đạt | AS/TGS cấp TGT và service ticket theo scope/service; Bank dùng AP exchange. |
| Audit log cho cấp khóa, cấp cert, đăng nhập/xác thực/truy cập tài nguyên | Đạt một phần | CA audit cert/RA/admin-ca login; KDC audit AS/TGS; Bank audit AP/transfer. Còn cần regression DB thật, trace-id xuyên service và ghi rõ event không audit chủ đích. |

### 9.7. Tình huống tấn công và cơ chế bảo vệ

| Tình huống tấn công | Cơ chế bảo vệ hiện tại | Lưu ý còn lại |
|---|---|---|
| Replay AS/TGS/AP request | Nonce, timestamp freshness, Redis replay marker; Bank có DB fallback cho used nonce/request. | Rate-limit hiện thấp nhưng không thay replay control. |
| MITM khi trao đổi public key | Public key nằm trong X.509 cert do CA ký; Gateway/service dùng trust bundle TLS. | Cần demo rõ chain/trust bundle. |
| Dùng cert revoked/expired | KDC/Bank kiểm tra cert status qua CA; Admin CA revoke client cert. | Cần testcase revoked cert không lấy TGS/Bank flow được. |
| Giả mạo owner_id hoặc dùng cert của người khác | KDC bind owner_id trong token/cert, Bank kiểm ownership account. | Register rollback còn cần sửa để tránh dữ liệu lệch. |
| Chữ ký payload transfer sai | Bank verify AP/cipher payload/signature và ghi audit `invalid_signature` hoặc reject tương ứng. | Cần payload test để chứng minh trong demo. |
| Chuyển tiền từ account không thuộc user | Bank ownership check và audit `forbidden_ownership`. | Cần negative testcase. |
| Brute-force OTP/login | OTP HMAC, max attempts, cooldown; AS/TGS/IP/cert rate-limit. | AS/TGS rate-limit đang đếm cả success, cần chỉnh cho demo. |
| Truy cập Admin Bank bằng customer cert | TGS/Bank kiểm role `bank_admin` và scope `bank-admin:read`. | Đã có testcase cần chạy lại. |
| Truy cập SOC bằng admin-ca token | Middleware role tách `admin-ca` và `security-admin`. | Admin CA demo token vẫn cần fail-closed env. |
| Sửa audit row ở giữa | Hash-chain CA/Bank/KDC phát hiện sửa field định danh hoặc xóa/sắp xếp giữa chuỗi. | Không phát hiện sửa timestamp/metadata hoặc xóa tail nếu không có external anchor. |
| Đọc Bank audit từ SOC không có quyền Bank | SOC không gộp Bank nếu thiếu `bank_admin_session`. | Đã chốt cookie-gated là thiết kế an toàn cho demo. |
