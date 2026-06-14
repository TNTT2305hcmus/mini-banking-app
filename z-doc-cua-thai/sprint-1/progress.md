# Sprint 1 — Progress Report

**Ngày:** 2026-06-07  
**Sprint Goal:** Tạo baseline code chạy được sau gen-proto, chốt contract module để Sprint 2 không bị nghẽn.

---

## Tóm tắt quá trình thực hiện

Sprint 1 tập trung vào việc dựng skeleton cho toàn bộ hệ thống sau khi proto được regenerate. Các task được phân theo owner: Thanh (CA), Quang (KDC), Thuận (Gateway skeleton + Customer routes), Thái (Bank Service + Gateway middleware).

Hiện tại **2/4 service build được**. CA và KDC còn lỗi compile do field/enum proto cũ chưa được sửa. Phần Thái phụ trách (Bank Service + Gateway middleware) đã hoàn thành và verify.

---

## Đã hoàn thành

### Thái — Bank Service Scaffold (Task 1) ✅

`go build ./...` PASS.

| File | Nội dung |
|---|---|
| `banking-service/go.mod` | Module `mini-banking/banking-service`, link pkg local |
| `banking-service/internal/grpc/handler.go` | 4 RPC stubs: `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory` |
| `banking-service/internal/grpc/server.go` | gRPC server, health check, reflection, graceful stop |
| `banking-service/cmd/server/main.go` | Boot: load config → handler → server → SIGTERM |
| `banking-service/internal/configs/config.go` | Thêm `GRPCPort` (default `50053`) |

### Thái — API Gateway Bank Routes (Task 1) ✅

`tsc --noEmit` PASS.

| File | Nội dung |
|---|---|
| `api-gateway/tsconfig.json` | TypeScript config, strict mode |
| `api-gateway/src/app.ts` | Express app, mount `/v1`, graceful shutdown |
| `api-gateway/src/grpc-clients/bank.client.ts` | `BankServiceClient` singleton, insecure credentials |
| `api-gateway/src/routes/bank.routes.ts` | 3 routes: `POST /bank/transfer`, `GET .../balance/query`, `GET .../transactions/query`; export `callCreateUser()` |

### Thái — Gateway Middleware (Task 2) ✅

`tsc --noEmit` PASS.

| File | Nội dung |
|---|---|
| `api-gateway/src/middleware/error-envelope.ts` | `AppError` class + Express error handler, map gRPC → HTTP, không lộ stack trace |
| `api-gateway/src/middleware/validate.ts` | `requireBodyFields` / `requireQueryFields`, check UUID v4 cho `request_id`, non-empty cho base64 fields |
| `api-gateway/src/middleware/rate-limit.ts` | In-memory Map theo IP, 60 req/phút, cleanup tự động |

### Tiền đề sẵn có (trước Sprint 1)

| Thành phần | Trạng thái |
|---|---|
| `proto/bank.proto`, `ca.proto`, `kdc.proto` | Đã có, đã gen stubs |
| `pkg/pb/bank`, `pkg/pb/ca`, `pkg/pb/kdc` | Go stubs đã gen |
| `api-gateway/src/proto/bank.ts`, `ca.ts`, `kdc.ts` | TypeScript stubs đã gen |
| KDC Service logic (AS/TGS handler) | Đã implement, chỉ còn 1 lỗi enum |
| CA Service logic (RegisterUser, GetCertificate...) | Đã implement, chỉ còn lỗi field name |
| DB migration SQL | `001_init_bank.sql`, `001_init_ca.sql` đã có |

---

## Đang thực hiện

### Quang — Fix KDC enum (Sprint 1, chưa xong)

**Lỗi hiện tại:**
```
kdc-service/internal/kdc/service.go:188:
undefined: capb.CertStatus_CERT_STATUS_VALID
```

**Nguyên nhân:** `ca.proto` đã đổi `CERT_STATUS_VALID = 1` thành `CERT_STATUS_ACTIVE = 1`. File `kdc-service/internal/kdc/service.go:188` vẫn dùng tên cũ trong hàm `mapCACertStatus`.

**Fix cần làm:** Đổi `capb.CertStatus_CERT_STATUS_VALID` → `capb.CertStatus_CERT_STATUS_ACTIVE` tại dòng 188.

### Thanh — Fix CA handler field name (Sprint 1, chưa xong)

**Lỗi hiện tại:**
```
ca-service/internal/grpc/handler.go:59,63,70:
req.UserId undefined (type *capb.RegisterUserRequest has no field or method UserId)
```

**Nguyên nhân:** `ca.proto` đã đổi `RegisterUserRequest.user_id` → `owner_id`. Handler còn dùng `req.UserId` (3 chỗ).

