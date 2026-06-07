# Sprint 1 - Nhiệm vụ của Thái

## Scaffold là gì?

Scaffold = **dựng khung xương** — tạo cấu trúc file/folder/function với đủ signature và kết nối cơ bản để code **build được và chạy được**, nhưng chưa có business logic thật.

Ví dụ scaffold một route:
```typescript
// Scaffold: route tồn tại, nhận request, forward gRPC, trả response — nhưng chưa validate gì cả
app.post('/bank/transfer', async (req, res) => {
  const result = await bankClient.transferMoney(req.body)
  res.json(result)
})
```
Sprint sau mới implement validation, error handling, business logic thật bên trong.

---

## Task 1: Scaffold Bank Service route/client surface

**Yêu cầu:**
Scaffold Bank Service route/client surface trong Gateway và Bank Service contract theo `bank.proto`.

**Output mong đợi:**
Bank route skeleton và Bank gRPC client interface rõ ràng.

**Liên quan đến phần nào:**
- **API Gateway** — tạo route handlers cho các endpoint Bank: `POST /bank/transfer`, `GET /bank/accounts/:id/balance/query`, `GET /bank/accounts/:id/transactions/query`, `POST /pki/register` (phần gọi Bank `CreateUser`)
- **Bank Service** — tạo cấu trúc file Go cơ bản: gRPC server setup, handler stubs cho 4 RPC theo proto (`CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory`)
- **Không liên quan đến KDC hay CA** trong task này

**Cụ thể cần làm:**

Phía API Gateway (TypeScript):
- Khởi tạo `BankServiceClient` từ stub đã generate, kết nối đến Bank Service qua địa chỉ từ env
- Tạo route group bank, mount các endpoint đúng path theo blueprint
- Mỗi route handler: nhận REST request → chuyển sang gRPC request → gọi `bankClient` → trả response (chưa cần validate)

Phía Bank Service (Go):
- Tạo gRPC server boot được
- Tạo handler struct implement đủ 4 method: `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory`
- Mỗi method trả `Unimplemented` hoặc stub response — đủ để compile và nhận gRPC call

**File cần đọc:**

| File | Tại sao cần đọc |
|---|---|
| `proto/bank.proto` | Contract gốc: tên RPC, field request/response, kiểu dữ liệu |
| `mini-banking-app/api-gateway/src/proto/bank.ts` | TypeScript stubs đã gen — dùng `BankServiceClient` từ đây để tạo gRPC client |
| `mini-banking-app/api-gateway/src/proto/ca.ts` | Tham khảo cách CA client được dùng — pattern tương tự cho Bank |
| `mini-banking-app/api-gateway/package.json` | Xem dependencies hiện có (hiện chỉ có `ts-proto`; cần thêm `@grpc/grpc-js`, express/fastify nếu chưa có) |
| `mini-banking-app/banking-service/internal/configs/config.go` | Config hiện tại của Bank Service — bổ sung thêm `GRPCPort`, `KVSecret` nếu cần |
| `mini-banking-app/pkg/pb/bank/bank_grpc.pb.go` | Go interface `BankServiceServer` — implement đủ 4 method này |
| `blueprint/design.md` (phần Authorization matrix) | Xác định đúng HTTP method và path cho từng endpoint |

**File cần tạo:**

Phía API Gateway:

| File | Nội dung |
|---|---|
| `api-gateway/src/app.ts` | Khởi tạo Express app, mount route groups, apply middleware |
| `api-gateway/src/grpc-clients/bank.client.ts` | File này đã tồn tại nhưng **rỗng** — điền vào: khởi tạo `BankServiceClient`, kết nối địa chỉ Bank Service từ env |
| `api-gateway/src/routes/bank.routes.ts` | Route handlers: `POST /bank/transfer`, `GET /bank/accounts/:id/balance/query`, `GET /bank/accounts/:id/transactions/query` |
| `api-gateway/tsconfig.json` | TypeScript compiler config |

Phía Bank Service:

| File | Nội dung |
|---|---|
| `banking-service/go.mod` | Go module declaration (`module mini_banking/banking-service`) |
| `banking-service/cmd/server/main.go` | Entry point: load config, khởi tạo gRPC server, listen port |
| `banking-service/internal/grpc/server.go` | Đăng ký `BankHandler` vào gRPC server |
| `banking-service/internal/grpc/handler.go` | Struct `BankHandler` implement `BankServiceServer` với 4 method stub: `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory` |

---

## Task 2: Scaffold shared validation/error envelope/rate-limit hook

**Yêu cầu:**
Scaffold shared validation/error envelope/rate-limit hook cho Bank/Admin-sensitive APIs.

**Output mong đợi:**
Gateway có middleware nền dùng lại được.

**Liên quan đến phần nào:**
- **API Gateway** hoàn toàn — middleware chạy trong Gateway trước khi forward sang CA/KDC/Bank
- **Không động vào KDC, CA, Bank Service** — các service đó không biết middleware này tồn tại

**Cụ thể cần làm:**

3 thứ cần scaffold:

**1. Error envelope** — mọi response lỗi đều có cùng format:
```json
{ "error": { "code": "INVALID_REQUEST", "message": "..." } }
```

**2. Request validation hook** — middleware kiểm tra các field bắt buộc trước khi forward:
- Binary/base64 fields (`ticket_v`, `authenticator`, `cipher_payload`) không được rỗng
- `request_id` phải là UUID hợp lệ
- Reject sớm → không tốn gRPC call khi input sai

**3. Rate-limit hook** — middleware đếm request theo IP hoặc user:
- Scaffold cấu trúc: nhận request → kiểm tra counter → cho qua hoặc trả 429
- Sprint 1 có thể dùng in-memory counter, Sprint sau nối Redis

**File cần đọc:**

| File | Tại sao cần đọc |
|---|---|
| `mini-banking-app/api-gateway/src/proto/bank.ts` | Xem các interface (`TransferRequest`, `BalanceRequest`...) để biết field nào cần validate |
| `mini-banking-app/api-gateway/package.json` | Xem đã có thư viện validation/rate-limit chưa (hiện chỉ có `ts-proto`) |
| `blueprint/design.md` (phần Authorization matrix) | Biết route nào cần rate-limit, route nào cần auth check |
| `blueprint/design.md` (phần Error handling security rule) | Quy tắc: fail closed, không lộ lý do nội bộ, audit event cho lỗi nhạy cảm |

**File cần tạo:**

| File | Nội dung |
|---|---|
| `api-gateway/src/middleware/error-envelope.ts` | Express error handler middleware — bắt mọi lỗi, format thành `{ error: { code, message } }`, không lộ stack trace |
| `api-gateway/src/middleware/validate.ts` | Middleware validate request — kiểm tra field bắt buộc, `request_id` UUID hợp lệ, binary fields không rỗng; trả 400 nếu sai |
| `api-gateway/src/middleware/rate-limit.ts` | Middleware rate-limit — đếm request theo IP; Sprint 1 dùng in-memory Map, Sprint sau thay bằng Redis |
