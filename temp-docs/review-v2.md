# Regard Final - Roadmap chốt dự án

Cập nhật: 10/07/2026.

File này là source of truth từ thời điểm chốt cuối. Khi implement các bước còn lại, follow theo các giai đoạn bên dưới. `temp-docs` chỉ dùng cho ghi chú nội bộ trên nhánh làm việc; tài liệu final để nộp hoặc merge `main` phải nằm trong `README.md` và `demo_test_guide`.

## 1. Hiện trạng ngắn gọn

Code nền đã đủ để dựng demo:

- User: OTP/register, PKI certificate, AS/TGS/AP, profile, balance, history, transfer.
- Admin CA: cert-based `ca_admin`, activate/login bằng PIN/cert, list/detail/revoke/audit.
- Admin Bank: cert-based activation/login/session, dashboard overview/users/accounts/transactions/audit.
- Admin SOC: login, KDC audit, timeline theo `operation_id`, verify hash-chain, summary, export.
- Audit: CA/KDC/Bank đều có audit/hash-chain, testcase đã có code evidence nhưng nhiều case còn runtime pending.

Chưa claim demo final vì phần runtime/vận hành còn phải chốt:

- `.env.demo.example` còn biến cũ hoặc thiếu biến demo cần thiết.
- Compose chưa chắc đã set `DATABASE_URL` cho `kdc-service`, nên KDC audit có thể no-op.
- Compose cần mount Client CA intermediate và set `CLIENT_CA_KEY_PATH`, `CLIENT_CA_CERT_PATH`.
- Smoke/runtime chưa được chạy end-to-end trên stack Docker thật.
- Testcase audit/security cần đổi từ `RUNTIME PENDING` sang `PASS/FAIL` sau rehearsal.

Ưu tiên cao nhất: biến dự án từ "code pass local" thành "demo stack chạy lặp lại được".

## 2. Hiện trạng theo module

| Module | Hiện trạng | Cần chốt final |
|---|---|---|
| Frontend User | Có register OTP/PKI, login cert/PIN, balance, history, transfer, cảnh báo fail/limit. | Rehearsal runtime để chắc UI không lệch dữ liệu sau transfer success/fail. |
| API Gateway | Có route user/Admin CA/Admin Bank/SOC, forward `X-Request-ID`, register consistency. | Chạy qua Gateway thật bằng Docker, không chỉ typecheck. |
| CA Service | Có Root CA, Client CA, issue/verify/revoke, role `customer`, `bank_admin`, `ca_admin`, audit/hash-chain. | Compose mount đúng Client CA intermediate; demo revoke và audit thật. |
| KDC Service | Có AS/TGS, KDC audit, verify hash-chain, nhận trace metadata. | Compose set `DATABASE_URL`; nếu thiếu, SOC timeline sẽ thiếu AS/TGS. |
| Banking Service | Có user/account, balance/history/transfer, Admin Bank dashboard, audit, negative cases nền. | Validate seed DB, transfer success/fail và dashboard trên runtime thật. |
| Admin CA | Đường chính là cert-based `ca_admin`; UI/API list/detail/revoke/audit. | Smoke dùng `/v1/admin-ca/*` và token từ cert-backed session, không dùng route/password cũ. |
| Admin Bank | Có cert-based activate/login/session và dashboard. | Quay flow dashboard sau khi có dữ liệu transfer thật. |
| Admin SOC | Có login, KDC audit, timeline `operation_id`, verify, summary, export. | Phụ thuộc KDC audit DB và testcase runtime; cần demo rõ security evidence. |
| Audit/Testcase | Có testcase nền và limitation hash-chain. | Sau rehearsal cập nhật `PASS/FAIL` kèm bằng chứng curl/UI/export. |
| DevOps/Demo | Có compose/env/seed/smoke/run guide nền. | Rủi ro chính: env/compose/smoke phải đồng bộ trước khi quay/nộp. |

## 3. Ghi chú SOC

SOC là màn hình giám sát bảo mật, không trực tiếp xử lý giao dịch.

SOC hiện gồm:

- Auth security admin qua nhóm route `/v1/admin-sec/*`.
- Frontend Admin SOC dashboard.
- KDC audit list cho event AS/TGS như ticket issued/rejected.
- Timeline theo `operation_id` để nối các event liên quan từ user flow, KDC, Bank và CA.
- Verify hash-chain để phát hiện sửa/xóa/đảo event trong phần được hash bảo vệ.
- Summary/export CSV/JSON làm bằng chứng demo/báo cáo.

