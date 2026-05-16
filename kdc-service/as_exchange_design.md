# Tài liệu mô tả KDC Service - AS Exchange (Phase 2)

---

## 1. File `internal/config/env.go` - Quản lý Cấu hình (Configuration Management)

File này tập trung quản lý các tham số cấu hình của KDC Service, tải cấu hình từ biến môi trường hoặc file `.env`.

### Cấu trúc dữ liệu (Structs)

#### `EnvConfig`
Chứa các thông tin cấu hình cần thiết để KDC hoạt động.
* `GRPCPort (string)`: Cổng lắng nghe của gRPC Server cho KDC Service.
* `CAPort (string)`: Địa chỉ cổng của CA Service để gọi gRPC.
* `TGTExp (time.Duration)`: Thời gian sống (Expiration) của Ticket Granting Ticket (TGT).
* `KTGSPath (string)`: Đường dẫn trỏ tới file chứa Symmetric AES key ($K_{tgs}$) chia sẻ giữa AS và TGS.
* `KDCPrivatePath (string)`: Đường dẫn trỏ tới file chứa RSA Private Key của KDC.
* `RedisURI (string)`: URI kết nối đến Redis, dùng cho mục đích chống Replay Attack (lưu Nonce).

### Các hàm chức năng
* **`LoadEnv() *EnvConfig`**: Nạp và xác thực các cấu hình từ môi trường. Nếu không truyền biến môi trường cần thiết, sử dụng `MustGetEnv` để "Fail-fast" (sử dụng `log.Fatalf`).

---

## 2. File `internal/kdc/key.go` - Quản lý Khóa Mật Mã (Cryptographic Keys)

File này xử lý việc đọc và kiểm tra các khóa mã hóa bảo mật từ ổ cứng để phục vụ cho nghiệp vụ mã hóa/giải mã và ký số.

### Cấu trúc dữ liệu
#### `KDCKeys`
* `KTGSKey ([]byte)`: Khóa AES-256 (32 bytes) dùng để mã hóa nội dung TGT ($K_{tgs}$).
* `PrivateKey (*rsa.PrivateKey)`: Khóa riêng tư RSA của KDC, dùng để ký (Sign) thông điệp phản hồi AS_REP.
* `RawPrivKey ([]byte)`: Dữ liệu thô định dạng PEM của Private Key.

### Các hàm chức năng
* **`LoadKeys(ktgsPath, privKeyPath string) (*KDCKeys, error)`**: Tải khóa từ hệ thống file. Hàm thực hiện kiểm tra độ dài chính xác của `KTGSKey` (32 byte) và parse `PrivateKey` theo chuẩn PKCS#1 hoặc PKCS#8. Đảm bảo khóa ở dạng chuẩn RSA.
* **`cleanNewline(data []byte) []byte`**: Hàm tiện ích giúp loại bỏ khoảng trắng hoặc ký tự xuống dòng dư thừa ở cuối file key, đảm bảo tính toàn vẹn của dữ liệu khóa.

---

## 3. File `internal/kdc/service.go` - Lõi Xử lý Nghiệp vụ AS Exchange

Đây là thành phần cốt lõi thực hiện các thuật toán bảo mật của quá trình trao đổi Authentication Service (Phase 2). Nó thực hiện việc cấp phát TGT và xác thực người dùng.

### Cấu trúc dữ liệu (Structs)
#### `Service`
* `caClient (capb.CAServiceClient)`: gRPC Client dùng để gọi sang CA Service nhằm lấy Public Key của Client phục vụ xác minh.
* `redisClient (*redis.Client)`: Redis Client được dùng để lưu trữ Nonce, một cơ chế để chống Replay Attack.
* `kdcKeys (*KDCKeys)`: Chứa các khóa bảo mật riêng biệt của KDC.

#### `TGT`
Đại diện cho Ticket Granting Ticket (Vé ủy quyền ticket).
* `ClientId`: Định danh người dùng.
* `SessionKey`: Khóa phiên $K_{c,tgs}$.
* `ExpiresAt`: Thời điểm hết hạn của vé.

#### `ASRepPayload` và `SignedData`
Đại diện cho dữ liệu gửi trả về cho Client. `ASRepPayload` gồm khóa phiên, TGT đã mã hóa và Nonce xác nhận. `SignedData` bao bọc Payload và kèm theo chữ ký RSA-PSS của KDC.

### Các hàm chức năng chính

#### `fetchPublicKeyFromCA(ctx, certSn string) (*rsa.PublicKey, error)`
Giao tiếp với CA Service thông qua gRPC API (`GetCertificate`), sau đó parse nội dung PEM của certificate trả về để trích xuất RSA Public Key của Client.

#### `VerifyPreAuthSignature(ctx, certSn, signature, dataToVerify)`
Xác minh chữ ký số của Client trong bước Pre-Authentication.
* Dùng Public Key của Client (lấy từ CA) và thuật toán `RSA-PKCS1v15` kết hợp với chuẩn hash SHA-256 để đảm bảo request thực sự do chính Client ký và gửi tới, tránh bị giả mạo.

