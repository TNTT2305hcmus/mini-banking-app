# Giai đoạn 0 - Chốt baseline Thanh/Thuận

Cập nhật: 09/07/2026.

## 1. Phạm vi

Giai đoạn này chỉ khóa baseline và phạm vi sửa cho phần Thanh/Thuận. Chưa sửa logic chạy thật, chưa trộn phần Quang về compose/smoke/full-stack.

Không đụng các thay đổi sẵn có trong `temp-docs`:

- `temp-docs/PROBLEM.md` đang bị xóa trong worktree.
- `temp-docs/kệ-file-này-đi-ô.md` đang bị xóa trong worktree.
- `temp-docs/problem2.md` đang chưa được Git theo dõi.

## 2. Baseline đã kiểm tra

Trạng thái Git lúc chốt baseline:

- Nhánh: `thanh...origin/thanh`.
- Worktree sẵn có: `regard.md` chưa được Git theo dõi, `temp-docs/problem2.md` chưa được Git theo dõi, 2 file `temp-docs` đang bị xóa.
- Build frontend tạo thư mục `.tmp/frontend-build` trong workspace.

Kết quả lệnh:

| Hạng mục | Lệnh | Kết quả |
| --- | --- | --- |
| Frontend build | `npm.cmd run build -- --outDir ..\..\.tmp\frontend-build --emptyOutDir` | Đạt; Vite cảnh báo bundle JS > 500 kB |
| API Gateway typecheck | `.\node_modules\.bin\tsc.cmd --noEmit` | Đạt |
| CA Service | `go test ./...` | Đạt |
| Banking Service | `go test ./...` | Đạt |
| KDC Service | `go test ./...` | Đạt |

Ghi chú: các lệnh `go test` lần đầu bị sandbox chặn vì Go ghi cache vào `C:\Users\PC\AppData\Local\go-build`; đã chạy lại đúng lệnh ngoài sandbox và đạt.

## 3. Checklist ngắn

### Thanh

- Tính nhất quán đăng ký:
  - Thêm Bank read path/RPC `CheckUserEmail(email)`.
  - Check email trước khi CA issue cert.
  - Chỉ mark `jti` used sau khi flow thành công, hoặc rollback khi fail.
  - Nếu Bank fail sau CA issue, revoke cert best-effort với reason `registration_rollback`.
  - Map email trùng thành `409 EMAIL_ALREADY_REGISTERED`.
- Rate-limit demo:
  - Thêm env disable/nâng ngưỡng rate-limit.
  - 429 có `Retry-After` và error code rõ.
- Môi trường Admin CA:
  - Bỏ default placeholder cho `ADMIN_CA_DEMO_EMAIL/PASSWORD/TOKEN`.
  - Thiếu config phải fail closed.
- Admin CA cert-based:
  - Ưu tiên thêm role/cert `ca_admin` và flow activation/session.
  - Nếu không kịp, fallback password/JWT/static token phải ghi rõ là giới hạn demo.

### Thuận

- Operation/trace id:
  - Cho frontend tạo một `operation_id` cho flow lớn: register/login/transfer.
  - Cho HTTP client nhận `X-Request-ID` từ caller thay vì luôn sinh mới mỗi call.
  - Giữ riêng AP `request_id` khi protocol Bank cần replay/idempotency riêng.
- Testcase audit:
  - Điền `Pass/Fail`, `Owner`, `Note` cho các case P0/P1 trong `docs/audit-testcases.md`.
  - Tách case chưa chạy được vì phụ thuộc stack Quang.
- Timeline SOC:
  - Kiểm tra timeline theo một id chung có CA + KDC, và Bank khi có cookie `bank_admin_session`.
- Giới hạn hash-chain:
  - Ghi rõ hash-chain chưa cover timestamp/metadata.
  - Ghi rõ chưa phát hiện tail truncation nếu không có external/manual anchor.
  - Ghi rõ audit insert là best-effort.

## 4. File dự kiến sửa

### Thanh - Tính nhất quán đăng ký

- `mini-banking-app/proto/bank.proto`
- `mini-banking-app/pkg/pb/bank/bank.pb.go`
- `mini-banking-app/pkg/pb/bank/bank_grpc.pb.go`
- `mini-banking-app/api-gateway/src/proto/bank.ts`
- `mini-banking-app/banking-service/internal/bank/repository.go`
- `mini-banking-app/banking-service/internal/bank/service.go`
- `mini-banking-app/banking-service/internal/bank/types.go`
- `mini-banking-app/banking-service/internal/bank/errors.go`
- `mini-banking-app/banking-service/internal/grpc/handler.go`
- `mini-banking-app/banking-service/internal/grpc/handler_test.go`
- `mini-banking-app/api-gateway/src/services/bank.service.ts`
- `mini-banking-app/api-gateway/src/controller/ca.controller.ts`