Lưu ý khi demo:

- SOC chỉ thuyết phục khi KDC audit ghi DB thật. Nếu `kdc-service` thiếu `DATABASE_URL`, timeline sẽ thiếu phần AS/TGS.
- `operation_id` là header `X-Request-ID` được frontend tái sử dụng xuyên flow; không nhầm với AP `request_id` dùng cho replay/idempotency.
- Hash-chain nên trình bày trung thực: phát hiện sửa/xóa/đảo event ở giữa chuỗi, nhưng chưa chống tail truncation tuyệt đối nếu chưa có external anchor.

## 4. Nguyên tắc tài liệu final

Không dùng `temp-docs` làm tài liệu final. Folder này chỉ chứa report, đánh giá, handoff và plan nháp cho thành viên đọc trên nhánh làm việc.

Tài liệu final nên gom vào:

```text
README.md
demo_test_guide/
  guide/
  tests/
  demo/
```

`README.md` là cửa vào nhanh. Tài liệu dài để trong `demo_test_guide/guide`, testcase để trong `demo_test_guide/tests`, script quay video để trong `demo_test_guide/demo`.

## 5. Roadmap implement

### Phase 1 - Khóa env và compose

Trạng thái: CONFIG DONE / RUNTIME PENDING.

Mục tiêu: stack demo chạy được lặp lại từ máy sạch.

Việc cần làm:

- Sửa `.env.demo.example`:
  - bỏ hoặc ghi deprecated biến cũ `CA_DEMO_EMAIL`, `CA_DEMO_PASSWORD`;
  - thêm `RATE_LIMIT_DISABLED=1` hoặc env limit/window cho rehearsal;
  - thêm `ADMIN_CA_TOKEN` optional nếu smoke cần gọi Admin CA API;
  - thêm `ADMIN_SEC_DEMO_EMAIL`, `ADMIN_SEC_DEMO_PASSWORD`, `ADMIN_SEC_DEMO_TOKEN` nếu smoke SOC tự động;
  - ghi rõ `operation_id` là `X-Request-ID`.
- Sửa compose local/demo:
  - set `DATABASE_URL` cho `kdc-service`;
  - đảm bảo migration KDC audit chạy;
  - mount `./ca-service/certs/intermediate` vào CA container;
  - set `CLIENT_CA_KEY_PATH=/certs/intermediate/client-ca.key`;
  - set `CLIENT_CA_CERT_PATH=/certs/intermediate/client-ca.crt`.
- Chạy stack và xác nhận:
  - containers healthy;
  - Postgres có seed demo;
  - CA Service load Root CA + Client CA;
  - KDC audit ghi DB thật;
  - Gateway gọi được CA/KDC/Bank.

Done khi:

- `docker compose up` chạy được lặp lại.
- Không còn lỗi thiếu Client CA intermediate.
- SOC timeline có dữ liệu KDC audit thật.

Ghi chú sau implement:

- `.env.demo.example` đã bỏ Admin CA password demo cũ, thêm `ADMIN_CA_TOKEN`, SOC demo env và rate-limit env.
- Compose local/demo đã mount Client CA intermediate và set `CLIENT_CA_KEY_PATH`, `CLIENT_CA_CERT_PATH`.
- Compose local/demo đã set `DATABASE_URL` cho `kdc-service` bằng Postgres nội bộ `bank-postgres`.
- `bank-postgres` đã mount thêm KDC audit migrations để tạo `kdc_audit_log`.
- KDC service đã depends_on `bank-postgres: service_healthy` để tránh start sớm rồi tắt audit.
- `docker compose config` cho local/demo đã pass; còn cảnh báo quyền đọc `~/.docker/config.json` trên máy hiện tại, không phải lỗi YAML.
- Chưa chạy `docker compose up` với secret thật, nên containers healthy và SOC timeline runtime vẫn pending.
- Nếu đã từng chạy compose trước đó và volume `bank_postgres_data` đã tồn tại, các migration KDC trong `/docker-entrypoint-initdb.d` sẽ không tự chạy lại. Khi rehearsal cần recreate volume có kiểm soát hoặc apply migration thủ công.

Quyết định cleanup sau Phase 1:

- Runtime final hiện phụ thuộc vào các file chính:
  - `mini-banking-app/.env.demo.example`: template env sạch để copy ra `.env`;
  - `mini-banking-app/.env`: env runtime local, không commit secret thật;
  - `mini-banking-app/docker-compose.local.yml`: stack local đầy đủ có frontend dev server;
  - `mini-banking-app/docker-compose.demo.yml`: stack demo production-like.
