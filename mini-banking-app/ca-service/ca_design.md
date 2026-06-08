# CA Service Design

Tài liệu này mô tả thiết kế hiện tại của `ca-service` sau các cập nhật trong `ca_update.md`.

Mục tiêu của tài liệu:

- Giúp người đọc hiểu CA Service đang làm gì và không làm gì.
- Ghi rõ các quyết định bảo mật quan trọng đã được áp dụng.
- Chỉ ra các khái niệm nên research thêm nếu mới học về PKI/bảo mật.

## 0. Phạm Vi Dự Án

CA Service trong project này là một CA nội bộ phục vụ demo/đồ án Mini Banking, không phải CA production đầy đủ như Let's Encrypt, DigiCert, hoặc một enterprise CA hoàn chỉnh.

| Nhóm | Đã xử lý trong project | Ngoài phạm vi hiện tại / hướng production |
| --- | --- | --- |
| Root CA lifecycle | Load Root CA bằng `LoadKeyAndCert` theo nguyên tắc fail-closed; không tự sinh Root CA khi thiếu file. | Production nên tách Root CA offline và dùng Intermediate CA để ký certificate hằng ngày. |
| Root CA private key | Từ chối plaintext private key; chỉ load key đã mã hóa bằng envelope AES-256-GCM + PBKDF2-HMAC-SHA256. | Production nên dùng HSM/KMS/secret manager thay vì private key trên disk. |
| Root CA validation | Validate key/cert khi startup: key match, CA constraints, cert-sign usage, validity, self-signature. | Có thể bổ sung quy trình rotation/key rollover đầy đủ. |
| Transport security | CA gRPC chạy qua mTLS. | Có thể bổ sung service mesh policy, certificate rotation tự động. |
| Revoke authorization | `RevokeCertificate` được phân quyền theo Common Name của client certificate. | Production nên có RBAC/audit log chi tiết hơn cho thao tác admin. |
| Durable revocation state | Issued certificate và revocation state được lưu bền vững bằng JSON state file. | Production nên dùng database transactional và/hoặc CRL/OCSP thật. |
| Concurrent safety | Store dùng lock và defensive copy để tránh data race quanh read/revoke. | Vẫn nên chạy race detector trong CI. |
| Certificate extensions | Issued certificate có SAN, SKI, AKI, CRL Distribution Points, OCSP Server metadata. | CRL/OCSP URL hiện mới là metadata; chưa triển khai CRL/OCSP responder thật. |
| Identity binding | CA ràng buộc CSR identity với `user_id` do Gateway truyền vào. | CA chưa tự verify JWT/OTP token vì proto hiện không mang token/claim. |
| Duplicate active cert | Một user chỉ được có một active certificate tại một thời điểm. | Production có thể thêm policy thay thế/rotation certificate rõ ràng hơn. |

Đọc thêm:

- PKI, X.509 certificate, CSR
- CA, Root CA, Intermediate CA
- mTLS
- CRL và OCSP
- Race condition trong Go

---

## 1. `config.go` - Quản Lý Cấu Hình

File: `ca-service/internal/config/config.go`

`Config` gom các tham số runtime đọc từ environment variables. Cách này giúp cùng một binary có thể chạy local, Docker, hoặc môi trường deploy khác mà không sửa code.

### Các Trường Chính

- `GRPCPort`: port gRPC, mặc định `50051`.
- `RootCAKeyPath`: đường dẫn Root CA private key, mặc định `certs/root-ca/ca.key`.
- `RootCACertPath`: đường dẫn Root CA certificate, mặc định `certs/root-ca/ca.crt`.
- `IssuedCertsPath`: thư mục backup certificate PEM đã cấp.
- `StoreStatePath`: file JSON lưu certificate/revocation state, mặc định `certs/ca-store/state.json`.
- `CertValidityDays`: số ngày hiệu lực của certificate mới.
- `CRLDistributionPoints`: danh sách URL CRL nhúng vào issued certificate.
- `OCSPServers`: danh sách URL OCSP nhúng vào issued certificate.
- `GRPCServerCertPath`, `GRPCServerKeyPath`, `GRPCClientCACertPath`: cấu hình mTLS cho CA gRPC server.
- `RevokeAllowedClientCNs`: danh sách Common Name của client certificate được phép gọi `RevokeCertificate`.