### Thanh - Rate-limit và môi trường Admin CA

- `mini-banking-app/api-gateway/src/config/env.ts`
- `mini-banking-app/api-gateway/src/middleware/rateLimiter.ts`
- `mini-banking-app/api-gateway/src/middleware/errorHandler.ts`
- `mini-banking-app/api-gateway/src/controller/ca.controller.ts`
- Tài liệu/curl Admin CA nếu cần ghi rõ credential fallback, nhưng không sửa compose/smoke của Quang trong giai đoạn này.

### Thanh - Admin CA cert-based

- `mini-banking-app/proto/ca.proto`
- `mini-banking-app/pkg/pb/ca/ca.pb.go`
- `mini-banking-app/pkg/pb/ca/ca_grpc.pb.go`
- `mini-banking-app/api-gateway/src/proto/ca.ts`
- `mini-banking-app/ca-service/internal/ca/identity_role.go`
- `mini-banking-app/ca-service/internal/ca/identity_role_test.go`
- `mini-banking-app/ca-service/internal/ca/service.go`
- `mini-banking-app/ca-service/internal/grpc/handler.go`
- `mini-banking-app/api-gateway/src/services/ca.service.ts`
- `mini-banking-app/api-gateway/src/controller/ca.controller.ts`
- `mini-banking-app/api-gateway/src/middleware/admin.middleware.ts`
- `mini-banking-app/api-gateway/src/routes/admin-ca.route.ts`
- `mini-banking-app/frontend/src/pages/AdminCA.tsx`
- `mini-banking-app/frontend/src/services/admin/ca-admin.api.ts`

### Thuận - Operation id và tương quan audit

- `mini-banking-app/frontend/src/services/api.service.ts`
- `mini-banking-app/frontend/src/services/pki-registration/registration.api.ts`
- `mini-banking-app/frontend/src/services/pki-registration/pki-registration.service.ts`
- `mini-banking-app/frontend/src/services/as-exchange/as-exchange.api.ts`
- `mini-banking-app/frontend/src/services/as-exchange/as-exchange.service.ts`
- `mini-banking-app/frontend/src/services/tgs-exchange/tgs-exchange.api.ts`
- `mini-banking-app/frontend/src/services/tgs-exchange/tgs-exchange.service.ts`
- `mini-banking-app/frontend/src/services/bank/transfer/transfer.service.ts`
- `mini-banking-app/frontend/src/services/bank/profile/profile.service.ts`
- `mini-banking-app/frontend/src/services/bank/history/history.service.ts`
- `mini-banking-app/frontend/src/pages/Register.tsx`
- `mini-banking-app/frontend/src/pages/Login.tsx`
- `mini-banking-app/frontend/src/pages/Home.tsx`
- `mini-banking-app/api-gateway/src/services/kdc.service.ts`
- `mini-banking-app/api-gateway/src/services/bank.service.ts`
- `mini-banking-app/api-gateway/src/controller/admin-timeline.controller.ts`

### Thuận - Testcase và báo cáo audit

- `mini-banking-app/docs/audit-testcases.md`
- `mini-banking-app/scripts/admin-ca-curl-examples.md` nếu cần bổ sung cách lấy bằng chứng CA audit.
- `temp-docs/thuan-report.md` nếu nhóm muốn cập nhật report cá nhân sau khi chạy runtime.

## 5. Thứ tự ưu tiên

1. Thanh P0 tính nhất quán đăng ký trước, vì đây là lỗi có thể tạo lệch dữ liệu CA/Bank.
2. Thanh P0 rate-limit demo và Admin CA env fail-closed ngay sau đó, vì ảnh hưởng rehearsal và credential demo.
3. Thuận P0 operation_id có thể làm song song nếu tránh đụng `ca.controller.ts`; trong đợt đầu chỉ cần mở API client để caller truyền `X-Request-ID`.
4. Thanh Admin CA cert-based: làm đầy đủ nếu kịp, nếu không phải chốt fallback có kiểm soát và ghi rõ trong report.
5. Thuận testcase audit/giới hạn hash-chain: điền bảng đạt/không đạt sau khi có runtime; case phụ thuộc Quang thì ghi `Blocked by runtime/compose`.
6. Chạy regression chung Thanh + Thuận: frontend build, Gateway typecheck, CA/Bank/KDC tests.

## 6. Ngoài phạm vi giai đoạn 0

- Không sửa compose, smoke script, `.env.demo.example`, hoặc KDC `DATABASE_URL`; đây là dependency phần Quang.
- Không sửa logic register/rate-limit/audit trong file này.
- Không revert các thay đổi sẵn có trong `temp-docs`.
- Không claim runtime E2E đã pass vì Docker/database thật chưa được chạy trong giai đoạn này.

