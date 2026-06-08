# KDC Test Design

Tài liệu này mô tả ngắn gọn cách tổ chức và các trường hợp kiểm thử trong `kdc-service/internal/tests`.

## Cách thực hiện

* Các file test nằm trong package `kdc_test`, tức là test package bên ngoài so với `internal/kdc`.
* File `kdc_alias_test.go` alias các type, const và error code được export từ package `kdc` để các test dễ đọc hơn.
* File `as_service_fixture_test.go` tạo fixture dùng chung: key tạm thời, certificate self-signed, fake CA client và biến môi trường tối thiểu cho test.
* File `as_service_for_test.go` là wrapper gọi `kdc.NewServiceForTest`, giúp test inject dependency thay vì dùng key/config thật.
* File `service_test_helper.go` trong `internal/kdc` export `NewServiceForTest` để phục vụ test sau khi chuyển test sang folder riêng.
* Các test TGS dùng fake clock, in-memory replay store, in-memory certificate repository và helper encrypt/decrypt local để kiểm tra luồng nghiệp vụ mà không cần Redis/CA service thật.

## Phạm vi kiểm thử

* AS exchange: verify pre-auth, tạo session key, tạo TGT và build AS_REP.
* TGS exchange: giải mã TGT, kiểm tra authenticator, replay, certificate status, scope và sinh service ticket.
* Các tình huống security: sai chữ ký, tamper ciphertext/signature, sai key, replay request, certificate revoked, identity mismatch.

## Fixture và helper

| File                         | Vai trò                                                                                                                        |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `as_service_fixture_test.go` | Tạo fixture AS service, fake CA client, RSA key, K_tgs key, certificate self-signed và helper `randBytes`, `assertBytesEqual`. |
| `as_service_for_test.go`     | Cung cấp wrapper `NewServiceForTest` cho test package.                                                                         |
| `kdc_alias_test.go`          | Alias các type/const/error exported từ package `kdc`.                                                                          |
| `service_test_helper.go`     | Constructor test-side trong package `kdc`, cho phép inject CA client, Redis client và `KDCKeys`.                               |

## Các trường hợp kiểm thử

### `as_rep_test.go`

| Test case                             | Mục đích                                                     | Kết quả mong đợi                                                 |
| ------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------- |
| `TestBuildASREP_InputValidation`      | Kiểm tra các input bắt buộc khi build AS_REP.                | Thiếu session key, TGT, nonce hoặc cert serial thì trả về lỗi.   |
| `TestDecryptASREP_HappyPath`          | Build AS_REP hợp lệ rồi client giải mã và verify chữ ký KDC. | Payload sau giải mã có session key, TGT và nonce đúng như input. |
| `TestDecryptASREP_WrongPrivKey`       | Client dùng sai private key để giải mã AS_REP.               | RSA-OAEP decrypt thất bại.                                       |
| `TestDecryptASREP_TamperedCiphertext` | Sửa ciphertext AS_REP sau khi build.                         | OAEP integrity check thất bại.                                   |
| `TestDecryptASREP_TamperedSignature`  | Sửa chữ ký KDC trong AS_REP.                                 | Verify RSA-PSS thất bại.                                         |
| `TestDecryptASREP_WrongKDCPubKey`     | Client verify bằng sai public key của KDC.                   | Verify RSA-PSS thất bại.                                         |

### `as_service_tgt_test.go`

| Test case                                  | Mục đích                                     | Kết quả mong đợi                                                     |
| ------------------------------------------ | -------------------------------------------- | -------------------------------------------------------------------- |
| `TestGenerateEncryptedTGT_InputValidation` | Kiểm tra validate `clientId` và session key. | Input rỗng thì trả về lỗi.                                           |
| `TestTGT_HappyPath`                        | Tạo encrypted TGT rồi giải mã bằng K_tgs.    | TGT plaintext có client ID, session key và thời gian hết hạn hợp lệ. |
| `TestTGT_NonDeterministic`                 | Tạo TGT hai lần với cùng input.              | Ciphertext khác nhau do nonce/IV ngẫu nhiên.                         |
| `TestTGT_TamperedCiphertext`               | Sửa ciphertext TGT.                          | AES-GCM authentication fail.                                         |
| `TestTGT_WrongKTGSKey`                     | Giải mã TGT bằng sai K_tgs.                  | AES-GCM decrypt fail.                                                |

### `preauth_test.go`

| Test case                                   | Mục đích                                  | Kết quả mong đợi       |
| ------------------------------------------- | ----------------------------------------- | ---------------------- |
| `TestVerifyPreAuthSignature_Valid`          | Verify chữ ký pre-auth hợp lệ của client. | Trả về nil.            |
| `TestVerifyPreAuthSignature_WrongSignature` | Dùng chữ ký random.                       | Verify thất bại.       |
| `TestVerifyPreAuthSignature_TamperedData`   | Data bị sửa nhưng giữ chữ ký cũ.          | Verify thất bại.       |
| `TestGenerateSessionKey_Length`             | Kiểm tra độ dài session key.              | Key có độ dài 32 byte. |
| `TestGenerateSessionKey_Entropy`            | Gọi tạo session key hai lần.              | Hai key khác nhau.     |

### `tgs_service_test.go`

| Test case                                               | Mục đích                                  | Kết quả mong đợi                                                                 |
| ------------------------------------------------------- | ----------------------------------------- | -------------------------------------------------------------------------------- |
| `TestDecryptTGTSuccessExtractsKCTGS`                    | Giải mã TGT fixture.                      | Lấy đúng client ID và K_c_tgs.                                                   |
| `TestValidAuthenticatorIssuesTicket`                    | Gửi TGS request hợp lệ.                   | Trả về encrypted reply, echo nonce, đúng service ID, scope và sinh K_c_v.        |
| `TestAuthenticatorTimestampTooOldRejectsRequestExpired` | Authenticator quá hạn timestamp window.   | Trả về `ErrRequestExpired`.                                                      |
| `TestReplaySameNonceAndTimestampRejectsSecondRequest`   | Gửi lại cùng nonce và timestamp.          | Request thứ hai bị từ chối với `ErrReplayDetected`.                              |
| `TestRevokedCertificateRejects`                         | Certificate của client bị revoke.         | Trả về `ErrCertRevoked`.                                                         |
| `TestTicketVContainsPublicKeyAndScope`                  | Giải mã service ticket được cấp.          | Ticket chứa public key, scope, cert serial, client ID, service ID và K_c_v đúng. |
| `TestInvalidRequestedScopeRejects`                      | Client xin scope không được cấp quyền.    | Trả về `ErrScopeDenied`.                                                         |
| `TestAuthenticatorClientMismatchRejects`                | Client ID trong authenticator khác TGT.   | Trả về `ErrIdentityMismatch`.                                                    |
| `TestAuthenticatorServiceMismatchRejects`               | Service trong authenticator khác request. | Trả về `ErrAuthInvalid`.                                                         |
| `TestAuthenticatorScopeMismatchRejects`                 | Scope trong authenticator khác request.   | Trả về `ErrScopeDenied`.                                                         |

## Cách chạy test

```powershell
cd kdc-service
go test ./...
```

Nếu máy gặp lỗi quyền với Go build cache trong `AppData`, có thể đặt cache tạm trong workspace khi chạy test:

```powershell
$env:GOCACHE=(Resolve-Path .).Path + '\.gocache'
go test ./...
```