- Đã xóa hai file root cũ khỏi hướng final:
  - `mini-banking-app/.env.example`;
  - `mini-banking-app/docker-compose.yml`.
- Phase 6 phải cập nhật root `README.md` vì README hiện còn nhắc `Copy-Item .env.example .env` và liệt kê `docker-compose.yml` là compose setup. README final phải trỏ sang `.env.demo.example`, `docker-compose.local.yml` và `docker-compose.demo.yml`.
- Một số file `.env.example` riêng của từng service vẫn còn giá trị sử dụng cho chạy terminal. Cần rà lại ở Phase 6 để đảm bảo nhất quán với `.env.demo.example` và compose final.

### Phase 2 - Chuẩn hóa smoke test

Trạng thái: SCRIPT DONE / RUNTIME PENDING.

Mục tiêu: smoke script khớp route/env hiện tại và chạy được trên Docker thật.

Việc cần làm:

- Bỏ route Admin CA cũ `/v1/admin/certificates`.
- Dùng route đúng `/v1/admin-ca/*`.
- Smoke các luồng:
  - register user mới;
  - duplicate email trả `409 EMAIL_ALREADY_REGISTERED`;
  - AS/TGS login;
  - balance/history;
  - transfer success;
  - transfer fail hợp lệ: insufficient funds, limit, replay hoặc forbidden ownership;
  - Admin CA list/detail/revoke/audit;
  - Admin Bank dashboard/audit;
  - SOC timeline/verify/summary/export.
- Nếu smoke Admin CA API tự động, dùng `ADMIN_CA_TOKEN` sinh từ cert-backed session.

Done khi:

- Smoke pass trên stack Docker thật.
- Smoke output đủ dùng làm bằng chứng.
- Negative cases có mã lỗi ổn định và giải thích được.

Ghi chú sau implement:

- Cập nhật `mini-banking-app/scripts/demo/smoke-test.ps1`:
  - nhận `ADMIN_SEC_DEMO_TOKEN` hoặc login bằng `ADMIN_SEC_DEMO_EMAIL`/`ADMIN_SEC_DEMO_PASSWORD`;
  - tự động kiểm SOC/KDC audit nếu có security-admin token;
  - gọi `/v1/admin-kdc/audit?limit=5`;
  - gọi `/v1/admin/audit/verify`;
  - gọi `/v1/admin/audit/summary?window=24h`;
  - gọi `/v1/admin/audit/export?source=all&format=json`;
  - gọi `/v1/admin/audit/timeline?request_id=<uuid>`;
  - negative test SOC endpoint không token phải trả `401/403`;
  - ghi skip rõ cho duplicate register `409 EMAIL_ALREADY_REGISTERED` vì cần OTP/CSR hoặc browser flow.
- Cập nhật `mini-banking-app/scripts/demo/smoke-test.sh` tương tự bản PowerShell.
- Nhiệm vụ của hai file smoke:
  - `smoke-test.ps1`: smoke chính cho môi trường Windows/PowerShell của nhóm, dùng khi chạy demo local trên máy Windows;
  - `smoke-test.sh`: smoke tương đương cho Linux/macOS/Git Bash/CI, giữ cùng route và cùng ý nghĩa pass/fail với bản PowerShell.
- Cả hai smoke script chỉ kiểm tự động các bề mặt có thể test bằng HTTP/token/session env. Các luồng cần private key trong browser hoặc CSR thật như full register, AS/TGS/AP signed request, Admin Bank cert session đầy đủ sẽ được kiểm ở functional rehearsal và testcase riêng.
- Sửa lỗi Bash smoke tiềm ẩn: `set -e` + `((PASS_COUNT++))` có thể làm script thoát ở PASS đầu tiên; đã đổi sang `((PASS_COUNT+=1))`, tương tự fail/skip.
- Đã rà không còn route Admin CA cũ `/v1/admin/certificates` trong smoke scripts/API route scan.
- Kiểm tra cú pháp PowerShell: pass bằng `[scriptblock]::Create(...)`.
- Chưa kiểm tra được `bash -n` vì máy hiện tại trỏ `bash` sang WSL nhưng chưa cài distro; cần chạy lại trên Git Bash/Linux/WSL khi có môi trường.
- Chưa chạy smoke runtime vì chưa bật stack bằng secret thật; cần chạy lại sau Phase 1 runtime.

### Phase 3 - Viết guide final

Trạng thái: DOC DONE / RUNTIME COMMANDS PENDING.

