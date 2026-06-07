# Phân tích Task 1: Scaffold Bank Service route/client surface

Task 1 chia làm 2 mặt trận độc lập: **API Gateway (TypeScript)** và **Bank Service (Go)**.

---

## Mặt trận A — API Gateway (TypeScript)

### Bước A1: Dựng Express app + tsconfig

**Làm gì:**
- Tạo `api-gateway/tsconfig.json` — cấu hình TypeScript compiler
- Tạo `api-gateway/src/app.ts` — khởi tạo Express, listen port từ env

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/api-gateway/package.json` | Xem dependencies đã có; cần thêm `express`, `@grpc/grpc-js`, `@types/express` |
| `mini-banking-app/ca-service/cmd/server/main.go` | Pattern graceful shutdown — áp dụng tương tự cho Express với `process.on('SIGTERM')` |
| `.env.example` | Xem env vars đã quy định sẵn (port, service address) |

**File tạo ra:**
- `api-gateway/tsconfig.json`
- `api-gateway/src/app.ts`

---

### Bước A2: Tạo Bank gRPC client

**Làm gì:**
- Điền vào `api-gateway/src/grpc-clients/bank.client.ts` (file đang rỗng)
- Import `BankServiceClient` từ proto stub đã generate
- Khởi tạo client với địa chỉ Bank Service lấy từ env (`BANK_SERVICE_ADDR`)
- Export một singleton client để các route dùng chung

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/api-gateway/src/proto/bank.ts` (dòng 1479–1546) | `BankServiceClient` interface và constructor — copy pattern khởi tạo `new BankServiceClient(address, credentials)` |
| `mini-banking-app/api-gateway/src/proto/ca.ts` | Xem cách CA client được export — dùng cùng pattern cho Bank |
| `.env.example` | Tên env var cho địa chỉ Bank Service |

**File tạo ra:**
- `api-gateway/src/grpc-clients/bank.client.ts` (điền vào file rỗng)

---

### Bước A3: Tạo Bank route handlers

**Làm gì:**
- Tạo `api-gateway/src/routes/bank.routes.ts`
- 4 endpoint cần mount:

| HTTP | Path | gRPC method | Ghi chú |
|---|---|---|---|
| `POST` | `/v1/bank/transfer` | `bankClient.transferMoney` | Nhận `ticket_v`, `authenticator`, `cipher_payload`, `iv`, `request_id` |
| `GET` | `/v1/bank/accounts/:id/balance/query` | `bankClient.getBalance` | Nhận `ticket_v`, `authenticator`, `account_id`, `request_id` |
| `GET` | `/v1/bank/accounts/:id/transactions/query` | `bankClient.getHistory` | Nhận `ticket_v`, `authenticator`, `account_id`, `limit`, `offset`, `request_id` |
| (nội bộ) | gọi sau PKI register | `bankClient.createUser` | Gọi trong flow `/v1/pki/register`, không phải route độc lập |

- Mỗi handler: nhận JSON body → map sang gRPC request type → gọi bankClient → map response → trả JSON
- Chưa cần validate; chưa cần xử lý lỗi chi tiết — trả 500 nếu gRPC fail

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `proto/bank.proto` | Tên field chính xác của từng request/response message |
| `mini-banking-app/api-gateway/src/proto/bank.ts` (dòng 114–184) | TypeScript interface `TransferRequest`, `BalanceRequest`, `HistoryRequest`, `CreateUserRequest` — dùng để type request body |
| `blueprint/design.md` (phần Authorization matrix, dòng 182–191) | Xác nhận đúng HTTP method và path cho từng endpoint |
| `blueprint/design.md` (phần Flow 3, bước 4) | Hiểu `CipherPayload` và `IV` đến từ đâu để map đúng field |

**File tạo ra:**
- `api-gateway/src/routes/bank.routes.ts`

---

### Bước A4: Mount routes và verify build

**Làm gì:**
- Trong `app.ts`, import và mount `bank.routes` vào prefix `/v1`
- Chạy `tsc` — đảm bảo TypeScript compile không lỗi

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `api-gateway/src/app.ts` (vừa tạo ở A1) | Nơi mount route |

**Verify:** `tsc --noEmit` pass, không còn type error.

---

## Mặt trận B — Bank Service (Go)

### Bước B1: Tạo go.mod

