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

## 3. Demo users

Email smoke mặc định:

```text
alice@demo.minibanking.local
```

Smoke OTP dùng biến:

```env
DEMO_EMAIL=alice@demo.minibanking.local
```

Nếu SMTP thật chưa sẵn sàng, chạy smoke với `-SkipSmtp` hoặc `SKIP_SMTP_CHECK=1`.

## 4. Admin CA

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

## 5. SOC / Security Admin

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

## 6. Admin Bank

Admin Bank dùng cert/session cookie, không phải token tĩnh.

Flow đầy đủ thường chạy bằng UI:

1. Activate Bank Admin.
2. Login bằng cert/PIN.
3. Gateway tạo `bank_admin_session` cookie.
4. Mở dashboard Admin Bank.

Smoke hiện chỉ test negative case không có session cookie. Full flow được kiểm trong functional demo/rehearsal.

## 7. Browser state

Credential local nằm trong IndexedDB. Khi đổi namespace/cert hoặc test lại từ đầu, dùng browser sạch hoặc clear IndexedDB của app.

Các scope credential:

- `customer:*`
- `bank_admin:*`
- `ca_admin:*`

Nếu UI đọc nhầm cert cũ, clear IndexedDB rồi activate/login lại.
