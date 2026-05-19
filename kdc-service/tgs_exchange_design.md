# KDC Module Description

## Ý tưởng chính

Module `kdc-service/internal/kdc` xử lý Phase 3 - TGS Exchange trong mô hình Kerberos hybrid PKI. Client gửi `TGT`, `Authenticator`, `cert_sn`, `nonce2`, `service_id` và `requested_scope`; KDC xác thực tất cả dữ liệu này rồi cấp `Ticket_v` và khóa phiên `K_c_v` cho client dùng với Bank Service.

Luồng chính: decrypt TGT -> decrypt Authenticator -> kiểm tra identity/scope/nonce/timestamp -> chống replay -> kiểm tra certificate -> authorize scope -> sinh `K_c_v` -> tạo `Ticket_v` -> trả `TGS_REP` mã hóa bằng `K_c_tgs`.

## Thứ tự đọc đề xuất

1. `types.go`: đọc contract dữ liệu, dependency interface, request/response, plaintext payload.
2. `errors.go`: đọc các mã lỗi nghiệp vụ chuẩn của KDC.
3. `crypto.go`: đọc helper AES-256-GCM dùng chung.
4. `service.go`: đọc logic xử lý TGS Exchange từ ngoài vào trong.
5. `service_test.go`: đọc fixture và test case để hiểu kỳ vọng hành vi.

## `types.go`

- `ErrCertificateMissing`: sentinel error khi repository không tìm thấy certificate.
- `Clock`: interface lấy thời gian hiện tại, giúp test có thể cố định thời gian.
- `SystemClock`: clock production dùng UTC system time.
- `SystemClock.Now`: trả thời gian UTC hiện tại.
- `ReplayStore`: interface lưu replay key kiểu `SET NX` với TTL.
- `CertificateRepository`: interface lấy thông tin certificate theo serial number.
- `ScopeAuthorizer`: interface kiểm tra client có được xin scope của service hay không.
- `CertificateStatus`: trạng thái vòng đời certificate.
- Certificate status constants: tập giá trị `VALID`, `ACTIVE`, `REVOKED`, `EXPIRED`.
- `Certificate`: metadata certificate cần cho kiểm tra revocation, identity và public key.
- `Service`: object chính của KDC, giữ key, dependency và TTL config.
- `Config`: cấu hình đầu vào để tạo `Service`.
- `TGSRequest`: request client gửi để xin service ticket.
- `TGSResponse`: response chứa encrypted payload và hạn ticket.
- `TGTPlaintext`: dữ liệu sau khi decrypt TGT bằng `K_tgs`.
- `AuthenticatorPlaintext`: dữ liệu sau khi decrypt authenticator bằng `K_c_tgs`.
- `ServiceTicketPlaintext`: dữ liệu nằm trong `Ticket_v`, mã hóa cho service đích.
- `TGSReplyPlaintext`: dữ liệu trả về client, mã hóa bằng `K_c_tgs`.
- `StaticScopeAuthorizer`: allowlist scope in-memory cho test/dev.
- `StaticScopeAuthorizer.Allowed`: trả `true` nếu service có bật scope được yêu cầu.

## `errors.go`

- `ErrorCode`: mã lỗi ổn định cho domain KDC.
- Error code constants: tập lỗi chuẩn cho auth invalid, TGT expired, replay, cert, scope, service và internal error.
- `KDCError`: wrapper giữ `ErrorCode` và lỗi gốc.
- `KDCError.Error`: format lỗi thành string.
- `KDCError.Unwrap`: trả lỗi gốc để dùng `errors.Is` hoặc `errors.As`.
- `kdcError`: tạo `KDCError` nội bộ.
- `ErrorCodeOf`: bóc mã lỗi từ error, trả `INTERNAL_ERROR` nếu lỗi không thuộc KDC.

## `crypto.go`

- AES-GCM constants: quy định key AES-256 là 32 bytes và nonce GCM là 12 bytes.
- `encryptJSON`: marshal payload sang JSON rồi encrypt bằng AES-256-GCM.
- `decryptJSON`: decrypt AES-256-GCM rồi unmarshal JSON sang type mong muốn.
- `encryptBytes`: encrypt raw bytes, prefix nonce vào đầu ciphertext.
- `decryptBytes`: tách nonce khỏi ciphertext rồi decrypt raw bytes.