Lưu ý: `ROOT_CA_KEY_PASSWORD` không nằm trong `Config`. Password này được đọc trực tiếp trong `rootca.go` khi decrypt Root CA private key.

### Helper

- `getEnv`: đọc string env, fallback default nếu env rỗng.
- `getEnvInt`: đọc integer env, fail-fast bằng `log.Fatalf` nếu giá trị không parse được.
- `getEnvCSV`: đọc danh sách phân tách bằng dấu phẩy, trim khoảng trắng, bỏ phần tử rỗng.

Fail-fast có nghĩa là service dừng ngay khi cấu hình sai. Với thành phần bảo mật như CA, dừng sớm tốt hơn chạy với cấu hình mơ hồ.

---

## 2. `rootca.go` - Load Và Validate Root CA

File: `ca-service/internal/ca/rootca.go`

Root CA là trust anchor của hệ thống. Nếu Root CA bị thay đổi hoặc private key bị lộ, toàn bộ certificate do hệ thống cấp có thể mất giá trị bảo mật.

### `RootCA`

- `PrivateKey`: RSA private key dùng để ký issued certificates.
- `Certificate`: Root CA certificate đã parse.
- `CertPEM`: Root CA certificate dạng PEM.

### `LoadKeyAndCert(keyPath, certPath string) (*RootCA, error)`

Hàm này load Root CA key/cert đã được pre-provision từ disk:

- Nếu cả key và cert đều tồn tại: load từ disk.
- Nếu thiếu key hoặc thiếu cert: trả lỗi và service không startup.
- Không tự động generate Root CA mới.

Lý do: CA không được âm thầm đổi trust root. Nếu service tự sinh Root CA mới khi file bị mất, các certificate cũ có thể không còn verify được, và lỗi vận hành bị che giấu.

### Private Key Encryption

Root CA private key plaintext bị từ chối. Key phải dùng PEM type `ENCRYPTED PRIVATE KEY` với envelope:

- PBKDF2-HMAC-SHA256: dẫn xuất key mã hóa từ password.
- AES-256-GCM: mã hóa có xác thực, phát hiện được tampering.
- `ROOT_CA_KEY_PASSWORD`: env bắt buộc để decrypt.

Đọc thêm:

- PBKDF2
- AES-GCM / authenticated encryption
- Vì sao private key phải được bảo vệ

### `validateLoadedRootCA`

Sau khi parse key/cert, service kiểm tra:

- RSA private key hợp lệ.
- Public key trong cert khớp private key.
- Certificate có `IsCA = true`.
- `KeyUsage` có `CertSign`.
- Certificate còn trong thời gian hiệu lực.
- Self-signature hợp lệ.

Đây là kiểm tra fail-closed: nếu Root CA material không đáng tin, service dừng startup.

---

## 3. `store.go` - Durable Certificate Store

File: `ca-service/internal/ca/store.go`

Store giữ certificate đã cấp và trạng thái revoke. Dữ liệu nằm trong RAM để truy cập nhanh, nhưng được persist xuống JSON file để không mất sau restart.

### `CertRecord`

- `UserID`: user sở hữu certificate.
- `Cert`: `*x509.Certificate` đã parse.
- `CertPEM`: certificate PEM trả cho client.
- `RevokedAt`: thời điểm revoke, `nil` nghĩa là chưa revoke.
- `RevokeReason`: lý do revoke.

### `Store`

