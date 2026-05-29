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
* `CAAddress`: địa chỉ gRPC đầy đủ của CA service sau khi resolve từ env.
* `TGTExp`: thời gian sống của TGT.
* `KTGSPath`: đường dẫn file khóa AES `K_tgs`.
* `KDCPrivatePath`: đường dẫn file RSA private key của KDC.
* `RedisURI`: URI kết nối Redis.

### Hàm chức năng

#### `LoadEnv() *EnvConfig`

* Load `.env`, đọc các biến môi trường bắt buộc, parse `TGT_EXP` thành `time.Duration`, sau đó trả về `EnvConfig`. Nếu thiếu biến hoặc format sai thì dừng chương trình bằng `log.Fatalf`.
* Resolve `CAAddress` theo thứ tự ưu tiên: `CA_ADDRESS` nếu có, `CA_PORT` nếu đã là address có dấu `:`, hoặc ghép `CA_HOST` với `CA_PORT`. Nếu không có `CA_HOST` thì fallback `localhost` để tương thích local dev.

#### `resolveCAAddress(caPort string) string`

* Chuẩn hóa endpoint CA service để production/container có thể cấu hình host riêng thay vì hardcode `localhost`.

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
* Khởi tạo CA gRPC client bằng `env.CAAddress`, tránh hardcode `localhost`.
* Tạo core KDC service bằng `kdc.NewService`; nếu constructor trả lỗi thì fail-fast bằng log fatal với thông tin cấu hình/khởi tạo cụ thể.
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
* Trả về `ASResponse` gồm encrypted payload và TGT expiry. `TgtExpiryUnix` được tính bằng `time.Now().Add(TGTExp)` vì `TGTExp` đã là `time.Duration`.

#### `RequestServiceTicket(...)`

Xử lý API xin service ticket:

* Validate các field bắt buộc: `service_id`, `tgt_ciphertext`, `authenticator`, `cert_sn`, `nonce2`, `requested_scope`.
* Map protobuf `pb.TGSRequest` sang domain `kdc.TGSRequest`.
* Gọi `h.svc.RequestServiceTicket`.
* Map domain response sang `pb.TGSResponse` gồm encrypted TGS_REP và ticket expiry.
* Nếu domain trả lỗi thì gọi `kdcErrorToStatus` để chuyển mã lỗi KDC sang gRPC status.

#### `kdcErrorToStatus(err error)`

* Dùng `kdc.ErrorCodeOf(err)` để phân loại lỗi domain.
* Map nhóm lỗi authentication material không hợp lệ/hết hạn như `AUTH_INVALID`, `TGT_EXPIRED`, `REQUEST_EXPIRED`, `REPLAY_DETECTED` sang `Unauthenticated`.
* Map nhóm lỗi không được phép như `IDENTITY_MISMATCH`, `CERT_REVOKED`, `CERT_EXPIRED`, `SCOPE_DENIED` sang `PermissionDenied`.
* Map request sai như `CERT_NOT_FOUND`, `SERVICE_UNKNOWN`, `INVALID_SCOPE` sang `InvalidArgument`.
* Các lỗi còn lại fallback về `Internal`.

---

## 6. `internal/kdc/types.go` - Contract dữ liệu và interface

File này khai báo các type dùng chung cho AS/TGS và dependency interface.

### Cấu trúc dữ liệu & interface

#### `ErrCertificateMissing`

* Sentinel error khi repository không tìm thấy certificate.

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

#### `TGT`

* TGT do AS phát hành trước khi mã hóa bằng `K_tgs`.
* Giữ field Go cũ như `ClientId`, `SessionKey`, `ExpiresAt` để tương thích test/caller hiện tại.
* JSON tag được đồng bộ với `TGTPlaintext`, gồm `client_id`, `k_c_tgs`, `tgt_expiry`, `expires_at`, giúp TGS giải mã và đọc trực tiếp bằng schema domain.

#### `TGTPlaintext`

