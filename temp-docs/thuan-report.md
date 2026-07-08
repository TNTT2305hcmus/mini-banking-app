# Báo cáo triển khai — Thuận: Audit Log (Ngày 1 + Ngày 2)

Trạng thái: **hoàn thành phần code Ngày 1 + Ngày 2** theo [plan-thuan-audit-log.md](plan-thuan-audit-log.md), hướng đã chốt: **Hướng A — gRPC cho cả CA và Bank**.

Quy ước request id (đã revise sau review): **trace id của Gateway (`X-Request-ID`) là transport concern → đi qua gRPC metadata key `x-request-id`**, không phải field trong proto message; chỉ dữ liệu domain mới nằm trong body — `performed_by` (được persist vào audit), `request_id` trong authenticator AP flow (một phần protocol, được persist), và `request_id` filter của API đọc audit Bank (query trên cột đã lưu).

Verify: `go build` + `go test ./...` pass ở `ca-service` và `banking-service`, `go build` pass ở `kdc-service`, `npx tsc --noEmit` sạch ở `api-gateway`.

---

## 1. Sửa ở đâu, để làm gì

### 1.1. Proto + generated code

| File | Sửa gì | Để làm gì |
|---|---|---|
| `mini-banking-app/proto/ca.proto` | Thêm RPC `ListAuditEvents` + message `ListAuditEventsRequest`/`AuditEventRecord`/`ListAuditEventsResponse`. **Không** thêm field `request_id` vào message nào — trace id đi qua gRPC metadata (có comment quy ước ngay trong proto) | Mở đường đọc audit CA qua gRPC; message chỉ chứa dữ liệu domain |
| `mini-banking-app/proto/bank.proto` | Thêm RPC `ListAuditEvents` + `ListAuditEventsRequest`/`BankAuditRecord`/`ListAuditEventsResponse`; field `request_id` trong request là **filter domain** trên cột `bank_audit_log.request_id` (có comment phân biệt với trace id) | Đọc `bank_audit_log` qua gRPC với filter action/user/cert/request_id/time + pagination |
| `pkg/pb/ca/*`, `pkg/pb/bank/*` | Regenerate bằng protoc (`--go_opt=module=mini_banking/pkg`) | Generated code Go khớp proto mới |
| `api-gateway/src/proto/ca.ts`, `bank.ts` | Regenerate bằng ts-proto, opts `outputServices=grpc-js,esModuleInterop=true,useOptionals=messages,env=node` | Client TS khớp proto mới; diff thuần additive so với bản cũ (đã so khớp style Buffer/optional) |

### 1.2. CA Service (Go)

| File | Sửa gì | Để làm gì |
|---|---|---|
| `ca-service/internal/ca/service.go` | Thêm struct `AuditFilter`; thêm `ListAudit` vào interface `Repository`; thêm `Service.ListAuditEvents` (validate action thuộc 5 enum, `to >= from`, limit 1–100 default 20); thay 4 chỗ `_ = s.repository.AppendAudit(...)` bằng helper `s.appendAudit(...)` | Read API audit ở tầng service; insert audit lỗi giờ log warning `[CA] warning: cannot append audit event...` thay vì nuốt im lặng (audit vẫn không làm fail request chính) |
| `ca-service/internal/ca/postgres_store.go` | Implement `ListAudit`: COUNT + SELECT trên `certificate_audit_log`, filter serial ILIKE / action = / performed_by ILIKE / `performed_at` khoảng nửa mở `[from, to)`, sort `performed_at DESC`, LIMIT/OFFSET (helper `auditListWhere` theo đúng pattern `certificateListWhere` sẵn có) | Đọc audit khi CA chạy Postgres; tận dụng index sẵn có trên serial/action/performed_at |
| `ca-service/internal/ca/store.go` | Implement `ListAudit` filter in-memory trên `auditLog`, cùng ngữ nghĩa filter/sort/pagination với Postgres | JSON store vẫn thỏa interface `Repository` — cả 2 backend đều đọc được audit |
| `ca-service/internal/grpc/handler.go` | Thêm handler `ListAuditEvents`; thêm helper `requestIDFromContext` đọc gRPC metadata `x-request-id`; `RegisterUser`/`GetCertificateDetail`/`RevokeCertificate` truyền trace id từ metadata thay vì `""` | Expose gRPC audit; trace id từ Gateway giờ được lưu thật vào `metadata.request_id` của event `issued`/`looked_up`/`revoked` mà không làm bẩn proto message |

### 1.3. Banking Service (Go)