- `records`: map `serial -> CertRecord`.
- `persistencePath`: đường dẫn state file JSON.
- `mu`: `sync.RWMutex` để bảo vệ map khi nhiều goroutine đọc/ghi.

### `NewPersistentStore(path string) (*Store, error)`

Khởi tạo store và load state từ JSON nếu file tồn tại.

Khi load, store:

- Parse lại certificate PEM.
- Kiểm tra serial trong PEM khớp key trong map.
- Khôi phục `RevokedAt` và `RevokeReason`.
- Fail-closed nếu phát hiện duplicate active certificate cho cùng user.

### `Save(serial string, record *CertRecord) error`

Lưu record và persist xuống disk. Hàm này là primitive chung, chủ yếu hữu ích cho test hoặc thao tác thấp cấp.

### `SaveIssued(serial string, record *CertRecord, now time.Time) error`

Đây là hàm service dùng khi cấp certificate mới.

Chính sách:

- Mỗi user chỉ được có một active certificate.
- Active nghĩa là chưa revoke và chưa expired.
- Nếu đã có active certificate, trả `ErrActiveCertificateExists`.
- Nếu ghi JSON lỗi, rollback thay đổi trong RAM.

Việc check duplicate và save diễn ra dưới cùng một write lock, nên hai request song song không thể cùng cấp hai cert active cho một user.

### `Get(serial string) *CertRecord`

Trả certificate record theo serial.

Điểm quan trọng: `Get` trả defensive copy, không trả pointer nội bộ trong map. Nếu trả pointer nội bộ, caller có thể đọc/ghi object sau khi lock đã unlock, tạo data race với `Revoke`.

Defensive copy không có nghĩa là “an toàn tuyệt đối 100%” cho mọi trường hợp, nhưng nó loại bỏ lỗi pointer escape khỏi shared state trong thiết kế hiện tại.

### `Revoke(serial, reason string) (bool, error)`

Đánh dấu certificate là revoked:

- Nếu serial không tồn tại hoặc đã revoked: trả `false, nil`.
- Nếu revoke được: set `RevokedAt`, `RevokeReason`, persist JSON.
- Nếu persist lỗi: rollback state RAM và trả error.

Đọc thêm:

- Go `sync.RWMutex`
- Data race và `go test -race`
- Atomicity trong lưu trữ dữ liệu

---

## 4. `service.go` - Core CA Logic

File: `ca-service/internal/ca/service.go`

`Service` chứa nghiệp vụ CA. Nó không biết request đến từ gRPC hay transport nào; handler chỉ gọi vào service.

### `Service`

- `rootCA`: Root CA đã load.
- `store`: certificate store.
- `issuedCertsPath`: thư mục backup PEM.
- `certValidityDays`: số ngày hiệu lực certificate.
- `extensions`: CRL/OCSP endpoint metadata để nhúng vào certificate.

### `RegisterUser(csrPEM, userID string)`

Luồng cấp certificate:

1. Decode PEM và parse CSR.
2. Verify chữ ký CSR bằng `csr.CheckSignature()`.
3. Chỉ chấp nhận RSA public key tối thiểu 2048-bit.
4. Kiểm tra `userID` là email hợp lệ.
5. Ràng buộc CSR identity với `userID`:
   - CSR `Subject.CommonName` phải bằng `userID`.
   - Nếu CSR có email SAN, mọi email SAN phải bằng `userID`.
   - Nếu CSR có URI SAN, mọi URI SAN phải bằng `urn:mini-banking:user:<escaped-userID>`.
6. Sinh serial ngẫu nhiên bằng CSPRNG.
7. Tạo X.509 certificate template:
   - End-entity certificate, không phải CA.
   - `KeyUsage`: digital signature và key encipherment.
   - `ExtKeyUsage`: client auth.
   - Email SAN và URI SAN.
   - Subject Key Identifier (SKI).
   - Authority Key Identifier (AKI).
   - CRL/OCSP metadata nếu được cấu hình.
