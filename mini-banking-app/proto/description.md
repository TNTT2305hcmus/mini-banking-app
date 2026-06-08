# gRPC Proto Design - Mini Banking

Tài liệu này mô tả contract nội bộ trong `mini-banking-app/proto/`. REST API public vẫn nằm ở `blueprint/api-design/`; proto chỉ dùng cho Gateway gọi CA/KDC/Bank qua gRPC.

---

## Nguyên tắc chung

- Payload mật mã giữ dạng `bytes` opaque; Gateway forward, không inspect key material.
- Lỗi nghiệp vụ trả qua gRPC status code, không dùng `success: bool`.
- Public key chỉ lấy từ CA qua certificate hợp lệ, không nhận raw public key từ request làm nguồn tin cậy.
- Field number đã publish không được tái sử dụng cho ý nghĩa khác.
- Generated code nằm trong `pkg/pb/` và `api-gateway/src/proto/`, không sửa tay.

---

## `ca.proto`

CA Service là nguồn sự thật duy nhất cho certificate lifecycle.

| RPC | Caller | Mục đích |
|---|---|---|
| `RegisterUser` | Gateway | Nhận CSR, verify proof-of-possession, cấp X.509 certificate |
| `VerifyCertificate` | KDC, Bank | Lookup status, validity, certificate/public key theo serial |
| `ListCertificates` | Gateway/Admin | List/search certificate cho Admin Dashboard |
| `GetCertificateDetail` | Gateway/Admin | Detail view và ghi audit lookup |
| `RevokeCertificate` | Gateway/Admin | Revoke certificate với reason và audit metadata |
| `GetCertificate` | Legacy | Deprecated compatibility method |
| `CheckRevocation` | Legacy | Deprecated compatibility method |

`VerifyCertificate` là fast path chính cho KDC/Bank vì gom status, validity và public key trong một response nhất quán. `GetCertificate` và `CheckRevocation` chỉ giữ tạm để code cũ có đường migrate.

---

## `kdc.proto`

KDC Service xử lý hai bước Kerberos-like:

| RPC | Phase | Mô tả |
|---|---|---|
| `RequestTGT` | AS Exchange | Client ký AS_REQ bằng private key; KDC verify cert qua CA và trả `as_rep` |
| `RequestServiceTicket` | TGS Exchange | Client dùng TGT xin `Ticket_v` theo `scope` và `service_id` |

`TicketPayload` mô tả nội dung logic của ticket đã mã hóa: `id_c`, `cert_sn`, session key, scope, service id, lifetime, key version và ticket id. Payload thực tế vẫn là bytes opaque sau AES-GCM.

---

## `bank.proto`

Bank Service sở hữu user/account/transaction domain, AP Exchange, idempotency và hash-chain ledger.

| RPC | REST mapping | Mục đích |
|---|---|---|
| `CreateUser` | `POST /v1/pki/register` orchestration | Tạo Bank user sau khi CA cấp certificate thành công |
| `TransferMoney` | `POST /v1/bank/transfer` | AP Exchange + transfer ACID + hash-chain ledger |
| `GetBalance` | `POST /v1/bank/accounts/{id}/balance/query` | Read path bảo mật với scope `balance:read` |
| `GetHistory` | `POST /v1/bank/accounts/{id}/transactions/query` | Read path bảo mật với scope `history:read` |

Transfer request không nhận `cert_sn` rời rạc từ REST body. Bank Service lấy `cert_sn`, `ID_c`, `scope` và `K_{c,v}` từ `Ticket_v`, sau đó gọi CA `VerifyCertificate`.

---

## Generate

Chạy từ thư mục `mini-banking-app/`:

```bash
./gen-proto.sh          # Go + TypeScript
./gen-proto.sh --go     # chỉ Go stubs vào pkg/pb/{ca,kdc,bank}
./gen-proto.sh --ts     # chỉ TypeScript stubs vào api-gateway/src/proto
```