#### `GenerateSessionKey() ([]byte, error)`
Tạo khóa phiên ngẫu nhiên an toàn (256-bit AES) làm khóa $K_{c,tgs}$. Dùng `crypto/rand` để sinh chuỗi byte một cách an toàn mật mã.

#### `CheckAndStoreNonce(ctx, nonce []byte)`
Lưu trữ Nonce do Client gửi tới vào Redis với thời gian sống (TTL) là 5 phút. Sử dụng thao tác `SetNX` (chỉ set khi key chưa tồn tại) để đảm bảo mỗi Nonce chỉ được phép sử dụng 1 lần duy nhất, chống lại tấn công Replay Attack.

#### `GenerateEncryptedTGT(clientId string, k_ctgs []byte)`
Đóng gói thông tin vào vé `TGT` sau đó mã hóa bảo mật toàn bộ JSON payload này bằng thuật toán `AES-GCM` sử dụng khóa bí mật $K_{tgs}$ của KDC. Nhờ có thẻ xác thực (Auth Tag) của GCM, TGT đảm bảo tính bảo mật và toàn vẹn, chống giả mạo nội dung.

#### `BuildAS_REP(ctx, k_ctgs, tgt, nonce1, certSn)`
Tạo thông điệp phản hồi `AS_REP`.
* **Ký điện tử:** Băm payload bằng SHA-256, sau đó dùng Private Key của KDC ký lên hash này bằng thuật toán bảo mật cao `RSA-PSS`.
* **Mã hóa bất đối xứng:** Toàn bộ thông điệp (Payload + Chữ ký) tiếp tục được mã hóa bằng thuật toán `RSA-OAEP` sử dụng Public Key của Client (cũng được truy xuất từ CA). Điều này đảm bảo chỉ duy nhất người dùng nắm giữ Private Key khớp với chứng chỉ mới có thể đọc được nội dung phản hồi từ KDC.

---

## 4. File `internal/grpc/handler.go` - Tầng Giao tiếp gRPC

File này đóng vai trò tiếp nhận yêu cầu gRPC từ Client, xác thực sơ bộ, ủy quyền xuống tầng Service (Business Logic), và trả về Response theo chuẩn gRPC.

### Cấu trúc dữ liệu
#### `Handler`
Triển khai interface `KDCServiceServer` được sinh ra tự động từ proto file (`ca.proto`/`kdc.proto`). Chứa tham chiếu đến instance lõi `svc` (*kdc.Service).

### Các API Endpoints
#### `RequestTGT(ctx, req)`
Thực hiện Phase 2 - Quá trình AS Exchange.
1. **Kiểm tra đầu vào:** Kiểm tra nhanh (Validation) chống rỗng đối với ClientId, CertSn, PreAuthSignature.
2. **Chống Replay Attack:** Chuyển Nonce sang lõi xử lý `CheckAndStoreNonce` để lưu lại.
3. **Pre-Authentication:** Tập hợp các tham số theo cấu trúc `ClientId | TgsId | Nonce1 | Timestamp` và ủy quyền cho `VerifyPreAuthSignature` để xác thực người dùng dựa trên chữ ký điện tử.
4. **Cấp phát:** Nếu mọi bước trên hợp lệ, cấp phát khóa phiên $K_{c,tgs}$ và đóng gói TGT với khóa mã hóa $K_{tgs}$.
5. **Phản hồi:** Ghép nối tất cả vào gói `AS_REP`, ký số bảo mật bởi KDC, mã hóa bằng Public Key của Client và gửi trả về qua chuẩn gRPC `ASResponse`. Mọi lỗi xảy ra trong quá trình này được bọc lại cẩn thận thành các gRPC Status Code tương ứng (Unauthenticated, PermissionDenied, Internal) nhằm hỗ trợ khả năng phân loại lỗi cho Client.

---

## 5. File `cmd/server/main.go` - Entry Point

File này chứa toàn bộ quá trình Bootstrap (khởi động) của KDC Service, lắp ráp các module lại thành một khối thống nhất.

### Luồng khởi động (Bootstrapping)
1. **Load Configuration:** Nạp các tham số cấu hình từ biến môi trường qua `config.LoadEnv()`.
2. **Init Redis Client:** Thiết lập kết nối đến Redis, ping thử nghiệm sức khỏe kết nối (Health Check) để đảm bảo Redis sẵn sàng phục vụ việc chống tấn công Reply Attack.
3. **Init CA gRPC Client:** Thiết lập kết nối gRPC (không bảo mật - insecure với scope nội mạng nội bộ) tới CA Service để thực hiện các nghiệp vụ tra cứu chứng chỉ số.
4. **Init Core Service:** Khởi tạo `kdc.Service` bằng cách truyền (Dependency Injection) CA Client và Redis Client, module sẽ tự động nạp các Key mật mã dự phòng từ ổ cứng.
5. **Init gRPC Server:** Gắn `Handler` và bọc vào đối tượng cấu hình gRPC Server quản lý vòng đời mạng.
6. **Graceful Shutdown:** Khởi chạy server trong một Goroutine và lắng nghe các tín hiệu SIGINT/SIGTERM từ OS (Docker/Kubernetes) để tiến hành tắt server một cách an toàn (Graceful Shutdown), đảm bảo không làm đứt kết nối đột ngột của client đang xử lý dang dở.