8. Ký certificate bằng Root CA private key.
9. Lưu bằng `store.SaveIssued` để chặn duplicate active cert.
10. Backup PEM ra thư mục `IssuedCertsPath`. Nếu backup lỗi, service log warning nhưng không fail vì durable state đã được lưu trong store.

### Vì Sao CA Không Tự Verify JWT?

Theo thiết kế hiện tại:

- Client gửi registration token cho API Gateway.
- Gateway verify OTP/JWT và kiểm tra token single-use.
- Gateway gọi CA gRPC với `user_id = JWT.sub`.

`ca.proto` hiện chỉ có `csr_pem` và `user_id`, không có JWT/claim. Vì vậy CA không thể tự verify JWT nếu không đổi contract proto và luồng Gateway.

Trong phạm vi project, CA tăng guardrail bằng cách:

- Tin Gateway như policy enforcement caller qua mTLS.
- Không ký nếu CSR identity không khớp `userID`.
- Không cấp duplicate active cert cho cùng user.

### Certificate Extensions

Issued certificate có:

- `SubjectKeyId`: định danh public key của subject.
- `AuthorityKeyId`: định danh key của CA đã ký.
- Email SAN: email user.
- URI SAN: `urn:mini-banking:user:<escaped-userID>`.
- CRL Distribution Points và OCSP Server nếu cấu hình.

Lưu ý: CRL/OCSP URL chỉ là metadata trong certificate. Project hiện vẫn kiểm tra revoke qua gRPC `CheckRevocation`.

### Revocation APIs

- `GetCertificate`: trả PEM, userID, status và expiration.
- `CheckRevocation`: trả status, revoke reason, revokedAt.
- `RevokeCertificate`: revoke cert và persist state.
- `resolveStatus`: ưu tiên `REVOKED > EXPIRED > VALID`.

Đọc thêm:

- PKCS#10 CSR
- X.509 v3 extensions
- Subject Alternative Name
- Key Usage và Extended Key Usage
- RFC 5280

---

## 5. `handler.go` - gRPC Transport Layer

File: `ca-service/internal/grpc/handler.go`

Handler nhận request gRPC, validate input cơ bản, gọi service, rồi map kết quả/lỗi sang gRPC status code.

Handler không chứa nghiệp vụ ký certificate. Quy tắc như “CSR phải khớp userID” nằm trong `service.go`.

### `RegisterUser`

Input:

- `csr_pem`: bắt buộc.
- `user_id`: bắt buộc.

Lỗi được map như sau:

- CSR identity mismatch (`ErrCSRIdentityMismatch`) -> `InvalidArgument`.
- Duplicate active cert (`ErrActiveCertificateExists`) -> `AlreadyExists`.
- CSR format/signature lỗi -> `InvalidArgument`.
- Lỗi khác -> `Internal`.

### `GetCertificate`

- Serial rỗng -> `InvalidArgument`.
- Không tìm thấy cert -> `NotFound`.
- Thành công -> trả certificate PEM, userID, status, notAfter.

### `CheckRevocation`

- Serial rỗng -> `InvalidArgument`.
- Không tìm thấy cert -> `NotFound`.
- Thành công -> trả status, reason, revokedAt.

### `RevokeCertificate`

- Serial rỗng -> `InvalidArgument`.
- Không tìm thấy cert -> `NotFound`.
- Cert đã revoked -> `AlreadyExists`.
- Lỗi khác -> `Internal`.

Authorization cho revoke không nằm trong handler mà nằm ở server interceptor trong `server.go`.

---

## 6. `server.go` - Secure gRPC Server

File: `ca-service/internal/grpc/server.go`

Server setup gRPC với mTLS, health check, reflection và interceptor phân quyền revoke.

### `SecurityConfig`

