# Task 1 Report: Scaffold Bank Service route/client surface

## Kết quả tổng quan

| Hạng mục | Kết quả |
|---|---|
| `go build ./...` trong `banking-service/` | PASS |
| `tsc --noEmit` trong `api-gateway/` | PASS (sau 1 lần fix) |
| Bank Service boot được | Có — log `[Bank] gRPC server listening on :50053` |
| API Gateway boot được với Bank routes | Có — routes mount tại `/v1/bank/*` |

---

## Mặt trận B — Bank Service (Go)

### B1: go.mod — DONE

**File tạo:** `banking-service/go.mod`

Module name: `mini-banking/banking-service` (theo pattern của `ca-service`).
Replace directive: `mini-banking/pkg => ../pkg` để link local pkg stubs.
Dependencies thêm: `github.com/joho/godotenv v1.5.1`, `google.golang.org/grpc v1.81.1`.

### B2: handler.go — DONE

**File tạo:** `banking-service/internal/grpc/handler.go`

Struct `Handler` embed `pb.UnimplementedBankServiceServer`. 4 method stubs trả `codes.Unimplemented`:
- `CreateUser`
- `TransferMoney`
- `GetBalance`
- `GetHistory`

### B3: server.go — DONE

**File tạo:** `banking-service/internal/grpc/server.go`

gRPC server không dùng mTLS (theo ADR-02 — network isolation). Đăng ký:
- `pb.RegisterBankServiceServer` — handler
- `grpc_health_v1` — health check
- `reflection` — debug với grpcurl

Method `Start()` listen TCP, `Stop()` graceful stop.

### B4: main.go + cập nhật config — DONE

**File tạo:** `banking-service/cmd/server/main.go`

Pattern: load config → khởi tạo Handler → khởi tạo Server → goroutine start → chờ SIGINT/SIGTERM → graceful stop.

**File cập nhật:** `banking-service/internal/configs/config.go`
- Thêm field `GRPCPort string` vào struct `Config`
- Đọc từ env `BANK_GRPC_PORT`, default `50053`
- Đổi `log.Fatal` khi thiếu `POSTGRES_CONNECTION_STRING` thành `log.Println` — DB chưa cần ở Sprint 1

### B5: Verify — PASS

```
go build ./...  →  (no output = success)
```

---

## Mặt trận A — API Gateway (TypeScript)

### A1: tsconfig.json + app.ts — DONE

**File tạo:** `api-gateway/tsconfig.json`

Target ES2020, module commonjs, strict mode, rootDir `./src`, outDir `./dist`.

**File tạo:** `api-gateway/src/app.ts`

Express app, mount `bankRouter` tại `/v1`, health endpoint `/health`, graceful shutdown khi `SIGTERM`.

### A2: bank.client.ts — DONE

**File điền:** `api-gateway/src/grpc-clients/bank.client.ts` (trước đó rỗng)

Import `BankServiceClient` từ proto stub, khởi tạo với địa chỉ từ `BANK_SERVICE_ADDR` (default `localhost:50053`), credentials insecure (không mTLS). Export singleton `bankClient`.

### A3: bank.routes.ts — DONE

**File tạo:** `api-gateway/src/routes/bank.routes.ts`

| Route | gRPC method |
|---|---|
| `POST /v1/bank/transfer` | `bankClient.transferMoney` |
| `GET /v1/bank/accounts/:id/balance/query` | `bankClient.getBalance` |
| `GET /v1/bank/accounts/:id/transactions/query` | `bankClient.getHistory` |

Export thêm `callCreateUser(req)` — helper để PKI register route (của Thuận) gọi `CreateUser` sau enrollment.

Chưa có validation — trả `500` nếu gRPC fail.

### A4: Verify — PASS (sau 1 lần fix)

**Lỗi gặp:** `tsc` báo lỗi `Type 'string | string[]' is not assignable to type 'string'` tại `accountId: req.params.id` trong 2 route GET.

**Nguyên nhân:** `@types/express` phiên bản mới type `req.params` index signature trả `string | string[]`.

**Fix:** đổi `req.params.id` thành `String(req.params.id)`.

```
tsc --noEmit  →  (no output = success)
```

---

## Dependencies đã cài thêm (api-gateway)

```
npm install express @grpc/grpc-js @types/express @types/node typescript
```

117 packages, 0 vulnerabilities.

---

## File summary

| File | Trạng thái |
|---|---|
| `banking-service/go.mod` | Tạo mới |
| `banking-service/go.sum` | Tạo mới (go mod tidy) |
| `banking-service/internal/grpc/handler.go` | Tạo mới |
| `banking-service/internal/grpc/server.go` | Tạo mới |
| `banking-service/cmd/server/main.go` | Tạo mới |
| `banking-service/internal/configs/config.go` | Cập nhật — thêm `GRPCPort` |
| `api-gateway/tsconfig.json` | Tạo mới |
| `api-gateway/src/app.ts` | Tạo mới |
| `api-gateway/src/grpc-clients/bank.client.ts` | Điền vào (trước rỗng) |
| `api-gateway/src/routes/bank.routes.ts` | Tạo mới |

---

## Definition of Done — checklist

- [x] `go build ./...` trong `banking-service/` pass
- [x] `tsc --noEmit` trong `api-gateway/` pass
- [x] Bank Service có đủ 4 gRPC method stubs theo `bank.proto`
- [x] API Gateway có đủ 3 Bank REST routes mount tại `/v1`
- [x] `callCreateUser` helper export sẵn cho PKI register flow (Thuận)
- [ ] Smoke test gRPC call Gateway → Bank nhận `Unimplemented` — chưa test (cần Docker/service chạy cùng lúc)
