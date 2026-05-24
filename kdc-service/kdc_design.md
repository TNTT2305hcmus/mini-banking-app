Dưới đây là bản đã được **chuẩn hóa tiếng Việt có dấu đầy đủ và định dạng lại nhất quán**, giữ nguyên cấu trúc nội dung của bạn:

---

# Tài liệu mô tả KDC Service

Tài liệu này mô tả ngắn gọn các file và hàm chính trong `kdc-service`. 

---

## 1. `internal/config/env.go` - Quản lý cấu hình môi trường

File này đọc cấu hình từ `.env` 

### Cấu trúc dữ liệu

#### `EnvConfig`

Chứa các giá trị cấu hình bắt buộc của KDC:

* `GRPCPort`: port gRPC server của KDC.
* `CAPort`: port/địa chỉ CA service.
* `TGTExp`: thời gian sống của TGT.
* `KTGSPath`: đường dẫn file khóa AES `K_tgs`.
* `KDCPrivatePath`: đường dẫn file RSA private key của KDC.
* `RedisURI`: URI kết nối Redis.

### Hàm chức năng

#### `LoadEnv() *EnvConfig`

* Load `.env`, đọc các biến môi trường bắt buộc, parse `TGT_EXP` thành `time.Duration`, sau đó trả về `EnvConfig`. Nếu thiếu biến hoặc format sai thì dừng chương trình bằng `log.Fatalf`.

#### `MustGetEnv(key string) string`

* Đọc một biến môi trường bắt buộc. Nếu giá trị rỗng thì fail-fast.

---

## 2. `internal/config/redis.go` - Cấu hình Redis

File này chuẩn hóa Redis connection string cho KDC.

### Cấu trúc dữ liệu

#### `RedisConfig`

* `URL`: Redis URL đã được chuẩn hóa.

### Hàm chức năng

#### `LoadRedisConfig() *RedisConfig`

* Lấy `RedisURI` từ `LoadEnv`, hỗ trợ cả format URL thông thường và format copy-paste từ `redis-cli`. Nếu connection string có `--tls`, hàm chuyển `redis://` thành `rediss://`.

---

## 3. `cmd/server/main.go` - Entry point của service

File này lắp ráp các dependency production và khởi động gRPC server.

### Hàm chức năng

#### `main()`

Thực hiện các việc chính:

* Load config và Redis config.
* Khởi tạo Redis client và ping để kiểm tra kết nối.
* Khởi tạo CA gRPC client.
* Tạo core KDC service bằng `kdc.NewService`.
* Tạo gRPC handler và server.
* Chạy server trong goroutine và hỗ trợ graceful shutdown khi nhận `SIGINT`/`SIGTERM`.

---

## 4. `internal/grpc/server.go` - gRPC server lifecycle

File này quản lý việc tạo, start và stop gRPC server.

### Cấu trúc dữ liệu

#### `Server`

* `grpcServer`: instance gRPC server.
* `port`: cổng lắng nghe.

### Hàm chức năng

#### `NewServer(handler *Handler, port string) *Server`

* Tạo gRPC server, register `KDCServiceServer`, bật health check và server reflection để debug bằng grpcurl/grpcui.

#### `Start() error`

*  Listen TCP trên port cấu hình và gọi `Serve`.

#### `Stop()`

* Dùng `GracefulStop` để tắt server an toàn.

---

## 5. `internal/grpc/handler.go` - gRPC handler

File này map request/response protobuf sang logic trong package `internal/kdc`.

### Cấu trúc dữ liệu

#### `Handler`

* Embed `pb.UnimplementedKDCServiceServer` và giữ `svc *kdc.Service`.

### Hàm chức năng

#### `NewHandler(svc *kdc.Service) *Handler`

* Tạo handler mới với core KDC service.

#### `RequestTGT(...)`

Xử lý API xin TGT:

* Validate các field bắt buộc.
* Gọi `CheckAndStoreNonce`.
* Tạo data cần verify từ request.
* Gọi `VerifyPreAuthSignature`.
* Gọi `GenerateSessionKey`.
* Gọi `GenerateEncryptedTGT`.
* Gọi `BuildAS_REP`.
* Trả về `ASResponse` gồm encrypted payload và TGT expiry.

#### `RequestServiceTicket(...)`

* Endpoint TGS hiện tại đang trả về `Unimplemented`. Logic TGS domain đã có trong `internal/kdc/service.go` và `tgs_service.go`, nhưng handler chưa map protobuf request sang `kdc.TGSRequest`.

---

## 6. `internal/kdc/types.go` - Contract dữ liệu và interface

File này khai báo các type dùng chung cho AS/TGS và dependency interface.

### Cấu trúc dữ liệu & interface

#### `ErrCertificateMissing`

Sentinel error khi repository không tìm thấy certificate.

#### `Clock` và `SystemClock`

* `Clock`: interface lấy thời gian hiện tại.
* `SystemClock`: implementation production trả về UTC time.

#### `ReplayStore`

* Interface lưu replay marker theo semantics `SET NX` với TTL.

#### `CertificateRepository`

* Interface lấy certificate metadata theo serial number.

#### `ScopeAuthorizer`

