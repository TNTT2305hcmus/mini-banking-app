# gRPC Proto Design — Mini_App_Banking

> **Nguyên tắc chung:**
> - `.proto` là **hợp đồng bất biến** — một khi team đã generate stub và code theo, không được đổi field name/number.
> - Payload mật mã giữ dạng `bytes` opaque — Gateway và KDC chỉ forward, không inspect.
> - Lỗi nghiệp vụ trả qua **gRPC Status Code**, không nhồi vào response payload.
> - Dùng **enum** thay `bool`/`string` cho các trạng thái có tập giá trị cố định — type-safe, tránh typo.

---

## 1. `ca.proto` — CA Service

**Vai trò:** Nguồn tin cậy duy nhất (*single source of truth*) cho public key của mọi client. Quản lý toàn bộ vòng đời X.509 certificate.

**4 RPC:**

| RPC | Ai gọi | Mô tả |
|-----|--------|-------|
| `RegisterUser` | Gateway | Nhận PKCS#10 CSR, xác minh chữ ký, ký và trả X.509 cert |
| `GetCertificate` | KDC, Bank | Lookup cert theo serial number để lấy `pub_c` và kiểm tra hạn |
| `CheckRevocation` | KDC, Bank | OCSP-like: trả `CertStatus` — phân biệt VALID / REVOKED / EXPIRED |
| `RevokeCertificate` | Admin | Thu hồi cert khẩn cấp — thất bại trả qua gRPC Status Code |

**Quyết định thiết kế:**

`enum CertStatus { VALID, REVOKED, EXPIRED }` thay vì `bool` — `bool` không phân biệt được cert hết hạn tự nhiên với cert bị thu hồi chủ động, trong khi OCSP cần phân biệt rõ hai trường hợp này.

`RevokeCertificateResponse` để **rỗng** — tránh anti-pattern `bool success`. Thất bại đã có `codes.NotFound`, `codes.AlreadyExists`, `codes.PermissionDenied` của gRPC xử lý.

---

## 2. `kdc.proto` — KDC Service

**Vai trò:** Triển khai Kerberos hybrid PKI. Xử lý 2 bước trao đổi ticket trước khi client được phép gọi Bank Service.

**2 RPC:**

| RPC | Phase | Input → Output |
|-----|-------|----------------|
| `RequestTGT` | Phase 2 — AS Exchange | `ASRequest` → `ASResponse` chứa TGT |
| `RequestServiceTicket` | Phase 3 — TGS Exchange | `TGSRequest` → `TGSResponse` chứa `Ticket_v` có scope |

**Quyết định thiết kế:**

**PKINIT — Proof of Possession (RFC 4556):**
`ASRequest` bắt buộc field `pre_auth_signature = Sign({client_id ‖ tgs_id ‖ nonce1 ‖ timestamp}, priv_c)`. KDC verify chữ ký bằng `pub_c` từ CA trước khi cấp TGT — chặn attacker dùng `cert_sn` của người khác mà không có private key.

**Bỏ IP binding:**
TGT không chứa `client_ip`. Môi trường web/mobile có NAT và DHCP — bind IP gây false-positive liên tục. Bù đắp bằng TTL ngắn (TGT: 30 phút, `Ticket_v`: 5 phút) và `Authenticator` bắt buộc kèm theo mọi request.

**Clock Skew tolerance ±5 phút:**
`timestamp` trong `ASRequest` bị reject nếu lệch quá ngưỡng → `codes.DeadlineExceeded`. Chống Replay Attack mà không phạt false-positive do NTP drift giữa client và server.

**Scope-based Authorization:**
`TGSRequest.requested_scope` (vd: `"transfer:internal"`, `"account:read"`) được KDC đóng dấu vào `Ticket_v`. Bank Service enforce cứng khi nhận ticket — không cần gọi lại KDC ở Phase 4.

**Payload opaque:**
`encrypted_payload` trong cả `ASResponse` và `TGSResponse` là `bytes` — Gateway chỉ forward, không thể đọc `K_{c,tgs}`, TGT, hay `K_{c,v}` bên trong.

