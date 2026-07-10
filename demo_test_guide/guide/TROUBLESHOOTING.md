# Troubleshooting

Các lỗi thường gặp khi chạy demo.

## 1. Docker compose config có cảnh báo `.docker/config.json`

Ví dụ:

```text
Error loading config file: open C:\Users\PC\.docker\config.json: Access is denied
```

Nếu `docker compose config` vẫn render ra config và exit code 0, đây không phải lỗi YAML. Khi cần chạy container thật, mở Docker Desktop hoặc sửa quyền Docker config.

## 2. CA container thiếu Client CA

Triệu chứng:

- CA service fail khi start hoặc khi issue cert.
- Log nhắc `client-ca.key` hoặc `client-ca.crt`.

Kiểm tra:

```powershell
Test-Path .\ca-service\certs\intermediate\client-ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.crt
```

Compose phải có:

```yaml
CLIENT_CA_KEY_PATH: /certs/intermediate/client-ca.key
CLIENT_CA_CERT_PATH: /certs/intermediate/client-ca.crt
volumes:
  - ./ca-service/certs/intermediate:/certs/intermediate:ro
```

## 3. SOC không thấy KDC audit

Nguyên nhân thường gặp:

- `kdc-service` không có `DATABASE_URL`.
- Bảng `kdc_audit_log` chưa được tạo vì Postgres volume cũ.
- Chưa có AS/TGS flow nào chạy sau khi audit DB được bật.

Kiểm tra:

```powershell
docker compose -f docker-compose.local.yml exec -T bank-postgres `
  psql -U banking -d banking -c "select count(*) from kdc_audit_log;"
```

Nếu bảng chưa tồn tại, apply migration:

```powershell
Get-Content -Raw .\db\kdc\migrations\001_init_kdc.sql |
  docker compose -f docker-compose.local.yml exec -T bank-postgres psql -U banking -d banking

Get-Content -Raw .\db\kdc\migrations\002_add_audit_hash_chain.sql |
  docker compose -f docker-compose.local.yml exec -T bank-postgres psql -U banking -d banking
```

## 4. Bị 429 khi rehearsal

Tạm tắt rate-limit trong `.env`:

```env
RATE_LIMIT_DISABLED=1
```

Restart API Gateway/compose sau khi đổi env:

```powershell
docker compose -f docker-compose.local.yml up --build -d api-gateway
```

Khi trình bày report, nói rõ đây là demo/rehearsal mode.

## 5. OTP không gửi email

Kiểm tra:

```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=<gmail>
SMTP_PASS=<gmail-app-password>
```

Gmail cần App Password, không dùng mật khẩu Gmail thường.

Nếu chưa cần OTP thật:

```powershell
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

## 6. Admin CA token không chạy

Admin CA không còn password/static token cũ. Cần:

1. Activate CA Admin bằng `/admin-ca/activate`.
2. Login `/admin-ca` bằng PIN/cert.
3. Lấy cert-backed session token.
4. Export token:

```powershell
$env:ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>"
```

Sau đó chạy smoke/curl.

## 7. SOC login fail

Nếu `/v1/admin-sec/auth` trả `503 ADMIN_SEC_NOT_CONFIGURED`, set:

```env
ADMIN_SEC_DEMO_EMAIL=security@minibanking.local
ADMIN_SEC_DEMO_PASSWORD=<secret>
ADMIN_SEC_DEMO_TOKEN=<dev-token>
```

Restart API Gateway.

## 8. Browser đọc nhầm cert cũ

Clear IndexedDB của origin frontend hoặc dùng browser profile sạch.

Các cert/key local được lưu theo scope:

- `customer:*`
- `bank_admin:*`
- `ca_admin:*`

Sau khi clear, activate/login lại.

## 9. Postgres seed/migration không chạy lại

Scripts trong `/docker-entrypoint-initdb.d` chỉ chạy khi volume Postgres được tạo lần đầu.

Reset volume:

```powershell
docker compose -f docker-compose.local.yml down -v
docker compose -f docker-compose.local.yml up --build -d
```

Chỉ dùng khi chấp nhận mất dữ liệu demo hiện tại.

## 10. Bash smoke không chạy trên Windows

Nếu `bash` báo WSL chưa cài distro, dùng PowerShell smoke:

```powershell
.\scripts\demo\smoke-test.ps1
```

Hoặc cài Git Bash/WSL rồi chạy:

```bash
./scripts/demo/smoke-test.sh
```
