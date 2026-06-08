# Mini App Banking — Setup Guide (Windows)

> Tất cả lệnh PowerShell chạy trong **PowerShell** (`Win + X` → Windows PowerShell / Terminal).
> Lệnh bash chạy trong **Git Bash** (chuột phải vào thư mục gốc → **Git Bash Here**).

---

## Phần 1 — Cài đặt công cụ

### 1.1 Winget

Windows 11 đã có sẵn. Windows 10 kiểm tra:

```powershell
winget --version
```

Nếu chưa có: https://aka.ms/getwinget

---

### 1.2 Git

```powershell
winget install Git.Git
```

Đóng và mở lại PowerShell, kiểm tra:

```powershell
git --version
```

---

### 1.3 Go 1.22+

```powershell
winget install GoLang.Go
```

Đóng và mở lại PowerShell:

```powershell
go version
# Expect: go version go1.22.x windows/amd64
```

Thêm Go bin vào PATH:

```powershell
$goPath = "$(go env GOPATH)\bin"
[Environment]::SetEnvironmentVariable("PATH", "$env:PATH;$goPath", "User")
$env:PATH += ";$goPath"
```

---

### 1.4 Docker Desktop

Tải từ: https://www.docker.com/products/docker-desktop/

Sau khi cài:
- Mở Docker Desktop
- Settings → General → bật **"Use WSL 2 based engine"**
- Chờ icon system tray chuyển sang running

```powershell
docker --version
docker compose version
```

---

### 1.5 protoc

```powershell
winget install Google.Protobuf
```

Nếu winget không tìm thấy: tải `protoc-xx.x-win64.zip` từ
https://github.com/protocolbuffers/protobuf/releases, giải nén, copy `protoc.exe` vào `C:\Windows\System32\`

```powershell
protoc --version
# Expect: libprotoc 3.x.x
```

Cài Go plugins:

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

---

### 1.6 Node.js

```powershell
winget install OpenJS.NodeJS.LTS
```

```powershell
node --version   # >= 18
npm --version
```

---

### 1.7 grpcurl (optional)

```powershell
winget install fullstorydev.grpcurl
```

Hoặc tải binary từ https://github.com/fullstorydev/grpcurl/releases →
giải nén, copy `grpcurl.exe` vào `C:\Windows\System32\`

---

## Phần 2 — Clone và cấu hình project

### 2.1 Clone repository

```powershell
git clone https://github.com/TNTT2305hcmus/mini-banking-app.git mini_banking
cd mini_banking
```

---

### 2.2 Tạo file `.env`

```powershell
Copy-Item .env.example .env
code .env   # hoặc: notepad .env
```

Generate secret ngay trong PowerShell:

```powershell
# JWT_SECRET (128 ký tự hex)
$bytes = New-Object byte[] 64
(New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes)
[BitConverter]::ToString($bytes).Replace("-","").ToLower()

# KDC_MASTER_KEY (64 ký tự hex)
$bytes = New-Object byte[] 32
(New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes)
[BitConverter]::ToString($bytes).Replace("-","").ToLower()
```

Copy output vào `.env` tương ứng.

---

### 2.3 Generate gRPC stubs

```powershell
# Cài ts-proto cho api-gateway (chạy 1 lần)
cd api-gateway
npm install
npm install ts-proto
cd ..
```

Mở **Git Bash** tại thư mục gốc:

```bash
chmod +x gen-proto.sh
./gen-proto.sh
```

Kiểm tra stub đã được tạo:

```powershell
ls pkg\pb\ca
# Expect: ca.pb.go  ca_grpc.pb.go

ls pkg\pb\kdc
# Expect: kdc.pb.go  kdc_grpc.pb.go

ls pkg\pb\bank
# Expect:  bank.pb.go  bank_grpc.pb.go
```

---

## Phần 3 — Chạy thủ công (từng lệnh)

> Cần **4 terminal** chạy song song.
> Thứ tự bắt buộc: **Redis → CA → KDC → Demo**

---

### 3.1 Terminal 1 — Khởi động Redis

```powershell
docker run --name mini-bank-redis -p 6379:6379 -d redis:7-alpine
```

Nếu container đã tồn tại từ lần trước:

```powershell
docker start mini-bank-redis
```

Kiểm tra Redis đang chạy:

```powershell
docker ps
# Expect: mini-bank-redis ... Up
```

---

### 3.2 Tạo secrets cho KDC (chạy 1 lần)

Mở Git Bash tại thư mục gốc mini_banking

```bash
mkdir -p kdc-service/secrets

