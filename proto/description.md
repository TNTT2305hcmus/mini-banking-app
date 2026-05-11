## Ý tưởng thiết kế 3 file `.proto`

### Nguyên tắc chung

Các file `.proto` đóng vai trò **hợp đồng bất biến** giữa các service — một khi team giai đoạn 2 đã generate stub và code theo, không được thay đổi field name/number. Vì vậy thiết kế cần **đúng ngay từ đầu**.

---

### `ca.proto` — CA Service

**Ý tưởng:** CA là service đơn giản nhất về gRPC — chủ yếu là CRUD certificate. 4 RPC:

```
RegisterUser     → nhận CSR, trả X.509
GetCertificate   → lookup theo serial number
CheckRevocation  → KDC và Bank đều cần gọi cái này
RevokeCertificate → admin dùng
```

Điểm quan trọng: `RegisterUser` nhận `csr_pem` dạng string PEM thay vì raw bytes — dễ debug hơn, và Go/Node đều handle PEM tốt.

---

### `kdc.proto` — KDC Service

**Ý tưởng:** Ánh xạ trực tiếp 2 giai đoạn Kerberos:

```
RequestTGT             → AS Exchange (Phase 2)
RequestServiceTicket   → TGS Exchange (Phase 3)
```

Điểm thiết kế quan trọng nhất: **payload được giữ dạng `bytes` opaque** — tức là Gateway và KDC không cần biết bên trong `encrypted_payload` chứa gì. Chỉ có Client mới decrypt được. Điều này đúng với đặc tính Kerberos: Gateway chỉ forward, không inspect.

`ASRequest` có thêm `cert_sn` để KDC fetch `pub_c` từ CA trước khi encrypt `AS_REP`.

---

### `bank.proto` — Bank Service

**Ý tưởng:** Thiết kế xoay quanh AP Exchange — mọi request đều phải kèm `ticket_v` + `authenticator`. Đây là enforced ở proto level, không phải middleware — tức là nếu client quên gửi ticket thì sẽ fail ngay khi validate message, không cần đến business logic.

3 RPC:
```
TransferMoney        → Phase 4 chính
GetBalance           → cũng cần authenticate đầy đủ
GetTransactions      → có pagination
```

`cipher` trong `TransferRequest` là `bytes` chứa `E_{K_{c,v}}[Payload + Signature]` — Bank decrypt ra rồi mới verify chữ ký bằng `pub_c` extract từ `Ticket_v`.

---

### `gen-proto.sh` — Script generate stub

**Ý tưởng:** Chạy 1 lệnh duy nhất, generate cho cả 3 target:

```
proto/ ──► Go stub cho ca-service, kdc-service, bank-service
       ──► TypeScript stub cho gateway (Node.js)
```

Dùng `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` cho Go, và `ts-proto` cho TypeScript. Script có check dependency trước khi chạy, in lỗi rõ ràng nếu thiếu tool.

---

### Tóm tắt quyết định thiết kế

| Quyết định | Lý do |
|---|---|
| Payload nhạy cảm dùng `bytes` thay vì struct | Không expose crypto internals qua proto |
| Mọi field đều có field number cố định | Tránh breaking change khi thêm field sau |
| `cert_sn` string hex thay vì int | Serial number CA có thể rất lớn, tránh overflow |
| Tách `CheckRevocation` riêng | KDC và Bank đều cần gọi độc lập |
| `GetTransactions` có `page` + `page_size` | Banking history có thể rất dài |

---