* Nội dung TGT sau khi giải mã bằng `K_tgs`.
* Có field `ClientID`, `KCTGS`, `IssuedAt`, `Expiry`, `ExpiresAt`, `ClientIP`.
* `Expiry` dùng JSON tag `tgt_expiry`; `ExpiresAt` dùng JSON tag `expires_at` để hỗ trợ tương thích dữ liệu cũ/mới.

#### `ASRepPayload`

* Payload trả về client trong AS_REP.
* Gồm session key `K_c_tgs`, encrypted TGT và `nonce_1`.

#### `SignedData`

* Wrapper chứa `ASRepPayload` và chữ ký RSA-PSS của KDC.
* Được mã hóa bằng public key client trước khi trả về trong `ASResponse`.

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

File này chuẩn hóa lỗi nghiệp vụ của KDC thành các mã ổn định để tầng gRPC/API có thể map sang response nhất quán, thay vì phụ thuộc vào text lỗi chi tiết.

### Cấu trúc dữ liệu

#### `ErrorCode`

* Kiểu `string` đại diện cho mã lỗi domain.
* Mỗi mã lỗi mô tả một nhóm lỗi xác thực, phân quyền, replay hoặc lỗi hệ thống.

#### `KDCError`

* Wrapper gồm `Code ErrorCode` và `Err error`.
* `Error()` format lỗi theo dạng `CODE` hoặc `CODE: original_error`.
* `Unwrap()` trả về lỗi gốc để hỗ trợ `errors.Is`/`errors.As`.
* Được tạo thông qua helper nội bộ `kdcError(code, err)`.

### Mã lỗi

* `AUTH_INVALID`: Dữ liệu xác thực không hợp lệ, ví dụ TGT/authenticator giải mã thất bại, payload bị thiếu field bắt buộc, service hoặc nonce trong authenticator không khớp request.
* `TGT_EXPIRED`: TGT đã hết hạn, TGS không được phép dùng tiếp session key `K_c_tgs` trong ticket này.
* `REQUEST_EXPIRED`: Timestamp trong authenticator nằm ngoài `TimestampWindow`, thường do request quá cũ hoặc lệch đồng hồ vượt ngưỡng cho phép.
* `IDENTITY_MISMATCH`: Danh tính client không khớp giữa TGT, authenticator hoặc certificate subject CN.
* `CERT_NOT_FOUND`: Không tìm thấy certificate trong CA, hoặc CA trả về trạng thái không thuộc nhóm hợp lệ.
* `CERT_REVOKED`: Certificate đã bị thu hồi theo kết quả `CheckRevocation`.
* `CERT_EXPIRED`: Certificate đã hết hạn theo trạng thái CA hoặc trường `NotAfter`.
* `REPLAY_DETECTED`: Nonce/timestamp đã từng được ghi nhận trong replay store, request bị xem là gửi lại.
* `SCOPE_DENIED`: Client không được cấp scope đang yêu cầu, hoặc scope trong authenticator không khớp request.
* `INVALID_SCOPE`: Scope request không hợp lệ về mặt cú pháp/ngữ nghĩa. Mã này được định nghĩa để dùng khi có bước validate scope riêng.
* `SERVICE_UNKNOWN`: `serviceID` không có khóa service tương ứng trong cấu hình TGS, nên không thể tạo `Ticket_v`.
* `INTERNAL_ERROR`: Lỗi hạ tầng hoặc lỗi không phân loại được, ví dụ Redis lỗi, CA lỗi ngoài `NotFound`, đọc random thất bại hoặc mã hóa thất bại.

### Hàm chức năng

#### `kdcError(code ErrorCode, err error) error`

* Tạo `KDCError` với mã lỗi ổn định và lỗi gốc tùy chọn.
* Chỉ dùng nội bộ package `kdc`, giúp các service trả lỗi domain theo cùng một format.

#### `ErrorCodeOf(err error)`

* Trả về chuỗi rỗng nếu `err == nil`.
* Nếu lỗi là `*KDCError` thì trả về `Code`.
* Nếu là lỗi thường không có mã domain thì fallback về `INTERNAL_ERROR`.

---

## 10. `internal/kdc/as_service.go` - AS Service