## 7. Ảnh hưởng tới phần Thái và Quang khi Thanh/Thuận hoàn thiện

Các thay đổi của Thanh và Thuận không chỉ nằm trong phạm vi CA/Audit. Khi triển khai xong, Thái và Quang cần cập nhật hoặc chạy lại một số phần liên quan để tránh demo lệch dữ liệu, sai env, hoặc timeline không khớp.

### Ảnh hưởng tới Thái

- Bank Admin và dashboard cần test lại sau khi Thanh thêm `CheckUserEmail(email)` và sửa register rollback, vì Bank sẽ có thêm read path/RPC mới và flow tạo user/account có thể đổi thứ tự xử lý lỗi.
- Các testcase Bank Admin liên quan user/account mới cần xác nhận lại: user đăng ký thành công phải có Bank user/account đầy đủ; email trùng không tạo cert rác và không tạo thêm Bank account.
- Nếu Thanh đổi error code register thành `EMAIL_ALREADY_REGISTERED`, UI/user error message và checklist demo của Thái nên dùng đúng mã lỗi mới khi mô tả case email trùng.
- Khi Thuận chuẩn hóa `operation_id`, các request Bank như balance/history/transfer có thể nhận cùng `X-Request-ID` theo flow. Thái cần kiểm tra Bank audit và Admin Bank tab Security Audit vẫn hiển thị đúng `request_id`, không bị nhầm với AP `request_id` dùng cho replay/idempotency.
- SOC timeline có thể gộp Bank event khi có cookie `bank_admin_session`; Thái cần xác nhận lại Bank Admin session/cookie hoạt động trong lúc demo SOC.
- Nếu rate-limit được nới hoặc disable bằng env, Thái nên chạy lại các luồng Admin Bank/login/dashboard nhiều lần để chắc chắn rehearsal không bị 429 ngoài ý muốn.
- Report/curl mẫu của Thái nên ghi rõ sau khi Thanh/Thuận hoàn thiện: register rollback đã không để lệch CA/Bank, Bank audit có thể correlate theo operation id, và Bank source trong SOC vẫn cookie-gated theo thiết kế.

### Ảnh hưởng tới Quang

- `.env.demo.example`, compose và smoke script phải đồng bộ với Admin CA env fail-closed: cần dùng đúng `ADMIN_CA_DEMO_EMAIL`, `ADMIN_CA_DEMO_PASSWORD`, `ADMIN_CA_DEMO_TOKEN` và không dựa vào placeholder mặc định.
- Nếu Thanh thêm `RATE_LIMIT_DISABLED` hoặc env cấu hình ngưỡng rate-limit, Quang cần đưa các biến này vào env template/runbook demo để rehearsal không bị chặn.
- Smoke script cần cập nhật theo error code/route mới của register: email trùng kỳ vọng `409 EMAIL_ALREADY_REGISTERED`; route Admin CA chính vẫn là `/v1/admin-ca/*`.
- Nếu Thanh thêm role/cert `ca_admin`, Quang cần bổ sung seed/provision/runbook để có Admin CA cert-based account dùng được trong demo, hoặc ghi rõ fallback nếu nhóm chốt chưa làm đầy đủ.
- Khi Thuận triển khai `operation_id`, smoke/manual checklist của Quang cần tạo và tái sử dụng một id chung cho các flow register/login/transfer để SOC timeline có CA + KDC + Bank cùng trace.
- Compose demo cần đảm bảo KDC có `DATABASE_URL` thật; nếu không, phần Thuận dù đã forward trace id vẫn không có KDC audit trong DB để SOC đọc.
- Seed demo nên tránh tạo sẵn dữ liệu gây xung đột với testcase rollback: email dùng cho duplicate test phải được kiểm soát, còn email dùng cho đăng ký mới phải sạch.
- Sau khi Thanh/Thuận xong, Quang cần chạy lại full smoke/runtime: register mới, register email trùng, AS/TGS, transfer, Admin CA audit, Admin Bank audit, SOC timeline/summary/export/verify.

### Điểm phối hợp bắt buộc trước khi chốt demo

- Thanh xác nhận danh sách env mới hoặc env đổi tên để Quang cập nhật template/runbook.
- Thuận xác nhận convention `operation_id`: khi nào tạo mới, khi nào tái sử dụng, và có khác AP `request_id` hay không.
- Thái xác nhận Bank Admin dashboard/audit không bị ảnh hưởng bởi Bank RPC mới và trace id mới.
- Quang xác nhận stack Docker thật có KDC audit DB, route Admin CA đúng, và smoke không còn gọi endpoint/env cũ.
