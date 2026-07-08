# Demo Bank Admin

## 1. Trạng thái migration CA

Migration [`mini-banking-app/db/ca/migrations/002_add_certificate_role.sql`](mini-banking-app/db/ca/migrations/002_add_certificate_role.sql) đã được áp dụng vào PostgreSQL Neon ngày **04/07/2026** bằng `CA_DATABASE_URL` trong `mini-banking-app/.env`.

Kết quả xác minh:

```text
certificates.role nullable=NO
default='customer'
constraint certificates_role_check=true
index idx_certs_role=true
```

Nếu tạo database Neon mới, mở Neon SQL Editor và chạy toàn bộ file migration trên. Migration có thể chạy lại an toàn vì sử dụng `IF NOT EXISTS` và backfill certificate cũ thành `customer`.

Kiểm tra thủ công trên Neon:

```sql
SELECT column_name, is_nullable, column_default
FROM information_schema.columns
WHERE table_name = 'certificates' AND column_name = 'role';

SELECT role, COUNT(*)
FROM certificates
GROUP BY role
ORDER BY role;
```

## 2. Chuẩn bị hệ thống

Chạy lệnh từ thư mục runtime:

```powershell
cd mini-banking-app
```

Đảm bảo các thành phần sau hoạt động và dùng đúng cấu hình trong `.env` của từng service:

- CA Service: `localhost:50051`, kết nối Neon CA.
- KDC Service: `localhost:50052`.
- Banking Service: `localhost:50053`, kết nối Bank PostgreSQL và Redis.
- API Gateway: `localhost:3000`.
- Frontend: mặc định `http://localhost:5173`.
- Gateway Redis dùng để lưu activation token; Bank Redis dùng để lưu Admin session.

Frontend và Gateway phải cùng origin qua Vite proxy `/v1`. Bank Admin phải dùng browser/profile riêng với Client.

Các lệnh chạy local tham khảo:

```powershell
# Terminal 1
cd ca-service
go run ./cmd/server

# Terminal 2
cd kdc-service
go run ./cmd/server

# Terminal 3
cd banking-service
go run ./cmd/server

# Terminal 4
cd api-gateway
npm.cmd run dev

# Terminal 5
cd frontend
npm.cmd run dev
```

## 3. Provision Bank Admin

Tại `mini-banking-app/api-gateway`:

```powershell
npm.cmd run provision:bank-admin -- --email seversingapore133@gmail.com --full-name "Tri Thanh"
```

Dùng email thật của Bank Admin cho tham số `--email`. Gateway gửi liên kết
kích hoạt một lần tới đúng địa chỉ này. Console chỉ in
trạng thái gửi cùng metadata không nhạy cảm:

```text
admin_id: <uuid>
email: <email-bank-admin>
full_name: <ho-ten-bank-admin>
expires_at: <ISO-8601>
activation_path: /admin-bank/activate
```

Lưu ý:

- Token mặc định hết hạn sau 15 phút.
- Token chỉ nằm trong URL fragment (`#token=...`), không nằm trong query string.
- Frontend xóa fragment khỏi thanh địa chỉ ngay sau khi đọc token.
- Không ghi token vào console, access log, Referer hoặc source code.
- Nếu SMTP gửi thất bại, pending identity và token vừa tạo sẽ bị xóa khỏi Redis.

## 4. Kích hoạt certificate

1. Mở liên kết kích hoạt trong email bằng browser/profile dành riêng cho Bank Admin.
2. Token trong URL fragment được điền vào form; nhập lại đúng email và họ tên đã provision.
3. Đặt PIN 6 chữ số và xác nhận PIN.
4. Nhấn **Kích hoạt**.

Browser thực hiện:

1. Đọc token từ URL fragment và xóa fragment khỏi browser history.
2. Bank Admin tự nhập email và họ tên đã provision.
3. Sinh RSA key pair và wrap private key bằng PIN trong IndexedDB.
4. Tạo CSR từ email/họ tên đã nhập; Gateway đối chiếu CSR với pending identity.
5. Chỉ gửi `activation_token` và `csr_pem` tới Gateway.
6. Lưu certificate được CA cấp vào IndexedDB.

