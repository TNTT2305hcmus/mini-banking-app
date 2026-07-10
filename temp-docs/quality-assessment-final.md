# Quality Assessment Final

Cập nhật: 10/07/2026.

File này đánh giá chất lượng code và tài liệu của Mini Banking App tại thời điểm chốt Phase 7. Phạm vi chính là static review, config review và docs review dựa trên source hiện tại, `README.md`, `demo_test_guide`, smoke scripts và roadmap `review-v2.md`.

Nguyên tắc đánh giá:

- Không claim runtime `PASS` nếu chưa chạy stack thật.
- Tách rõ phần đã có bằng chứng code/static review với phần còn `RUNTIME PENDING`.
- Tập trung vào chất lượng kỹ thuật để chuẩn bị cho Phase 8 `report-final.md`.
- Không dùng file này làm tài liệu final để nộp; đây là tài liệu đánh giá nội bộ trong `temp-docs`.

## 1. Tóm tắt chất lượng tổng thể

Đánh giá tổng quan: dự án đã đủ nền tảng kỹ thuật để dựng demo Applied Cryptography hoàn chỉnh, gồm OTP, PKI, certificate lifecycle, AS/TGS/AP, signed banking request, replay/idempotency protection, Admin CA, Admin Bank, Admin SOC và audit/hash-chain.

Mức độ hoàn thiện hiện tại:

| Nhóm | Đánh giá | Trạng thái |
|---|---|---|
| Core feature | Đủ luồng chính cho customer, admin và SOC. | CODE READY / RUNTIME PENDING |
| Applied crypto | Có PKI, CSR, cert roles, AS/TGS/AP, signed request, replay protection. | CODE READY / RUNTIME PENDING |
| Audit/SOC | Có CA/KDC/Bank audit, timeline, verify, summary, export. | CODE READY / RUNTIME PENDING |
| DevOps/config | Đã chuẩn hóa env/compose final, bỏ root `.env.example` và `docker-compose.yml` cũ. | CONFIG READY / RUNTIME PENDING |
| Test/demo docs | Đã có guide, testcase, runtime result template và demo scripts. | DOC READY / REHEARSAL PENDING |

Kết luận: có thể bước sang giai đoạn rehearsal và report final, nhưng chưa nên ghi "đã pass runtime" cho các testcase nếu chưa chạy Docker/terminal stack thật và cập nhật `runtime-results.md`.

## 2. Phạm vi đánh giá

Các module được đánh giá:

- Frontend React/Vite.
- API Gateway Node.js/TypeScript.
- CA Service Go.
- KDC Service Go.
- Banking Service Go.
- Admin CA, Admin Bank, Admin SOC.
- Docker Compose, env templates, scripts demo/smoke.
- Final docs trong `README.md` và `demo_test_guide`.

Loại đánh giá đã thực hiện:

| Loại | Kết quả |
|---|---|
| Static source review | Đã rà cấu trúc source, route, controller, middleware, service, audit và crypto-related code path. |
| Config review | Đã rà README, env templates, compose direction và final docs. |
| Docs review | Đã rà guide/test/demo docs |
| Runtime smoke | Chưa chạy stack thật trong phase này. |
| Browser rehearsal | Chưa chạy quay demo thật trong phase này. |

## 3. Kiến trúc và phân tách trách nhiệm

Kiến trúc hiện tại có phân tách trách nhiệm tương đối rõ:

| Thành phần | Vai trò chính | Đánh giá |
|---|---|---|
| Frontend | UI customer/admin, sinh key/CSR, giữ cert/private key, gọi Gateway. | Hợp lý cho demo crypto vì private key ở browser. |
| API Gateway | REST boundary, validate request, rate-limit, forward gRPC, gom audit/SOC. | Là điểm điều phối tốt, nhưng cần runtime test để chắc route/env khớp. |
| CA Service | Root CA/Client CA, issue/verify/revoke cert, audit CA. | Ownership rõ, có test và code path cho cert lifecycle. |
| KDC Service | AS/TGS, ticket/session key, role/scope, audit KDC. | Thiết kế đúng trọng tâm Applied Cryptography; phụ thuộc `DATABASE_URL` để SOC thấy audit. |
| Banking Service | Account/transaction, AP auth, replay/idempotency, bank audit. | Tách nghiệp vụ bank khỏi Gateway tốt, có negative paths quan trọng. |
| SOC | Audit timeline, KDC audit, verify, summary, export. | Tạo được lớp quan sát bảo mật thuyết phục cho demo. |