**Fix cần làm:** Đổi `req.UserId` → `req.OwnerId` tại 3 dòng 59, 63, 70 trong `handler.go`. Ngoài ra cần stub các RPC mới chưa có handler: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail`.

### Thuận — API Gateway CA/KDC client + Customer routes (Sprint 1, chưa bắt đầu)

Các task Thuận chưa thực hiện:
- Khởi tạo `ca.client.ts` và `kdc.client.ts` (file proto stub đã có nhưng chưa có client singleton)
- Tạo `src/routes/otp.routes.ts` — `POST /otp/request`, `POST /otp/verify`
- Tạo `src/routes/pki.routes.ts` — `POST /pki/register` (gọi CA → Bank `CreateUser`)
- Tạo `src/routes/auth.routes.ts` — `POST /auth/as-req`, `POST /auth/tgs-req`
- Mount các route trên vào `app.ts`

---

## Chưa hoàn thành

Theo thứ tự ưu tiên để unblock Sprint 2:

| # | Task | Owner | Unblock |
|---|---|---|---|
| 1 | Fix KDC `CERT_STATUS_VALID` → `CERT_STATUS_ACTIVE` (1 dòng) | Quang | KDC build, KDC tests |
| 2 | Fix CA `req.UserId` → `req.OwnerId` (3 dòng) | Thanh | CA build, CA tests |
| 3 | Stub CA handler methods còn thiếu: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail` | Thanh | CA build hoàn chỉnh |
| 4 | Tạo `ca.client.ts`, `kdc.client.ts` trong Gateway | Thuận | OTP/PKI/Auth routes |
| 5 | Tạo OTP/PKI/Auth route skeleton trong Gateway | Thuận | Gateway smoke test |
| 6 | Mount CA/KDC routes vào `app.ts` | Thuận | End-to-end boot |

---

## Vấn đề còn tồn tại

### Bug / Build blocker

| Vấn đề | File | Severity |
|---|---|---|
| `CertStatus_CERT_STATUS_VALID` không tồn tại | `kdc-service/internal/kdc/service.go:188` | HIGH — KDC không build |
| `req.UserId` không tồn tại | `ca-service/internal/grpc/handler.go:59,63,70` | HIGH — CA không build |
| CA handler thiếu `VerifyCertificate` stub | `ca-service/internal/grpc/handler.go` | HIGH — KDC gọi CA sẽ fail khi runtime |

### Technical debt

| Vấn đề | Mô tả |
|---|---|
| Module naming inconsistent | `ca-service` dùng `mini-banking/pkg` (hyphen), `kdc-service` dùng `mini_banking/pkg` (underscore), `banking-service` theo pattern `mini-banking`. Cần thống nhất 1 convention |
| Banking config dùng `godotenv` | `banking-service/internal/configs/config.go` dùng `godotenv.Load()` khác với CA pattern (đọc env trực tiếp). Không nhất quán nhưng không block |
| `log.Fatal` khi thiếu `POSTGRES_CONNECTION_STRING` | Đã đổi thành `log.Println` để scaffold chạy được. Cần đổi lại thành fatal khi Bank Service bắt đầu dùng DB thật ở Sprint 3 |
| Rate limit dùng in-memory | `rate-limit.ts` dùng `Map` in-memory, không share state giữa nhiều instance Gateway. Cần đổi sang Redis ở Sprint 2 khi OTP rate limit strict cần thiết |

### Hạn chế hiện tại

| Hạn chế | Tác động |
|---|---|
| Bank Service 4 RPC đều trả `Unimplemented` | Gateway → Bank thông kết nối nhưng mọi request đều fail `501` |
| Chưa có Admin routes | Admin Dashboard chưa có REST surface |
| Chưa có OTP/PKI/Auth routes | Không thể test đăng ký hay đăng nhập end-to-end |
| Smoke test Gateway → Bank chưa chạy | Cần Docker compose chạy cả 2 service đồng thời |
| CA Service còn dùng JSON store | Chưa migrate sang PostgreSQL — Admin dashboard và audit log chưa đạt blueprint |

### Sprint 1 Definition of Done — checklist

| Tiêu chí | Trạng thái |
|---|---|
| CA không còn lỗi compile do field proto cũ | ❌ `req.UserId` chưa fix |
| KDC không còn lỗi compile do enum proto cũ | ❌ `CERT_STATUS_VALID` chưa fix |
| API Gateway có skeleton route groups cho CA/KDC/Bank | ⚠️ Bank xong, CA/KDC chưa |
| Bank Service có contract baseline theo proto | ✅ |
| Gap còn lại được ghi rõ | ✅ (file này) |
