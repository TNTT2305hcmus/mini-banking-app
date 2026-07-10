# Docker Compose Guide

Compose final dùng hai file:

- `docker-compose.local.yml`: local full stack, có frontend dev server.
- `docker-compose.demo.yml`: demo production-like, không có frontend dev server.

Không dùng root `docker-compose.yml` cũ.

## 1. Chuẩn bị

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
Copy-Item .\.env.demo.example .\.env
```

Điền secret trong `.env`, rồi sinh cert/key:

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

.\scripts\gen-certs\gen-certs.ps1
go run .\kdc-service\scripts\provision_kdc_dev.go
```

## 2. Chạy local compose

```powershell
docker compose -f docker-compose.local.yml up --build -d
```

Kiểm tra:

```powershell
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs -f api-gateway
```

Các port local:

- Frontend: `http://localhost:5173`
- API Gateway: `http://localhost:3000`
- CA gRPC: `localhost:50051`
- KDC gRPC: `localhost:50052`
- Bank gRPC: `localhost:50053`
- Redis: `localhost:6379`
- Bank Postgres: `localhost:5432`

## 3. Chạy demo compose

```powershell
docker compose -f docker-compose.demo.yml up --build -d
```

Demo compose không expose frontend dev server. Dùng khi frontend được build/serve riêng.

## 4. KDC audit DB

`kdc-service` trong compose được set:

```env
DATABASE_URL=postgres://banking:<BANK_DB_PASSWORD>@bank-postgres:5432/banking?sslmode=disable
```

`bank-postgres` mount thêm KDC migrations:

- `db/kdc/migrations/001_init_kdc.sql`
- `db/kdc/migrations/002_add_audit_hash_chain.sql`

Nếu volume `bank_postgres_data` đã tồn tại trước khi thêm migrations, initdb không chạy lại. Khi đó chọn một trong hai cách:

1. Reset volume nếu chấp nhận mất dữ liệu demo:

```powershell
docker compose -f docker-compose.local.yml down -v
docker compose -f docker-compose.local.yml up --build -d
```

2. Apply migration thủ công vào container Postgres:

```powershell
Get-Content -Raw .\db\kdc\migrations\001_init_kdc.sql |
  docker compose -f docker-compose.local.yml exec -T bank-postgres psql -U banking -d banking

Get-Content -Raw .\db\kdc\migrations\002_add_audit_hash_chain.sql |
  docker compose -f docker-compose.local.yml exec -T bank-postgres psql -U banking -d banking
```

## 5. Client CA intermediate

CA container phải có:

```env
CLIENT_CA_KEY_PATH=/certs/intermediate/client-ca.key
CLIENT_CA_CERT_PATH=/certs/intermediate/client-ca.crt
```

Và mount:

```text
./ca-service/certs/intermediate:/certs/intermediate:ro
```

Nếu thiếu, CA service có thể fail khi cấp identity cert.

## 6. Smoke test

PowerShell:

```powershell
.\scripts\demo\smoke-test.ps1
```

Bash:

```bash
./scripts/demo/smoke-test.sh
```

Với SOC/Admin CA token:

```powershell
$env:ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>"
$env:ADMIN_SEC_DEMO_TOKEN="<security-admin-token>"
.\scripts\demo\smoke-test.ps1
```

## 7. Dừng stack

```powershell
docker compose -f docker-compose.local.yml down
```

Xóa cả volume:

```powershell
docker compose -f docker-compose.local.yml down -v
```

Chỉ dùng `down -v` khi muốn seed/migration chạy lại từ đầu.
