# Báo cáo: Luồng API Gateway ↔ Bank gRPC và khởi động Bank Service

> Phạm vi: `mini-banking-app/api-gateway`, `mini-banking-app/proto/bank.proto`, `mini-banking-app/banking-service`, `mini-banking-app/db/bank/migrations/001_init_bank.sql`.

## 1. Tổng quan

Bank Service là service nội bộ xử lý user ngân hàng, tài khoản, chuyển tiền, truy vấn số dư/lịch sử và ledger. Client không gọi trực tiếp Bank Service; mọi request đi theo luồng:

```text
Customer Web App
  -> HTTPS/REST
API Gateway
  -> gRPC BankService
Bank Service
  -> Bank DB / Redis / CA Service
```

Trạng thái hiện tại: API Gateway đã mount route Bank và forward sang gRPC, nhưng `banking-service` mới là skeleton. gRPC server đăng ký đủ 4 RPC nhưng handler đang trả `codes.Unimplemented`.

## 2. REST API tại API Gateway

Gateway mount `bankRouter` trong `server.ts`:

```text
app.use("/v1", bankRouter)
```

Các endpoint Bank hiện có:

| REST endpoint | Middleware | gRPC gọi sang Bank | Dữ liệu chuyển tiếp |
|---|---|---|---|
| `POST /v1/bank/transfer` | `rateLimitBankByIP`, `validateTransferRequest` | `TransferMoney` | `ticket_v`, `authenticator`, `cipher_payload`, `iv` base64 -> `bytes` |
| `POST /v1/bank/accounts/:id/balance/query` | `rateLimitBankByIP`, `validateBalanceQuery` | `GetBalance` | `ticket_v`, `authenticator`, `account_id` |
| `POST /v1/bank/accounts/:id/transactions/query` | `rateLimitBankByIP`, `validateHistoryQuery` | `GetHistory` | `ticket_v`, `authenticator`, `account_id`, `limit`, `offset` |
| Nội bộ sau `/v1/pki/register` | `pki.controller.ts` | `CreateUser` | `email`, `full_name` |

Các read API dùng `POST` và set `Cache-Control: no-store`, đúng mục tiêu không đưa `Ticket_v`/`Authenticator` lên URL.

## 3. Validate và rate limit tại Gateway

`bank.middleware.ts` kiểm tra trước khi gọi gRPC:

| Nhóm | Kiểm tra |
|---|---|
| Transfer body | `ticket_v`, `authenticator`, `cipher_payload`, `iv`, `request_id` |
| Read body | `ticket_v`, `authenticator`, `request_id` |
| Account id | UUID path param |
| History pagination | `limit` từ 1 đến 100, `offset >= 0` |
| IV transfer | base64 decode đúng 12 bytes |

`rateLimiter.ts` giới hạn nhóm `/v1/bank/*` theo IP bằng Redis key `BANK_RATE_LIMIT`. Comment ghi 30 req/phút, code hiện cấu hình `{ window: 60, max: 20 }`.

Lưu ý hiện tại: `request_id` được validate ở REST body nhưng chưa forward sang gRPC vì `bank.proto` chưa có field `request_id`; Bank chỉ có thể lấy request id nếu nó nằm trong Authenticator đã mã hóa, hoặc nếu Gateway bổ sung gRPC metadata sau này.

## 4. gRPC client tại Gateway

Hiện Gateway có 2 Bank gRPC client khác nhau:

| File | Dùng cho | Địa chỉ env | Credential |
|---|---|---|---|
| `grpc-clients/bank.client.ts` | `TransferMoney`, `GetBalance`, `GetHistory`, `callCreateUser` trong `bank.routes.ts` | `BANK_SERVICE_ADDR`, default `localhost:50053` | `grpc.credentials.createInsecure()` |
| `services/bank.service.ts` | `createUser` trong `pki.controller.ts` | `BANK_GRPC_ADDR`, default `localhost:50053` | `sslCredentials` từ `config/grpc.ts` |

Theo blueprint/ADR-02, đường nội bộ phải dùng gRPC + TLS một chiều. Vì vậy client `createInsecure()` là điểm lệch thiết kế và nên được gộp về một client TLS duy nhất.

## 5. Contract `bank.proto`

`BankService` định nghĩa 4 RPC:

| RPC | Request | Response | Vai trò |
|---|---|---|---|
| `CreateUser` | `email`, `full_name` | `user_id`, `status`, `created_at_unix` | Tạo user Bank sau khi CA cấp cert |
| `TransferMoney` | `ticket_v`, `authenticator`, `cipher_payload`, `iv` | `ap_rep`, `transaction_id` | AP Exchange chuyển tiền |
| `GetBalance` | `ticket_v`, `authenticator`, `account_id` | `ap_rep`, account metadata, balance | Xem số dư |
| `GetHistory` | `ticket_v`, `authenticator`, `account_id`, `limit`, `offset` | `ap_rep`, transaction list, pagination | Xem lịch sử |

Gateway chỉ base64-decode dữ liệu opaque rồi forward. Bank Service mới là nơi giải mã `Ticket_v`, Authenticator và payload.

## 6. Luồng giao tiếp API -> gRPC

### 6.1. Transfer

```text
Client
  POST /v1/bank/transfer
  body: ticket_v, authenticator, cipher_payload, iv, request_id

Gateway
  1. rate limit theo IP
  2. validate base64, UUID request_id, IV 12 bytes
  3. decode base64 -> Buffer
  4. gọi BankService.TransferMoney(...)

Bank Service Core
  1. giải mã Ticket_v bằng K_v
  2. verify scope = transfer:create, TTL, service_id
  3. giải mã Authenticator bằng K_{c,v}
  4. freshness + replay check Redis/used_nonces
  5. verify cert qua CA Service
  6. giải mã cipher_payload
  7. verify client_signature
  8. ownership + business rules
  9. DB transaction: update balances + append ledger
  10. trả AP_REP + transaction_id

Gateway
  trả JSON response cho client
```

### 6.2. Balance / History

```text
Client
  POST /v1/bank/accounts/{id}/balance/query
  POST /v1/bank/accounts/{id}/transactions/query

Gateway
  1. rate limit
  2. validate account_id, ticket_v, authenticator, request_id
  3. set Cache-Control: no-store
  4. gọi GetBalance/GetHistory

Bank Service Core
  1. verify Ticket_v + Authenticator
  2. freshness + replay check
  3. verify cert qua CA
  4. check scope: balance:read hoặc history:read
  5. check account ownership: account.user_id == ID_c
  6. query DB và trả metadata cần thiết
```

### 6.3. CreateUser sau PKI register

```text
Client -> Gateway
  POST /v1/pki/register

Gateway pki.controller
  1. verify registration_token
  2. gọi CA RegisterUser(csr_pem, ownerId)
  3. nếu CA cấp cert thành công -> gọi Bank CreateUser(email, fullName)
  4. trả cert cho client
```

Điểm lệch thiết kế hiện tại: blueprint yêu cầu `users.id = owner_id = ID_c`, nhưng proto `CreateUserRequest` chưa nhận `user_id`; Bank sẽ phải tự sinh `user_id` nếu implement theo schema DB hiện tại. Cần sửa contract hoặc thống nhất lại ID trước khi implement nghiệp vụ.

## 7. Error flow Gateway <- Bank

`errorHandler.ts` bắt lỗi gRPC có `err.code` dạng số và map sang HTTP:

| gRPC status | HTTP | Default `error_code` |
|---|---|---|
| `INVALID_ARGUMENT` | 400 | `BAD_REQUEST` |
| `UNAUTHENTICATED` | 401 | `UNAUTHENTICATED` |
| `PERMISSION_DENIED` | 403 | `FORBIDDEN` |
| `NOT_FOUND` | 404 | `NOT_FOUND` |
| `ALREADY_EXISTS` | 409 | `ALREADY_EXISTS` |
| `FAILED_PRECONDITION` | 422 | `UNPROCESSABLE_ENTITY` |
| `RESOURCE_EXHAUSTED` | 429 | `RATE_LIMITED` |
| `UNAVAILABLE` / `DEADLINE_EXCEEDED` | 503 | `SERVICE_UNAVAILABLE` |
| khác | 500 | `INTERNAL_ERROR` |

Nếu Bank Service set trailing metadata key `error-code`, Gateway sẽ relay mã domain đó thay vì default. Đây là contract cần Bank Service tuân thủ để trả đúng các mã như `INVALID_TICKET`, `WRONG_SCOPE`, `INSUFFICIENT_FUNDS`.

## 8. Khởi động Bank Service hiện tại

Entry point: `banking-service/cmd/server/main.go`.

