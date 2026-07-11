# Run Guide

Hướng dẫn chính để chạy Mini Banking App sau khi tài liệu final được gom vào `demo_test_guide`.

Root runtime nằm tại:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

## 1. File chính cần biết

Các file final:

- `.env.demo.example`: template env chính, copy ra `.env`.
- `.env`: env runtime local, không commit secret thật.
- `docker-compose.local.yml`: stack local đầy đủ, có frontend dev server.
- `docker-compose.demo.yml`: stack demo production-like, không có frontend dev server.
- `scripts/demo/smoke-test.ps1`: smoke test chính cho Windows PowerShell.
- `scripts/demo/smoke-test.sh`: smoke test cho Linux/macOS/Git Bash/CI.

Tài liệu chi tiết:

- `demo_test_guide/guide/ENV_GUIDE.md`
- `demo_test_guide/guide/COMPOSE_GUIDE.md`
- `demo_test_guide/guide/TERMINAL_GUIDE.md`
- `demo_test_guide/guide/SEED_AND_ACCOUNTS.md`
- `demo_test_guide/guide/TROUBLESHOOTING.md`

## 2. Chuẩn bị tool

Kiểm tra các tool:

```powershell
go version
node -v
npm.cmd -v
docker version
openssl version
```

Nếu dùng Bash smoke trên Windows, cần Git Bash hoặc WSL đã cài distro.

## 3. Chuẩn bị env

Copy env template:

```powershell
Copy-Item .\.env.demo.example .\.env
```

Mở `.env` và thay các secret bắt buộc:

- `ROOT_CA_KEY_PASSWORD`
- `CA_DATABASE_URL`
- `BANK_DB_PASSWORD`
- `JWT_SECRET`
- `GATEWAY_OTP_SECRET`
- `SMTP_USER`
- `SMTP_PASS`
- `ADMIN_SEC_DEMO_PASSWORD`
- `ADMIN_SEC_DEMO_TOKEN`

Khi rehearsal nhiều lần có thể set:

```env
RATE_LIMIT_DISABLED=1
```

## 4. Sinh certificate/key local

Chạy từ root runtime.

Provision Root CA, Client CA, gRPC Transport CA và CA server cert:

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..
```

Sinh cert TLS cho KDC/Bank và copy trust bundle:

```powershell
.\scripts\gen-certs\gen-certs.ps1
```

Sinh hai khóa ticket độc lập `K_tgs` và `K_v`:

```powershell
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Script tạo `kdc-service/certs/k_tgs.key` cho TGT và hai bản `K_v` giống hệt nhau tại `kdc-service/certs/k_v.key` và `banking-service/certs/k_v.key`. Mỗi service chỉ mount bản nằm trong thư mục của mình.

Kiểm tra nhanh:

```powershell
Test-Path .\ca-service\certs\root-ca\ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.crt
Test-Path .\api-gateway\certs\grpc-ca.crt
Test-Path .\kdc-service\certs\k_tgs.key
Test-Path .\kdc-service\certs\k_v.key
Test-Path .\banking-service\certs\k_v.key
Test-Path .\kdc-service\certs\kdc-server.crt
Test-Path .\banking-service\certs\grpc\bank-server.crt
```

Tất cả nên trả `True`.

## 5. Chạy bằng Docker Compose local

Đây là cách chạy ưu tiên khi rehearsal đầy đủ.

```powershell
docker compose -f docker-compose.local.yml up --build -d
```

Kiểm tra service:

```powershell
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs -f api-gateway
```

Frontend:

```text
http://localhost:5173
```

API Gateway:

```text
http://localhost:3000
```

## 6. Chạy bằng Docker Compose demo

Bản demo không có frontend dev server.

```powershell
docker compose -f docker-compose.demo.yml up --build -d
```

Dùng khi frontend đã được build/serve riêng.

## 7. Chạy smoke test

PowerShell:

```powershell
.\scripts\demo\smoke-test.ps1
```

Nếu muốn bỏ SMTP:

```powershell
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

Nếu có Admin CA token và SOC token:

```powershell
$env:ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>"
$env:ADMIN_SEC_DEMO_TOKEN="<security-admin-token>"
.\scripts\demo\smoke-test.ps1
```

Bash:

```bash
ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>" \
ADMIN_SEC_DEMO_TOKEN="<security-admin-token>" \
./scripts/demo/smoke-test.sh
```

Smoke script kiểm tự động các endpoint có thể kiểm bằng HTTP/token. Các flow cần private key trong browser như full register, AS/TGS/AP signed request, Admin Bank cert session đầy đủ sẽ được kiểm trong rehearsal và testcase functional/security.

## 8. Route demo chính

Frontend:

- Customer register: `http://localhost:5173/register`
- Customer login: `http://localhost:5173/login`
- Customer home: `http://localhost:5173/home`
- Admin CA: `http://localhost:5173/admin-ca`
- Admin CA activate: `http://localhost:5173/admin-ca/activate`
- Admin Bank: `http://localhost:5173/admin-bank`
- Admin SOC: `http://localhost:5173/admin-soc`

API/SOC:

- Admin CA API: `/v1/admin-ca/*`
- SOC login: `/v1/admin-sec/auth`
- KDC audit: `/v1/admin-kdc/audit`
- SOC timeline: `/v1/admin/audit/timeline`
- SOC verify: `/v1/admin/audit/verify`
- SOC summary: `/v1/admin/audit/summary`
- SOC export: `/v1/admin/audit/export`

## 9. Khi chạy terminal riêng

Nếu không dùng compose đầy đủ, xem:

```text
demo_test_guide/guide/TERMINAL_GUIDE.md
```

Lưu ý: các `.env.example` riêng trong từng module cần được rà lại ở Phase 6 để khớp `.env.demo.example` và cấu hình final.

## 10. Cleanup nhanh

Dừng stack:

```powershell
docker compose -f docker-compose.local.yml down
```

Xóa volume để chạy initdb/migration lại từ đầu:

```powershell
docker compose -f docker-compose.local.yml down -v
```

Chỉ xóa volume khi bạn chấp nhận mất dữ liệu demo hiện tại.