Điểm mạnh:

- Route ownership đã tách theo nhóm `/v1/auth`, `/v1/bank`, `/v1/admin-ca`, `/v1/admin/bank`, `/v1/admin-sec`, `/v1/admin-kdc`, `/v1/admin/audit`.
- `X-Request-ID`/`operation_id` được dùng để liên kết audit cross-service.
- Admin CA, Admin Bank và SOC không bị gộp chung một vai trò admin mơ hồ.

Rủi ro còn lại:

- Cross-service timeline phụ thuộc dữ liệu audit thật từ CA/KDC/Bank; nếu KDC audit DB chưa ghi được thì SOC mất phần quan trọng nhất của AS/TGS.
- Cần chạy Gateway thật trong Docker để phát hiện các lỗi do env, cert mount, service hostname hoặc migration.

## 4. Chất lượng triển khai cryptography

Các điểm đã có trong code/docs:

- PKI có Root CA, Client CA và gRPC Transport CA.
- Identity cert có role như `customer`, `bank_admin`, `ca_admin`.
- Frontend sinh key pair/CSR, private key không rời browser theo hướng thiết kế.
- AS/TGS/AP triển khai theo hướng Kerberos-like flow.
- Banking request có service ticket, authenticator, request id, payload/signature path.
- Replay/idempotency được xử lý ở banking flow bằng request fingerprint/idempotency key.
- Certificate verification/revocation được dùng trong admin và banking auth path.

Đánh giá:

| Nhóm | Mức độ | Ghi chú |
|---|---|---|
| PKI hierarchy | Tốt | Có phân tách Root CA, Client CA, Transport CA. |
| CSR/cert lifecycle | Tốt | Có issue, verify, revoke, role metadata và Admin CA UI/API. |
| AS/TGS/AP | Tốt cho demo | Đúng trọng tâm môn Applied Cryptography, cần runtime evidence. |
| Private key handling | Khá tốt | Thiết kế browser-side phù hợp, cần demo rõ IndexedDB/PIN/wrapped key. |
| Replay/idempotency | Tốt | Có cả Redis/DB fallback path trong Banking Service. |
| Hash-chain | Khá tốt | Có tamper-evidence, nhưng limitation cần trình bày trung thực. |

Giới hạn cần nói rõ:

- Đây là demo/coursework, không claim production banking compliance.
- Hash-chain chưa có external anchor tự động.
- Tail truncation chưa được phát hiện tuyệt đối nếu không có checkpoint ngoài hệ thống.
- Audit insert là best-effort, không nên nói là đảm bảo bất biến tuyệt đối.
- Admin CA cert login không đi qua AS/TGS/AP như Bank Admin; cần mô tả là một flow cert-based riêng.

## 5. Authentication và authorization

Đánh giá theo nhóm:

| Nhóm | Cơ chế | Đánh giá |
|---|---|---|
| Customer | OTP/register, cert/PIN, AS/TGS/AP, ticket scope. | Đủ mạnh cho demo, cần browser rehearsal. |
| Admin CA | Cert-based activation/login/session, route `/v1/admin-ca/*`. | Hợp lý, đã loại hướng password/static-token cũ khỏi final docs. |
| Admin Bank | Cert-based activation/login/session, dashboard/audit. | Hợp lý, cần dữ liệu runtime sau transfer thật. |
| Admin SOC | Security-admin login/token, KDC audit, timeline/verify/export. | Phù hợp demo SOC, nhưng demo token/password cần quản lý như demo secret. |

Negative cases đã được thiết kế/testcase hóa:

