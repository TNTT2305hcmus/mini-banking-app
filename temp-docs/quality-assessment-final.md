# Quality Assessment Final

Cập nhật: 11/07/2026.

File này đánh giá trạng thái source chốt của Mini Banking App sau khi tự chạy các bước kiểm tra khả dụng trong môi trường hiện tại. Kết quả dưới đây tách rõ giữa kiểm tra code/build đã chạy thật và phần runtime Docker/UI chưa thể xác nhận.

## 1. Kết Luận Nhanh

Source hiện đủ nền tảng kỹ thuật cho demo Applied Cryptography: OTP, PKI/X.509, certificate roles, AS/TGS/AP, signed banking request, replay/idempotency protection, Admin CA, Admin Bank, Admin SOC và audit/hash-chain.

Trạng thái sau kiểm tra:

| Nhóm | Kết quả | Ghi chú |
|---|---|---|
| CA Service | PASS | `go test ./...` pass. |
| Banking Service | PASS | `go test ./...` pass. |
| KDC Service | PASS | `go test ./...` pass. |
| API Gateway | PASS | `npm.cmd exec tsc -- --noEmit` pass. |
| Frontend | PASS có warning | `npm.cmd run build` pass; Vite cảnh báo chunk JS > 500 kB. |
| Docker compose/runtime smoke | NOT VERIFIED | Docker daemon không truy cập được và `.env` hiện thiếu biến bắt buộc cho compose. |
| Browser/UI rehearsal | NOT RUN | Cần stack Docker chạy thật. |

Không nên ghi runtime `PASS` cho smoke, functional, security hoặc audit testcase cho tới khi chạy được Docker stack, smoke script và UI rehearsal thật.

## 2. Các Lệnh Đã Chạy

Chạy trong workspace `D:\U\Y3\S2\Applied Cryptography\mini-banking-app`.

| Lệnh | Kết quả |
|---|---|
| `go test ./...` tại `mini-banking-app/ca-service` với `GOCACHE` trong `.tmp/go-build` | PASS. |
| `go test ./...` tại `mini-banking-app/banking-service` với `GOCACHE` trong `.tmp/go-build` | PASS. |
| `go test ./...` tại `mini-banking-app/kdc-service` với `GOCACHE` trong `.tmp/go-build` | PASS. |
| `npm.cmd exec tsc -- --noEmit` tại `mini-banking-app/api-gateway` | PASS. |
| `npm.cmd run build` tại `mini-banking-app/frontend` | PASS; warning bundle size. |
| `docker version` tại runtime root | FAIL do không có quyền Docker API: `permission denied while trying to connect to the docker API`. |
| `docker compose -f docker-compose.local.yml config` | FAIL trước runtime vì `.env` thiếu `BANK_DB_PASSWORD`; đồng thời có warning không đọc được `C:\Users\PC\.docker\config.json`. |
| `bash -n scripts/demo/smoke-test.sh` | Không chạy được vì máy gọi sang WSL nhưng chưa cài distro. |

Ghi chú môi trường:

- Lần chạy Go đầu tiên bị chặn cache tại `C:\Users\PC\AppData\Local\go-build`; đã chạy lại bằng `GOCACHE` trong workspace.
- PowerShell chặn `npm.ps1`; đã dùng `npm.cmd`.
- `.env` hiện có placeholder SMTP và chưa có `BANK_DB_PASSWORD`, nên compose chưa đủ điều kiện chạy.

## 3. Thay Đổi Source/DOC Đi Kèm

Thay đổi code:

- `mini-banking-app/kdc-service/internal/kdc/service.go`: thêm import chuẩn `log` để các dòng `log.Printf` trong `loadSigningChain` compile được.

Thay đổi documentation cleanup:

- Giữ `RUN.md` làm hướng dẫn chạy chính ở root repo.
- Xóa các guide vận hành bị trùng trong `demo_test_guide/guide`: `RUN_GUIDE.md`, `ENV_GUIDE.md`, `COMPOSE_GUIDE.md`, `TERMINAL_GUIDE.md`, `TROUBLESHOOTING.md`.
- Giữ `demo_test_guide/guide/SEED_AND_ACCOUNTS.md` vì đây là reference dữ liệu seed cụ thể.
- Cập nhật `README.md` và `RUN.md` để không còn link tới các guide đã xóa.

## 4. Đánh Giá Theo Thành Phần