---

## 3. `bank.proto` — Bank Service

**Vai trò:** Thực thi AP Exchange — nhận `Ticket_v` + `Authenticator` + `Cipher`, thực hiện giao dịch ACID sau khi vượt qua toàn bộ chuỗi kiểm tra bảo mật.

**3 RPC:**

| RPC | Mô tả |
|-----|-------|
| `TransferMoney` | Chuyển tiền — đầy đủ non-repudiation + anti-replay + revocation check |
| `GetBalance` | Truy vấn số dư — cũng yêu cầu `Ticket_v` + `Authenticator` đầy đủ |
| `GetTransactions` | Lịch sử giao dịch với cursor-based pagination và filter theo thời gian |

**Cấu trúc AP_REP — Mutual Authentication:**

Mỗi response đều có field `ap_rep = E_{K_{c,v}}[...]`. Client **bắt buộc** verify trước khi tin kết quả. Có 3 loại tương ứng 3 RPC:

| Message | Dùng cho | Nội dung |
|---------|----------|----------|
| `APRepResult` | `TransferResponse` | `ts_5_plus_1`, `status` (enum), `amount`, `balance_after`, `completed_at` |
| `APRepBalance` | `BalanceResponse` | `ts_plus_1`, `account_id`, `balance`, `last_transaction_at` |
| `APRepTransactions` | `TransactionHistoryResponse` | `ts_plus_1` — chỉ mutual auth, records để plaintext tránh encrypt list lớn |

`TransactionStatus enum { SUCCESS, FAILED }` dùng trong `APRepResult.status` thay `string` — lý do tương tự `CertStatus`.

**Quyết định thiết kế:**

**`cert_sn` trên mọi request:**
Bank gọi `CA.CheckRevocation(cert_sn)` trước `BEGIN TRANSACTION`, cache Redis TTL 3 phút tránh CA thành bottleneck. Đảm bảo cert bị thu hồi khẩn cấp vẫn bị chặn ngay cả khi `Ticket_v` còn hạn.

**`idempotency_key` tách khỏi `cipher`:**
Đặt ngoài payload mã hóa để Bank check double-spend mà không cần decrypt toàn bộ `cipher` — tối ưu cho trường hợp client retry do mất kết nối mạng.

**Cursor-based pagination:**
`TransactionHistoryRequest` dùng `cursor_last_tx_id` + `from_ts`/`to_ts` thay `page`/`offset`. Dữ liệu tài chính insert liên tục — offset-based gây trùng/sót bản ghi khi chuyển trang.

**`TransactionRecord.status_trail`:**
`repeated TransactionStatusEvent` map với bảng `transaction_details` trong DB — cho phép UI hiển thị audit trail đầy đủ lịch sử thay đổi trạng thái của từng giao dịch.

---

## 4. Khả năng mở rộng

**Thêm service mới** (vd: `loan-service`, `card-service`): chỉ cần đăng ký `service_id` mới với KDC và định nghĩa scope tương ứng — không đụng vào `ca.proto` hay `kdc.proto`.

**Multi-tenant / Intermediate CA**: thêm field `issuer_chain_pem` vào `RegisterUserResponse` — không breaking change nhờ field numbering của protobuf, client cũ bỏ qua field mới.

**Streaming audit log**: `GetTransactions` đang dùng unary RPC. Khi dữ liệu lớn, đổi sang `server-streaming` (`returns (stream TransactionRecord)`) mà không thay đổi logic business.

**Tích hợp HSM**: CA Service hiện ký cert bằng key trên disk. Để dùng HSM chỉ cần thay implementation Go của `RegisterUser` — proto contract không đổi.

**Ticket Renewal**: thêm `RenewTGT(RenewRequest) returns (ASResponse)` vào `KDCService` mà không ảnh hưởng flow hiện tại — client cũ vẫn dùng `RequestTGT` bình thường.
