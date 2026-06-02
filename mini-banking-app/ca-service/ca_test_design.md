# CA Test Design

Tài liệu này mô tả ngắn gọn các test trong `ca-service/internal`. Mục tiêu là giúp đọc nhanh từng hàm test đang kiểm tra điều gì, vì sao cần kiểm tra, và hành vi mong đợi của CA Service.

## 1. `internal/ca/rootca_test.go`

Nhóm test này kiểm tra quá trình load Root CA từ disk và các guardrail bảo mật quanh key/certificate.

### `TestLoadFromDiskAcceptsValidRootCA`

Kiểm tra happy path khi private key và certificate Root CA hợp lệ. Test tạo key RSA, tạo self-signed CA certificate, ghi key đã mã hóa và cert ra file tạm, sau đó gọi `loadFromDisk`.

Kỳ vọng:

- Không có lỗi.
- `RootCA.PrivateKey`, `RootCA.Certificate`, và `RootCA.CertPEM` đều được load đầy đủ.

### `TestLoadFromDiskRejectsMismatchedRootCAKeyAndCert`

Kiểm tra trường hợp private key trên disk không khớp public key nằm trong Root CA certificate. Đây là lỗi cấu hình nghiêm trọng vì service có thể không ký/verify đúng chuỗi tin cậy.

Kỳ vọng:

- `loadFromDisk` trả lỗi.
- Nội dung lỗi có nhắc tới việc key/cert không khớp.

### `TestLoadFromDiskRejectsRootCACertWithoutCAFlag`

Kiểm tra certificate không có cờ `IsCA = true`. Một certificate không được đánh dấu CA thì không nên được dùng làm Root CA.

Kỳ vọng:

- `loadFromDisk` trả lỗi.
- Lỗi cho biết certificate không được đánh dấu là CA.

### `TestLoadFromDiskRejectsRootCACertWithoutCertSignUsage`

Kiểm tra Root CA certificate thiếu `KeyUsageCertSign`. Dù certificate có thể là CA, nếu không có quyền ký certificate thì không được dùng để cấp cert con.

Kỳ vọng:

- `loadFromDisk` trả lỗi.
- Lỗi có nhắc tới `KeyUsageCertSign`.

### `TestLoadFromDiskRejectsExpiredRootCACert`

Kiểm tra Root CA certificate đã hết hạn. CA hết hạn không nên tiếp tục ký hoặc xác thực certificate.

Kỳ vọng:

- `loadFromDisk` trả lỗi.
- Lỗi có nhắc tới trạng thái hết hạn.

### `TestLoadFromDiskRejectsRootCACertWithInvalidSelfSignature`

Kiểm tra Root CA certificate có self-signature không hợp lệ. Test tạo certificate dùng public key của một key, nhưng ký bằng private key khác.

Kỳ vọng:

- `loadFromDisk` trả lỗi.
- Lỗi có nhắc tới self-signature.

### Helper trong `rootca_test.go`

- `newRootCATestKey`: tạo RSA private key dùng cho Root CA test.
- `newRootCATestCert`: tạo certificate Root CA test, cho phép mutate template để dựng case lỗi.
- `writeRootCATestFiles`: ghi key mã hóa và certificate PEM vào thư mục tạm.
- `encryptedRootCATestKeyPEM`: mã hóa private key theo format mà Root CA loader yêu cầu.
- `randomBytes`: tạo dữ liệu ngẫu nhiên cho salt/nonce.
- `assertErrorContains`: kiểm tra thông báo lỗi có chứa chuỗi mong muốn.

## 2. `internal/ca/service_test.go`

Nhóm test này dùng `package ca_test`, nghĩa là kiểm tra CA service như một caller bên ngoài package. Trọng tâm là API nghiệp vụ: cấp certificate, lấy certificate, kiểm tra revoke, revoke, và isolation giữa nhiều user.

### `TestRegisterUser_ValidCSR`

Kiểm tra luồng cấp certificate thành công với CSR hợp lệ và `userID` khớp CSR.

Kỳ vọng:

- `RegisterUser` không trả lỗi.
- Certificate PEM, serial, và `notAfter` hợp lệ.
- Certificate trả về có Common Name bằng user ID.
- Certificate không phải CA certificate.
- Có `KeyUsageDigitalSignature`.
- Có email SAN và URI SAN đúng user.
- Có `SubjectKeyId` và `AuthorityKeyId`.

### `TestRegisterUser_WithCRLAndOCSPExtensions`

