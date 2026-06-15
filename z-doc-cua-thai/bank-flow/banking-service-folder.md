# Báo cáo folder `mini-banking-app/banking-service`

> Phạm vi: mô tả cấu trúc hiện tại của Bank Service và vai trò dự kiến của từng folder theo `blueprint/design.md`, `database-design.md` và các flow Bank.

## 1. Tổng quan

`banking-service` là service nội bộ xử lý nghiệp vụ ngân hàng. Theo thiết kế, service này là verifier trong AP Exchange: giải mã `Ticket_v`, kiểm tra scope/ownership/replay/certificate/chữ ký, xử lý chuyển tiền bằng ACID transaction và ghi immutable ledger.

Trạng thái code hiện tại: service mới ở mức skeleton gRPC. Server có thể khởi tạo handler và đăng ký service, nhưng các RPC chính (`CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory`) vẫn đang trả `Unimplemented`.

## 2. Cấu trúc hiện tại

```text
mini-banking-app/banking-service/
+-- cmd/
|   +-- server/
|       +-- main.go
+-- config/
+-- grpc/
+-- internal/
|   +-- bank/
|   +-- configs/
|   |   +-- config.go
|   +-- grpc/
|       +-- handler.go
|       +-- server.go
+-- store/
+-- go.mod
+-- go.sum
```

## 3. Vai trò từng folder/file

| Đường dẫn | Vai trò hiện tại | Vai trò nên đảm nhiệm |
|---|---|---|
| `cmd/server/` | Entry point chạy Bank Service. | Chỉ chứa code bootstrap: load config, tạo dependency, start/stop gRPC server. Không đặt business logic ở đây. |
| `cmd/server/main.go` | In log khởi động, load config, tạo gRPC handler/server, xử lý SIGINT/SIGTERM. | Sau này cần wire DB, Redis, CA client, TLS credentials và service/usecase vào handler. |
| `internal/configs/` | Đọc cấu hình từ `.env` hoặc environment. | Tập trung config runtime: `BANK_GRPC_PORT`, `POSTGRES_CONNECTION_STRING`, Redis, CA gRPC addr, TLS cert/trust bundle, timeout. |
| `internal/configs/config.go` | Đọc `POSTGRES_CONNECTION_STRING`, `BANK_GRPC_PORT`; default port `50053`. | Cần mở rộng thêm config cho Redis, CA Service, Bank `K_v`, TLS và các timeout theo ADR-02/fail-closed. |
| `internal/grpc/` | Code gRPC đang dùng thật. | Lớp transport gRPC: register protobuf service, map request/response, convert lỗi domain sang gRPC status + metadata `error-code`. |
| `internal/grpc/server.go` | Tạo `grpc.NewServer()`, register `BankService`, health check, reflection, listen TCP. | Cần bật TLS một chiều thay vì server insecure; giữ health/reflection nếu phục vụ demo/dev. |
| `internal/grpc/handler.go` | Chứa handler cho 4 RPC nhưng đều `Unimplemented`. | Nhận RPC từ Gateway và gọi domain/usecase: `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory`. Không nên nhét toàn bộ nghiệp vụ vào handler. |
| `internal/bank/` | Đang trống. | Nên là lõi nghiệp vụ Bank: auth pipeline, ticket/authenticator validation, transfer usecase, balance/history query, ownership/business rules, hash-chain orchestration. |
| `store/` | Đang trống. | Nên chứa adapter persistence/cache: PostgreSQL repositories (`users`, `accounts`, `transactions`, `used_nonces`, `bank_audit_log`, `ledger_state`) và Redis replay/revocation cache. |
| `config/` | Đang trống. | Có thể chứa file cấu hình local/demo như TLS cert path, env example hoặc sample config. Nếu không dùng, nên bỏ để tránh nhiễu. |
| `grpc/` | Đang trống. | Hiện trùng ý nghĩa với `internal/grpc`. Nên hoặc xóa, hoặc chỉ dùng nếu muốn public package ngoài `internal`; với service hiện tại, nên giữ một nơi duy nhất là `internal/grpc`. |
| `go.mod`, `go.sum` | Khai báo module `mini-banking/banking-service`, phụ thuộc `grpc`, `godotenv`, protobuf package `mini-banking/pkg`. | Cần tiếp tục quản lý dependency Go; `replace mini-banking/pkg => ../pkg` cho phép dùng generated protobuf local. |

## 4. Luồng nghiệp vụ mà folder này phải hỗ trợ

| RPC | Endpoint Gateway tương ứng | Trách nhiệm chính |
|---|---|---|
| `CreateUser` | `POST /v1/pki/register` gọi nội bộ sau khi CA cấp cert | Tạo `users` trong Bank DB bằng cùng `user_id = owner_id = ID_c`; fail thì Gateway phải revoke cert bù trừ. |
| `TransferMoney` | `POST /v1/bank/transfer` | Giải mã ticket, chống replay, verify cert/chữ ký, kiểm tra ownership/số dư/hạn mức, cập nhật balance và append ledger. |
| `GetBalance` | `POST /v1/bank/accounts/{id}/balance/query` | Verify ticket scope `balance:read`, replay, cert, ownership; trả metadata số dư, không ghi ledger. |
| `GetHistory` | `POST /v1/bank/accounts/{id}/transactions/query` | Verify ticket scope `history:read`, replay, cert, ownership; trả lịch sử đã phân trang, không trả `client_signature`/`payload_hash`. |

## 5. Các điểm còn thiếu so với blueprint

- gRPC server chưa bật TLS một chiều.
- Chưa có kết nối PostgreSQL/Redis/CA Service.
- Chưa có `K_v` để giải mã `Ticket_v`.
- Chưa có pipeline xác thực fail-closed cho Bank request.
- Chưa có repositories cho Bank DB và Redis replay cache.
- Chưa có ledger append/hash chaining.
- Chưa có mapping lỗi domain sang gRPC status + `error-code` metadata.
- Một số folder đang trống hoặc trùng nghĩa (`config/`, `grpc/`, `store/`, `internal/bank/`) cần được dùng rõ ràng hoặc dọn sau.

## 6. Đề xuất tổ chức tiếp theo

Giữ `cmd/server` mỏng, dùng `internal/grpc` làm transport, đưa nghiệp vụ vào `internal/bank`, và đưa database/cache adapter vào `store`. Khi implement, nên đi theo thứ tự:

1. Hoàn thiện config + TLS + dependency wiring.
2. Implement `CreateUser` trước để chốt bất biến `users.id = owner_id = ID_c`.
3. Implement shared auth pipeline cho `TransferMoney`, `GetBalance`, `GetHistory`.
4. Implement transfer ACID + hash chain.
5. Implement read path balance/history và error mapping theo contract Gateway ↔ Bank.
