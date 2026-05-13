# gRPC Proto Design — Mini_App_Banking

> **Nguyên tắc chung:** Các file `.proto` là hợp đồng bất biến giữa các service. Payload nhạy cảm về mặt mật mã được giữ dạng `bytes` opaque — Gateway và KDC chỉ forward, không inspect nội dung. Lỗi nghiệp vụ trả qua gRPC Status Code, không nhồi vào payload.

---

## 1. `ca.proto` — CA Service

**Vai trò:** Quản lý toàn bộ vòng đời X.509 certificate, là nguồn tin cậy duy nhất (`single source of truth`) cho public key của mọi client.

**4 RPC:**

| RPC | Mô tả |
|-----|-------|
| `RegisterUser` | Nhận PKCS#10 CSR, xác minh chữ ký, ký và trả về X.509 cert |
| `GetCertificate` | Lookup cert theo serial number — KDC và Bank đều gọi để lấy `pub_c` |
| `CheckRevocation` | OCSP-like: kiểm tra cert còn hiệu lực hay đã bị revoke/expired |
| `RevokeCertificate` | Thu hồi cert khẩn cấp (admin) — thất bại trả qua gRPC Status Code |

**Quyết định thiết kế nổi bật:**
- `enum CertStatus { VALID, REVOKED, EXPIRED }` thay `bool` — phân biệt được cert hết hạn tự nhiên với cert bị thu hồi chủ động, bám sát chuẩn OCSP.
- `RevokeCertificateResponse` để rỗng — tránh anti-pattern `bool success` vì gRPC đã có cơ chế lỗi riêng (`codes.NotFound`, `codes.AlreadyExists`...).

---

## 2. `kdc.proto` — KDC Service

**Vai trò:** Triển khai Kerberos hybrid PKI, xử lý 2 bước trao đổi ticket trước khi client được phép gọi Bank Service.

**2 RPC:**

| RPC | Phase | Mô tả |
|-----|-------|-------|
| `RequestTGT` | Phase 2 — AS Exchange | Client xác thực danh tính, nhận Ticket Granting Ticket |
| `RequestServiceTicket` | Phase 3 — TGS Exchange | Client dùng TGT để xin Service Ticket có gắn scope |

**Quyết định thiết kế nổi bật:**
- **PKINIT (RFC 4556):** `ASRequest` bắt buộc `pre_auth_signature = Sign({client_id ‖ tgs_id ‖ nonce1 ‖ timestamp}, priv_c)`. KDC chỉ cấp TGT sau khi verify chữ ký — đảm bảo *Proof of Possession*, chặn attacker replay `cert_sn` của người khác.
- **Bỏ IP binding:** TGT không bind `client_ip` để tránh false-positive trên môi trường web/mobile có NAT và DHCP. Bù đắp bằng TTL ngắn (TGT: 30 phút, `Ticket_v`: 5 phút) và `Authenticator` bắt buộc trong mọi request.
- **Clock Skew tolerance ±5 phút:** `timestamp` trong `ASRequest` bị reject nếu lệch quá ngưỡng — chống Replay Attack mà không phạt false-positive do NTP drift.
- **Scope-based Authorization:** `TGSRequest` mang `requested_scope` (vd: `transfer:internal`, `account:read`). KDC đóng dấu scope vào `Ticket_v` — Bank Service enforce cứng, không cần gọi lại KDC.
- **Payload mật mã dạng `bytes` opaque:** Gateway chỉ forward `encrypted_payload`, không thể đọc `K_{c,tgs}` hay nội dung TGT bên trong.

---

## 3. `bank.proto` — Bank Service

**Vai trò:** Thực thi AP Exchange — nhận `Ticket_v` + `Authenticator` + `Cipher`, thực hiện giao dịch ACID sau khi vượt qua toàn bộ chuỗi kiểm tra bảo mật.

**3 RPC:**

| RPC | Mô tả |
|-----|-------|
| `TransferMoney` | Giao dịch chuyển tiền — đầy đủ non-repudiation + anti-replay + revocation check |
| `GetBalance` | Truy vấn số dư — cũng yêu cầu `Ticket_v` + `Authenticator` đầy đủ |
| `GetTransactions` | Lịch sử giao dịch với cursor-based pagination |

**Quyết định thiết kế nổi bật:**
- **`cert_sn` trên mọi request:** Bank gọi `CA.CheckRevocation(cert_sn)` trước `BEGIN TRANSACTION`, cache Redis TTL 3 phút tránh CA thành bottleneck. Đảm bảo cert bị thu hồi khẩn cấp sẽ bị chặn ngay cả khi `Ticket_v` vẫn còn hạn.
- **`idempotency_key` tách khỏi `cipher`:** Đặt ngoài payload mã hóa để Bank có thể check double-spend mà không cần decrypt toàn bộ `cipher` — tối ưu cho trường hợp client retry do mất kết nối.
- **Cursor-based pagination:** `TransactionHistoryRequest` dùng `cursor_last_tx_id` thay `page`/`offset`. Dữ liệu tài chính insert liên tục — offset-based gây trùng/sót bản ghi khi chuyển trang.
- **`TransactionRecord` có `status_trail`:** Kết nối với bảng `transaction_details` trong DB, cho phép UI hiển thị audit trail đầy đủ của từng giao dịch.

---

## 4. Khả năng mở rộng

**Thêm service mới:** Thiết kế scope-based (`transfer:internal`, `account:read`...) cho phép thêm service mới (vd: `loan-service`, `card-service`) chỉ bằng cách đăng ký `service_id` mới với KDC và định nghĩa scope tương ứng — không đụng vào `ca.proto` hay `kdc.proto`.

**Multi-tenant / nhiều CA:** `ca.proto` có thể mở rộng thêm RPC `ListCertificates` hoặc hỗ trợ intermediate CA bằng cách thêm field `issuer_chain_pem` vào `RegisterUserResponse` — không breaking change với client cũ nhờ field numbering của protobuf.

**Streaming cho audit log:** `GetTransactions` hiện dùng unary RPC. Khi dữ liệu lớn, có thể chuyển sang `server-streaming RPC` (`rpc GetTransactions(...) returns (stream TransactionRecord)`) mà không cần thay đổi logic business.

**Hardware Security Module (HSM):** CA Service hiện ký cert bằng key trên disk. Để tích hợp HSM sau này, chỉ cần thay implementation của `RegisterUser` và `RevokeCertificate` ở tầng Go — proto contract không đổi.

**Refresh Token / Ticket Renewal:** Có thể thêm RPC `RenewTGT(RenewRequest) returns (ASResponse)` vào `KDCService` mà không ảnh hưởng các flow hiện tại — client cũ vẫn dùng `RequestTGT` bình thường.