- Duplicate registration.
- Revoked cert.
- Forbidden account ownership.
- Insufficient funds/invalid transfer.
- Replay/idempotency duplicate.
- SOC endpoint without token.
- Rate-limit.
- Hash-chain tamper detection.

Rủi ro:

- Một số negative cases cần thao tác runtime hoặc DB có kiểm soát, không thể claim bằng static review.
- Security-admin demo token là cơ chế demo; không nên trình bày như production IAM.

## 6. Audit, SOC và tamper-evidence

Điểm mạnh:

- CA/KDC/Bank đều có hướng audit riêng.
- SOC có nhiều bề mặt trình diễn: KDC audit, timeline, verify, summary, export.
- Timeline theo `operation_id` giúp nối event từ nhiều service.
- Hash-chain verify có giá trị trình diễn tốt cho tamper-evidence.
- Docs/testcases đã tách audit testcase riêng.

Đánh giá module SOC:

| Tính năng | Đánh giá | Trạng thái |
|---|---|---|
| KDC audit list | Cần thiết và đúng trọng tâm | RUNTIME PENDING |
| Timeline theo `operation_id` | Rất hữu ích cho demo | RUNTIME PENDING |
| Verify hash-chain | Có giá trị non-functional/security rõ | CODE READY / RUNTIME PENDING |
| Summary/export | Tốt cho evidence report/video | RUNTIME PENDING |

Giới hạn:

- Timeline chỉ thuyết phục khi `X-Request-ID` được truyền nhất quán qua Gateway/Frontend và các service ghi audit đúng request id.
- KDC audit phụ thuộc Postgres/migration/env `DATABASE_URL`.
- Hash-chain hiện phù hợp phát hiện sửa/xóa/đảo event ở giữa chuỗi; chưa đủ để chống mọi dạng xóa đuôi nếu không có anchor ngoài.

## 7. Data consistency và transaction safety

Điểm tốt:

- Registration flow có xử lý consistency giữa CA và Bank, có rollback/revoke path khi Bank create user/account lỗi sau khi CA issue cert.
- Banking transfer có validation nghiệp vụ, idempotency key và ledger/audit hash-chain.
- Replay protection có Redis và DB fallback trong Banking Service.
- Seed/migration đã được tài liệu hóa trong guide.

Rủi ro:

- Các rollback path cần runtime test vì phụ thuộc lỗi thật giữa CA/Gateway/Bank.
- Nếu volume Postgres cũ đã tồn tại, migration trong `/docker-entrypoint-initdb.d` có thể không chạy lại; guide đã ghi nhưng cần rehearsal.
- Audit best-effort không nên được xem là transaction bắt buộc của business flow.

## 8. API quality và error handling

Điểm tốt:

- API Gateway có middleware validate request bằng schema.
- Error response có xu hướng dùng code ổn định như `EMAIL_ALREADY_REGISTERED`, `INVALID_CSR_FORMAT`, `ADMIN_SEC_LOGIN_FAILED`.
- Route nhóm rõ, dễ ghi testcase và demo.
- Smoke scripts đã được chỉnh để gọi các route hiện tại thay vì route Admin CA cũ.

Điểm cần kiểm tra khi runtime:

- Mapping gRPC error sang HTTP status có nhất quán trong mọi negative case không.
- Response shape có đủ ổn định cho frontend sau các lỗi OTP/cert/replay/rate-limit không.
- Các endpoint admin có trả `401/403` đúng khi thiếu token/session không.
- Export JSON/CSV có hoạt động qua Gateway thật không.

## 9. Frontend quality

Điểm mạnh:

- Có route/page cho customer register/login/home.
- Có page riêng cho Admin CA, Admin Bank, Admin SOC.
- Frontend services được tách theo domain: PKI registration, AS exchange, TGS exchange, bank transfer/profile/history, admin APIs.
- Có `operation-id` service để giữ trace id xuyên flow.
- Có component `AuditTimeline` hỗ trợ trình bày SOC/timeline.

Điểm cần rehearsal:

- Luồng IndexedDB/cert/key cũ khi chạy lại demo nhiều lần.
- UX khi OTP/SMTP fail hoặc bị rate-limit.
- UI có phản ánh đúng balance/history sau transfer success/fail không.
- Admin dashboards có đủ dữ liệu thật sau seed/transfer không.
- SOC export/timeline có hiển thị đủ CA/KDC/Bank events không.

## 10. DevOps và config quality

Điểm đã cải thiện:

- Runtime final chuyển sang `.env.demo.example`, `.env`, `docker-compose.local.yml`, `docker-compose.demo.yml`.
- Compose đã được chuẩn hóa để mount Client CA intermediate và cấp `DATABASE_URL` cho KDC audit.
- `.env.example` riêng của các service đã được rà lại để khớp hướng terminal/compose.
- README đã trở thành quick start sạch, link sang `demo_test_guide`.

Đánh giá:

| Nhóm | Đánh giá |
|---|---|
| Env template | Khá tốt, đã tách demo shared env và service env. |
| Compose | Hợp lý cho local/demo, nhưng cần `docker compose up` thật để xác nhận health. |
| Cert provisioning | Có scripts rõ, cần chạy theo thứ tự trước compose. |
| Migration/seed | Đã có guide, rủi ro chính là volume cũ không chạy init lại. |
| README | Gọn, đúng vai trò quick start. |

Rủi ro:

- Chưa có bằng chứng final runtime smoke.
- Cảnh báo Docker config trên máy hiện tại không phải lỗi YAML, nhưng có thể làm người chạy mới bối rối; guide đã ghi troubleshooting.
- Secret demo cần được thay trong `.env`, không commit secret thật.

## 11. Testability và test coverage

Hiện trạng testcase:

| Nhóm | Tài liệu/script | Đánh giá |
|---|---|---|
| Smoke | `scripts/demo/smoke-test.ps1`, `smoke-test.sh`, `smoke-testcases.md` | Đã chuẩn hóa route/env, runtime pending. |
| Functional | `functional-testcases.md` | Đủ luồng chính để rehearsal UI. |
| Security | `security-testcases.md` | Đủ nhóm PKI, AS/TGS/AP, replay, revoked, hash-chain. |
| Audit | `audit-testcases.md` | Có nhiều case audit/SOC chi tiết. |
| Runtime result | `runtime-results.md` | Đang là template, chưa có pass/fail thật. |

Đánh giá:

- Test planning tốt hơn nhiều so với trạng thái ban đầu.
- Smoke scripts có giá trị làm bước kiểm tra nhanh trước khi quay demo.
- Testcase đã phân loại functional và non-functional/security đúng yêu cầu.
- Điểm thiếu lớn nhất hiện tại là execution evidence: terminal output, screenshot, audit export, operation id.

Khuyến nghị:

- Chạy smoke trước, sau đó mới chạy UI functional/security.
- Sau mỗi lần chạy, cập nhật `runtime-results.md` ngay để tránh nhầm trạng thái.
- Không sửa testcase thành `PASS` nếu không có bằng chứng.

## 12. Documentation quality

Điểm tốt:

- `README.md` đã là cửa vào nhanh, không nhồi chi tiết dài.
- `demo_test_guide/guide` có run/env/compose/terminal/seed/troubleshooting.
- `demo_test_guide/tests` tách rõ testcase index, functional, security, smoke, audit, runtime results.
- `demo_test_guide/demo` có prep, functional script và non-functional/security script.

Điểm cần nhớ:

- Phase 8 `report-final.md` nên dựa trên `README.md` và `demo_test_guide`
- Khi runtime pass/fail thay đổi, cần cập nhật đồng thời `runtime-results.md`, testcase tương ứng và report final.

## 13. Findings theo mức độ

### High

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| H-01 | Chưa có final runtime smoke trên stack thật. | Không thể claim demo pass end-to-end. | Chạy compose local/demo, chạy smoke, lưu terminal output. |
| H-02 | SOC timeline phụ thuộc KDC audit DB/migration/env. | Nếu KDC audit không ghi DB, non-functional demo yếu đi rõ. | Xác nhận `DATABASE_URL`, migration `kdc_audit_log`, và event AS/TGS trong SOC. |