```text
main()
  1. log "[Bank] Starting Bank Service..."
  2. cfg := config.LoadConfig()
  3. log grpc_port
  4. handler := bankgrpc.NewHandler()
  5. server := bankgrpc.NewServer(handler, cfg.GRPCPort)
  6. chạy server.Start() trong goroutine
  7. chờ một trong hai sự kiện:
     - server trả error -> log stderr, exit(1)
     - SIGINT/SIGTERM -> server.Stop()
  8. log "[Bank] Bank Service stopped."
```

`LoadConfig()`:

| Config | Nguồn | Default |
|---|---|---|
| `POSTGRES_CONNECTION_STRING` | env / `.env` | không có default, chỉ log nếu thiếu |
| `BANK_GRPC_PORT` | env / `.env` | `50053` |

Hiện config chưa được dùng để mở kết nối DB. Chưa có config cho Redis, CA gRPC address, `K_v`, TLS cert, timeout.

## 9. Khởi động gRPC Bank Service

`internal/grpc/server.go` làm các bước:

```text
NewServer(handler, port)
  1. grpcSrv := grpc.NewServer()
  2. pb.RegisterBankServiceServer(grpcSrv, handler)
  3. đăng ký grpc health service
  4. set health status "bank.BankService" = SERVING
  5. set health status "" = SERVING
  6. bật server reflection
  7. trả Server{grpcServer, port}

Start()
  1. net.Listen("tcp", ":" + port)
  2. log "[Bank] gRPC server listening on :{port}"
  3. grpcServer.Serve(lis)

Stop()
  1. grpcServer.GracefulStop()
```

Điểm cần lưu ý: server hiện dùng `grpc.NewServer()` không TLS. Theo thiết kế, Bank gRPC server cần TLS một chiều để Gateway verify server certificate.

## 10. Bank Service Core

Trong code hiện tại, `internal/bank/` đang trống nên chưa có Bank Service Core thật sự. Handler hiện tại:

```text
CreateUser     -> Unimplemented
TransferMoney  -> Unimplemented
GetBalance     -> Unimplemented
GetHistory     -> Unimplemented
```

Core nên là lớp nghiệp vụ không phụ thuộc trực tiếp HTTP/gRPC. gRPC handler chỉ nên map protobuf request/response và gọi core.

Khởi động mục tiêu nên là:

```text
main()
  load config
  open PostgreSQL connection
  open Redis client
  load Bank master key K_v
  create CA gRPC client
  create repositories/cache adapters
  create Bank Core service
  create gRPC handler with Core dependency
  create TLS gRPC server
  serve and graceful shutdown
```

Core cần các dependency chính:

| Dependency | Vai trò |
|---|---|
| Bank DB repositories | `users`, `accounts`, `transactions`, `used_nonces`, `bank_audit_log`, `ledger_state` |
| Redis replay cache | `SET NX EX` chống replay |
| Redis revocation cache | cache trạng thái cert ngắn hạn |
| CA gRPC client | `VerifyCertificate(cert_sn)` để lấy status + public key |
| Crypto/ticket module | giải mã `Ticket_v`, Authenticator, CipherPayload, tạo AP_REP |
| Clock/ID generator | timestamp, freshness window, transaction id |

## 11. Bank DB mà Core sẽ dùng

Migration hiện có các bảng:

| Bảng | Core dùng để làm gì |
|---|---|
| `users` | tạo user khi enrollment thành công; dùng làm `ID_c` nếu contract được sửa đúng |
| `accounts` | kiểm tra ownership, status, số dư, hạn mức; cập nhật balance |
| `transactions` | immutable ledger, lưu signature/hash/idempotency |
| `used_nonces` | fallback replay protection khi Redis restart |
| `bank_audit_log` | audit replay, invalid signature, cert rejected, forbidden ownership, insufficient funds |
| `ledger_state` | lock `id='main'` bằng `SELECT ... FOR UPDATE` để serialize hash-chain |

## 12. Kết luận ngắn

Luồng Gateway -> Bank gRPC đã có khung tương đối rõ: REST validate/rate-limit -> decode opaque fields -> gọi protobuf RPC -> map lỗi gRPC về HTTP. Phần còn thiếu nằm chủ yếu ở Bank Service: chưa có core nghiệp vụ, chưa có DB/Redis/CA wiring, chưa có TLS server, và các RPC vẫn `Unimplemented`.

Ưu tiên tiếp theo nên là thống nhất Bank gRPC client TLS ở Gateway, sửa contract `CreateUser` để truyền `user_id`, rồi dựng Bank Core theo thứ tự `CreateUser` -> shared auth pipeline -> transfer ledger -> balance/history.