File này triển khai phần AS Exchange: xác minh pre-authentication của client, sinh session key `K_c_tgs`, tạo TGT và đóng gói AS_REP trả về client.

### Thành phần chính

#### Biến và constructor

* `ENV`: cache cấu hình môi trường đã load.
* `getEnvConfig()`: lazy-load `.env` qua `config.LoadEnv()` để tránh load nhiều lần.
* `NewASService(caClient, redisClient) (*ASService, error)`: tạo `ASService`, load `K_tgs` và RSA private key của KDC từ filesystem bằng `LoadKeys`. Nếu load key thất bại thì trả lỗi `load_kdc_keys_failed` thay vì bỏ qua lỗi và để service panic khi dùng key nil.

#### `ASService`

* Giữ `caClient` để lấy certificate/public key từ CA.
* Giữ `redisClient` để chống replay bằng nonce.
* Giữ `kdcKeys` gồm `K_tgs` và private key của KDC.
* Các struct dữ liệu như `TGT`, `ASRepPayload`, `SignedData` đã được khai báo tập trung trong `types.go`.

### Luồng chính

1. Handler nhận request xin TGT và validate các field bắt buộc.
2. `CheckAndStoreNonce` ghi `nonce_1` vào Redis bằng `SET NX` với TTL 5 phút để chống replay.
3. `VerifyPreAuthSignature` lấy public key từ CA và verify chữ ký client trên dữ liệu pre-auth.
4. `GenerateSessionKey` sinh khóa ngẫu nhiên 32 byte làm `K_c_tgs`.
5. `GenerateEncryptedTGT` tạo TGT chứa client id, `K_c_tgs`, issued time, expiry và mã hóa bằng AES-GCM với `K_tgs`.
6. `BuildAS_REP` tạo payload trả về client, ký payload bằng RSA-PSS với private key KDC, sau đó mã hóa toàn bộ bằng RSA-OAEP với public key của client.

### Hàm chức năng

#### `fetchPublicKeyFromCA(ctx, certSn)`

* Gọi CA service `GetCertificate` theo serial number.
* Decode PEM, parse X.509 certificate và trích RSA public key của client.
* Trả lỗi nếu CA lỗi, PEM không hợp lệ, certificate parse lỗi hoặc public key không phải RSA.

#### `VerifyPreAuthSignature(ctx, certSn, signature, dataToVerify)`

* Lấy public key client từ CA.
* Hash `dataToVerify` bằng SHA-256.
* Verify chữ ký bằng RSA-PKCS1v15.
* Dùng để chứng minh client sở hữu private key tương ứng với certificate.

#### `GenerateSessionKey()`

* Sinh 32 byte bằng `crypto/rand`.
* Khóa này được dùng làm `K_c_tgs` trong AS Exchange và cùng kích thước AES-256.

#### `CheckAndStoreNonce(ctx, nonce)`

* Chuyển nonce sang hex và lưu key dạng `kdc:nonce:<nonce_hex>`.
* Dùng Redis `SetNX` để chỉ chấp nhận nonce chưa từng xuất hiện.
* TTL cố định 5 phút, đủ để chống replay trong cửa sổ request ngắn.

#### `GenerateEncryptedTGT(clientId, k_ctgs)`

* Validate `clientId` và session key.
* Lấy thời gian UTC hiện tại, tính expiry bằng `now.Add(TGTExp)`.
* Tạo `TGT` với `ClientId`, `SessionKey`, `IssuedAt`, `Expiry`, `ExpiresAt`.
* JSON tag của `TGT` khớp với `TGTPlaintext`, nên TGS có thể giải mã bằng `decryptJSON[TGTPlaintext]`.
* Gọi helper `encryptJSON` để marshal và mã hóa AES-GCM bằng `K_tgs`; output có dạng `nonce || ciphertext || auth_tag`.

#### `BuildAS_REP(ctx, k_ctgs, tgt, nonce1, certSn)`

