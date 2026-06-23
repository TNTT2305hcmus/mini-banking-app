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

Xử lý API xin TGT (thin handler):

* Validate các field bắt buộc (`owner_id`, `cert_sn`, `nonce`, `timestamp`, `signature`).
* Gọi `svc.IssueTGT(...)` để chạy toàn bộ AS Exchange (freshness, replay, ràng buộc danh tính + verify chữ ký, tạo TGT, đóng gói AS_REP).
* Map lỗi domain bằng `kdcErrorToStatus` và trả về `pb.ASResponse{AsRep, TgtExpiresAtUnix}` với expiry do service trả về (không load lại env mỗi request).

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

* Metadata certificate dùng chung cho AS/TGS: serial, `owner_id` (do CA cấp), subject CN, public key PEM, status, `NotBefore` và `NotAfter`.

#### `TGSService`

* Service domain cho TGS, giữ `tgsKey`, service keys, replay store, certificate repo, scope authorizer, clock, random source và TTL config.

#### `ASService`

* Service domain cho AS. Dùng chung `certRepo`, `replayStore`, `clock`, `rand` với TGS; field riêng là `kdcKeys` (RSA private key + K_tgs) và `tgtTTL`/`timestampWindow`.

#### `ASConfig`

* Input cấu hình cho `NewASService`: `CertRepo`, `ReplayStore`, `Clock`, `Random`, `Keys`, `TGTTTL`, `TimestampWindow`.

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

#### `newSessionKey(random)`

* Sinh khóa AES-256 (32 byte) từ nguồn random được inject. Dùng chung cho `K_c_tgs` (AS) và `K_c_v` (TGS).

#### `encryptJSON(...)`

* Marshal payload sang JSON và mã hóa bằng AES-GCM.

#### `decryptJSON[T any](...)`

* Giải mã ciphertext và unmarshal JSON.

#### `encryptBytes(...)`

* Mã hóa raw bytes, output dạng `nonce || ciphertext || auth_tag`.

#### `decryptBytes(...)`

* Giải mã raw bytes AES-GCM.

---

## 8b. `internal/kdc/cert.go` - Helper certificate dùng chung

File này tập trung logic xử lý certificate mà cả AS và TGS đều dùng, thay vì mỗi exchange tự gọi CA và parse riêng.

### Hàm chức năng

#### `loadUsableCert(ctx, repo, certSN, now)`

* Gọi `repo.GetCertificate`, map `ErrCertificateMissing` thành `CERT_NOT_FOUND`, lỗi khác thành `INTERNAL_ERROR`.
* Gọi `validateCertUsable` rồi trả về certificate hợp lệ. Là điểm vào chung cho `authenticateClient` (AS) và `checkRevocation` (TGS).

#### `validateCertUsable(cert, now)`

* Reject theo status (`REVOKED`/`EXPIRED`/không xác định), kiểm tra cửa sổ `NotBefore`/`NotAfter`, và bắt buộc có public key.

#### `parseRSAPublicKeyPEM(pem)`

* Decode PKIX PEM và trích RSA public key (trước đây nằm trong `as_service.go`, nay dùng chung).

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

File này triển khai phần AS Exchange: ràng buộc danh tính client với certificate, xác minh pre-authentication, sinh session key `K_c_tgs`, tạo TGT và đóng gói AS_REP trả về client.

### Thành phần chính

#### Constructor

* `NewASService(cfg ASConfig) (*ASService, error)`: tạo `ASService` từ các dependency được inject (`CertRepo`, `ReplayStore`, `Clock`, `Random`, `Keys`, `TGTTTL`, `TimestampWindow`). Validate `Keys` (K_tgs 32 byte + RSA private key) và các dependency bắt buộc; áp default an toàn cho `Clock`/`Random`/TTL.
* `ASService` dùng chung các abstraction với TGS: `certRepo` (lấy certificate/public key từ CA), `replayStore` (chống replay), `clock`, `rand`. Field riêng của AS là `kdcKeys` (RSA private key của KDC để ký AS_REP) và `tgtTTL`. Không còn `var ENV` global hay `getEnvConfig()`; config được load một lần trong `NewService` và inject xuống.

### Luồng chính - `IssueTGT(ctx, ownerID, certSn, nonce, timestamp, signature)`

Đây là entrypoint điều phối toàn bộ AS Exchange (handler chỉ map proto và gọi hàm này):