Mục tiêu: người khác có thể đọc guide và chạy lại dự án.

Tạo/cập nhật trong `demo_test_guide/guide`:

- `RUN_GUIDE.md`: hướng dẫn tổng hợp.
- `ENV_GUIDE.md`: biến môi trường theo service.
- `COMPOSE_GUIDE.md`: chạy Docker Compose local/demo, migration, seed, health check.
- `TERMINAL_GUIDE.md`: chạy từng service bằng terminal.
- `SEED_AND_ACCOUNTS.md`: dữ liệu seed, tài khoản demo, role, cert/token cần chuẩn bị.
- `TROUBLESHOOTING.md`: lỗi thường gặp và cách xử lý.

Done khi:

- Guide không còn phụ thuộc `temp-docs`.
- Các lệnh trong guide khớp compose/env thật.
- Có mục xử lý lỗi KDC audit, Client CA intermediate, rate-limit, IndexedDB/cert cũ.

Ghi chú sau implement:

- Đã thay `demo_test_guide/guide/RUN_GUIDE.md` bằng guide tổng hợp mới:
  - dùng `.env.demo.example` để copy ra `.env`;
  - dùng `docker-compose.local.yml` và `docker-compose.demo.yml`;
  - liệt kê route demo chính và smoke test;
  - trỏ sang các guide chi tiết.
- Đã tạo `demo_test_guide/guide/ENV_GUIDE.md`:
  - mô tả env theo CA/KDC/Bank/API Gateway/Frontend;
  - ghi rõ KDC `DATABASE_URL` là điều kiện để SOC thấy AS/TGS audit;
  - ghi rõ Admin CA dùng cert-backed session, không dùng password/static-token cũ;
  - ghi Phase 6 cần rà `.env.example` riêng của từng service.
- Đã tạo `demo_test_guide/guide/COMPOSE_GUIDE.md`:
  - hướng dẫn compose local/demo;
  - ghi cách xử lý volume cũ không chạy lại KDC migration;
  - ghi Client CA intermediate mount/path.
- Đã tạo `demo_test_guide/guide/TERMINAL_GUIDE.md`:
  - hướng dẫn chạy Redis/Postgres bằng Docker;
  - apply migration Bank/KDC/CA;
  - chạy CA/KDC/Bank/Gateway/Frontend bằng terminal riêng.
- Đã tạo `demo_test_guide/guide/SEED_AND_ACCOUNTS.md`:
  - mô tả seed demo, KDC audit schema, Admin CA, SOC, Admin Bank và browser IndexedDB.
- Đã tạo `demo_test_guide/guide/TROUBLESHOOTING.md`:
  - lỗi Docker config warning;
  - CA thiếu Client CA;
  - SOC không thấy KDC audit;
  - 429 rate-limit;
  - OTP/SMTP;
  - Admin CA token;
  - SOC login;
  - IndexedDB cert cũ;
  - Postgres seed/migration không chạy lại;
  - Bash smoke trên Windows.
- Đã rà guide final: không còn hướng dẫn dùng root `.env.example` hoặc root `docker-compose.yml`; chỉ nhắc hai file đó là hướng cũ đã loại.
- Chưa chạy toàn bộ command trong guide trên stack thật vì còn phụ thuộc secret/runtime.

### Phase 4 - Viết testcase final và cập nhật kết quả runtime

Trạng thái: DOC DONE / RUNTIME RESULTS PENDING.

Mục tiêu: testcase rõ input, bước chạy, expected result, actual result và pass/fail.

Tạo/cập nhật trong `demo_test_guide/tests`:

- `testcases.md`: index/checklist tổng.
- `audit-testcases.md`: audit CA/KDC/Bank/SOC, hash-chain, timeline, export.
- `functional-testcases.md`: register, login, balance, history, transfer, Admin CA, Admin Bank, SOC.
- `security-testcases.md`: cert auth, revoked cert, replay, forbidden ownership, rate-limit, duplicate registration, rollback, hash-chain verify.
- `smoke-testcases.md`: bộ testcase ngắn trước khi quay.
- `runtime-results.md`: kết quả rehearsal cuối, môi trường, commit/branch, pass/fail, bằng chứng curl/UI/export.

Done khi:

- Không ghi runtime `PASS` nếu chưa chạy stack thật.
- Case còn chờ chạy ghi `RUNTIME PENDING`.
- Sau rehearsal, các case quan trọng đã có `PASS/FAIL` và bằng chứng.

Ghi chú sau implement:

- Đã refactor `demo_test_guide/tests/testcases.md` thành file index:
  - quy ước trạng thái `PASS`, `FAIL`, `SKIP`, `RUNTIME PENDING`, `CODE PASS / RUNTIME PENDING`, `PENDING_MANUAL_DB`;
  - thứ tự chạy đề xuất;
  - checklist nhóm testcase;
  - danh sách bằng chứng cần lưu.
- Đã tạo `demo_test_guide/tests/functional-testcases.md`:
  - customer OTP/register/login/profile/balance/history;
  - transfer success/fail;
  - Admin CA activate/login/list/detail/revoke;
  - Admin Bank activate/login/dashboard/audit;
  - Admin SOC login/KDC audit/timeline/verify/summary/export.
- Đã tạo `demo_test_guide/tests/security-testcases.md`:
  - PKI/certificate role;
  - private key trong browser;
  - AS/TGS/AP;
  - replay/idempotency;
  - Bank forbidden/invalid/insufficient/wrong-scope;
  - registration consistency/rollback;
  - SOC/hash-chain/export/rate-limit.
- Đã tạo `demo_test_guide/tests/smoke-testcases.md`:
  - bám theo `smoke-test.ps1` và `smoke-test.sh`;
  - ghi rõ các flow không thuộc smoke tự động vì cần browser/private key/CSR thật.
- Đã tạo `demo_test_guide/tests/runtime-results.md`:
  - metadata rehearsal;
  - summary pass/fail;
  - vị trí lưu evidence;
  - issue table.
- Đã giữ `demo_test_guide/tests/audit-testcases.md` làm tài liệu audit chi tiết và cập nhật trạng thái sang Phase 4.
- Tất cả testcase mới đang để `RUNTIME PENDING` hoặc `PENDING_MANUAL_DB`; chưa claim runtime `PASS`.

### Phase 5 - Viết script demo

Trạng thái: DOC DONE / REHEARSAL PENDING.

Mục tiêu: có script quay video rõ functional và non-functional/security.

Tạo/cập nhật trong `demo_test_guide/demo`:

- `00-demo-prep.md`: checklist trước khi quay.
- `01-functional-demo-script.md`: customer register/login, balance/history, transfer, Admin Bank, Admin CA, SOC.
- `02-non-functional-security-demo-script.md`: PKI, private key wrapped by PIN, cert-based admin, AS/TGS/AP, replay/revoked/forbidden, audit hash-chain, limitations.

Functional demo cần chứng minh hệ thống dùng được:

1. Setup stack/frontend.
2. Customer registration.
3. Customer login.
4. Balance/history/transfer success.
5. Transfer fail.
6. Admin Bank dashboard.
7. Admin CA certificate management.
8. SOC timeline/export.

Non-functional/security demo cần chứng minh phần Applied Cryptography:

1. PKI hierarchy: Root CA, Client CA, gRPC Transport CA, identity cert.
2. Private key không rời browser, wrapped bằng PIN.
3. Cert-based admin login.
4. AS/TGS/AP và service ticket.
5. Replay/idempotency và negative cases.
6. Audit timeline theo `operation_id`.
7. Hash-chain verify và limitation.

Done khi:

- Script khớp UI/route hiện tại.
- Có checklist chuẩn bị để tránh thiếu seed/token/cert lúc quay.
- Limitation được nói trung thực, không overclaim.

Ghi chú sau implement:

- Đã tạo `demo_test_guide/demo/00-demo-prep.md`:
  - checklist env/runtime;
  - kiểm tra cert/key;
  - kiểm tra compose stack;
  - smoke checklist;
  - browser/IndexedDB checklist;
  - data/token cần chuẩn bị;
  - backup plan cho OTP, cert, SOC audit, rate-limit.
- Đã tạo `demo_test_guide/demo/01-functional-demo-script.md`:
  - customer registration;
  - customer login;
  - balance/history;
  - transfer success;
  - transfer fail;
  - Admin Bank dashboard;
  - Admin CA certificate management;
  - Admin SOC dashboard/timeline/export.
- Đã tạo `demo_test_guide/demo/02-non-functional-security-demo-script.md`:
  - PKI hierarchy;
  - private key protection;
  - cert-based admin access;
  - Kerberos-like AS/TGS/AP;
  - replay/idempotency;
  - registration consistency/rollback;
  - revoked cert rejected;
  - SOC timeline theo `operation_id`;
  - hash-chain verify;
  - summary/export;
  - rate-limit/demo mode;
  - limitations.