Lưu ý: ciphertext luôn có format `nonce || encrypted_body`, nonce dài 12 bytes, key bắt buộc 32 bytes.

## `service.go`

- `NewService`: validate config, clone key để tránh mutation bên ngoài, set default TTL/clock/random.
- `RequestServiceTicket`: entrypoint chính của TGS Exchange, gom toàn bộ bước xác thực và cấp ticket.
- `decryptTGT`: decrypt TGT bằng `K_tgs`, kiểm tra shape dữ liệu và expiry.
- `decryptAuthenticator`: decrypt authenticator bằng `K_c_tgs`, kiểm tra các field bắt buộc.
- `validateTimestampWindow`: reject request nếu timestamp lệch quá window cấu hình.
- `checkReplay`: hash `clientID:nonceReq:timestamp`, ghi replay key bằng `SET NX`, reject nếu trùng.
- `checkRevocation`: lấy certificate, reject nếu missing/revoked/expired/không có public key.
- `buildServiceTicket`: tạo `Ticket_v` chứa identity, scope, public key, cert serial, nonce và `K_c_v`, rồi encrypt bằng key của service đích.
- `encryptTGSReply`: tạo `TGS_REP` chứa `K_c_v`, `Ticket_v`, nonce echo, scope và expiry, rồi encrypt bằng `K_c_tgs`.

## `service_test.go`

- `fixedClock` / `fixedClock.Now`: cố định thời gian để test deterministic.
- `memoryReplayStore` / `SetNX`: fake replay cache để test chống replay.
- `memoryCertRepo` / `GetCertificate`: fake certificate repository.
- `TestDecryptTGTSuccessExtractsKCTGS`: TGT hợp lệ decrypt được và lấy đúng `K_c_tgs`.
- `TestValidAuthenticatorIssuesTicket`: request hợp lệ cấp được ticket và echo đúng nonce/scope/service.
- `TestAuthenticatorTimestampTooOldRejectsRequestExpired`: timestamp quá cũ bị reject.
- `TestReplaySameNonceAndTimestampRejectsSecondRequest`: request trùng nonce/timestamp bị reject lần hai.
- `TestRevokedCertificateRejects`: cert revoked bị chặn.
- `TestTicketVContainsPublicKeyAndScope`: `Ticket_v` chứa đúng public key, scope, identity và session key.
- `TestInvalidRequestedScopeRejects`: scope không được cấp quyền bị reject.
- `TestAuthenticatorClientMismatchRejects`: client trong authenticator phải khớp client trong TGT.
- `TestAuthenticatorServiceMismatchRejects`: service trong authenticator phải khớp service được request.
- `TestAuthenticatorScopeMismatchRejects`: scope trong authenticator phải khớp requested scope.
- `harness` / `newHarness`: gom fixture key, nonce, cert, repo, replay store và service.
- `harness.request`: gửi request qua service bằng request fixture.
- `harness.validRequest`: tạo TGS request hợp lệ với vài field tùy chỉnh.
- `harness.mustTGT`: tạo TGT mã hóa bằng `K_tgs`.
- `harness.mustEncrypt`: encrypt payload test hoặc fail test ngay.
- `harness.decryptReply`: decrypt `TGS_REP` trong test.
- `harness.decryptTicket`: decrypt `Ticket_v` trong test.
- `assertCode`: assert lỗi có đúng `ErrorCode`.
- `fixtureKey`: tạo AES-256 key deterministic từ label.

## Lưu ý đặc biệt

- KDC không lưu ticket vào database; ticket là encrypted blob stateless.
- Replay protection dựa trên `clientID + nonceReq + timestamp`, lưu TTL ngắn.
- `requested_scope` phải khớp cả request ngoài và authenticator bên trong.
- Certificate phải còn hạn, không revoked/expired và có `PublicKeyPEM`.
- `Ticket_v` được encrypt bằng key riêng của service đích, còn `TGS_REP` encrypt bằng `K_c_tgs` cho client.
