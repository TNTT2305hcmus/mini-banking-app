# Mini-Banking-App - Workflow Handoff

Tài liệu này giúp một context window mới nắm nhanh dự án, trạng thái hiện tại và cách tiếp tục làm việc trong repo.

---

## 1. Snapshot hiện tại

Mini-Banking-App là đồ án mô phỏng hệ thống ngân hàng số tập trung vào xác thực an toàn, quản lý chứng chỉ và giao dịch có chữ ký số.

Thiết kế hiện tại đã được chuẩn hóa trong:

- `blueprint/proposal.md`: proposal dự án.
- `blueprint/design.md`: technical design chính, là nguồn tham chiếu quan trọng nhất.
- `README.md`: mô tả ngắn gọn dự án.

Repo đã được tái cấu trúc:

```text
.
+-- blueprint/
|   +-- proposal.md
|   +-- design.md
+-- mini-banking-app/
|   +-- api-gateway/
|   +-- banking-service/
|   +-- ca-service/
|   +-- db/
|   +-- frontend/
|   +-- kdc-service/
|   +-- pkg/
|   +-- proto/
+-- README.md
+-- set_up_guide.md
+-- WORKFLOW.md
+-- CODEX.md
```

`api-design-docs/` cũ đã bị xóa. Source/app hiện nằm trong `mini-banking-app/`. Blueprint nằm ở root-level `blueprint/`.

---

## 2. Phạm vi và actor đã chốt

Actor chỉ gồm:

| Actor | Entry point | Vai trò |
|---|---|---|
| Khách hàng | Customer Web App | Đăng ký OTP/PKI, lấy ticket, xem số dư/lịch sử, chuyển tiền |
| Admin | Admin Web App Dashboard | Quản lý PKI/CA, tra cứu certificate, xem trạng thái, revoke X.509 |

Không còn actor riêng kiểu `Nhân sự vận hành` hoặc `Admin/Audit nội bộ` như bản cũ. Admin hiện được hiểu là người quản trị certificate X.509 qua dashboard.

---

## 3. Kiến trúc đã chốt

Kiến trúc chính: **Layered Service Architecture với gRPC Internal Communication**.

| Layer | Thành phần |
|---|---|
| Client | Customer Web App, Admin Web App Dashboard |
| Gateway / DMZ | API Gateway |
| Internal Services | CA Service, KDC Service, Bank Service |
| Data Stores | CA PostgreSQL DB, Bank PostgreSQL DB, Redis |
| External | Email/OTP Provider |

Các điểm quan trọng:

- Client gọi API Gateway qua HTTPS/REST.
- API Gateway forward vào internal services bằng `gRPC` (không dùng mTLS; network isolation trong Docker bảo vệ internal traffic).
- CA Service có PostgreSQL DB riêng để lưu certificate metadata và phục vụ Admin Dashboard.
- Bank Service có PostgreSQL DB riêng để lưu accounts, transactions, audit logs và immutable ledger.
- Redis dùng cho OTP TTL, replay cache, rate limit counters và revocation cache.

---

## 4. Luồng chính

1. Customer Registration & PKI Enrollment:
   Khách hàng xác minh OTP, sinh key pair ở browser, gửi CSR, nhận X.509 certificate từ CA Service.

2. Kerberos-like Authentication:
   Khách hàng thực hiện AS Exchange để lấy TGT + `K_{c,tgs}`, sau đó TGS Exchange để lấy `Ticket_v` + `K_{c,v}` theo scope.

3. Secure Banking Transaction:
   Khách hàng ký số payload giao dịch, mã hóa request, gửi `Ticket_v`; Bank Service kiểm tra replay, revocation, chữ ký, authorization rồi ghi ledger.

4. PKI Admin Certificate Management:
   Admin dùng Dashboard để list/search/detail/revoke certificate X.509. Dữ liệu lấy từ CA PostgreSQL DB qua CA Service.

Chi tiết sequence diagram nằm trong `blueprint/design.md`.

---

## 5. Các quyết định kỹ thuật quan trọng

Các ADR đã được ghi trong `blueprint/design.md`, tóm tắt:

| ADR | Quyết định |
|---|---|
| ADR-01 | Layered Service Architecture với Gateway/DMZ và internal services |
| ADR-02 | Internal communication dùng gRPC, không dùng mTLS |
| ADR-03 | Zero-Knowledge private key bằng WebCrypto |
| ADR-04 | Certificate-based trust với X.509 và CA Service |
| ADR-05 | CA có PostgreSQL DB riêng |
| ADR-06 | Kerberos-like ticket flow thay JWT dài hạn |
| ADR-07 | `Ticket_v` reusable trong TTL nhưng mỗi request phải chống replay |
| ADR-08 | Không dùng `K_sub`; dùng `K_{c,v}` trực tiếp với AES-GCM random IV |
| ADR-09 | Bank Service là điểm ACID transaction duy nhất |
| ADR-10 | Immutable ledger bằng Hash Chaining |

---

## 6. Cơ chế bảo mật cần giữ nhất quán

Khi sửa thiết kế hoặc code, luôn giữ các invariant sau:

- Private key của khách hàng sinh ở browser, không gửi plaintext lên server.
- Public key chỉ được tin khi nằm trong X.509 certificate hợp lệ do CA ký.
- KDC và Bank Service không nhận raw public key từ request làm nguồn tin cậy.
- AS_REQ, TGS_REQ và AP_REQ đều cần nonce + timestamp + request id.
- Replay cache dùng Redis `SET NX EX` hoặc cơ chế tương đương.
- Bank Service phải strict revocation check trước giao dịch.
- `Ticket_v` phải có scope, `service_id`, lifetime và session key.
- Bank Service kiểm tra scope, ownership, account status, daily limit trước khi ghi DB.
- Transfer cần idempotency key để tránh double processing khi retry.
- Ledger là append-only và có hash chaining.
- Admin revoke certificate phải có Admin Auth, reason và audit log.

---

## 7. Tài liệu cần đọc khi mở context mới

Đọc theo thứ tự:

1. `WORKFLOW.md` - file này, để nắm snapshot.
2. `README.md` - mô tả ngắn gọn dự án.
3. `blueprint/design.md` - nguồn thiết kế chính.
4. `blueprint/proposal.md` - scope/problem/risk nếu cần đối chiếu.
5. File code liên quan trực tiếp trong `mini-banking-app/`.

Không dùng `mini-banking-app/db/design.md` hoặc `mini-banking-app/detailSequenceDiagram.txt` làm nguồn thiết kế chính nữa nếu mâu thuẫn với `blueprint/design.md`; chúng là tài liệu cũ/triển khai ban đầu.

---

## 8. Trạng thái implementation

Hiện tại:

- CA Service và KDC Service đã có implementation ban đầu.
- Thiết kế cũ có nhiều phần chưa khớp scope/API/spec mới.
- Blueprint hiện là nguồn sự thật để tái cấu trúc implementation.
- README đã được rút gọn để giới thiệu dự án.

Ưu tiên tiếp theo nên là:

1. Tạo API/spec mới trong `blueprint/` hoặc thư mục con phù hợp.
2. Rà proto hiện có trong `mini-banking-app/proto/` theo `blueprint/design.md`.
3. Chuẩn hóa CA Service trước vì CA DB/Admin Dashboard/revocation là nền cho KDC và Bank.
4. Chuẩn hóa KDC AS/TGS theo ticket policy mới.
5. Chuẩn hóa Bank Service AP Exchange, authorization, idempotency và hash-chain ledger.

---

## 9. Cách làm việc tiếp trong repo

Khi bắt đầu task mới:

1. Chạy `git status --short` để tránh ghi đè thay đổi của người khác.
2. Đọc `blueprint/design.md` phần liên quan trước khi sửa code.
3. Nếu task chạm security flow, cập nhật design trước hoặc song song với code.
4. Không tự gộp CA/KDC/Bank Service; giữ service boundary đã chốt.
5. Không thêm feature ngoài scope đồ án nếu chưa được yêu cầu.
6. Sau thay đổi đáng kể về scope, API, security decision hoặc folder structure, cập nhật lại `WORKFLOW.md`.
