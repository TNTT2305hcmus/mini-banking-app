# Seed And Accounts

File này mô tả dữ liệu demo và các tài khoản/token cần chuẩn bị trước rehearsal.

## 1. Seed database

Bank seed chính:

```text
mini-banking-app/db/bank/seed_demo.sql
```

Compose local/demo mount seed này vào `bank-postgres`:

```text
/docker-entrypoint-initdb.d/020_seed_demo.sql
```

Seed chỉ chạy tự động khi Postgres volume được tạo lần đầu. Nếu volume đã tồn tại, seed không tự chạy lại.

Apply seed thủ công:

```powershell
Get-Content -Raw .\db\bank\seed_demo.sql |
  docker compose -f docker-compose.local.yml exec -T bank-postgres psql -U banking -d banking
```

## 2. KDC audit schema

KDC audit dùng chung `bank-postgres` trong compose final.

Migrations:

```text
db/kdc/migrations/001_init_kdc.sql
db/kdc/migrations/002_add_audit_hash_chain.sql
```

Nếu SOC không thấy AS/TGS event, kiểm tra bảng:

```powershell
docker compose -f docker-compose.local.yml exec -T bank-postgres `
  psql -U banking -d banking -c "select count(*) from kdc_audit_log;"
```

## 3. Demo identities

Các email demo nên dùng thống nhất khi chấm/rehearsal:

```text
customer.demo@demo.minibanking.local
ca.admin@demo.minibanking.local
bank.admin@demo.minibanking.local
security@minibanking.local
```

Smoke OTP dùng biến:

```env
DEMO_EMAIL=customer.demo@demo.minibanking.local
```

Customer không phải password-seeded. Customer cần đăng ký một lần trên browser
để sinh private key, CSR và certificate trong IndexedDB. Seed Bank DB chỉ cung
cấp dữ liệu tài khoản/transaction để admin dashboard và transfer có dữ liệu.

Nếu SMTP thật chưa sẵn sàng, chạy smoke với `-SkipSmtp` hoặc
`SKIP_SMTP_CHECK=1`. OTP thật chỉ cần bật khi muốn test đường email thật.

## 4. Gửi mail activation cho Admin CA và Admin Bank

Code đã có sẵn 2 script gửi activation email:

```text
mini-banking-app/api-gateway/src/scripts/provision-ca-admin.ts
mini-banking-app/api-gateway/src/scripts/provision-bank-admin.ts
```

Các script này tạo pending admin trong Redis và sinh activation token. Mặc định
script gửi link activation về Gmail/email được chỉ định bằng tham số `--email`.
Khi demo nhanh không có SMTP, thêm `--print-only` để in `activation_url` ra
terminal và không gửi email.

### 4.1. Chạy khi stack Docker Compose đang bật

Chạy từ runtime root:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

Gửi mail kích hoạt CA Admin:

```powershell
docker compose -f docker-compose.local.yml exec api-gateway `
  node dist/scripts/provision-ca-admin.js `
  --email "ca.admin@demo.minibanking.local" `
  --full-name "CA Administrator" `
  --print-only
```

Gửi mail kích hoạt Bank Admin:

```powershell
docker compose -f docker-compose.local.yml exec api-gateway `
  node dist/scripts/provision-bank-admin.js `
  --email "bank.admin@demo.minibanking.local" `
  --full-name "Bank Administrator" `
  --print-only
```

Nếu dùng demo compose không có frontend dev server:

```powershell
docker compose -f docker-compose.demo.yml exec api-gateway `
  node dist/scripts/provision-ca-admin.js `
  --email "ca.admin@demo.minibanking.local" `
  --full-name "CA Administrator" `
  --print-only

docker compose -f docker-compose.demo.yml exec api-gateway `
  node dist/scripts/provision-bank-admin.js `
  --email "bank.admin@demo.minibanking.local" `
  --full-name "Bank Administrator" `
  --print-only
```

Điều kiện nếu muốn gửi email thật thay vì `--print-only`:

- `api-gateway` container đang chạy.
- `.env` đã cấu hình Gmail SMTP:

```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=<gmail-sender@gmail.com>
SMTP_PASS=<gmail-app-password>
FRONTEND_BASE_URL=http://localhost:5173
ADMIN_ACTIVATION_TTL_SECONDS=900
```

Lưu ý: `SMTP_PASS` phải là Gmail App Password, không phải mật khẩu Gmail thường.

### 4.2. Chạy bằng terminal trong `api-gateway`

Chạy cách này khi đang chạy service thủ công, không qua compose container.

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\api-gateway"
```