PIN và private key không rời browser.

Sau khi thành công, kiểm tra Neon:

```sql
SELECT owner_id, subject_email, role, status, not_after
FROM certificates
WHERE subject_email = 'bank.admin@example.com'
ORDER BY issued_at DESC
LIMIT 1;
```

Kết quả mong đợi: `role = 'bank_admin'`, `status = 'active'`.

## 5. Đăng nhập Bank Admin

1. Truy cập `http://localhost:5173/admin-bank/login`.
2. Nhập PIN đã đặt.
3. Frontend thực hiện AS Exchange.
4. Frontend xin Ticket_v với scope `bank-admin:read` qua TGS Exchange.
5. Frontend tạo AP Authenticator và gọi `POST /v1/admin/bank/session`.
6. Frontend giải mã AP_REP, kiểm tra `nonce`, `request_id` và `role=bank_admin`.
7. Gateway lưu opaque session token vào cookie `bank_admin_session` với `HttpOnly`, `SameSite=Strict`.

Session token không được trả trong JSON và không được lưu bằng JavaScript.

## 6. Kiểm tra dashboard

Sau đăng nhập, frontend chuyển đến `http://localhost:5173/admin-bank`.

Kiểm tra lần lượt:

- **Tổng quan:** tổng user, account, balance, transaction và audit 24 giờ.
- **Người dùng:** lọc theo email/status, phân trang và nhấn một user để xem account.
- **Ledger:** lọc trạng thái, xem số tiền và current hash.
- **Security Audit:** lọc action và xem request/certificate/reason.

Dashboard chỉ gọi POST:

```text
POST /v1/admin/bank/overview/query
POST /v1/admin/bank/users/query
POST /v1/admin/bank/users/:userId/accounts/query
POST /v1/admin/bank/transactions/query
POST /v1/admin/bank/audit/query
```

## 7. Body mẫu cho Postman

Mọi request cần header:

```text
Content-Type: application/json
X-Request-ID: <UUID v4 mới cho mỗi request>
Cookie: bank_admin_session=<opaque-token>
```

Overview:

```json
{}
```

Users:

```json
{
  "email": "customer@example.com",
  "status": "active",
  "limit": 20,
  "offset": 0
}
```

Transactions:

```json
{
  "status": "completed",
  "from_unix": 0,
  "to_unix": 0,
  "limit": 20,
  "offset": 0
}
```

Audit:

```json
{
  "action": "replay_detected",
  "from_unix": 0,
  "to_unix": 0,
  "limit": 20,
  "offset": 0
}
```

Cookie chỉ được tạo sau khi hoàn tất AS/TGS/AP session. Khi demo bằng browser, không cần đọc hoặc copy cookie bằng JavaScript.

## 8. Các trường hợp cần kiểm tra

| Trường hợp | Kết quả mong đợi |
|---|---|
| Thiếu activation token | `ADMIN_ACTIVATION_INVALID` hoặc lỗi validation 400 |
| Token hết hạn | `ADMIN_ACTIVATION_EXPIRED` |
| Kích hoạt lại Admin đã active | `ADMIN_ALREADY_ACTIVE` |
| Dùng certificate Client xin `bank-admin:read` | `SCOPE_DENIED` |
| Thiếu cookie Admin khi query | `ADMIN_SESSION_REQUIRED` |
| Cookie sai | `ADMIN_SESSION_INVALID` |
| Cookie hết hạn | `ADMIN_SESSION_EXPIRED` |
| Role không phải Bank Admin | `ADMIN_ROLE_REQUIRED` |
| `limit > 100` hoặc `offset < 0` | `INVALID_PAGINATION` |
| `from_unix > to_unix` | `INVALID_DATE_RANGE` |
| Status/action/UUID sai | `INVALID_FILTER` |

## 9. Dọn dữ liệu demo

Không xóa certificate trực tiếp trong database nếu muốn giữ audit trail. Có thể dùng email Admin demo mới cho mỗi lần chạy. Activation identity và token nằm trong Gateway Redis; Admin session tự hết hạn theo Ticket_v.