**Làm gì:**
- Tạo `banking-service/go.mod`
- Module name phải là `mini_banking/banking-service` (khớp import path với `pkg/pb/bank`)
- Thêm dependencies: `google.golang.org/grpc`, `google.golang.org/protobuf`, `github.com/joho/godotenv`

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/ca-service/go.mod` | Module name pattern, version Go, dependency list để copy |
| `mini-banking-app/kdc-service/go.mod` | So sánh thêm để xác nhận version thống nhất |
| `mini-banking-app/pkg/go.mod` | Xem module name của pkg (`mini_banking/pkg`) để import đúng |

**File tạo ra:**
- `banking-service/go.mod`

---

### Bước B2: Tạo handler.go

**Làm gì:**
- Tạo `banking-service/internal/grpc/handler.go`
- Khai báo struct `Handler` embed `bankpb.UnimplementedBankServiceServer`
- Implement 4 method với stub body (trả `codes.Unimplemented` hoặc empty response)

```go
type Handler struct {
    bankpb.UnimplementedBankServiceServer
}

func (h *Handler) CreateUser(ctx context.Context, req *bankpb.CreateUserRequest) (*bankpb.CreateUserResponse, error) {
    return nil, status.Error(codes.Unimplemented, "not implemented")
}
// tương tự cho TransferMoney, GetBalance, GetHistory
```

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/pkg/pb/bank/bank_grpc.pb.go` (dòng 95–123) | `BankServiceServer` interface và `UnimplementedBankServiceServer` — biết chính xác 4 method signature cần implement |
| `mini-banking-app/pkg/pb/bank/bank.pb.go` | Struct của từng request/response type (`CreateUserRequest`, `TransferRequest`...) |
| `mini-banking-app/ca-service/internal/grpc/handler.go` | Pattern: struct embed Unimplemented, method nhận ctx+req, trả response+error |

**File tạo ra:**
- `banking-service/internal/grpc/handler.go`

---

### Bước B3: Tạo server.go

**Làm gì:**
- Tạo `banking-service/internal/grpc/server.go`
- Khởi tạo `grpc.NewServer()` (không cần mTLS ở Sprint 1 — network isolation)
- Gọi `bankpb.RegisterBankServiceServer(grpcSrv, handler)`
- Thêm health check: `grpc_health_v1.RegisterHealthServer`
- Method `Start()` listen TCP, method `Stop()` graceful stop

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/ca-service/internal/grpc/server.go` (dòng 69–99, 189–209) | Pattern `NewServer`, `Start`, `Stop` — copy và bỏ phần mTLS |
| `mini-banking-app/pkg/pb/bank/bank_grpc.pb.go` (dòng 132–141) | `RegisterBankServiceServer` — tên function và signature chính xác |
| `blueprint/design.md` (phần ADR-02) | Xác nhận: Bank Service không dùng mTLS, chỉ network isolation |

**File tạo ra:**
- `banking-service/internal/grpc/server.go`

---

### Bước B4: Tạo main.go

**Làm gì:**
- Tạo `banking-service/cmd/server/main.go`
- Load config từ `internal/configs/config.go` (bổ sung thêm field `GRPCPort`)
- Khởi tạo `Handler`, `Server`
- Graceful shutdown khi nhận `SIGINT`/`SIGTERM`

**Tham khảo:**
| Nguồn | Lấy gì |
|---|---|
| `mini-banking-app/ca-service/cmd/server/main.go` | Pattern toàn bộ: load config → init dependencies → start server → graceful shutdown — copy và đơn giản hóa |
| `mini-banking-app/banking-service/internal/configs/config.go` | Config hiện tại chỉ có `PostgresDSN` — cần bổ sung `GRPCPort` vào struct và `LoadConfig()` |
| `.env.example` | Tên env var cho port Bank Service |

**File tạo ra:**
- `banking-service/cmd/server/main.go`
- Cập nhật: `banking-service/internal/configs/config.go` (thêm `GRPCPort`)

---

### Bước B5: Verify build

**Làm gì:**
- Chạy `go build ./...` trong `banking-service/`
- Đảm bảo không còn lỗi compile

**Verify:** Build pass. Chạy binary lên thì server listen port và log `[Bank] gRPC server listening on :PORT`.

---

## Thứ tự thực hiện đề xuất

```
B1 (go.mod) → B2 (handler) → B3 (server) → B4 (main) → B5 (verify Go)
A1 (app+tsconfig) → A2 (grpc client) → A3 (routes) → A4 (mount + verify TS)
```

B1–B5 và A1–A4 độc lập nhau — có thể làm song song nếu muốn.

---

## Definition of Done cho Task 1

- `go build ./...` trong `banking-service/` pass
- `tsc --noEmit` trong `api-gateway/` pass
- Bank Service khởi động được, log listening port
- API Gateway khởi động được, route `/v1/bank/transfer` nhận POST request (dù chưa xử lý đúng)
- Gọi gRPC từ Gateway sang Bank Service nhận được response `Unimplemented` — chứng minh kết nối thông
