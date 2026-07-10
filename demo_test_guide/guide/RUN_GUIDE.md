# RUN GUIDE

Hướng dẫn này ưu tiên cách chạy local trên Windows PowerShell:

- Redis và PostgreSQL chạy bằng Docker.
- CA, KDC, Bank, API Gateway và Frontend chạy bằng terminal riêng để dễ xem log.

Tất cả lệnh bên dưới giả sử bạn đang ở thư mục:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

## 1. Kiểm Tra Tool

Cần có các tool sau trong PATH:

```powershell
go version
node -v
npm.cmd -v
docker version
openssl version
```

## 2. Kiểm Tra File Env

Mỗi service dùng file `.env` riêng:

```text
mini-banking-app\.env                  # docker compose shared values
mini-banking-app\ca-service\.env       # CA service
mini-banking-app\kdc-service\.env      # KDC service
mini-banking-app\banking-service\.env  # Bank service
mini-banking-app\api-gateway\.env      # API Gateway
mini-banking-app\frontend\.env         # Vite frontend
```

Những biến quan trọng cần dùng local:

```text
ca-service\.env
  ROOT_CA_KEY_PASSWORD=dev-root-ca-password-change-me
  CA_STORE_BACKEND=postgres
  CA_DATABASE_URL=postgres://ca_user:MiniBankingDev123!@localhost:5433/ca_db?sslmode=disable

kdc-service\.env
  CA_HOST=localhost
  CA_PORT=50051
  CA_CERT_PATH=certs/grpc-ca.crt
  CA_SERVER_NAME=ca-service

banking-service\.env
  DATABASE_URL=postgres://banking:MiniBankingDev123!@localhost:5432/banking?sslmode=disable
  REDIS_URI=redis://localhost:6379/0
  BANK_KEY=../kdc-service/certs/k_tgs.key
  CA_SERVICE_ADDRESS=localhost:50051
  CA_CERT_PATH=certs/grpc-ca.crt
  CA_TLS_SERVER_NAME=ca-service

api-gateway\.env
  FRONTEND_BASE_URL=http://localhost:5173
  GATEWAY_REDIS_URL=redis://localhost:6379/0
  CA_CERT_PATH=certs/grpc-ca.crt  # trust bundle: gRPC Transport CA + Root CA
  CA_GRPC_ADDR=localhost:50051
  KDC_GRPC_ADDR=localhost:50052
  BANK_GRPC_ADDR=localhost:50053
```

Nếu đổi `ROOT_CA_KEY_PASSWORD` sau khi đã provision cert, hãy xoá `ca-service\certs` và provision lại. Theo kiến trúc CA mới, Root CA chỉ ký Intermediate CA; không ký trực tiếp cert user hoặc cert service trong runtime bình thường.

## 3. Chạy Redis Và PostgreSQL Bằng Docker

Tạo container lần đầu:

```powershell
docker run --name mini-bank-redis `
  -p 6379:6379 `
  -d redis:7.4-alpine

docker run --name mini-bank-postgres `
  -e POSTGRES_DB=banking `
  -e POSTGRES_USER=banking `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5432:5432 `
  -d postgres:17-alpine

docker run --name mini-ca-postgres `
  -e POSTGRES_DB=ca_db `
  -e POSTGRES_USER=ca_user `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5433:5432 `
  -d postgres:17-alpine
```

Nếu container đã tồn tại:

```powershell
docker start mini-bank-redis
docker start mini-bank-postgres
docker start mini-ca-postgres
```

Kiểm tra container:

```powershell
docker ps
docker exec mini-bank-redis redis-cli ping
```

Kết quả Redis mong muốn:

```text
PONG
```

## 4. Apply Database Migration Không Cần psql Local

Dùng `Get-Content` để đẩy SQL vào `psql` bên trong container Postgres.

Bank DB:

```powershell
Get-Content -Raw .\db\bank\migrations\001_init_bank.sql |
  docker exec -i mini-bank-postgres `
    psql -U banking -d banking