Kiểm tra service có nhúng CRL Distribution Point và OCSP Server vào certificate khi được cấu hình.

Kỳ vọng:

- Cấp certificate thành công.
- Certificate chứa URL CRL đã cấu hình.
- Certificate chứa URL OCSP đã cấu hình.

### `TestRegisterUser_RejectsCSRIdentityMismatch`

Kiểm tra CA từ chối CSR có identity không khớp `userID` do caller truyền vào. Đây là guardrail để Gateway đã verify user nào thì CA chỉ cấp cert cho đúng user đó.

Kỳ vọng:

- `RegisterUser` trả lỗi khi CSR thuộc `mallory@example.com` nhưng request lại yêu cầu cấp cho `alice@example.com`.

### `TestRegisterUser_RejectsDuplicateActiveCertificateForUser`

Kiểm tra policy mỗi user chỉ có một active certificate tại một thời điểm.

Kỳ vọng:

- Lần cấp certificate đầu tiên thành công.
- Lần cấp thứ hai cho cùng user, khi cert cũ vẫn active, bị từ chối.

### `TestRegisterUser_TamperedCSR`

Kiểm tra CA từ chối CSR bị sửa chữ ký. Test tạo CSR rồi flip một byte trong DER để chữ ký không còn hợp lệ.

Kỳ vọng:

- `RegisterUser` trả lỗi.
- CA không cấp certificate cho CSR bị giả mạo/tamper.

### `TestRegisterUser_EmptyCSR`

Kiểm tra validation khi CSR input rỗng.

Kỳ vọng:

- `RegisterUser` trả lỗi.

### `TestRegisterUser_InvalidPEM`

Kiểm tra validation khi input không phải PEM hợp lệ.

Kỳ vọng:

- `RegisterUser` trả lỗi.

### `TestRegisterUser_WrongPEMType`

Kiểm tra trường hợp input là PEM hợp lệ về format nhưng sai type, ví dụ gửi `CERTIFICATE` thay vì `CERTIFICATE REQUEST`.

Kỳ vọng:

- `RegisterUser` trả lỗi.

### `TestGetCertificate_AfterRegister`

Kiểm tra lấy lại certificate sau khi đã đăng ký thành công.

Kỳ vọng:

- `GetCertificate` không trả lỗi.
- Certificate PEM không rỗng.
- `userID` đúng với user đã đăng ký.
- Trạng thái là `CertStatusValid`.
- `notAfter` nằm trong tương lai.

### `TestGetCertificate_NotFound`

Kiểm tra lấy certificate bằng serial không tồn tại.

Kỳ vọng:

- `GetCertificate` trả lỗi.

### `TestCheckRevocation_ValidCert`

Kiểm tra trạng thái revoke của certificate còn hiệu lực và chưa bị revoke.

Kỳ vọng:

- `CheckRevocation` trả `CertStatusValid`.
- `reason` rỗng.
- `revokedAt` bằng `0`.

### `TestCheckRevocation_AfterRevoke`

Kiểm tra trạng thái sau khi certificate đã bị revoke.

Kỳ vọng:

- `RevokeCertificate` thành công.
- `CheckRevocation` trả `CertStatusRevoked`.
- `reason` đúng với lý do revoke.
- `revokedAt` được set.

### `TestRevokeCertificate_AlreadyRevoked`

Kiểm tra không cho revoke cùng một certificate hai lần.

Kỳ vọng:

- Lần revoke đầu tiên thành công.
- Lần revoke thứ hai trả lỗi.

### `TestMultipleUsers_IsolatedCerts`

Kiểm tra nhiều user có certificate độc lập và thao tác revoke của user này không ảnh hưởng user khác.

Kỳ vọng:

- Mỗi user được cấp serial khác nhau.
- Revoke certificate của user đầu tiên không làm certificate của user thứ hai bị đổi trạng thái.

### Helper trong `service_test.go`

- `newTestRootCA`: tạo Root CA in-memory cho test.
- `newTestService`: tạo `ca.Service` với Root CA và store tạm.
- `newTestServiceWithExtensions`: tạo service có cấu hình CRL/OCSP extension.
- `generateValidCSR`: tạo CSR hợp lệ mặc định.
- `generateValidCSRForUser`: tạo CSR hợp lệ có CN, email SAN, và URI SAN khớp user.
- `parseIssuedCert`: parse certificate PEM do service trả về.
- `containsString`: kiểm tra một chuỗi có nằm trong slice string.
- `containsURI`: kiểm tra một URI có nằm trong slice URI.
- `generateTamperedCSR`: tạo CSR bị sửa chữ ký để test nhánh từ chối.

