# Guide chạy mini-banking-app

Guide này ưu tiên cách chạy local ổn định nhất: Redis/Postgres chạy bằng Docker, còn 4 service chính chạy bằng terminal riêng để dễ nhìn log và debug.

## 1. Yêu cầu máy

Cài các tool sau:

| Tool | Gợi ý version | Dùng để |
|---|---|---|
| Go | 1.25.x | Chạy CA/KDC/Bank |
| Node.js | 24.x hoặc 22.x LTS | Chạy API Gateway/Frontend |
| npm | Theo Node | Cài package JS |
| Docker Desktop | Bản mới | Chạy Redis/Postgres local |
| OpenSSL | 1.1.1+ hoặc 3.x | Sinh cert gRPC |
| psql | PostgreSQL client | Import schema Bank/CA |

Kiểm tra nhanh:

```powershell
go version
node -v
npm -v
docker version
openssl version
psql --version
```

Tất cả lệnh bên dưới giả sử bạn đứng ở thư mục:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

## 2. Các file `.env` đã được cấu hình

Các file local đã có sẵn:

| File | Mục đích |
|---|---|
| `.env` | Biến dùng cho `docker compose` ở root app. |
| `.env.example` | Template để copy lại khi cần. |
| `ca-service/.env` | CA local, mặc định dùng JSON store để dễ chạy. |
| `kdc-service/.env` | KDC local, trỏ CA/Redis/key/cert. |
| `banking-service/.env` | Bank local, trỏ Postgres/Redis/CA/key/cert. |
| `api-gateway/.env` | Gateway local, trỏ Redis/cert/gRPC/SMTP. |
| `frontend/.env` | Frontend dev proxy tới Gateway. |

Bạn cần sửa các giá trị này trước khi demo thật:

```text
api-gateway/.env
  EMAIL_USER=<gmail gửi OTP>
  EMAIL_PASS=<gmail app password>
  GATEWAY_JWT_SECRET=<secret dài, random>
  GATEWAY_OTP_SECRET=<secret dài, random>

ca-service/.env
  ROOT_CA_KEY_PASSWORD=<đổi nếu muốn>

banking-service/.env
  DATABASE_URL=<DB Bank thật nếu không dùng Docker local>
```

Nếu đổi `ROOT_CA_KEY_PASSWORD` sau khi đã sinh root CA, hãy xóa/sinh lại root CA hoặc giữ nguyên
password cũ. File private key CA đã encrypt bằng password lúc provision.

## 3. Chạy Redis và Postgres local

Bank cần Postgres. KDC, Bank và Gateway cùng dùng Redis local.

```powershell
docker run --name mini-bank-redis -p 6379:6379 -d redis:7.4-alpine

docker run --name mini-bank-postgres `
  -e POSTGRES_DB=banking `
  -e POSTGRES_USER=banking `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5432:5432 `
  -d postgres:17-alpine
```

Nếu container đã tồn tại:

```powershell
docker start mini-bank-redis
docker start mini-bank-postgres
```

Import schema Bank:

```powershell
psql "postgres://banking:MiniBankingDev123!@localhost:5432/banking?sslmode=disable" `
  -f .\db\bank\migrations\001_init_bank.sql
```

CA local mặc định dùng `CA_STORE_BACKEND=json`, Nhưng ưu tiên dùng CA Postgres:

```powershell
docker run --name mini-ca-postgres `
  -e POSTGRES_DB=ca_db `
  -e POSTGRES_USER=ca_user `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5433:5432 `
  -d postgres:17-alpine

psql "postgres://ca_user:MiniBankingDev123!@localhost:5433/ca_db?sslmode=disable" `
  -f .\db\ca\migrations\001_init_ca.sql
```

Sau đó sửa `ca-service/.env`:

```text
CA_STORE_BACKEND=postgres
CA_DATABASE_URL=postgres://ca_user:MiniBankingDev123!@localhost:5433/ca_db?sslmode=disable
```

## 4. Sinh certificate và key

### 4.1. Sinh Root CA cho CA Service

Chạy một lần:

```powershell
cd .\ca-service
$env:ROOT_CA_KEY_PASSWORD = "dev-root-ca-password-change-me"
go run .\scripts\provision_ca_dev.go
cd ..
```

Kết quả chính:

```text
ca-service/certs/root-ca/ca.key
ca-service/certs/root-ca/ca.crt
```

### 4.2. Sinh gRPC TLS cert cho CA/KDC/Bank/Gateway

Windows PowerShell:

```powershell
.\scripts\gen-certs\gen-certs.ps1
```

Git Bash/Linux/macOS:

```bash
./scripts/gen-certs/gen-certs.sh
```

Kết quả chính:

```text
ca-service/certs/grpc/ca-server.crt
ca-service/certs/grpc/ca-server.key
kdc-service/certs/kdc-server.crt
kdc-service/certs/kdc-server.key
banking-service/certs/grpc/bank-server.crt
banking-service/certs/grpc/bank-server.key
api-gateway/certs/grpc-ca.crt
kdc-service/certs/grpc-ca.crt
banking-service/certs/grpc-ca.crt
```

### 4.3. Sinh key cho KDC

```powershell
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Kết quả chính:

```text
kdc-service/certs/k_tgs.key
kdc-service/certs/kdc-private.pem
kdc-service/certs/kdc-public.pem
```

`banking-service/.env` đã trỏ `BANK_KEY=../kdc-service/certs/k_tgs.key`, nên không cần copy thủ công.

## 5. Cài dependencies

```powershell
cd .\api-gateway
npm install
cd ..

cd .\frontend
npm install
cd ..
```

Go sẽ tự tải module khi `go run`; nếu muốn tải trước:

```powershell
cd .\ca-service; go mod download; cd ..
cd .\kdc-service; go mod download; cd ..
cd .\banking-service; go mod download; cd ..
```

## 6. Chạy backend bằng 4 terminal

Chạy đúng thứ tự để service phụ thuộc không fail kết nối.

Terminal 1 - CA:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\ca-service"
go run .\cmd\server
```

Terminal 2 - KDC:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\kdc-service"
go run .\cmd\server
```

Terminal 3 - Banking:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\banking-service"
go run .\cmd\server
```

Terminal 4 - API Gateway:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\api-gateway"
npm run dev
```

Gateway chạy tại:

```text
http://localhost:3000
```

## 7. Chạy frontend

Terminal 5:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\frontend"
npm run dev
```

Vite sẽ in URL, thường là:

```text
http://localhost:5173
```

Frontend gọi API qua proxy `/v1` tới `http://localhost:3000`.

Các màn hình hiện nằm chung trong một React/Vite app:

| Màn hình | URL local |
|---|---|
| User login | `http://localhost:5173/login` |
| User register | `http://localhost:5173/register` |
| User home/banking | `http://localhost:5173/home` |
| Admin CA | `http://localhost:5173/admin-ca` |
| Admin Bank | `http://localhost:5173/admin-bank` |

## 8. Kiểm tra nhanh sau khi chạy

Kiểm tra port đang listen:

```powershell
netstat -ano | findstr ":3000 :50051 :50052 :50053 :6379 :5432"
```

Kiểm tra Redis:

```powershell
docker exec mini-bank-redis redis-cli ping
```

Kết quả mong muốn:

```text
PONG
```

Kiểm tra Gateway có lên chưa:

```powershell
curl http://localhost:3000
```

Nếu route root chưa được định nghĩa, chỉ cần Gateway terminal không báo crash là ổn. Hãy test bằng
frontend hoặc các endpoint `/v1/...` đã implement.