| File | Sửa gì | Để làm gì |
|---|---|---|
| `banking-service/internal/bank/repository.go` | Thêm `AuditRecord`, `AuditListFilter`, `Repository.ListAudit` (COUNT + SELECT trên `bank_audit_log`, filter bằng `($n = '' OR col = ...)`, user_id cast qua `NULLIF(...)::uuid` để filter rỗng không lỗi cast, sort `created_at DESC`) | Query read-only audit đúng schema, nullable columns trả về chuỗi rỗng qua COALESCE |
| `banking-service/internal/bank/audit.go` | Thêm map `auditActions` (mirror CHECK constraint DB); `Service.Audit` log warning khi insert fail thay vì `_ =`; thêm `Service.ListAuditEvents` (validate action thuộc enum → `ErrBadRequest` → gRPC InvalidArgument → HTTP 400, validate range, limit 1–100) | Chặn rủi ro "action ngoài enum insert fail âm thầm" ở tầng đọc, và mất event giờ nhìn thấy được qua log `[BANK] warning: cannot insert audit event...` |
| `banking-service/internal/grpc/handler.go` | Thêm handler `ListAuditEvents` | Expose gRPC audit. RPC này chủ đích **không** đòi ticket/AP exchange — admin authn/authz làm ở Gateway, Bank chỉ reachable trên mạng TLS nội bộ (đã ghi comment trust boundary trong proto + handler) |
| `banking-service/internal/grpc/auth.go` | Thêm helper `authScopeMeta`; 12 event auth-layer (`invalid_ticket`, `ticket_expired`, `wrong_scope`, `invalid_authenticator` ×2, `authenticator_client_mismatch`, `authenticator_missing_fields`, `stale_request`, `ca_unavailable`, `certificate_not_active`, `certificate_owner_mismatch`, `redis_replay`, `db_replay`) giờ kèm `Metadata: {"scope": ...}` | Sửa điểm gây hiểu nhầm: auth fail của balance/history trước đây ghi action `transfer_rejected` không phân biệt — giờ đọc `metadata.scope` biết là flow nào, không cần đổi enum DB (không cần migration) |
| `banking-service/internal/grpc/handler_test.go` | Thêm `mockCA.ListAuditEvents` stub | mockCA tiếp tục thỏa interface `CAServiceClient` mới |

### 1.4. API Gateway (TypeScript)

| File | Sửa gì | Để làm gì |
|---|---|---|
| `api-gateway/src/middleware/admin.middleware.ts` (mới) | `ensureRequestId` (lấy `X-Request-ID` client gửi hoặc tự sinh UUID, echo lại response header); `requireAdmin(roles)` (JWT ký `GATEWAY_JWT_SECRET` có claim `role` ∈ {`admin`, role được phép}, hoặc static token `GATEWAY_ADMIN_TOKEN` mức demo); helper `performedBy(req)` → `admin:<email>` | Admin auth tối thiểu cho `/v1/admin/*` — dùng chung được cho route của Thanh/Thái |
| `api-gateway/src/services/admin-audit.service.ts` (mới) | Client wrapper `listCaAuditEvents`, `listBankAuditEvents` qua TLS gRPC; gắn trace id vào gRPC `Metadata` key `x-request-id` | Theo pattern service client sẵn có + quy ước trace-qua-metadata |
| `api-gateway/src/services/ca.service.ts`, `controller/ca.controller.ts` | `registerUser` nhận thêm `requestId` và gắn vào gRPC metadata; controller truyền `X-Request-ID` của request đăng ký xuống | Audit event `issued` của CA giờ có `metadata.request_id` thật từ Gateway (trước đây luôn rỗng) |
| `api-gateway/src/controller/admin-audit.controller.ts` (mới) | Validate query bằng zod: action đúng enum từng service, `user_id` phải UUID, `limit` 1–100 default 20, `from`/`to` ISO 8601 → unix; map camelCase gRPC → snake_case JSON; envelope `{success, data:{items,total,limit,offset}, request_id, timestamp}`; lỗi gRPC ném cho `errorHandler` sẵn có map sang HTTP | Contract REST đúng chuẩn §3.4 của plan, thống nhất với response shape Thái dùng |
| `api-gateway/src/routes/admin-audit.route.ts` (mới) + `server.ts` | Mount `GET /v1/admin/audit/ca` (role `ca_admin`) và `GET /v1/admin/audit/bank` (role `bank_admin`) dưới `/v1/admin/audit` | Endpoint đọc audit chạy được qua Gateway |
| `api-gateway/src/config/env.ts` | Thêm `GATEWAY_ADMIN_TOKEN` (optional), `GATEWAY_ADMIN_EMAIL` (default `admin@demo.local`), `BANK_GRPC_ADDR` (default `localhost:50053`) | Config cho admin demo; BANK addr vào schema env thay vì chỉ `process.env` rải rác |