* Interface kiểm tra client có được dùng scope của service đích hay không.

#### `CertificateStatus`

* Trạng thái certificate: `VALID`, `ACTIVE`, `REVOKED`, `EXPIRED`.

#### `Certificate`

* Metadata certificate dùng trong TGS: serial, subject CN, public key PEM, status và expiry.

#### `TGSService`

* Service domain cho TGS, giữ `tgsKey`, service keys, replay store, certificate repo, scope authorizer, clock, random source và TTL config.

#### `ASService`

* Service domain cho AS, giữ CA client, Redis client và `KDCKeys`.

#### `Config`

* Input cấu hình cho `NewTGSService`.

#### `TGSRequest` và `TGSResponse`

* Request/response domain cho việc xin service ticket.

#### `TGTPlaintext`

* Nội dung TGT sau khi giải mã bằng `K_tgs`.

#### `AuthenticatorPlaintext`

* Nội dung authenticator sau khi giải mã bằng `K_c_tgs`.

#### `ServiceTicketPlaintext`

* Nội dung `Ticket_v`, được mã hóa cho service đích.

#### `TGSReplyPlaintext`

* Nội dung response trả về client, được mã hóa bằng `K_c_tgs`.

#### `StaticScopeAuthorizer`

* Allowlist scope dạng map in-memory.

### Hàm chức năng

#### `SystemClock.Now() time.Time`

* Trả về thời gian UTC hiện tại.

#### `StaticScopeAuthorizer.Allowed(...)`

* Trả về `true` nếu service có bật scope được yêu cầu.

---

## 7. `internal/kdc/key.go` - Quản lý khóa mật mã

File này load và validate khóa của KDC từ filesystem.

### Cấu trúc dữ liệu

#### `KDCKeys`

* `KTGSKey`: AES key chia sẻ cho AS/TGS (bắt buộc 32 byte).
* `PrivateKey`: RSA private key của KDC.
* `RawPrivKey`: PEM bytes gốc của private key.

### Hàm chức năng

#### `LoadKeys(...)`

* Đọc `K_tgs`, validate độ dài 32 byte, đọc RSA private key PEM và parse PKCS#1 hoặc PKCS#8.

#### `cleanNewline(data []byte) []byte`

* Loại bỏ `\n` và `\r` ở cuối file key.

---

## 8. `internal/kdc/crypto.go` - Helper AES-GCM

File này chứa helper mã hóa/giải mã JSON và bytes bằng AES-256-GCM.

### Hằng số

* `aes256KeySize`: 32 byte.
* `gcmNonceSize`: 12 byte.

### Hàm chức năng

#### `encryptJSON(...)`

* Marshal payload sang JSON và mã hóa bằng AES-GCM.

#### `decryptJSON[T any](...)`

* Giải mã ciphertext và unmarshal JSON.

#### `encryptBytes(...)`

* Mã hóa raw bytes, output dạng `nonce || ciphertext || auth_tag`.

#### `decryptBytes(...)`

* Giải mã raw bytes AES-GCM.

---

## 9. `internal/kdc/errors.go` - Lỗi domain KDC

### Cấu trúc dữ liệu

#### `ErrorCode`

* String đại diện mã lỗi domain.

#### `KDCError`

* Wrapper gồm `Code` và error gốc.

### Mã lỗi

* `AUTH_INVALID`
* `TGT_EXPIRED`
* `REQUEST_EXPIRED`
* `IDENTITY_MISMATCH`
* `CERT_NOT_FOUND`
* `CERT_REVOKED`
* `CERT_EXPIRED`
* `REPLAY_DETECTED`
* `SCOPE_DENIED`
* `INVALID_SCOPE`
* `SERVICE_UNKNOWN`
* `INTERNAL_ERROR`

### Hàm chức năng

#### `ErrorCodeOf(err error)`

* Trích mã lỗi từ error (fallback `INTERNAL_ERROR`).

---

## 10. `internal/kdc/as_service.go` - AS Service

### Thành phần chính

* `ASService`: xử lý AS Exchange
* `TGT`, `ASRepPayload`, `SignedData`

### Luồng chính

1. Verify pre-auth signature qua CA.
2. Generate session key `K_c_tgs`.
3. Check nonce Redis (chống replay).
4. Generate encrypted TGT bằng AES-GCM (`K_tgs`).
5. Build AS_REP:

   * ký RSA-PSS
   * mã hóa RSA-OAEP cho client

---

## 11. `internal/kdc/tgs_service.go` - TGS Service

### Luồng chính

1. Giải mã TGT bằng `K_tgs`.
2. Giải mã authenticator bằng `K_c_tgs`.
3. Validate identity, timestamp, replay.
4. Check certificate (CA).
5. Check scope authorization.
6. Tạo `K_c_v` + `Ticket_v`.
7. Trả `TGS_REP` mã hóa bằng `K_c_tgs`.

---

## 12. `internal/kdc/service.go` - Facade production

* Gộp AS + TGS thành một service duy nhất.
* Inject Redis + CA client.
* Expose `RequestServiceTicket`.

---

## 13. `internal/kdc/service_test_helper.go`

* Constructor phục vụ test injection.
* Tránh phụ thuộc filesystem thật.

---