- `ServerCertPath`: certificate của CA gRPC server.
- `ServerKeyPath`: private key của CA gRPC server.
- `ClientCACertPath`: CA certificate dùng để verify client certificates.
- `RevokeAllowedClientCNs`: danh sách Common Name được phép revoke.

### `NewServer(handler *Handler, port string, security SecurityConfig) (*Server, error)`

Các bước chính:

1. Load server certificate/key.
2. Load client CA certificate.
3. Tạo TLS config:
   - TLS tối thiểu 1.2.
   - `ClientAuth = tls.RequireAndVerifyClientCert`.
   - Plaintext client bị từ chối.
4. Tạo `grpc.Server` với transport credentials.
5. Gắn unary interceptor `authorizeRevokeInterceptor`.
6. Register CA service handler.
7. Register gRPC health check.
8. Register gRPC reflection.

### Revoke Authorization Interceptor

Interceptor chỉ áp dụng cho method `RevokeCertificate`.

Luồng:

- Lấy verified client certificate từ mTLS peer.
- Đọc `Subject.CommonName`.
- Nếu CN không nằm trong `RevokeAllowedClientCNs`, trả `PermissionDenied`.
- Nếu thiếu verified client cert, trả `Unauthenticated`.

Đọc thêm:

- gRPC credentials
- mTLS
- gRPC interceptor
- Zero Trust service-to-service communication

---

## 7. `main.go` - Entry Point

File: `ca-service/cmd/server/main.go`

`main` ghép các thành phần lại thành service chạy được.

Luồng startup hiện tại:

1. Load config bằng `config.Load()`.
2. Load Root CA bằng `ca.LoadKeyAndCert(cfg.RootCAKeyPath, cfg.RootCACertPath)`.
   - Nếu thiếu key/cert hoặc key/cert không hợp lệ, service log fatal và exit.
   - Không tự sinh Root CA.
3. Khởi tạo durable store bằng `ca.NewPersistentStore(cfg.StoreStatePath)`.
   - Nếu state file lỗi hoặc duplicate active cert, service exit.
4. Tạo CA service bằng `ca.NewServiceWithExtensionConfig`.
   - Truyền CRL/OCSP endpoint config vào service.
5. Tạo gRPC handler.
6. Tạo secure gRPC server bằng `cagrpc.NewServer`.
   - Truyền mTLS config và revoke authz config.
7. Chạy server trong goroutine.
8. Lắng nghe `SIGINT`/`SIGTERM`.
9. Khi shutdown, gọi `server.Stop()` để graceful stop.

---

## 8. Test Layout

Các test của package CA nằm trong `ca-service/internal/ca`.

- `rootca_test.go`: test load/validate Root CA.
- `store_test.go`: test persistent store, defensive copy, duplicate active cert.
- `service_test.go`: test public API của CA service bằng `package ca_test`.

`package ca_test` giúp test nhìn package `ca` như một package bên ngoài. Đây là cách tốt để tránh test phụ thuộc quá nhiều vào chi tiết private/internal function.

Chạy test:

```powershell
cd ca-service
$env:GOCACHE = (Join-Path (Get-Location) '.gocache')
go test ./internal/...
go test -race ./internal/...
go test ./...
```

---

## 9. Hướng Nghiên Cứu Tiếp

Nếu muốn nâng từ project scope lên production-grade CA, các hướng quan trọng là:

- Dùng database transactional thay cho JSON state file.
- Xuất bản CRL thật và/hoặc triển khai OCSP responder thật.
- Tách Root CA offline và dùng Intermediate CA để ký certificate hằng ngày.
- Bảo vệ private key bằng HSM/KMS.
- Thiết kế certificate rotation và key rollover.
- Thêm audit log bất biến cho issuance/revocation.
- Chuẩn hóa error type thay vì string matching ở một số helper.
- Cân nhắc đưa JWT/claim hoặc signed registration assertion vào proto nếu muốn CA tự verify identity thay vì tin Gateway.
