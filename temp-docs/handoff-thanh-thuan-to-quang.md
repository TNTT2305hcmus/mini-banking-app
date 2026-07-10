# Bàn giao Thanh + Thuận cho Quang

Cập nhật: 10/07/2026.

Mục tiêu file này: khóa phần implement Thanh + Thuận sau giai đoạn 6, ghi rõ phần đã pass local và phần còn phụ thuộc Quang ở compose/env/smoke/runtime.

## 1. Kết quả regression local

Các kiểm tra đã chạy trong workspace hiện tại:

| Hạng mục | Lệnh | Kết quả |
|---|---|---|
| Frontend build | `npm.cmd run build -- --outDir ..\..\.tmp\frontend-build --emptyOutDir` | PASS; còn warning bundle JS > 500 kB sau minify |
| API Gateway typecheck | `npm.cmd exec -- tsc --noEmit` | PASS |
| CA Service tests | `go test ./...` với `GOCACHE` trỏ vào `.tmp/go-build-cache` | PASS |
| Banking Service tests | `go test ./...` với `GOCACHE` trỏ vào `.tmp/go-build-cache` | PASS |
| KDC Service tests | `go test ./...` với `GOCACHE` trỏ vào `.tmp/go-build-cache` | PASS |

Không có test local đỏ ở giai đoạn 6.

## 2. Checklist logic đã khóa bằng code/test

| Luồng | Trạng thái | Bằng chứng chính |
|---|---|---|
| Register consistency CA/Bank | CODE PASS, runtime pending | Gateway pre-check `CheckUserEmail`, email trùng trả `409 EMAIL_ALREADY_REGISTERED`, `jti` chỉ mark used sau khi flow thành công, revoke best-effort reason `registration_rollback` nếu Bank fail sau CA issue |
| Bank `CheckUserEmail` | CODE PASS | Banking Service có test `TestCheckUserEmail`; Banking tests pass |
| Rate-limit rehearsal | CODE PASS, runtime pending | `RATE_LIMIT_DISABLED`, env window/max và `Retry-After` đã có trong Gateway; cần Quang đưa env vào `.env.demo.example` |
| Admin CA cert-based | CODE PASS, runtime pending | Role `ca_admin`, `/v1/admin-ca/activate`, `/v1/admin-ca/session`, UI PIN/cert; password/static-token Admin CA cũ đã bỏ khỏi route chính |
| Operation id | CODE PASS, runtime pending | Frontend tạo/tái sử dụng `operation_id` qua `X-Request-ID`; AS/TGS/Bank user calls forward trace metadata; AP `request_id` vẫn độc lập |
| Audit testcase | DOC PASS, runtime pending | `mini-banking-app/docs/audit-testcases.md` đã điền 27 case, owner/note/endpoint đầy đủ, không claim runtime pass khi chưa chạy stack |
| Hash-chain limitation | DOC PASS | Tài liệu ghi rõ hash-chain đạt một phần nếu chưa có external anchor, không cover timestamp/metadata và không phát hiện tail truncation |

## 3. Dependency Quang cần xử lý trước rehearsal

### 3.1. `.env.demo.example`

File hiện vẫn có biến cũ:

- `CA_DEMO_EMAIL`
- `CA_DEMO_PASSWORD`

Trong code sau cleanup Admin CA, đường chính là cert-backed session. Smoke script dùng `ADMIN_CA_TOKEN` nếu muốn test Admin CA API tự động. Quang cần:

- bỏ hoặc ghi deprecated cho `CA_DEMO_*`;
- bổ sung `ADMIN_CA_TOKEN` như optional runtime token lấy sau `/admin-ca` cert login nếu smoke cần test API;
- bổ sung env rate-limit demo:
  - `RATE_LIMIT_DISABLED`;
  - hoặc các biến limit/window tương ứng trong Gateway;
- bổ sung `ADMIN_SEC_DEMO_EMAIL`, `ADMIN_SEC_DEMO_PASSWORD`, `ADMIN_SEC_DEMO_TOKEN` nếu muốn SOC smoke tự động;
- ghi rõ `operation_id` convention trong runbook/smoke.

### 3.2. KDC audit DB trong compose

`docker-compose.local.yml` và `docker-compose.demo.yml` hiện chưa set `DATABASE_URL` cho `kdc-service`. Nếu không set, KDC vẫn cấp AS/TGS nhưng audit KDC no-op, làm SOC timeline/summary thiếu KDC.

Quang cần thêm `DATABASE_URL` cho `kdc-service` và đảm bảo migration `db/kdc/migrations` được chạy trên DB tương ứng.

### 3.3. CA Client CA intermediate trong compose

CA Service code có default:

- `CLIENT_CA_KEY_PATH=certs/intermediate/client-ca.key`
- `CLIENT_CA_CERT_PATH=certs/intermediate/client-ca.crt`

Compose hiện mount root-ca và grpc certs, nhưng chưa mount `./ca-service/certs/intermediate` vào container và chưa set `CLIENT_CA_*`. Quang cần thêm mount/env tương ứng, nếu không CA container có rủi ro fail khi image không chứa cert intermediate.

### 3.4. Smoke/runtime checklist cần chạy lại

Sau khi sửa env/compose:

1. Admin CA provision/activate/login bằng cert `ca_admin`.
2. Register user mới bằng OTP thật hoặc flow demo đã chốt.
3. Register email trùng, kỳ vọng `409 EMAIL_ALREADY_REGISTERED`, không tạo cert rác.
4. AS/TGS login nhiều lần, không bị 429 trong rehearsal khi env demo đã nới hoặc disable.
5. Balance/history/transfer thành công.
6. Negative Bank: replay, forbidden ownership, insufficient funds hoặc cert revoked bằng tài khoản phụ.
7. Admin CA list/detail/revoke/audit đúng route `/v1/admin-ca/*`.
8. Admin Bank session/dashboard/audit.
9. SOC KDC audit, timeline theo `operation_id`, verify, summary, export.
10. Export CSV/JSON sau rehearsal để lưu bằng chứng.

## 4. Trạng thái audit testcase khi bàn giao

`mini-banking-app/docs/audit-testcases.md` đã khóa theo quy ước:

- `CODE PASS / RUNTIME PENDING`: code path/route/test local đã có, cần curl/UI thật để chốt.
- `RUNTIME PENDING`: chỉ kết luận được khi chạy stack thật.
- `PENDING_MANUAL_DB`: cần thao tác DB demo có kiểm soát.

Quang sau khi bật stack chỉ cần đổi từng dòng runtime sang `PASS` hoặc `FAIL`, ghi thêm bằng chứng curl/UI vào `Note`.

## 5. Lưu ý worktree hiện tại

`git status` hiện báo:

- `M temp-docs/regard.md`
- `D temp-docs/thuan-report.md`

Không tự restore `temp-docs/thuan-report.md` trong giai đoạn 6 vì file này đang bị xóa trong worktree. File bàn giao này thay thế phần trạng thái cần đưa cho Quang. Nhóm nên quyết định sau: giữ deletion hay restore/update report cá nhân của Thuận.

## 6. Kết luận bàn giao

Phần implement Thanh + Thuận đã đủ điều kiện bàn giao cho Quang ở mức local regression: build/typecheck/test đều pass, các P0 đã có hướng xử lý hoặc trạng thái runtime pending rõ ràng.

Phần còn lại không nên tiếp tục sửa rải rác trong code Thanh/Thuận trước khi Quang xử lý compose/env/smoke, vì rủi ro chính hiện nằm ở vận hành runtime: KDC audit DB, Client CA mount, env demo, token Admin CA cert session và smoke E2E.