1. `validateFreshness` đảm bảo timestamp nằm trong `timestampWindow`.
2. `checkASReplay` ghi marker `replay:as:<hash>` bằng `SetNX` để chống replay.
3. `buildASCanonicalPayload` dựng lại dữ liệu pre-auth chuẩn (cùng schema client đã ký).
4. `authenticateClient` lấy certificate, kiểm tra usable, **ràng buộc `owner_id`**, rồi verify chữ ký; trả về public key client.
5. `newSessionKey` sinh `K_c_tgs` 32 byte.
6. `encryptTGT` tạo TGT (client id, cert serial, `K_c_tgs`, issued/expiry) và mã hóa AES-GCM bằng `K_tgs`.
7. `BuildAS_REP` đóng gói AS_REP, **tái sử dụng public key** đã lấy ở bước 4 (chỉ 1 lần gọi CA cho mỗi request).
8. Trả về AS_REP đã mã hóa và `tgtExpiryUnix` thực tế đã dùng để tạo TGT (không tính lại từ env).

### Hàm chức năng

#### `authenticateClient(ctx, certSn, ownerID, signature, dataToVerify)`

* Gọi `loadUsableCert` (xem `cert.go`) để lấy certificate và kiểm tra status/hạn/public key.
* **Ràng buộc danh tính fail-closed**: bắt buộc `cert.OwnerID` (do CA cấp, server-side) không rỗng và bằng `ownerID` client gửi lên. Đây là biện pháp chống giả mạo `owner_id`: chữ ký hợp lệ chỉ chứng minh client sở hữu private key của certificate, **không** chứng minh client là chủ của `owner_id`. Nếu thiếu kiểm tra này, người dùng có certificate hợp lệ của chính mình có thể đặt `owner_id` của nạn nhân và nhận TGT mạo danh.
* Lưu ý: **không** dùng `SubjectCN` làm khóa danh tính — trong hệ thống này CA đặt `SubjectCN = full_name` (tên hiển thị do client chọn), không phải `owner_id`. Định danh xác thực là `owner_id` (và SAN URI `urn:mini-banking:owner:<owner_id>`).
* Parse RSA public key từ `cert.PublicKeyPEM` và `verifySignature` (chấp nhận PKCS#1 v1.5 hoặc RSA-PSS).
* Trả về public key để `BuildAS_REP` dùng lại.

#### `VerifyPreAuthSignature(ctx, certSn, signature, dataToVerify)`

* Primitive chỉ verify chữ ký (không ràng buộc `owner_id`), giữ lại cho unit test. Luồng production dùng `authenticateClient`.

#### `GenerateSessionKey()`

* Sinh 32 byte bằng `newSessionKey(s.rand)`; dùng làm `K_c_tgs`.

#### `GenerateEncryptedTGT(clientId, k_ctgs, certSn)`

* Validate input, tính expiry bằng `clock.Now().Add(tgtTTL)`.
* Tạo `TGT` (gồm cả `cert_sn`) với JSON tag khớp `TGTPlaintext`, mã hóa AES-GCM bằng `K_tgs`.

#### `BuildAS_REP(clientPubKey, k_ctgs, tgt, nonce1)`

* Nhận sẵn public key client (không tự gọi CA).
* Tạo `ASRepPayload`, ký RSA-PSS bằng private key KDC.
* **Mã hóa lai**: mã hóa payload bằng AES-256-GCM với khóa ngẫu nhiên, rồi bọc khóa AES đó bằng RSA-OAEP (label `AS_REP`) với public key client. Trả về `ASResponse{KDCSignature, EncryptedKey, EncryptedPayload}`. Lai hóa là bắt buộc vì payload lớn hơn một block RSA.

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

* Ủy quyền cho helper dùng chung `loadUsableCert` (xem `cert.go`): lấy certificate, map `ErrCertificateMissing` thành `CERT_NOT_FOUND`, và `validateCertUsable` reject certificate revoked/expired/chưa hiệu lực/thiếu public key.

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

* Embed `*ASService`, nên handler có thể gọi trực tiếp các hàm AS như `IssueTGT`, `VerifyPreAuthSignature`, `GenerateSessionKey`, `GenerateEncryptedTGT`, `BuildAS_REP`.
* Giữ `tgsService *TGSService` để xử lý request xin service ticket.

#### `NewService(caClient, redisClient) (*Service, error)`

* Load env production và load `K_tgs` + RSA private key của KDC bằng `LoadKeys` (fail-fast nếu lỗi).
* Tạo các collaborator dùng chung **một lần** — `CACertificateRepository`, `RedisReplayStore`, `SystemClock` — rồi inject vào cả `NewASService` (qua `ASConfig`) và `NewTGSService` (qua `Config`).
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
* `GetCertificate(ctx, certSN)` gọi `VerifyCertificate` (kèm `include_public_key_pem`/`include_certificate_pem`) để lấy trạng thái, `owner_id` và key material.
* Map gRPC `NotFound` thành `ErrCertificateMissing`.
* Parse X.509 certificate, marshal public key sang PEM dạng `PUBLIC KEY`.
* Trả về domain `Certificate` gồm serial, `owner_id`, subject CN, public key PEM, status, `NotBefore` và `NotAfter`.

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