* Validate input bắt buộc.
* Tạo `ASRepPayload` gồm session key, TGT và nonce của client.
* Hash payload bằng SHA-256 và ký RSA-PSS bằng private key của KDC.
* Bọc payload + signature thành `SignedData`.
* Lấy public key client từ CA rồi mã hóa `SignedData` bằng RSA-OAEP với label `AS_REP`.

---

## 11. `internal/kdc/tgs_service.go` - TGS Service

File này triển khai TGS Exchange: nhận TGT và authenticator từ client, kiểm tra quyền truy cập, rồi cấp service ticket `Ticket_v` cho service đích.

### Constructor và cấu hình

#### `NewTGSService(cfg Config)`

* Validate `TGSKey` phải đúng 32 byte.
* Bắt buộc có `ReplayStore`, `CertRepo`, `ScopeAuthorizer`.
* Nếu thiếu `Clock`, `Random`, `TicketTTL`, `TimestampWindow`, `ReplayTTL` thì dùng default production-safe.
* Copy `TGSKey` và từng service key để tránh caller sửa key sau khi inject.
* Validate mỗi service key phải là AES-256 key 32 byte.

### Luồng chính

1. `decryptTGT` giải mã TGT bằng `K_tgs`, kiểm tra payload đủ field và TGT chưa hết hạn.
2. `decryptAuthenticator` giải mã authenticator bằng `K_c_tgs` lấy từ TGT.
3. So khớp `ClientID`, `RequestedService`, `Scope` và `Nonce2` giữa request và authenticator.
4. `validateTimestampWindow` đảm bảo timestamp nằm trong cửa sổ lệch đồng hồ cho phép.
5. `checkReplay` ghi replay marker theo tổ hợp client, nonce và timestamp.
6. `checkRevocation` lấy certificate từ CA repository, kiểm tra revoked/expired/public key và identity.
7. Gọi `ScopeAuthorizer.Allowed` để đảm bảo client được dùng scope đã yêu cầu.
8. Sinh session key `K_c_v` 32 byte cho client và service đích.
9. `buildServiceTicket` tạo `Ticket_v` mã hóa bằng khóa riêng của service đích.
10. `encryptTGSReply` tạo TGS_REP mã hóa bằng `K_c_tgs` để chỉ client có session AS/TGS đọc được.

### Hàm chức năng

#### `RequestServiceTicket(ctx, req)`

* Điều phối toàn bộ TGS Exchange.
* Trả `TGSResponse` gồm `EncryptedPayload` và `TicketExpiryUnix`.
* Trả lỗi domain như `AUTH_INVALID`, `TGT_EXPIRED`, `REQUEST_EXPIRED`, `CERT_REVOKED`, `SCOPE_DENIED`, `SERVICE_UNKNOWN`.

#### `decryptTGT(tgtCiphertext)`

* Dùng `decryptJSON[TGTPlaintext]` với `s.tgsKey`.
* Hỗ trợ cả `tgt_expiry` và `expires_at` bằng cách lấy `ExpiresAt` làm fallback khi `Expiry` rỗng.
* Reject TGT malformed hoặc expired.

#### `decryptAuthenticator(kctgs, authenticator)`

* Dùng `K_c_tgs` để giải mã authenticator.
* Bắt buộc có `ClientID`, `Timestamp`, `NonceReq`, `RequestedService`, `Scope`.

#### `validateTimestampWindow(ts)`

* So sánh timestamp request với `clock.Now()`.
* Chấp nhận lệch cả hai chiều nhưng không vượt quá `timestampWindow`.

#### `checkReplay(ctx, clientID, nonceReq, ts)`

* Hash chuỗi `clientID:nonceReq:timestamp` bằng SHA-256.
* Lưu key `replay:tgs:<hash>` vào replay store bằng `SET NX`.
* Nếu key đã tồn tại thì trả `REPLAY_DETECTED`.

#### `checkRevocation(ctx, certSN)`

* Gọi `CertRepo.GetCertificate`.
* Map `ErrCertificateMissing` thành `CERT_NOT_FOUND`.
* Reject certificate revoked, expired, không có public key hoặc trạng thái không hợp lệ.

#### `buildServiceTicket(...)`

