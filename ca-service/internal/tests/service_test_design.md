# Service Test Design

Tài liệu này mô tả ngắn gọn các trường hợp kiểm thử trong `service_test.go` cho CA service.

## Mục tiêu kiểm thử

* Đảm bảo CA service cấp certificate đúng cho CSR hợp lệ.
* Từ chối các CSR hoặc input không hợp lệ.
* Truy xuất certificate theo serial sau khi đã cấp.
* Kiểm tra trạng thái thu hồi certificate.
* Đảm bảo certificate của nhiều user độc lập với nhau.

## Test Helpers

| Helper                | Mô tả                                                                 |
| --------------------- | --------------------------------------------------------------------- |
| `newTestRootCA`       | Tạo Root CA tạm thời trong temp directory để dùng riêng cho mỗi test. |
| `newTestService`      | Tạo CA service với Root CA, store và thư mục tạm thời.                |
| `generateValidCSR`    | Tạo CSR hợp lệ bằng RSA-2048 key, dùng cho happy path.                |
| `generateTamperedCSR` | Tạo CSR bị sửa signature để kiểm tra việc từ chối CSR giả mạo.        |

## Các trường hợp kiểm thử

### RegisterUser

| Test case                       | Mục đích                                                                                    | Kết quả mong đợi                                                                                                                     |
| ------------------------------- | ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `TestRegisterUser_ValidCSR`     | Kiểm tra cấp certificate khi CSR hợp lệ.                                                    | Trả về cert PEM, serial, `notAfter` trong tương lai; cert có `CommonName` bằng user ID, không phải CA cert và có `DigitalSignature`. |
| `TestRegisterUser_TamperedCSR`  | Kiểm tra bảo vệ khi CSR bị sửa chữ ký.                                                      | Service trả về lỗi và không cấp certificate.                                                                                         |
| `TestRegisterUser_EmptyCSR`     | Kiểm tra input CSR rỗng.                                                                    | Service trả về lỗi.                                                                                                                  |
| `TestRegisterUser_InvalidPEM`   | Kiểm tra chuỗi PEM không hợp lệ.                                                            | Service trả về lỗi.                                                                                                                  |
| `TestRegisterUser_WrongPEMType` | Kiểm tra PEM đúng format nhưng sai type, ví dụ `CERTIFICATE` thay vì `CERTIFICATE REQUEST`. | Service trả về lỗi.                                                                                                                  |

### GetCertificate

| Test case                          | Mục đích                                                     | Kết quả mong đợi                                                           |
| ---------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------------------------- |
| `TestGetCertificate_AfterRegister` | Lấy certificate bằng serial sau khi đăng ký user thành công. | Trả về cert PEM, đúng user ID, status `VALID`, `notAfter` trong tương lai. |
| `TestGetCertificate_NotFound`      | Lấy certificate với serial không tồn tại.                    | Service trả về lỗi.                                                        |

### CheckRevocation và RevokeCertificate

| Test case                              | Mục đích                                        | Kết quả mong đợi                                                         |
| -------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------ |
| `TestCheckRevocation_ValidCert`        | Kiểm tra certificate mới cấp và chưa bị revoke. | Status là `VALID`, reason rỗng, `revokedAt` bằng 0.                      |
| `TestCheckRevocation_AfterRevoke`      | Revoke certificate rồi kiểm tra lại trạng thái. | Status là `REVOKED`, reason đúng với lý do revoke, `revokedAt` được gán. |
| `TestRevokeCertificate_AlreadyRevoked` | Thử revoke cùng một certificate hai lần.        | Lần revoke thứ hai trả về lỗi.                                           |

### Anti-replay / Isolation

| Test case                         | Mục đích                                           | Kết quả mong đợi                                                                                         |
| --------------------------------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `TestMultipleUsers_IsolatedCerts` | Đăng ký nhiều user và đảm bảo certificate độc lập. | Mỗi user có serial khác nhau; revoke certificate của user này không ảnh hưởng certificate của user khác. |

## Phạm vi hiện tại

Bộ test hiện tại tập trung vào service layer với Root CA và store tạm thời. Các test chưa kiểm tra tích hợp qua network/API, database thật, hoặc các trường hợp certificate hết hạn theo thời gian thực.