### Medium

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| M-01 | Hash-chain chưa có external anchor. | Chưa chống được tail truncation tuyệt đối. | Ghi limitation trong demo/report; nếu có thời gian, thêm checkpoint/export anchor. |
| M-02 | Audit insert là best-effort. | Một số event audit có thể thiếu nếu service/audit DB lỗi. | Trình bày trung thực; ưu tiên kiểm tra audit availability trong rehearsal. |
| M-03 | Một số negative/security cases cần thao tác DB hoặc browser thật. | Static review không đủ claim pass. | Đánh dấu `RUNTIME PENDING` hoặc `PENDING_MANUAL_DB` cho đến khi chạy. |
| M-04 | Demo token/security-admin chỉ phù hợp demo. | Dễ bị hiểu nhầm thành production auth. | Ghi rõ trong report là demo credential path, không phải production IAM. |

### Low

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| L-01 | Bash smoke chưa được syntax/runtime check trên Git Bash/Linux/WSL trong môi trường hiện tại. | Có thể lỗi khi chạy ngoài PowerShell. | Chạy `bash -n` và smoke thật trên môi trường có bash. |
| L-02 | Docker config warning trên máy hiện tại có thể gây nhiễu. | Người demo có thể tưởng là lỗi compose. | Giữ troubleshooting và giải thích đây không phải lỗi YAML. |
| L-03 | Browser IndexedDB/cert cũ có thể làm rehearsal lệch. | Demo có thể dùng nhầm cert/key cũ. | Làm sạch browser profile hoặc dùng profile mới trước khi quay. |

### Documentation/Cleanup

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| D-01 | `temp-docs` hiện là nơi chứa planning/review nội bộ. | Không nên lẫn với tài liệu final. | Khi chốt main, chỉ giữ tài liệu final trong README và `demo_test_guide`. |
| D-02 | `runtime-results.md` chưa có evidence thật. | Report final thiếu bằng chứng thực nghiệm. | Cập nhật sau smoke/rehearsal trước Phase 8. |

## 14. Checklist trước khi chốt report final

| Việc cần làm | Trạng thái | Ghi chú |
|---|---|---|
| Chạy `docker compose -f docker-compose.local.yml up --build -d` | RUNTIME PENDING | Cần `.env` thật và cert provisioning. |
| Kiểm tra containers healthy | RUNTIME PENDING | Ghi output `docker compose ps`. |
| Chạy `smoke-test.ps1` | RUNTIME PENDING | Lưu terminal output. |
| Chạy `smoke-test.sh` nếu có Git Bash/Linux/WSL | RUNTIME PENDING | Ít nhất cần syntax check. |
| Rehearsal functional demo | RUNTIME PENDING | Lưu screenshot/video notes. |
| Rehearsal non-functional/security demo | RUNTIME PENDING | Lưu operation id, audit export, verify output. |
| Cập nhật `runtime-results.md` | PENDING | Không để TBD khi viết report final. |
| Rà README link final | STATIC READY | Nên kiểm lại lần cuối trước commit. |
| Rà env/compose trên máy sạch | RUNTIME PENDING | Tránh phụ thuộc state local. |

## 15. Kết luận Phase 7

Chất lượng code và tài liệu hiện đủ tốt để chuyển sang giai đoạn runtime verification và report final. Phần kỹ thuật nổi bật nhất của dự án là sự kết hợp giữa PKI, Kerberos-like AS/TGS/AP, signed banking requests, replay/idempotency protection và SOC audit/hash-chain.

Điều kiện còn thiếu để chốt mạnh trong `report-final.md` là bằng chứng runtime:

- smoke output;
- screenshot các flow chính;
- SOC timeline theo `operation_id`;
- audit export CSV/JSON;
- hash-chain verify result;
- bảng `runtime-results.md` được điền pass/fail thật.

Sau khi các evidence này có đủ, Phase 8 có thể viết `report-final.md` với mức tự tin cao hơn và không cần overclaim.
