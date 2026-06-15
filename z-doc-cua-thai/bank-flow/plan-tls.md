# Plan: Thống nhất TLS cho Bank gRPC

## Mục tiêu

Đưa giao tiếp API Gateway ↔ Bank Service về đúng thiết kế `gRPC + TLS một chiều`, xóa đường client insecure và giữ một Bank gRPC client duy nhất trong Gateway.

## Phạm vi

- API Gateway:
  - `src/services/bank.service.ts`
  - `src/routes/bank.routes.ts`
  - `src/grpc-clients/bank.client.ts`
  - env liên quan `BANK_GRPC_ADDR` / `BANK_SERVICE_ADDR`
- Bank Service:
  - `banking-service/internal/grpc/server.go`
  - `banking-service/internal/configs/config.go`
  - `banking-service/cmd/server/main.go` nếu cần wire config TLS

## Các bước thực hiện

1. **Gộp Bank gRPC client ở Gateway**
   - Thêm `transferMoney`, `getBalance`, `getHistory` vào `api-gateway/src/services/bank.service.ts`.
   - Dùng `sslCredentials` giống `createUser`.
   - Thống nhất địa chỉ service về `BANK_GRPC_ADDR`.

2. **Sửa Bank routes dùng service TLS**
   - Sửa `api-gateway/src/routes/bank.routes.ts` để import các hàm từ `services/bank.service.ts`.
   - Giữ nguyên validate/rate-limit/decode base64 hiện có.
   - Không import `grpc-clients/bank.client.ts` nữa.

3. **Xóa client insecure**
   - Kiểm tra không còn reference tới `grpc-clients/bank.client.ts`.
   - Xóa `api-gateway/src/grpc-clients/bank.client.ts`.
   - Nếu không còn dùng folder `grpc-clients/`, cân nhắc xóa folder rỗng.

4. **Bật TLS phía Bank Service**
   - Thêm config cho server cert/key, ví dụ `BANK_TLS_CERT_PATH`, `BANK_TLS_KEY_PATH`.
   - Tạo `credentials.NewServerTLSFromFile(...)` trong `banking-service/internal/grpc/server.go`.
   - Khởi tạo `grpc.NewServer(grpc.Creds(creds))`.
   - Giữ health check và reflection như hiện tại.

5. **Đồng bộ env/config**
   - Gateway dùng CA/trust cert qua `CA_CERT_PATH`.
   - Gateway gọi Bank qua `BANK_GRPC_ADDR`.
   - Bank Service expose `BANK_GRPC_PORT` và đọc cert/key TLS.
   - Loại bỏ hoặc không dùng `BANK_SERVICE_ADDR` để tránh config drift.

6. **Kiểm tra**
   - Chạy TypeScript build cho API Gateway.
   - Chạy `go test` hoặc tối thiểu `go test ./...` trong `banking-service`.
   - Search xác nhận không còn `createInsecure()` cho Bank client.
   - Nếu có cert dev sẵn: chạy Gateway gọi thử Bank health/RPC và xác nhận TLS handshake thành công.

## Tiêu chí hoàn thành

- Gateway chỉ còn một Bank gRPC client trong `services/bank.service.ts`.
- Không còn file/client Bank dùng `grpc.credentials.createInsecure()`.
- Bank gRPC server chạy với TLS server credentials.
- REST Bank routes vẫn giữ endpoint và behavior hiện tại.
- Lỗi gRPC từ Bank vẫn đi qua `errorHandler.ts` và map đúng HTTP/error_code.

## Lưu ý rủi ro

- Nếu bật TLS ở Gateway trước nhưng Bank server chưa bật TLS, mọi RPC Bank sẽ fail handshake.
- Cần cert/key dev hợp lệ và CA cert mà Gateway trust được.
- `CreateUserRequest` vẫn chưa có `user_id`; đây là vấn đề M4 riêng, không gộp vào plan TLS để tránh mở rộng phạm vi.
