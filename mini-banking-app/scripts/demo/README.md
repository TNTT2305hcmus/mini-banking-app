# scripts/demo — Hướng dẫn chạy Demo End-to-End

Tài liệu này hướng dẫn dựng lại toàn bộ demo `mini-banking-app` từ đầu, lặp lại được,
tối thiểu thao tác tay.

---

## Mục lục

1. [Tiên quyết](#1-tiên-quyết)
2. [Chuỗi lệnh chạy demo theo thứ tự](#2-chuỗi-lệnh-chạy-demo-theo-thứ-tự)
3. [Bypass OTP cho demo](#3-bypass-otp-cho-demo)
4. [Chạy flow đầy đủ (AS → TGS → Bank)](#4-chạy-flow-đầy-đủ)
5. [Admin Bank session](#5-admin-bank-session)
6. [Checklist trước demo quan trọng](#6-checklist-trước-demo-quan-trọng)
7. [Xử lý sự cố thường gặp](#7-xử-lý-sự-cố-thường-gặp)

---

## 1. Tiên quyết

| Công cụ | Phiên bản tối thiểu | Kiểm tra |
|---|---|---|
| Docker Desktop | ≥4.x | `docker --version` |
| Docker Compose | v2 (plugin) | `docker compose version` |
| OpenSSL | ≥1.1 | `openssl version` |
| Go | ≥1.22 | `go version` |
| curl | bất kỳ | `curl --version` |
| jq (tùy chọn) | bất kỳ | `jq --version` |

**Trên Windows**: dùng Git Bash hoặc WSL2 để chạy các script `.sh`.
PowerShell script (`.ps1`) chạy trực tiếp trên PowerShell 5.1+.

---

## 2. Chuỗi lệnh chạy demo theo thứ tự

### Bước 1: Clone repo và setup env

```bash
git clone <repo-url>
cd mini-banking-app

# Copy và điền env
cp .env.demo.example .env
# Điền các secret bắt buộc trong .env (xem danh sách trong .env.demo.example)
```

### Bước 2: Provision CA (tạo Root CA + gRPC Transport CA)

```bash
cd ca-service
ROOT_CA_KEY_PASSWORD=<your-password> go run ./scripts/provision_ca_dev.go
cd ..
```

> **Lưu ý**: Script này tạo `ca-service/certs/root-ca/ca.{key,crt}`,
> `ca-service/certs/intermediate/grpc-ca.{crt,key}` và
> `ca-service/certs/grpc/ca-server.{crt,key}`.

### Bước 3: Sinh cert cho KDC và Bank

```bash
# Linux/macOS/Git Bash:
bash scripts/gen-certs/gen-certs.sh

# Windows PowerShell:
.\scripts\gen-certs\gen-certs.ps1
```

Cert được tạo tại:
- `kdc-service/certs/kdc-server.{crt,key}`
- `banking-service/certs/grpc/bank-server.{crt,key}`
- `api-gateway/certs/grpc-ca.crt` (trust bundle)
- `kdc-service/certs/grpc-ca.crt` (trust bundle)
- `banking-service/certs/grpc/grpc-ca.crt` (trust bundle)

### Bước 4: Dựng Docker Compose

```bash
# Local demo (có frontend dev server):
docker compose -f docker-compose.local.yml up --build -d

# Hoặc deploy demo (không có frontend dev):
docker compose -f docker-compose.demo.yml up --build -d
```

### Bước 5: Đợi services healthy

```bash
docker compose -f docker-compose.local.yml ps
# Tất cả services phải ở trạng thái "healthy" hoặc "running"
```

### Bước 6: Chạy smoke test

```bash
# Linux/macOS/Git Bash:
bash scripts/demo/smoke-test.sh

# Windows PowerShell:
.\scripts\demo\smoke-test.ps1
```

### Tóm tắt chuỗi lệnh duy nhất (sau khi đã có .env):

```bash
# Từ thư mục mini-banking-app:
(cd ca-service && ROOT_CA_KEY_PASSWORD=$(grep ROOT_CA_KEY_PASSWORD ../.env | cut -d= -f2) go run ./scripts/provision_ca_dev.go) \
  && bash scripts/gen-certs/gen-certs.sh \
  && docker compose -f docker-compose.local.yml up --build -d \
  && echo "Đợi services khởi động..." && sleep 30 \
  && bash scripts/demo/smoke-test.sh
```

---

## 3. Bypass OTP cho demo

Demo thực tế thường không có email thật cấu hình. Với local demo, `.env.demo.example` đã đặt:

```env
DEMO_OTP=123456
```

Trên UI, mở `http://localhost:5173/register`, nhập `customer.demo@demo.minibanking.local`, nhập OTP `123456`, rồi đặt PIN `123456`.

Có 3 cách kiểm tra qua script:

### Cách 1: Dùng `SKIP_SMTP_CHECK=1` trong smoke test

```bash
SKIP_SMTP_CHECK=1 bash scripts/demo/smoke-test.sh
# Script bỏ qua bước OTP, các bước khác vẫn chạy
```

### Cách 2: Dùng email demo thống nhất

```bash
DEMO_EMAIL=customer.demo@demo.minibanking.local SKIP_SMTP_CHECK=1 \
  bash scripts/demo/smoke-test.sh
```

> **Lưu ý**: Gateway lưu OTP dạng HMAC trong Redis, không lưu plaintext OTP.
> Vì vậy không thể đọc OTP thật trực tiếp từ Redis. Muốn test OTP thật thì dùng
> Gmail App Password ở cách 3.

### Cách 3: Cấu hình Gmail App Password thật

1. Tạo [Gmail App Password](https://myaccount.google.com/apppasswords)
2. Set trong `.env`:
   ```
   SMTP_USER=your.gmail@gmail.com
   SMTP_PASS=xxxx-xxxx-xxxx-xxxx
   ```
3. Chạy smoke test bình thường — OTP được gửi tới email thật

---

## 4. Chạy flow đầy đủ

Flow đầy đủ: OTP → PKI Register → AS Request → TGS Request → Balance → Transfer

### Bước 4.1: Tạo RSA key pair (client)

```bash
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
  -subj "/C=VN/O=Mini_App_Banking/CN=customer.demo@demo.minibanking.local"
```

### Bước 4.2: PKI Register

```bash
GW=http://localhost:3000
RID=$(uuidgen)

# 1. OTP request
curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $RID" \
  -d '{"email":"customer.demo@demo.minibanking.local"}' "$GW/v1/otp/request"

# 2. OTP verify (lấy OTP từ email)
REG_TOKEN=$(curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(uuidgen)" \
  -d '{"email":"customer.demo@demo.minibanking.local","otp":"<OTP_FROM_EMAIL>"}' \
  "$GW/v1/otp/verify" | jq -r .data.registration_token)

# 3. PKI register với CSR
CSR_PEM=$(cat client.csr | awk '{printf "%s\\n", $0}')
curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(uuidgen)" \
  -d "{\"csr_pem\":\"$(cat client.csr)\",\"registration_token\":\"$REG_TOKEN\"}" \
  "$GW/v1/pki/register" | jq .
```

### Bước 4.3: AS Request

> AS_REQ yêu cầu ký payload bằng RSA private key — thực hiện qua Frontend UI
> hoặc dùng script client riêng. Xem `blueprint/api-design/02-as-exchange.md`
> để biết format payload cần ký.

### Bước 4.4: Xem balance (sau khi có Ticket_v scope balance:read)

```bash
ACCOUNT_ID="<account_id_from_customer_profile>"
curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(uuidgen)" \
  -d '{"ticket_v":"<BASE64_TICKET_V>","authenticator":"<BASE64_AUTHENTICATOR>"}' \
  "$GW/v1/bank/accounts/$ACCOUNT_ID/balance/query" | jq .
```

---

## 5. Admin Bank session

Admin Bank dùng session cookie. Workflow:

```bash
GW=http://localhost:3000
COOKIES_FILE=/tmp/admin-bank-cookies.txt

# Step 1: Activate (nhận activation code — flow phụ thuộc implementation)
# Xem api-gateway/src/ để biết endpoint cụ thể

# Step 2: Tạo session
curl -s -c "$COOKIES_FILE" -X POST \
  -H "Content-Type: application/json" -H "X-Request-ID: $(uuidgen)" \
  -d '{"activation_code":"<CODE>"}' \
  "$GW/v1/admin/bank/session"

# Step 3: Query audit
curl -s -b "$COOKIES_FILE" -X POST \
  -H "Content-Type: application/json" -H "X-Request-ID: $(uuidgen)" \
  -d '{"action":"transfer_completed","limit":20,"offset":0}' \
  "$GW/v1/admin/bank/audit/query" | jq .
```

---

## 6. Checklist trước demo quan trọng

### Trước demo thông thường:
- [ ] Xóa data cũ nếu cần: `docker compose -f docker-compose.local.yml down -v` (xóa volume)
- [ ] Chạy lại seed: restart bank-postgres (seed tự động qua init script)
- [ ] Kiểm tra port với smoke test: `bash scripts/demo/smoke-test.sh`
- [ ] Đăng nhập Admin CA thành công
- [ ] Chạy 1 transfer mẫu qua Frontend UI

### Trước demo quan trọng (khách hàng, bảo vệ đồ án):
- [ ] Tất cả bước trên
- [ ] **Backup DB**: `docker compose exec bank-postgres pg_dump -U banking banking > backup_$(date +%Y%m%d_%H%M%S).sql`
- [ ] **Backup certs**: `tar czf certs_backup_$(date +%Y%m%d).tar.gz ca-service/certs kdc-service/certs banking-service/certs api-gateway/certs`
- [ ] Test toàn bộ flow 1 lần trên máy demo thật (không phải máy dev)
- [ ] Tắt notification, set Do Not Disturb
- [ ] Chuẩn bị plan B: screenshots/video recording sẵn

---

## 7. Xử lý sự cố thường gặp

| Triệu chứng | Nguyên nhân thường gặp | Giải pháp |
|---|---|---|
| `ca-service` không khởi động | CA_DATABASE_URL sai hoặc Neon offline | Kiểm tra `docker compose logs ca-service` |
| `kdc-service` không kết nối CA | ca-service chưa healthy | Đợi ca-service healthy, kiểm tra port 50051 |
| `banking-service` crash vì thiếu/sai `BANK_KEY` | Chưa sinh hoặc mount `k_v.key` | Tại root runtime, chạy `go run .\kdc-service\scripts\provision_kdc_dev.go`, rồi recreate KDC và Banking Service |
| OTP không đến email | SMTP_PASS sai | Dùng Gmail App Password, không phải mật khẩu thường |
| `replay_detected` khi test lại | Redis cache nonce cũ | Chạy `FLUSHDB` qua smoke test hoặc thủ công |
| cert mount lỗi | gen-certs.sh chưa chạy | Chạy lại `scripts/gen-certs/gen-certs.sh` |
| `invalid_signature` | Cert path sai trong env | Kiểm tra volume mount trong compose file |

**Xem log chi tiết:**

```bash
# Tất cả services:
docker compose -f docker-compose.local.yml logs -f

# Một service cụ thể:
docker compose -f docker-compose.local.yml logs -f banking-service
```