## 3. `internal/ca/store_test.go`

Nhóm test này kiểm tra certificate store: persistence qua restart, defensive copy, và policy duplicate active certificate.

### `TestPersistentStoreSurvivesRestart`

Kiểm tra store lưu trạng thái xuống disk và khôi phục được sau khi tạo lại store.

Kỳ vọng:

- Certificate đã lưu vẫn tồn tại sau khi reload store từ cùng state file.
- `UserID` được giữ nguyên.
- Trạng thái revoke, thời điểm revoke, và lý do revoke được khôi phục đúng.

### `TestStoreGetReturnsDefensiveCopy`

Kiểm tra `Store.Get` trả bản copy thay vì pointer nội bộ. Điều này giúp caller không thể sửa state trong store sau khi lock đã được thả.

Kỳ vọng:

- Sửa `UserID`, `Cert.NotAfter`, `RevokedAt`, hoặc `RevokeReason` trên object trả về từ `Get` không làm thay đổi dữ liệu thật trong store.
- Sau khi revoke, sửa pointer `RevokedAt` của bản copy cũng không làm thay đổi state thật.

### `TestStoreSaveIssuedRejectsDuplicateActiveCertificateForUser`

Kiểm tra `SaveIssued` chặn cấp nhiều active certificate cho cùng một user.

Kỳ vọng:

- Lưu certificate đầu tiên thành công.
- Lưu certificate thứ hai cho cùng user khi cert đầu còn active trả `ErrActiveCertificateExists`.
- Sau khi revoke cert đầu, lưu cert thứ hai thành công.

### Helper trong `store_test.go`

- `newStoreTestCert`: tạo `CertRecord` test kèm serial tương ứng.
- `x509Serial`: sinh serial X.509 ngẫu nhiên cho certificate test.

## 4. `internal/grpc/server_test.go`

Nhóm test này kiểm tra unary interceptor phân quyền thao tác `RevokeCertificate` dựa trên Common Name của mTLS client certificate.

### `TestAuthorizeRevokeInterceptorAllowsConfiguredClientCN`

Kiểm tra client có Common Name nằm trong danh sách cho phép được gọi `RevokeCertificate`.

Kỳ vọng:

- Interceptor không trả lỗi.
- Handler thật được gọi.
- Response từ handler được trả về nguyên vẹn.

### `TestAuthorizeRevokeInterceptorRejectsUnauthorizedClientCN`

Kiểm tra client có mTLS certificate hợp lệ nhưng Common Name không nằm trong danh sách được phép revoke.

Kỳ vọng:

- Interceptor trả gRPC code `PermissionDenied`.
- Handler thật không được gọi.

### `TestAuthorizeRevokeInterceptorRejectsMissingClientCertificate`

Kiểm tra request revoke không có verified client certificate trong context.

Kỳ vọng:

- Interceptor trả gRPC code `Unauthenticated`.
- Handler thật không được gọi.

### `TestAuthorizeRevokeInterceptorAllowsNonRevokeMethods`

Kiểm tra interceptor chỉ áp dụng authorization đặc biệt cho `RevokeCertificate`. Các method khác được đi tiếp bình thường.

Kỳ vọng:

- Method không phải revoke không bị chặn.
- Handler thật được gọi.

### Helper trong `server_test.go`

- `contextWithVerifiedClientCN`: tạo `context.Context` giả lập peer gRPC có TLS verified chain và Common Name mong muốn.

## 5. Tổng Kết Phạm Vi Kiểm Thử

Các test hiện tại bao phủ những rủi ro chính của CA Service:

- Root CA material phải hợp lệ, tự ký đúng, còn hạn, có quyền CA và quyền ký certificate.
- CSR phải đúng format, chữ ký hợp lệ, và identity phải khớp user được Gateway truyền vào.
- Certificate được cấp phải có các extension quan trọng như SAN, SKI, AKI, CRL, OCSP.
- Store phải bền vững qua restart và không để lộ pointer nội bộ gây mutation/race.
- Một user không thể có nhiều active certificate cùng lúc.
- Revocation phải chính xác, không ảnh hưởng chéo giữa user.
- gRPC revoke endpoint phải được bảo vệ bằng mTLS client Common Name authorization.