| Thành phần | Đánh giá | Rủi ro còn lại |
|---|---|---|
| Frontend React/Vite | Build production pass; cấu trúc page/service đủ cho customer/admin/SOC. | Cần UI rehearsal thật để xác nhận IndexedDB, cert/PIN, AS/TGS/AP và dashboard dữ liệu thật. |
| API Gateway | TypeScript compile pass; route/middleware/service tách rõ. | Cần runtime để xác nhận mapping HTTP/gRPC, env, cert mount và admin auth. |
| CA Service | Unit/integration-style Go tests pass. | Cần chạy với Postgres/cert provision thật trong compose. |
| KDC Service | Go tests pass sau khi sửa compile import; AS/TGS code path có test. | Cần runtime để xác nhận Redis, CA gRPC, `DATABASE_URL` audit và signing chain mount. |
| Banking Service | Go tests pass; bank/admin gRPC tests pass. | Cần runtime để xác nhận DB migration/seed, replay store và transaction history qua Gateway/UI. |
| Docker/Env | Compose file có cấu trúc final, nhưng chưa validate trọn vẹn trên máy này. | `.env` phải điền đủ secret, đặc biệt `BANK_DB_PASSWORD`; Docker Desktop/daemon phải cấp quyền truy cập. |

## 5. Findings

### High

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| H-01 | Chưa chạy được Docker compose và smoke runtime thật. | Chưa thể claim end-to-end demo pass. | Điền `.env`, đảm bảo Docker Desktop chạy, rồi chạy `docker compose -f docker-compose.local.yml up --build -d` và `scripts/demo/smoke-test.ps1 -SkipSmtp`. |
| H-02 | `.env` hiện thiếu `BANK_DB_PASSWORD`; SMTP vẫn là placeholder. | Compose config/runtime bị chặn trước khi dựng stack. | Copy lại từ `.env.demo.example` hoặc điền đủ các biến bắt buộc trước rehearsal. |

### Medium

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| M-01 | Frontend build pass nhưng bundle JS khoảng 803 kB, vượt ngưỡng cảnh báo Vite 500 kB. | Không chặn demo, nhưng ảnh hưởng tải lần đầu. | Có thể code-split sau khi nộp nếu cần; không phải blocker final. |
| M-02 | `smoke-test.sh` chưa syntax-check được do WSL không có distro. | Chưa xác nhận đường Bash/Linux. | Chạy lại trên Git Bash/Linux/WSL có distro. |
| M-03 | Hash-chain chưa có external anchor tự động. | Chưa chống được mọi dạng tail truncation. | Ghi rõ limitation trong report/demo. |

### Low

| ID | Finding | Ảnh hưởng | Khuyến nghị |
|---|---|---|---|
| L-01 | Docker CLI cảnh báo không đọc được `C:\Users\PC\.docker\config.json`. | Có thể gây nhiễu cho người chạy demo. | Kiểm tra quyền file Docker config hoặc chạy Docker Desktop bằng user hiện tại. |
| L-02 | Browser state cũ có thể ảnh hưởng cert/key demo. | Dễ dùng nhầm IndexedDB/cert cũ khi rehearsal nhiều lần. | Dùng browser profile mới hoặc clear site data `localhost:5173`. |

## 6. Checklist Runtime Còn Lại

Trước khi ghi `PASS` vào `demo_test_guide/tests/runtime-results.md`, cần chạy thêm:

| Việc cần làm | Trạng thái hiện tại |
|---|---|
| Điền đủ `.env`, đặc biệt `BANK_DB_PASSWORD`, `SMTP_*` hoặc quyết định `-SkipSmtp`. | PENDING |
| Provision CA/certs/KDC keys theo `RUN.md`. | PENDING |
| `docker compose -f docker-compose.local.yml config`. | PENDING |
| `docker compose -f docker-compose.local.yml up --build -d`. | PENDING |
| `docker compose -f docker-compose.local.yml ps`. | PENDING |
| `scripts/demo/smoke-test.ps1 -SkipSmtp`. | PENDING |
| UI rehearsal customer register/login/transfer. | PENDING |
| UI rehearsal Admin CA/Admin Bank/Admin SOC. | PENDING |
| SOC timeline/hash-chain verify/export evidence. | PENDING |
| Cập nhật `demo_test_guide/tests/runtime-results.md`. | PENDING |

## 7. Kết Luận

Ở mức source/build/test cục bộ, chất lượng hiện tại tốt hơn trạng thái trước vì các module chính đã compile/test pass, trong đó KDC đã được sửa lỗi compile nhỏ. Phần còn thiếu không phải là logic code đã thấy ngay, mà là evidence runtime: Docker compose, smoke output, UI flow, SOC timeline và audit export.

Source có thể dùng làm bản chốt kỹ thuật, nhưng báo cáo cuối nên diễn đạt trung thực: code-level verification đã pass; runtime end-to-end cần được chạy lại trên máy có Docker/env đầy đủ trước khi claim PASS toàn hệ thống.