# K_TGS: AES-256 key để encrypt TGT (đúng 32 bytes, không newline)
printf '0123456789abcdef0123456789abcdef' > kdc-service/secrets/k_tgs.key

# K_BANK: AES-256 key để encrypt Ticket_v cho Bank Service
printf 'abcdef0123456789abcdef0123456789' > kdc-service/secrets/k_bank.key

# KDC RSA private key (PKCS1 - chuẩn Go crypto)
openssl genrsa -out kdc-service/secrets/kdc_private.pem 2048

# Xác nhận 3 file đã tồn tại
ls kdc-service/secrets/
# Expect: k_bank.key  k_tgs.key  kdc_private.pem

# Kiểm tra key file hợp lệ
openssl rsa -in kdc-service/secrets/kdc_private.pem -check -noout
# Expect: RSA key ok
```
---

### 3.3 Tạo `.env` cho KDC (chạy 1 lần)

```powershell
@"
GRPC_PORT=50052
CA_PORT=50051
CA_HOST=localhost
TGT_EXP=30m
K_TGS_PATH=./secrets/k_tgs.key
BANK_SERVICE_KEY_PATH=./secrets/k_bank.key
BANK_SERVICE_ID=bank-service
KDC_PRIVATE_KEY_PATH=./secrets/kdc_private.pem
REDIS_URI=redis://localhost:6379
CA_CLIENT_CERT_PATH=../ca-service/certs/grpc/kdc-client.crt
CA_CLIENT_KEY_PATH=../ca-service/certs/grpc/kdc-client.key
CA_CLIENT_CA_CERT_PATH=../ca-service/certs/grpc/ca-server-ca.crt
CA_SERVER_NAME=ca-service
"@ | Set-Content -Encoding ASCII kdc-service\.env
```

---

### 3.4 Provision cert/key cho CA Service (chạy 1 lần)

CA Service hiện tại **không tự sinh Root CA khi thiếu file**. Trước khi chạy service, cần có:

- Root CA private key đã mã hóa: `ca-service/certs/root-ca/ca.key`
- Root CA certificate: `ca-service/certs/root-ca/ca.crt`
- gRPC server certificate/key cho CA: `ca-service/certs/grpc/ca-server.crt`, `ca-service/certs/grpc/ca-server.key`
- CA certificate dùng để verify client mTLS: `ca-service/certs/grpc/client-ca.crt`
- Client certificate cho KDC: `ca-service/certs/grpc/kdc-client.crt`, `ca-service/certs/grpc/kdc-client.key`

Mở PowerShell tại thư mục gốc `mini_banking`, chọn password dev cho Root CA key:

```powershell
$env:ROOT_CA_KEY_PASSWORD = "dev-root-ca-password-change-me"
```

Chạy helper provision local:

```powershell
go run .\ca-service\scripts\provision_ca_dev.go
```

> Lưu ý: chạy lại helper này sẽ tạo Root CA mới và làm các certificate cũ không còn thuộc cùng trust root. Với môi trường local mới thì không sao; nếu đang có state cũ, hãy xóa `ca-service/certs/ca-store/state.json` khi reset toàn bộ cert.

---

### 3.5 Terminal 2 — Chạy CA Service

```powershell
cd ca-service
$env:ROOT_CA_KEY_PASSWORD = "dev-root-ca-password-change-me"
go run ./cmd/server
```

Expected output:

```
[CA] Starting CA Service...
[CA] Config: port=50051 certValidity=365 days
[CA] Loaded Root CA from disk (Subject: Mini_App_Banking Root CA)
[CA] gRPC server listening on :50051
```

Nếu thiếu cert/key hoặc chưa set `ROOT_CA_KEY_PASSWORD`, service sẽ dừng ngay thay vì tự sinh Root CA mới.

---

### 3.6 Terminal 3 — Chạy KDC Service

```powershell
cd kdc-service
go run ./cmd/server
```

Expected output:

```
[INFO] Successfully connected to Redis.
[INFO] Successfully initialized CA Service client.
[KDC] Starting gRPC Server on :50052
```

---

### 3.7 Unit test từng service

```powershell
# CA
cd ca-service
go test ./...
cd ..

# KDC
cd kdc-service
go test ./...
go vet ./...
go test -race ./...
cd ..
```

---