- Script demo có lời dẫn ngắn để quay video, thao tác cần làm, expected result và evidence cần lưu.
- Chưa rehearsal quay thật, nên vẫn cần chạy qua script một lần sau khi stack/runtime sẵn sàng.

### Phase 6 - Chốt README và worktree sạch

Trạng thái: DOC/CLEANUP DONE / FINAL RUNTIME SMOKE PENDING.

Mục tiêu: root `README.md` là quick start sạch, còn chi tiết nằm trong `demo_test_guide`.

Nhiệm vụ bắt buộc sau khi đã xóa root `.env.example` và `docker-compose.yml`:

- Cập nhật mọi reference trong README và docs final:
  - đổi `Copy-Item .env.example .env` hoặc `cp .env.example .env` thành copy từ `.env.demo.example`;
  - bỏ nhắc tới `docker-compose.yml` root;
  - hướng dẫn rõ hai file compose final là `docker-compose.local.yml` và `docker-compose.demo.yml`.
- Kiểm tra các script/docs còn reference root `.env.example` hoặc root `docker-compose.yml`; nếu chỉ là tài liệu nháp trong `temp-docs` thì không cần chốt vào main, nhưng README/final guide phải sạch.
- Rà lại các `.env.example` riêng của module để đảm bảo không lệch cấu hình hiện tại:
  - `mini-banking-app/api-gateway/.env.example`;
  - `mini-banking-app/ca-service/.env.example`;
  - `mini-banking-app/kdc-service/.env.example`;
  - `mini-banking-app/banking-service/.env.example`.
- Các `.env.example` riêng phải phản ánh đúng:
  - CA dùng Root CA + Client CA intermediate;
  - KDC có optional `DATABASE_URL` cho audit khi chạy terminal;
  - API Gateway có `RATE_LIMIT_*`, `ADMIN_SEC_DEMO_*`, không còn Admin CA password/static-token cũ;
  - Banking Service DB/Redis/CA TLS path khớp guide terminal;
  - không còn placeholder hoặc biến đã bị code bỏ.

README nên có:

- Giới thiệu ngắn: Mini Banking App, PKI, Kerberos-like AS/TGS/AP, audit/SOC.
- Quick start bằng Docker Compose.
- Quick start bằng terminal, link sang `TERMINAL_GUIDE.md`.
- Demo accounts và vai trò: customer, Bank Admin, CA Admin, SOC/Security Admin.
- Smoke test nhanh và expected output ngắn.
- Link tài liệu chi tiết:
  - `demo_test_guide/guide/RUN_GUIDE.md`;
  - `demo_test_guide/guide/ENV_GUIDE.md`;
  - `demo_test_guide/guide/COMPOSE_GUIDE.md`;
  - `demo_test_guide/tests/testcases.md`;
  - `demo_test_guide/tests/audit-testcases.md`;
  - `demo_test_guide/demo/01-functional-demo-script.md`;
  - `demo_test_guide/demo/02-non-functional-security-demo-script.md`.
- Known limitations:
  - hash-chain chưa có external anchor;
  - chưa chống tail truncation tuyệt đối;
  - audit insert best-effort;
  - demo mode có thể disable rate-limit.

Worktree clean checklist:

- Cleanup `temp-docs` trước khi chốt:
  - chỉ giữ `temp-docs/regard.md`;
  - chỉ giữ `temp-docs/regard-final.md`;
  - xóa các file report nháp, handoff cá nhân, plan rác, demo overview cũ và process nháp trong `temp-docs`.
- Không merge toàn bộ `temp-docs` vào `main`; nếu cần giữ hai file regard trên nhánh làm việc thì không dùng chúng làm tài liệu final.
- Tài liệu final chỉ giữ trong `README.md` và `demo_test_guide`.
- Chạy `git status --short`.
- Chạy smoke test cuối.
- Kiểm tra link trong README trỏ đúng file final.

Done khi:

- README là cửa vào nhanh và không chứa chi tiết nháp.
- `demo_test_guide` đủ guide/test/demo.
- `temp-docs` chỉ còn `regard.md` và `regard-final.md`.
- Worktree sạch theo phạm vi final, không còn file nháp lẫn vào tài liệu nộp.

Ghi chú sau implement:

- Đã rewrite root `README.md` thành quick start sạch:
  - giới thiệu dự án ngắn gọn;
  - hướng dẫn Docker Compose local/demo;
  - smoke test;
  - demo routes;
  - link tới guide/test/demo final trong `demo_test_guide`;
  - known limitations;
  - ghi rõ root `.env.example` và root `docker-compose.yml` không còn thuộc final run path.