Cấu hình env cho terminal:

```powershell
$env:FRONTEND_BASE_URL="http://localhost:5173"
$env:GATEWAY_REDIS_URL="redis://localhost:6379/0"
$env:CA_CERT_PATH="certs/grpc-ca.crt"
$env:EMAIL_USER="<gmail-sender@gmail.com>"
$env:EMAIL_PASS="<gmail-app-password>"
$env:SMTP_HOST="smtp.gmail.com"
$env:SMTP_PORT="587"
$env:GATEWAY_OTP_SECRET="<secret>"
$env:GATEWAY_JWT_SECRET="<at-least-32-chars>"
$env:OTP_MAX_ATTEMPTS="5"
$env:OTP_EXPIRES_IN="300"
$env:OTP_COOLDOWN="60"
$env:RATE_LIMIT_DISABLED="1"
```

Gửi mail kích hoạt CA Admin:

```powershell
npm run provision:ca-admin -- --email "<gmail-ca-admin@gmail.com>" --full-name "CA Administrator"
```

Gửi mail kích hoạt Bank Admin:

```powershell
npm run provision:bank-admin -- --email "<gmail-bank-admin@gmail.com>" --full-name "Bank Administrator"
```

Khác biệt tên biến:

- Khi chạy Docker Compose, `.env` dùng `SMTP_USER` và `SMTP_PASS`; compose map hai biến này vào container thành `EMAIL_USER` và `EMAIL_PASS`.
- Khi chạy script trực tiếp trong folder `api-gateway`, code đọc `EMAIL_USER` và `EMAIL_PASS`.

Sau khi nhận mail hoặc copy `activation_url` từ terminal:

- CA Admin link mở `/admin-ca/activate#token=...`.
- Bank Admin link mở `/admin-bank/activate#token=...`.

Sau khi activate xong, browser sinh keypair/CSR, service cấp cert admin tương ứng, rồi admin login bằng PIN/cert.

## 5. Admin CA

Admin CA không dùng password/static-token cũ.

Luồng đúng:

1. Provision pending CA Admin bằng script Gateway.
2. Mở link `/admin-ca/activate`.
3. Browser sinh keypair/CSR.
4. CA cấp cert role `ca_admin`.
5. Login `/admin-ca` bằng PIN/cert.
6. Dùng cert-backed session token cho curl/smoke nếu cần.

Biến smoke:

```env
ADMIN_CA_TOKEN=<cert-backed-admin-ca-session-token>
```

## 6. SOC / Security Admin

SOC login dùng:

```env
ADMIN_SEC_DEMO_EMAIL=security@minibanking.local
ADMIN_SEC_DEMO_PASSWORD=<secret>
ADMIN_SEC_DEMO_TOKEN=<dev-static-token>
```

Smoke có thể:

- dùng `ADMIN_SEC_DEMO_TOKEN` trực tiếp;
- hoặc login `/v1/admin-sec/auth` bằng email/password để lấy JWT.

SOC surfaces:

- `/v1/admin-kdc/audit`
- `/v1/admin/audit/timeline`
- `/v1/admin/audit/verify`
- `/v1/admin/audit/summary`
- `/v1/admin/audit/export`

## 7. Admin Bank

Admin Bank dùng cert/session cookie, không phải token tĩnh.

Flow đầy đủ thường chạy bằng UI:

1. Activate Bank Admin.
2. Login bằng cert/PIN.
3. Gateway tạo `bank_admin_session` cookie.
4. Mở dashboard Admin Bank.

Smoke hiện chỉ test negative case không có session cookie. Full flow được kiểm trong functional demo/rehearsal.

## 8. Browser state

Credential local nằm trong IndexedDB. Khi đổi namespace/cert hoặc test lại từ đầu, dùng browser sạch hoặc clear IndexedDB của app.

Các scope credential:

- `customer:*`
- `bank_admin:*`
- `ca_admin:*`

Nếu UI đọc nhầm cert cũ, clear IndexedDB rồi activate/login lại.