```

CA DB:

```powershell
Get-Content -Raw .\db\ca\migrations\001_init_ca.sql |
  docker exec -i mini-ca-postgres `
    psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\002_add_certificate_role.sql |
  docker exec -i mini-ca-postgres `
    psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\003_add_ra_audit_actions.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\004_add_audit_hash_chain.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db

Get-Content -Raw .\db\ca\migrations\005_add_ca_admin_role.sql |
  docker exec -i mini-ca-postgres psql -U ca_user -d ca_db
```

Nếu muốn reset DB local thật sạch, xoá container và volume implicit của container:

```powershell
docker rm -f mini-bank-postgres
docker rm -f mini-ca-postgres
```

Sau đó tạo lại container ở mục 3 và apply migration lại.

## 5. Provision Certificate Và Key

### 5.1. CA Service provision Root CA và 2 Intermediate CA

`provision_ca_dev.go` tạo folder `ca-service\certs`. Script này đã load `ca-service\.env`, nên chỉ cần đảm bảo `.env` có `ROOT_CA_KEY_PASSWORD`.

Kiến trúc cert mục tiêu:

```text
Root CA
  certs\root-ca\ca.crt
  certs\root-ca\ca.key
  Chỉ ký Intermediate CA.

gRPC Transport CA
  certs\intermediate\grpc-ca.crt
  certs\intermediate\grpc-ca.key
  Ký cert TLS cho CA/KDC/Bank: ca-server.crt, kdc-server.crt, bank-server.crt.
  Khi phân phối cho Gateway/KDC/Bank, file grpc-ca.crt là bundle gồm gRPC Transport CA + Root CA.

Client CA
  certs\intermediate\client-ca.crt
  certs\intermediate\client-ca.key
  Ký user/client certificate từ CSR khi đăng ký.
```

`ca-server.crt` là cert TLS của chính CA Service, do gRPC Transport CA ký. Nó không phải CA certificate và không được dùng để ký cert khác.

Nếu muốn tạo lại từ đầu:

```powershell
cd .\ca-service

Remove-Item -Recurse -Force .\certs -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force .\ca-service -ErrorAction SilentlyContinue

go run .\scripts\provision_ca_dev.go

Test-Path .\certs\root-ca\ca.key
Test-Path .\certs\root-ca\ca.crt
Test-Path .\certs\intermediate\grpc-ca.key
Test-Path .\certs\intermediate\grpc-ca.crt
Test-Path .\certs\intermediate\client-ca.key
Test-Path .\certs\intermediate\client-ca.crt
Test-Path .\certs\grpc\ca-server.crt
Test-Path .\certs\grpc\ca-server.key

cd ..
```

Tất cả kết quả `Test-Path` nên là `True`.

### 5.2. Sinh cert service cho CA/KDC/Bank và copy trust bundle

Sau khi CA đã provision xong, chạy:

```powershell
.\scripts\gen-certs\gen-certs.ps1
```

Script này:

- Dùng gRPC Transport CA đã được Root CA ký.
- Tạo CA Service gRPC cert/key.
- Tạo KDC gRPC cert/key.
- Tạo Bank gRPC cert/key.
- Copy gRPC trust bundle vào Gateway/KDC/Bank để verify `ca-server.crt`, `kdc-server.crt`, `bank-server.crt`.
  File bundle vẫn tên `grpc-ca.crt`, nhưng nội dung gồm cả gRPC Transport CA và Root CA để OpenSSL/Node/Go dựng được chain đầy đủ.

Lưu ý khi chuyển đổi code: nếu script còn tự sinh `scripts\gen-certs\out\grpc-ca.*` dạng self-signed hoặc còn dùng `ca-server-ca.crt`, đó là logic cũ và cần được thay bằng Intermediate gRPC Transport CA ở `ca-service\certs\intermediate`, cộng Root CA khi tạo trust bundle cho verifier.

Kiểm tra nhanh:

```powershell
Test-Path .\api-gateway\certs\grpc-ca.crt
Test-Path .\ca-service\certs\grpc\ca-server.crt
Test-Path .\ca-service\certs\grpc\ca-server.key
Test-Path .\kdc-service\certs\kdc-server.crt
Test-Path .\kdc-service\certs\kdc-server.key
Test-Path .\kdc-service\certs\grpc-ca.crt
Test-Path .\banking-service\certs\grpc\bank-server.crt
Test-Path .\banking-service\certs\grpc\bank-server.key
Test-Path .\banking-service\certs\grpc-ca.crt
```

### 5.3. Sinh key cho KDC và Bank ticket

```powershell
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Kiểm tra:

```powershell
Test-Path .\kdc-service\certs\k_tgs.key
Test-Path .\kdc-service\certs\kdc-private.pem
Test-Path .\kdc-service\certs\kdc-public.pem
```

`banking-service\.env` đang trỏ `BANK_KEY=../kdc-service/certs/k_tgs.key`, nên không cần copy key thủ công.

## 6. Cài Dependencies

API Gateway:

```powershell
cd .\api-gateway
npm.cmd install
cd ..
```

Frontend:

```powershell
cd .\frontend
npm.cmd install
cd ..
```

Go modules:

```powershell
cd .\ca-service
go mod download
cd ..

cd .\kdc-service
go mod download
cd ..

cd .\banking-service
go mod download
cd ..
```

## 7. Chạy Backend Bằng 4 Terminal

Mở 4 terminal PowerShell riêng.

Terminal 1 - CA:

```terminal
cd mini-banking-app\ca-service
go run .\cmd\server
```

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\ca-service"
go run .\cmd\server
```

Terminal 2 - KDC:

```terminal
cd mini-banking-app\kdc-service
go run .\cmd\server
```


```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\kdc-service"
go run .\cmd\server
```

Terminal 3 - Bank:

```terminal
cd mini-banking-app\banking-service
go run .\cmd\server
```

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\banking-service"
go run .\cmd\server
```

Terminal 4 - API Gateway:

```terminal
cd mini-banking-app\api-gateway
npm.cmd run dev
```

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\api-gateway"
npm.cmd run dev
```

Gateway chạy tại:

```text
http://localhost:3000
```

## 8. Chạy Frontend

Terminal 5:

```terminal
cd mini-banking-app\frontend
npm.cmd run dev
```

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\frontend"
npm.cmd run dev
```

Vite thường in URL:

```text
http://localhost:5173
```

Màn hình local:

```text
User login     http://localhost:5173/login
User register  http://localhost:5173/register
User home      http://localhost:5173/home
Admin CA       http://localhost:5173/admin-ca
Admin CA activate http://localhost:5173/admin-ca/activate
Admin Bank     http://localhost:5173/admin-bank
```

## 9. Provision CA Admin Cert-Based

Để test giai đoạn 3, không dùng tuỳ tiện bất kỳ Gmail nào truy cập `/admin-ca/activate`. Gateway chỉ cấp cert `ca_admin` cho email đã được provision pending trong Redis bằng script dưới đây.

Sau khi Redis, CA Service và API Gateway đã chạy, mở terminal mới:

```powershell
cd .\api-gateway
npm.cmd run provision:ca-admin -- --email your.gmail@example.com --full-name "CA Administrator"
cd ..
```

Script này:

- tạo pending CA Admin trong Redis với namespace `admin:ca:*`;
- tạo activation token ngẫu nhiên, lưu dạng SHA-256 hash và có TTL mặc định 900 giây;
- gửi email chứa link `/admin-ca/activate#token=...` tới đúng email đã provision.

Mở link activation trong email, nhập đúng email/full name đã provision và đặt PIN. Browser sẽ sinh keypair, tạo CSR, lưu private key đã wrap bằng PIN trong IndexedDB, rồi Gateway yêu cầu CA cấp cert role `ca_admin`.

Sau khi activate thành công, mở:

```text
http://localhost:5173/admin-ca
```

Đăng nhập bằng PIN/cert. Admin CA không còn password/static-token fallback; session quản trị chỉ được phát sau khi Gateway verify cert role `ca_admin` qua `/v1/admin-ca/session`.