- Đã rà và cập nhật `.env.example` riêng của từng module:
  - `mini-banking-app/api-gateway/.env.example`: thêm `RATE_LIMIT_*`, `ADMIN_SEC_DEMO_*`, bỏ hướng Admin CA password/static-token cũ;
  - `mini-banking-app/ca-service/.env.example`: thêm `CLIENT_CA_KEY_PATH`, `CLIENT_CA_CERT_PATH`, mô tả Root CA + Client CA;
  - `mini-banking-app/kdc-service/.env.example`: thêm optional `DATABASE_URL` cho KDC audit và KDC server TLS paths;
  - `mini-banking-app/banking-service/.env.example`: đồng bộ DB/Redis/CA TLS/Bank TLS paths với terminal guide.
- Đã cleanup `temp-docs`:
  - giữ `temp-docs/regard.md`;
  - giữ `temp-docs/regard-final.md`;
  - xóa các file nháp/report/handoff còn lại trong `temp-docs`.
- Đã rà README/final docs:
  - không còn hướng dẫn copy root `.env.example`;
  - không còn hướng dẫn chạy root `docker-compose.yml`;
  - các nơi nhắc file cũ chỉ là ghi chú không dùng nữa.
- Chưa chạy smoke runtime cuối vì còn phụ thuộc stack/secret thật.

### Phase 7 - Tạo quality assessment final

Trạng thái: DOC DONE / RUNTIME EVIDENCE PENDING.

Mục tiêu: đánh giá chất lượng code và tài liệu hóa vào `quality-assessment-final.md`. File này dùng để chốt chất lượng kỹ thuật trước khi viết `report-final.md` ở Phase 8. Sẽ còn cập nhật phần đánh giá test case sau khi có kết quả runtime/rehearsal cuối.

Quy trình bắt buộc:

1. Trước tiên đề xuất các mục sẽ có trong `quality-assessment-final.md` để người chốt duyệt.
2. Sau khi được duyệt, mới tiến hành đọc code, đánh giá và viết nội dung.
3. Đánh giá phải tách rõ phần đã xác minh bằng code/static review, phần đã xác minh bằng runtime test, và phần còn pending.
4. Không overclaim các phần chưa chạy runtime hoặc chưa có bằng chứng test.

Đề xuất mục ban đầu cho `quality-assessment-final.md`:

- Tóm tắt chất lượng tổng thể:
  - mức độ hoàn thiện hiện tại;
  - điểm mạnh chính;
  - rủi ro còn lại;
  - kết luận có đủ điều kiện chốt demo/report hay chưa.
- Phạm vi đánh giá:
  - module được đánh giá: Frontend, API Gateway, CA Service, KDC Service, Banking Service, Admin CA, Admin Bank, Admin SOC, scripts, compose/env, docs;
  - loại đánh giá: static code review, config review, docs review, smoke/runtime pending hoặc pass/fail.
- Kiến trúc và phân tách trách nhiệm:
  - ranh giới giữa Gateway, CA, KDC, Bank, Frontend;
  - mức độ rõ ràng của route/service ownership;
  - trace/request id và liên kết audit.
- Chất lượng triển khai cryptography:
  - PKI Root CA, Client CA, identity cert;
  - CSR/certificate lifecycle;
  - AS/TGS/AP, service ticket, scope;
  - private key handling trong browser;
  - điểm cần nói trung thực về demo/security limitation.
- Authentication và authorization:
  - customer auth;
  - Admin CA cert-based session;
  - Admin Bank cert-based session;
  - SOC/security admin;
  - negative cases như revoked cert, forbidden ownership, replay/idempotency.
- Audit, SOC và tamper-evidence:
  - CA/KDC/Bank audit;
  - timeline theo `operation_id`;
  - hash-chain verify;
  - limitation như tail truncation và external anchor.
- Data consistency và transaction safety:
  - registration rollback;
  - transfer validation;
  - idempotency/replay;
  - seed/migration/DB dependency.
- API quality và error handling:
  - HTTP status;
  - error code ổn định;
  - validation input;
  - response shape cho frontend/demo.
- Frontend quality:
  - user flow;
  - admin dashboards;
  - state handling;
  - IndexedDB/cert/key handling;
  - UX khi lỗi OTP/cert/session/rate-limit.
- DevOps/config quality:
  - `.env.demo.example`;
  - `.env.example` riêng từng service;
  - `docker-compose.local.yml`;
  - `docker-compose.demo.yml`;
  - healthcheck, migration, seed, cert mount.
