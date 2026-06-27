# Hướng dẫn chạy mini-banking-app

| Service | Thư mục | Port gRPC/HTTP | Database / Redis |
|---|---|---|---|
| CA | `ca-service` | gRPC 50051 | Postgres (Neon) |
| KDC | `kdc-service` | gRPC 50052 | Redis (Upstash) |
| Banking | `banking-service` | gRPC 50053 | Postgres + Redis |
| API Gateway | `api-gateway` | HTTP 3000 | Redis (Upstash) |

---

## 1. Cấu hình `.env` cho từng service

Mỗi service đã có file `.env` riêng:

- `ca-service/.env` — `CA_DATABASE_URL` (Neon), `CA_STORE_BACKEND=postgres`, đường dẫn root-CA…
- `kdc-service/.env` — `KDC_REDIS_URL`, `CA_PORT=localhost:50051`, `TGT_EXP`, đường dẫn khóa…
- `banking-service/.env` — `BANK_DATABASE_URL`, `BANK_REDIS_URL`, `BANK_KEY_K_V`…
- `api-gateway/.env` — `GATEWAY_REDIS_URL`, `EMAIL_USER/PASS`, OTP policy, địa chỉ gRPC…

Điền các secret còn để placeholder (`replace_with_...`): `GATEWAY_JWT_SECRET`,
`GATEWAY_OTP_SECRET`, `ROOT_CA_KEY_PASSWORD`, `KDC_MASTER_KEY`.

## 2. Khởi tạo schema database KHỎI CHẠY NỮA, DO TUI CHẠY Ở MÁY TUI LÀ CẬP NHẬT TRÊN NEON RỒI

CA và Bank dùng Postgres ngoài (Neon). Áp migration vào từng DB:

```bash
# CA database
psql "<CA_DATABASE_URL>" -f db/ca/migrations/001_init_ca.sql

# Bank database
psql "<BANK_DATABASE_URL>" -f db/bank/bank-server.sql
```

## 3. Sinh chứng chỉ & khóa (chỉ chạy lần đầu)

Đứng tại thư mục repo `mini-banking-app/mini-banking-app`:

### 3a. Root CA của ca-service

```bash
ROOT_CA_KEY_PASSWORD="<đúng giá trị trong ca-service/.env>" go run ./ca-service/scripts/provision_ca_dev.go
```

Tạo `ca-service/certs/root-ca/ca.{key,crt}` — trust anchor mà CA từ chối tự sinh khi khởi động.

### 3b. gRPC transport certs (TLS giữa các service)

```bash
# Linux / macOS / Git-bash
./scripts/gen-certs/gen-certs.sh
# Windows PowerShell
./scripts/gen-certs/gen-certs.ps1
```

Tạo CA nội bộ + leaf cert cho ca-service / kdc-service / banking-service và bản
`grpc-ca.crt` công khai cho gateway.

### 3c. Khóa Kerberos của KDC

```bash
go run ./kdc-service/scripts/provision_kdc_dev.go
```

Tạo:

- `kdc-service/certs/k_tgs.key` — khóa AES-256 (K_tgs, 32 byte)
- `kdc-service/certs/kdc-private.pem` — RSA private key của KDC
- `kdc-service/certs/kdc-public.pem` — public key cho client mã hóa pre-auth

Script in ra giá trị `BANK_KEY_K_V` (hex của K_tgs) — đặt vào `banking-service/.env`
(banking-service dùng K_tgs làm service key trong demo). Chạy lại sẽ **bỏ qua** nếu
khóa đã có; đặt `FORCE=1` để sinh lại (nhớ cập nhật lại `BANK_KEY_K_V`).

## 4. Khởi động 4 service

Mở 4 terminal, mỗi service một cửa sổ (thứ tự: CA → KDC → Bank → Gateway):

```bash
# Terminal 1 — CA
cd ca-service && go run ./cmd/server

# Terminal 2 — KDC
cd kdc-service && go run ./cmd/server

# Terminal 3 — Banking
cd banking-service && go run ./cmd/server

# Terminal 4 — API Gateway
cd api-gateway && npm install && npm run dev
```

API Gateway lắng nghe ở `http://localhost:3000`.

## 5. Frontend (tuỳ chọn)

```bash
cd frontend && npm install && npm run dev
```

Vite proxy `/v1` → `http://localhost:3000`