* Tìm service key theo `serviceID`; nếu không có thì trả `SERVICE_UNKNOWN`.
* Tạo `ServiceTicketPlaintext` gồm client id, service id, `K_c_v`, public key client, cert serial, scope, nonce và thời điểm phát hành/hết hạn.
* Mã hóa ticket bằng service key để chỉ service đích giải mã được.

#### `encryptTGSReply(...)`

* Tạo `TGSReplyPlaintext` chứa `K_c_v`, `Ticket_v`, `Nonce2`, `NonceReq`, service id, scope và expiry.
* Mã hóa reply bằng `K_c_tgs`, vì khóa này chỉ client và TGS biết.

---

## 12. `internal/kdc/service.go` - Facade production

File này là facade production của package `kdc`, dùng để gộp AS Service và TGS Service thành một dependency duy nhất cho tầng gRPC handler.

### Thành phần chính

#### `Service`

* Embed `*ASService`, nên handler có thể gọi trực tiếp các hàm AS như `CheckAndStoreNonce`, `VerifyPreAuthSignature`, `GenerateSessionKey`, `GenerateEncryptedTGT`, `BuildAS_REP`.
* Giữ `tgsService *TGSService` để xử lý request xin service ticket.

#### `NewService(caClient, redisClient) (*Service, error)`

* Tạo `ASService` bằng CA client và Redis client; nếu `NewASService` trả lỗi load key thì propagate lỗi lên caller.
* Load env production.
* Đọc `BANK_SERVICE_ID`, default là `bank-service`.
* Đọc `BANK_SERVICE_KEY_PATH`, default tạm thời dùng `K_TGS_PATH` cho demo nếu chưa cấu hình riêng.
* Load service key bằng `loadAES256Key`.
* Tạo `TGSService` với:
  * `TGSKey`: dùng `K_tgs` đã load trong AS service.
  * `ServiceKeys`: map service id sang AES-256 key của service đích.
  * `ReplayStore`: adapter Redis.
  * `CertRepo`: adapter CA service.
  * `ScopeAuthorizer`: allowlist tĩnh gồm `transfer:internal` và `account:read`.
  * TTL/timestamp/replay window đều là 5 phút.
* Nếu load service key hoặc init TGS thất bại thì trả lỗi cho caller. Entry point `cmd/server/main.go` chịu trách nhiệm log fatal để dừng chương trình trong production.

#### `RequestServiceTicket(ctx, req)`

* Facade method chuyển tiếp request sang `s.tgsService.RequestServiceTicket`.

### Adapter production

#### `RedisReplayStore`

* Bọc `*redis.Client` để implement interface `ReplayStore`.
* `SetNX(ctx, key, value, ttl)` gọi trực tiếp Redis `SET NX` và trả về kết quả insert.

#### `CACertificateRepository`

* Bọc `capb.CAServiceClient` để implement interface `CertificateRepository`.
* `GetCertificate(ctx, certSN)` gọi `CheckRevocation` để lấy trạng thái và `GetCertificate` để lấy certificate PEM.
* Map gRPC `NotFound` thành `ErrCertificateMissing`.
* Parse X.509 certificate, marshal public key sang PEM dạng `PUBLIC KEY`.
* Trả về domain `Certificate` gồm serial, subject CN, public key PEM, status và `NotAfter`.

#### `mapCACertStatus(st)`

* Chuyển enum protobuf của CA sang enum domain của KDC.
* Hỗ trợ `VALID`, `REVOKED`, `EXPIRED`; trạng thái không biết trả chuỗi rỗng để TGS reject.

#### `loadAES256Key(path)`

* Đọc key raw từ filesystem.
* Gọi `cleanNewline` để bỏ newline cuối file.
* Validate key đúng 32 byte, nếu sai trả lỗi cấu hình rõ ràng.

#### `getEnvDefault(key, fallback)`

* Đọc biến môi trường.
* Nếu biến rỗng thì dùng fallback.

---

## 13. `internal/kdc/service_test_helper.go`

* Constructor phục vụ test injection.
* Tránh phụ thuộc filesystem thật.

---