- Testability và test coverage:
  - smoke scripts;
  - functional testcase;
  - security testcase;
  - audit testcase;
  - runtime result còn pending/pass/fail.
- Documentation quality:
  - README quick start;
  - guide/env/compose/terminal/troubleshooting;
  - testcase/demo script;
  - mức độ khớp giữa docs và code.
- Findings theo mức độ:
  - Critical;
  - High;
  - Medium;
  - Low;
  - Documentation/cleanup.
- Checklist cần hoàn tất trước khi chốt:
  - runtime smoke;
  - rehearsal demo;
  - cập nhật `runtime-results.md`;
  - xác nhận env/compose trên máy sạch;
  - rà link README/docs.

Done khi:

- Cấu trúc `quality-assessment-final.md` được duyệt.
- Có đánh giá code theo module và theo mức độ rủi ro.
- Có danh sách finding/action item rõ ràng, không lẫn với report final.
- Các phần chưa runtime phải ghi rõ `RUNTIME PENDING`.

Ghi chú sau implement:

- Đã tạo `temp-docs/quality-assessment-final.md`.
- File đã đánh giá theo các nhóm: tổng quan chất lượng, phạm vi đánh giá, kiến trúc, cryptography, auth/authz, audit/SOC/hash-chain, data consistency, API/error handling, frontend, DevOps/config, testability, documentation và findings theo severity.
- Findings hiện không ghi nhận Critical từ static/docs review.
- High findings chính:
  - chưa có final runtime smoke trên stack thật;
  - SOC timeline phụ thuộc KDC audit DB/migration/env.
- Các phần chưa có bằng chứng chạy thật đều được ghi `RUNTIME PENDING`, không claim `PASS`.
- Phase 8 `report-final.md` nên dùng file này cộng với `runtime-results.md` sau rehearsal để viết phần đánh giá cuối.

### Phase 8 - Tạo report final

Mục tiêu: tạo `report-final.md` sau khi có quality assessment và đánh giá test case. Phần này hiện tại chưa cần làm.

Quy trình bắt buộc:

1. Trước tiên đề xuất các mục sẽ có trong `report-final.md` để người chốt duyệt.
2. Sau khi được duyệt, mới tiến hành đánh giá và viết nội dung.
3. Report final phải dựa trên tài liệu final trong `demo_test_guide`, không dựa vào report nháp trong `temp-docs`.

Đề xuất mục ban đầu cho `report-final.md`:

- Tổng quan dự án và mục tiêu Applied Cryptography.
- Kiến trúc hệ thống: Frontend, Gateway, CA, KDC, Bank, SOC.
- Thiết kế PKI: Root CA, Client CA, gRPC Transport CA, identity cert.
- Luồng đăng ký: OTP, CSR, cấp cert, tạo Bank user/account, rollback consistency.
- Luồng đăng nhập và ủy quyền: AS/TGS/AP, ticket scope, replay protection.
- Luồng banking: balance, history, transfer, validation, idempotency.
- Admin CA: cert-based `ca_admin`, certificate management, revoke, audit.
- Admin Bank: cert-based session, dashboard, audit.
- Admin SOC: KDC audit, timeline `operation_id`, verify, summary, export.
- Audit và tamper-evidence: CA/KDC/Bank audit, hash-chain, limitation.
- Demo/test evidence: smoke, functional, security, runtime results.
- Hướng dẫn chạy nhanh và cấu hình env/compose.
- Giới hạn đã biết và hướng phát triển.

Done khi:

- Có `report-final.md` được duyệt cấu trúc.
- Nội dung report khớp code/docs final.
- Không overclaim các phần chưa chạy runtime hoặc còn limitation.

## 6. Thứ tự thực thi ngay

1. Phase 1: sửa env/compose cho KDC audit DB và Client CA intermediate.
2. Phase 2: sửa smoke theo route/env hiện tại.
3. Phase 3: hoàn thiện guide.
4. Phase 4: hoàn thiện testcase và runtime result.
5. Phase 5: hoàn thiện script demo.
6. Phase 6: chốt README, env templates, cleanup temp-docs và worktree sạch.
7. Phase 7: đề xuất mục rồi tạo `quality-assessment-final.md`.
8. Phase 8: đề xuất mục rồi tạo `report-final.md`.

Nếu thời gian gấp, không mở thêm feature mới. Chỉ tập trung vào demo stack, smoke, audit evidence, guide và script quay video.