### 1.5. Tài liệu

- [docs/audit-testcases.md](mini-banking-app/docs/audit-testcases.md) (mới): chuẩn field audit CA/Bank, contract 2 endpoint, **16 testcase** dạng event → cách kích hoạt → nơi kiểm tra (có cột pass/fail/owner/note), 8 curl mẫu cho Quang, và danh sách "quyết định chủ đích không audit".

## 2. Những gì đã rà và kết luận giữ nguyên

- Cả 5 action CA và 7 action Bank đều đã có điểm ghi đúng từ trước — không thêm điểm ghi mới.
- `ClientID` là UUID (đã xác nhận) → `user_id` insert bình thường, không cần chuyển vào metadata.
- `ListCertificates`, `CreateUser`, balance/history thành công: chủ đích không audit (đã ghi vào docs).

## 3. Vấn đề đang block / cần người khác

| # | Vấn đề | Ảnh hưởng | Cần ai |
|---|---|---|---|
| 1 | **Chưa chạy được test end-to-end với DB thật.** `ListAudit` (Postgres CA + Bank) mới pass build/unit test có sẵn; SQL filter chưa chạy trên Postgres thật với data audit thật. | Có thể còn lỗi SQL runtime (đặc biệt cast `$5::timestamptz` với tham số NULL) | Cần stack local full (Quang) hoặc tự dựng Postgres để chạy regression §6 của plan — việc của Ngày 3 |
| 2 | **Admin token demo chưa có flow cấp phát.** `requireAdmin` chấp nhận JWT role admin nhưng chưa có `POST /v1/admin/auth` để cấp JWT đó (việc P0 của Thanh). Tạm thời demo bằng `GATEWAY_ADMIN_TOKEN` static — phải set env này thì mới gọi được endpoint. | Không set env → chỉ 401; UI admin chưa login được bằng tài khoản | Thanh (admin auth endpoint); đã thiết kế middleware để Thanh reuse |
| 3 | **CA `looked_up`/`revoked` chỉ có `performed_by`/`request_id` thật khi Gateway admin CA route ra đời.** Route list/detail/revoke REST là việc của Thanh; quy ước đã sẵn: truyền `performedBy(req)` qua field proto `performed_by`, còn trace id gắn vào gRPC `Metadata` key `x-request-id` (xem mẫu trong `admin-audit.service.ts` / `ca.service.ts`). | Trước đó event vẫn ghi nhưng rơi về `admin:unknown` | Thanh |
| 4 | **Endpoint bank audit đặt ở `/v1/admin/audit/bank`**, trong khi contract của Thái trong process.md là `/v1/admin/bank/audit`. Cả hai đều nằm trong danh sách "API audit đề xuất" — cần chốt 1 đường hoặc Thái mount alias gọi lại cùng controller. | UI Admin Bank của Thái cần trỏ đúng URL | Thái + Thuận chốt trong 15 phút; controller/service tái sử dụng được nguyên vẹn |
| 5 | **Bank `ListAuditEvents` không có auth ở tầng gRPC** (chủ đích, trust boundary là Gateway + TLS nội bộ). Nếu sau này Bank port 50053 bị expose ra ngoài mạng nội bộ thì đây là lỗ hổng đọc audit không cần auth. | Rủi ro chỉ khi deploy sai topology | Quang lưu ý khi viết compose: không publish port 50051/50052/50053 ra ngoài |
| 6 | **Frontend Audit tab chưa nối** — ngoài scope của Thuận; response shape đã cố định trong docs để Thanh/Thái nối thẳng. | — | Thanh/Thái (Ngày 3) |
| 7 | `ip`/`user_agent` trong metadata audit (chuẩn §3.3 mức best-effort) chưa truyền — proto hiện chỉ có `performed_by`/`request_id`. Nếu cần, thêm 2 field proto sau; không block demo. | Thiếu 2 field phụ trong audit admin | Để P2 |

## 4. Việc còn lại của Thuận (Ngày 3)

1. Chạy regression 16 testcase trong `docs/audit-testcases.md` trên stack full, điền pass/fail.
2. Xác nhận event hiển thị trên UI Admin CA/Bank sau khi Thanh/Thái nối API.
3. Chốt với Thái về URL bank audit (vấn đề #4), với Thanh về admin token (vấn đề #2).
