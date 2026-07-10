# Terminal Guide

Hướng dẫn chạy từng service bằng terminal riêng. Cách này hữu ích khi cần xem log chi tiết hoặc debug từng service.

Nếu chỉ muốn demo nhanh, ưu tiên `docker-compose.local.yml`.

## 1. Chuẩn bị infra bằng Docker

Redis:

```powershell
docker run --name mini-bank-redis -p 6379:6379 -d redis:7.4-alpine
```

Bank Postgres:

```powershell
docker run --name mini-bank-postgres `
  -e POSTGRES_DB=banking `
  -e POSTGRES_USER=banking `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5432:5432 `
  -d postgres:17-alpine
```

CA Postgres:

```powershell
docker run --name mini-ca-postgres `
  -e POSTGRES_DB=ca_db `
  -e POSTGRES_USER=ca_user `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5433:5432 `
  -d postgres:17-alpine
```

Nếu container đã có:

```powershell
docker start mini-bank-redis
docker start mini-bank-postgres
docker start mini-ca-postgres
```

## 2. Apply migrations

Bank:

```powershell
Get-Content -Raw .\db\bank\migrations\001_init_bank.sql |
  docker exec -i mini-bank-postgres psql -U banking -d banking
```

KDC audit:

```powershell
Get-Content -Raw .\db\kdc\migrations\001_init_kdc.sql |
  docker exec -i mini-bank-postgres psql -U banking -d banking

Get-Content -Raw .\db\kdc\migrations\002_add_audit_hash_chain.sql |
  docker exec -i mini-bank-postgres psql -U banking -d banking
```

Seed Bank demo:

```powershell
Get-Content -Raw .\db\bank\seed_demo.sql |
  docker exec -i mini-bank-postgres psql -U banking -d banking
```

CA:

```powershell
Get-Content -Raw .\db\ca\migrations\001_init_ca.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\002_add_certificate_role.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\003_add_ra_audit_actions.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\004_add_audit_hash_chain.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\005_add_ca_admin_role.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db
```

## 3. Provision cert/key

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

.\scripts\gen-certs\gen-certs.ps1
go run .\kdc-service\scripts\provision_kdc_dev.go
```

## 4. Chạy services

Mở các terminal riêng.

CA:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\ca-service"
go run .\cmd\server
```

KDC:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\kdc-service"
go run .\cmd\server
```

Bank:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\banking-service"
go run .\cmd\server
```

API Gateway:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\api-gateway"
npm.cmd install
npm.cmd run dev
```

Frontend:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\frontend"
npm.cmd install
npm.cmd run dev
```

## 5. Env local cần chú ý

KDC terminal cần `DATABASE_URL` nếu muốn SOC thấy KDC audit:

```env
DATABASE_URL=postgres://banking:MiniBankingDev123!@localhost:5432/banking?sslmode=disable
```

CA terminal cần Client CA paths nếu không dùng default:

```env
CLIENT_CA_KEY_PATH=certs/intermediate/client-ca.key
CLIENT_CA_CERT_PATH=certs/intermediate/client-ca.crt
```

API Gateway cần SOC env nếu muốn login SOC:

```env
ADMIN_SEC_DEMO_EMAIL=security@minibanking.local
ADMIN_SEC_DEMO_PASSWORD=<secret>
ADMIN_SEC_DEMO_TOKEN=<dev-token>
```

## 6. Kiểm tra nhanh

```powershell
Test-NetConnection localhost -Port 3000
Test-NetConnection localhost -Port 50051
Test-NetConnection localhost -Port 50052
Test-NetConnection localhost -Port 50053
docker exec mini-bank-redis redis-cli ping
```

Sau đó chạy:

```powershell
.\scripts\demo\smoke-test.ps1
```
