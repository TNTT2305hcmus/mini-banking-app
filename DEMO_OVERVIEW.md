# DEMO_OVERVIEW.md — Tổng hợp Demo Mini Banking App

> **Mục đích**: Tài liệu này giúp bất kỳ thành viên nào trong nhóm đọc 1 file là hiểu và chạy được demo end-to-end mà không phải lục lại nhiều file rải rác. Mỗi phần chứa tóm tắt và link tới file chi tiết.
>
> **Không thay thế** các file gốc. Nếu có thông tin mâu thuẫn giữa file này và file gốc, ưu tiên file gốc.

---

## 1. Giới thiệu

Demo `mini-banking-app` mô phỏng hệ thống ngân hàng bảo mật theo mô hình **Kerberos-like PKI + mTLS**, gồm 5 service:

- **CA Service** — Cấp phát/thu hồi certificate (PKI), lưu audit log CA.
- **KDC Service** — AS (Authentication Server) + TGS (Ticket Granting Server), cấp TGT và Ticket_v.
- **Banking Service** — Xử lý balance, transfer, lưu audit log Bank.
- **API Gateway** — Node.js entry point duy nhất ra ngoài (port 3000), xử lý OTP, routing.
- **Frontend** — Giao diện Vite/React (dev server port 5173 — chỉ có trong `docker-compose.local.yml`).

---

## 2. Kiến trúc & Docker Compose

### Bảng service

| Service | gRPC/HTTP Port (host) | Vai trò | local.yml | demo.yml |
|---|---|---|---|---|
| `bank-postgres` | 5432 (local) / internal (demo) | Postgres cho Banking Service, tự động chạy schema + seed | ✅ | ✅ |
| `ca-service` | 50051 (local) / internal (demo) | CA PKI — cấp/thu hồi cert, gRPC | ✅ | ✅ |
| `gateway-redis` | 6379 (local) / internal (demo) | Redis: session OTP, nonce cache, TGT cache | ✅ | ✅ |
| `kdc-service` | 50052 (local) / internal (demo) | AS + TGS Kerberos-like, gRPC | ✅ | ✅ |
| `banking-service` | 50053 (local) / internal (demo) | Bank logic, gRPC | ✅ | ✅ |
| `api-gateway` | **3000 (expose ra ngoài cả 2 file)** | HTTP REST Gateway, điểm duy nhất client gọi | ✅ | ✅ |
| `frontend` | 5173 (local) | Vite dev server | ✅ | ❌ |
| `ca-postgres` | comment-out (offline option) | Postgres cho CA Service (thay Neon) | comment | comment |

> **CA Postgres**: Mặc định CA dùng Neon/external DB qua biến `CA_DATABASE_URL`. Block `ca-postgres` có sẵn trong cả 2 file nhưng bị comment-out — uncomment nếu muốn chạy hoàn toàn offline.

### Khác biệt giữa hai file compose

| Tính năng | `docker-compose.local.yml` | `docker-compose.demo.yml` |
|---|---|---|
| Frontend | ✅ Vite dev server (node:20-alpine, port 5173) | ❌ Không có (build static riêng) |
| Expose port nội bộ | bank-postgres: 5432, CA: 50051, KDC: 50052, Bank: 50053, Redis: 6379 ra host | Chỉ api-gateway: 3000 expose; còn lại internal |
| Redis password | Không có `--requirepass` | Không có `--requirepass` (đã xóa — Redis internal-only, không expose port) |
| Redis name | `gateway-redis` (giữ nguyên theo `docker-compose.yml` gốc) | `gateway-redis` (giữ nguyên) |
| Mục đích | Chạy local, dev/debug | Deploy demo production-like |

> ⚠️ **Tên `gateway-redis`** được giữ nguyên trong cả 2 file mới để không phá vỡ `GATEWAY_REDIS_URL=redis://gateway-redis:6379/0` trong các service. Nếu đổi tên phải cập nhật cùng lúc cả `GATEWAY_REDIS_URL` và `REDIS_URI`.

Xem chi tiết tại: [`docker-compose.local.yml`](../docker-compose.local.yml), [`docker-compose.demo.yml`](../docker-compose.demo.yml)

---

## 3. Chuỗi lệnh chạy demo từ đầu

| # | Bước | Lệnh mẫu |
|---|---|---|
| 1 | Copy và điền env | `cp .env.demo.example .env` rồi điền 8 secret bắt buộc |
| 2 | Provision Root CA + gRPC Transport CA | `cd ca-service && ROOT_CA_KEY_PASSWORD=<pass> go run ./scripts/provision_ca_dev.go` |
| 3 | Sinh cert KDC / Bank / Gateway | `bash scripts/gen-certs/gen-certs.sh` (hoặc `.ps1` trên Windows) |
| 4 | Khởi động Docker Compose | `docker compose -f docker-compose.local.yml up --build -d` |
| 5 | Đợi services healthy | `docker compose -f docker-compose.local.yml ps` — tất cả `healthy` |
| 6 | Chạy smoke test | `bash scripts/demo/smoke-test.sh` |

**Lệnh duy nhất (sau khi đã có `.env`):**
```bash
(cd ca-service && ROOT_CA_KEY_PASSWORD=$(grep ROOT_CA_KEY_PASSWORD ../.env | cut -d= -f2) go run ./scripts/provision_ca_dev.go) \
  && bash scripts/gen-certs/gen-certs.sh \
  && docker compose -f docker-compose.local.yml up --build -d \
  && echo "Đợi services khởi động..." && sleep 30 \
  && bash scripts/demo/smoke-test.sh
```

Xem đầy đủ tại: [`scripts/demo/README.md`](../scripts/demo/README.md)

---

## 4. Secret / Biến môi trường cần cấu hình

Copy từ `.env.demo.example` sang `.env` rồi điền các biến sau:

| Biến | Mô tả | Bắt buộc đổi |
|---|---|---|
| `ROOT_CA_KEY_PASSWORD` | Mật khẩu bảo vệ private key Root CA | ⚠️ Bắt buộc |
| `CA_DATABASE_URL` | Connection string Postgres cho CA (Neon URL hoặc `postgresql://...@ca-postgres:5432/ca_db`) | ⚠️ Bắt buộc |
| `JWT_SECRET` | Secret ký JWT của API Gateway (≥32 ký tự) | ⚠️ Bắt buộc |
| `GATEWAY_OTP_SECRET` | Secret HMAC cho OTP | ⚠️ Bắt buộc |
| `SMTP_USER` | Gmail/SMTP account để gửi OTP | ⚠️ Bắt buộc (hoặc dùng `SKIP_SMTP_CHECK=1`) |
| `SMTP_PASS` | Gmail App Password (không phải mật khẩu Gmail thường) | ⚠️ Bắt buộc (hoặc `SKIP_SMTP_CHECK=1`) |
| `BANK_DB_PASSWORD` | Mật khẩu Postgres cho Banking Service | ⚠️ Bắt buộc |
| `BANK_KEY` | 64-char hex key AES Banking Service (`openssl rand -hex 32`) | ⚠️ Bắt buộc |
| `CA_DEMO_EMAIL` | Email tài khoản Admin CA demo | Default: `admin@minibanking.local` |
| `CA_DEMO_PASSWORD` | Password tài khoản Admin CA demo | ⚠️ Bắt buộc đổi |
| `BANK_DB_NAME` | Tên DB Banking | Default: `banking` |
| `BANK_DB_USER` | User Postgres Banking | Default: `banking` |
| `TGT_EXP` | Thời hạn TGT | Default: `10m` |
| `CERT_VALIDITY_DAYS` | Hiệu lực cert cấp | Default: `365` |
| `SKIP_SMTP_CHECK` | `1` = bỏ qua OTP trong smoke test | Default: `0` |
| `DEMO_EMAIL` | Email dùng để test OTP (phải là email alice) | Default: `alice@demo.minibanking.local` |
| `COMPOSE_FILE` | File compose đang dùng | Default: `docker-compose.local.yml` |

> Nếu dùng `ca-postgres` nội bộ thay Neon: uncomment block trong compose và thêm `CA_DB_PASSWORD`.

Xem chi tiết tại: [`.env.demo.example`](../.env.demo.example)

---

## 5. Seed Data — Tài khoản Test

### 5.1 Users & Accounts

Dữ liệu lấy trực tiếp từ [`db/bank/seed_demo.sql`](../db/bank/seed_demo.sql).

> ⚠️ **Đơn vị số dư**: Comment trong `seed_demo.sql` có mâu thuẫn nội bộ — ghi rằng `balance` lưu dạng cents (VND × 100), nhưng ví dụ trong comment lại tính `10,000,000 VND = 10_000_000` (tức 1 VND = 1 đơn vị). **Giá trị thực tế INSERT vào DB**: `balance = 1000000000` cho tài khoản Alice acct1. Người đọc cần kiểm tra `db/bank/migrations/001_init_bank.sql` để xác nhận convention thật của cột `balance` trước khi hiển thị số dư trong UI.

| User | Full Name | Email | User UUID |
|---|---|---|---|
| alice | Nguyễn Thị Alice | `alice@demo.minibanking.local` | `a0000000-0000-0000-0000-000000000001` |
| bob | Trần Văn Bob | `bob@demo.minibanking.local` | `b0000000-0000-0000-0000-000000000001` |
| charlie | Lê Văn Charlie | `charlie@demo.minibanking.local` | `c0000000-0000-0000-0000-000000000001` |

| User | Account UUID | Số tài khoản | Giá trị `balance` trong DB | Comment trong seed |
|---|---|---|---|---|
| alice (acct1) | `a0000000-0000-0000-0001-000000000001` | `110001000001` | `1000000000` | "10,000,000 VND" |
| alice (acct2) | `a0000000-0000-0000-0001-000000000002` | `110001000002` | `500000000` | "5,000,000 VND" |
| bob (acct1) | `b0000000-0000-0000-0001-000000000001` | `110002000001` | `2000000000` | "20,000,000 VND" |
| charlie (acct1) | `c0000000-0000-0000-0001-000000000001` | `110003000001` | `1500000000` | "15,000,000 VND" |

Daily transfer limit: tất cả account đều có giá trị `5000000000` trong DB ("50,000,000 VND" theo comment seed).

### 5.2 Transactions mẫu đã seed

| TX UUID | Từ | Đến | `amount` trong DB | Comment trong seed | Status |
|---|---|---|---|---|---|
| `d1000000-0000-0000-0000-000000000001` | alice `110001000001` | bob `110002000001` | `500000000` | "5,000,000 VND" | completed |
| `d2000000-0000-0000-0000-000000000001` | bob `110002000001` | charlie `110003000001` | `200000000` | "2,000,000 VND" | completed |
| `d3000000-0000-0000-0000-000000000001` | alice `110001000001` | charlie `110003000001` | `100000000` | "1,000,000 VND" | completed |

Ngoài ra có 4 bản ghi `bank_audit_log` mẫu: 3 `transfer_completed` + 1 `insufficient_funds` (charlie cố chuyển số tiền vượt số dư).

> ⚠️ **Quan trọng**: Tất cả transactions và audit log seed dùng `client_signature = 'SEED_DEMO_PLACEHOLDER'` — **không phải chữ ký RSA thật**. Chúng chỉ nhằm mục đích cho Admin Bank có data hiển thị. Nếu banking-service validate signature tại read-path, các record này có thể bị từ chối khi query. **Để demo transfer thật**, phải chạy đầy đủ flow: PKI register → AS Request → TGS Request → Transfer qua Gateway.

### 5.3 Tài khoản Admin

| Role | Cách đăng nhập | Biến env liên quan |
|---|---|---|
| **Admin CA** | `POST /v1/admin-ca/auth` với email + password | `CA_DEMO_EMAIL`, `CA_DEMO_PASSWORD` trong `.env` |
| **Admin Bank** | Flow activate → session cookie (không có password đơn giản) | Xem `scripts/demo/README.md` §5 — endpoint: `POST /v1/admin/bank/activate` → `POST /v1/admin/bank/session` |

> Admin Bank không có tài khoản seed sẵn với password vì flow yêu cầu activation code (thường gửi qua email/cơ chế riêng). Xem `api-gateway/src/` để biết endpoint cụ thể.

> ⚠️ **Mâu thuẫn spec token field**: `audit-testcases.md §4` ghi curl mẫu dùng `.data.token`, còn `api-design/06` dùng `.data.access_token`. Smoke test hiện fallback thử `.data.token` trước rồi `.data.access_token`. Team cần thống nhất implementation.

---

## 6. Testcase List

Tổng cộng **78 testcase** chia 5 nhóm (đếm từ `docs/testcases.md`):

| # | Nhóm | Số case | Kiểm tra |
|---|---|---|---|
| 1 | **User Flow** | 17 case (TC-U-01 → TC-U-17) | OTP, PKI register, AS/TGS request, balance, transfer, idempotency |
| 2 | **Admin CA** | 13 case (TC-CA-01 → TC-CA-13) | Auth Admin CA, list/detail cert, revoke, negative filter |
| 3 | **Admin Bank** | 9 case (TC-AB-01 → TC-AB-09) | Session, overview, users, accounts, transactions, audit query, unauthorized |
| 4 | **Audit Log** | 24 case (TC-AUD-01 → TC-AUD-24) + 1 ghi chú | CA audit actions, Bank audit actions, filter theo serial/performed_by/time/request_id/cert_serial, best-effort |
| 5 | **Negative Tests** | 15 case (TC-NEG-01 → TC-NEG-15) | Missing header, sai role, hết hạn token, service down, wrong scope, invalid body |

**Ownership**: User Flow — người chạy demo cuối | Admin CA — Quang | Admin Bank — Thái | Audit — Thuận | Negative — Quang.

**Chủ đích KHÔNG ghi audit** (không báo FAIL khi không thấy event):
`ListCertificates` thành công, `balance`/`history`/`profile` thành công, `CreateUser` Bank — thiết kế cố ý theo `docs/audit-testcases.md §5`.

**API đọc audit** (2 endpoint khác nhau, không dùng nhầm):
- CA: `GET /v1/admin-ca/audit?action&serial&performed_by&from&to&limit&offset` — Bearer token JWT, `from`/`to` là **ISO 8601 UTC string**.
- Bank: `POST /v1/admin/bank/audit/query` body JSON — session cookie, `from_unix`/`to_unix` là **Unix epoch (int64)**.

Xem đầy đủ: [`docs/testcases.md`](testcases.md), [`docs/audit-testcases.md`](audit-testcases.md)

---

## 7. Smoke Test

### 7.1 Lệnh chạy

```bash
# Linux/macOS/Git Bash:
bash scripts/demo/smoke-test.sh

# Với tùy chọn:
SKIP_SMTP_CHECK=1 GW=http://localhost:3000 bash scripts/demo/smoke-test.sh

# Windows PowerShell:
.\scripts\demo\smoke-test.ps1 -SkipSmtp -GW "http://localhost:3000"
```

**Biến môi trường quan trọng** (cho `.sh`) / **tham số** (cho `.ps1`):

| Biến (sh) | Tham số (ps1) | Mô tả | Default |
|---|---|---|---|
| `GW` | `-GW` | Base URL API Gateway | `http://localhost:3000` |
| `SKIP_SMTP_CHECK=1` | `-SkipSmtp` | Bỏ qua bước OTP | `0` |
| `DEMO_OTP` | `-DemoOtp` | OTP để tự động verify (nếu đã lấy được) | trống |
| `COMPOSE_FILE` | `-ComposeFile` | File compose đang dùng | `docker-compose.local.yml` |

### 7.2 Các bước đã tự động hoá

Đọc từ `smoke-test.sh` — script thực sự làm 9 bước:

| Bước | Nội dung | Tự động |
|---|---|---|
| 1 | Kiểm tra Docker daemon (`docker info`) | ✅ |
| 2 | Kiểm tra 6 port đang lắng nghe (3000, 50051, 50052, 50053, 6379, 5432) | ✅ |
| 3 | Redis PING + FLUSHDB db0 (clear cache trước test) | ✅ |
| 4 | API Gateway HTTP health check | ✅ |
| 5 | OTP request (bỏ qua nếu `SKIP_SMTP_CHECK=1` hoặc thiếu `SMTP_USER`/`SMTP_PASS`) | ✅ (conditional) |
| 6 | OTP verify + lấy `registration_token` (chỉ khi `DEMO_OTP` được set) | ✅ (conditional) |
| 7 | Admin CA auth → lấy token | ✅ |
| 8 | Admin CA list certificates + detail + negative (token sai) | ✅ (nếu có token) |
| 9 | **Negative test**: `POST /v1/admin/bank/audit/query` không có session cookie → kỳ vọng 401/403 | ✅ |

### 7.3 Bước chưa tự động hoá

| Flow | Lý do chưa tự động | Làm thủ công ở đâu |
|---|---|---|
| PKI register đầy đủ (CSR + cert) | Cần `openssl genrsa` + ký CSR = thao tác file system ngoài scope smoke test | `scripts/demo/README.md §4` |
| AS Request (TGT) | Cần ký payload bằng RSA private key (crypto operation) | README §4 hoặc Frontend UI |
| TGS Request (Ticket_v) | Cần giải mã `as_rep` bằng session key | README §4 hoặc Frontend UI |
| Transfer thật | Cần `Ticket_v` hợp lệ + AP protocol | README §4 hoặc Frontend UI |
| Admin Bank session đầy đủ | Cần activation code (flow phụ thuộc implementation cụ thể) | `scripts/demo/README.md §5` |

---

## 8. Checklist Trước Khi Demo

**Demo thông thường:**
- [ ] Reset data nếu cần: `docker compose -f docker-compose.local.yml down -v` rồi `up` lại
- [ ] Chạy smoke test: `bash scripts/demo/smoke-test.sh` — không có FAIL
- [ ] Đăng nhập Admin CA thành công
- [ ] Chạy 1 transfer mẫu thật qua Frontend UI

**Demo quan trọng (bảo vệ đồ án / khách hàng):**
- [ ] Tất cả bước trên
- [ ] Backup DB: `docker compose exec bank-postgres pg_dump -U banking banking > backup_$(date +%Y%m%d_%H%M%S).sql`
- [ ] Backup certs: `tar czf certs_backup.tar.gz ca-service/certs kdc-service/certs banking-service/certs api-gateway/certs`
- [ ] Test đầy đủ 1 lần trên máy demo thật (không phải máy dev)
- [ ] Tắt notification, chuẩn bị plan B (screenshot/video)

Xem chi tiết tại: [`scripts/demo/README.md §6`](../scripts/demo/README.md)

---

## 9. Vấn Đề Đã Biết / Giới Hạn Hiện Tại

Phần này liệt kê trung thực các giới hạn tại thời điểm viết tài liệu:

1. **Smoke test không tự động hoá AS/TGS/transfer**: Các flow này yêu cầu thao tác crypto (ký RSA, encrypt/decrypt) nên smoke test chỉ in hướng dẫn thủ công. Người demo phải chạy tay hoặc qua Frontend UI.

2. **Admin Bank smoke test chỉ test negative**: Bước 9 của smoke test chỉ xác nhận endpoint trả 401/403 khi không có cookie — **không** test được flow đầy đủ (activate → session → query) vì cần activation code từ cơ chế ngoài script.

3. **Seed transactions không có `client_signature` thật**: 3 transaction và audit log seed dùng `'SEED_DEMO_PLACEHOLDER'`. Nếu banking-service validate signature ở read-path, các record này có thể không hiển thị đúng trong Admin Bank UI. Cần chạy 1 transfer thật qua flow PKI → AS → TGS → transfer để có data ký thật.

4. **Mâu thuẫn spec token field `POST /v1/admin-ca/auth`**: `audit-testcases.md §4` dùng `.data.token`, `api-design/06` dùng `.data.access_token`. Smoke test fallback cả 2 nhưng team cần thống nhất implementation.

5. **Mâu thuẫn comment đơn vị `balance` trong seed**: `seed_demo.sql` comment nói lưu dạng cents (VND×100) nhưng ví dụ tính lại theo 1 VND = 1 đơn vị. Giá trị INSERT thực tế: `1000000000` cho "10,000,000 VND". Cần kiểm tra `db/bank/migrations/001_init_bank.sql` để xác nhận convention thật trước khi hiển thị số dư.

6. **CA Postgres mặc định dùng Neon (external)**: Nếu Neon offline, `ca-service` không khởi động được. Cần uncomment `ca-postgres` block trong compose và thêm `CA_DB_PASSWORD` để chạy offline hoàn toàn.

7. **Frontend chỉ có trong `docker-compose.local.yml`**: `docker-compose.demo.yml` không có Vite dev server — frontend phải build static và serve riêng (nginx/CDN) nếu dùng file này.

8. **Tên `gateway-redis` được giữ nguyên**: Đây là quyết định chủ ý để đồng nhất với `docker-compose.yml` gốc. Nếu đổi tên service Redis, phải cập nhật đồng thời `GATEWAY_REDIS_URL` và `REDIS_URI` trong tất cả service.

---

## 10. Bảng Tham Chiếu Nhanh

| Nội dung | Đường dẫn file |
|---|---|
| Compose gốc (không sửa) | [`docker-compose.yml`](../docker-compose.yml) |
| Compose local demo (có frontend dev) | [`docker-compose.local.yml`](../docker-compose.local.yml) |
| Compose deploy demo (production-like) | [`docker-compose.demo.yml`](../docker-compose.demo.yml) |
| Env template đầy đủ | [`.env.demo.example`](../.env.demo.example) |
| Dockerfile Banking Service (mới) | [`banking-service/Dockerfile`](../banking-service/Dockerfile) |
| Dockerfile KDC Service (đã sửa port 50052) | [`kdc-service/Dockerfile`](../kdc-service/Dockerfile) |
| Schema Banking DB | [`db/bank/migrations/001_init_bank.sql`](../db/bank/migrations/001_init_bank.sql) |
| Seed data Banking DB (idempotent) | [`db/bank/seed_demo.sql`](../db/bank/seed_demo.sql) |
| Sinh cert KDC/Bank/Gateway | [`scripts/gen-certs/gen-certs.sh`](../scripts/gen-certs/gen-certs.sh) |
| Sinh cert (Windows) | [`scripts/gen-certs/gen-certs.ps1`](../scripts/gen-certs/gen-certs.ps1) |
| Hướng dẫn gen-certs | [`scripts/gen-certs/README.md`](../scripts/gen-certs/README.md) |
| Provision Root CA | [`ca-service/scripts/provision_ca_dev.go`](../ca-service/scripts/provision_ca_dev.go) |
| Hướng dẫn demo đầy đủ (bypass OTP, Admin Bank session, checklist) | [`scripts/demo/README.md`](../scripts/demo/README.md) |
| Smoke test Bash | [`scripts/demo/smoke-test.sh`](../scripts/demo/smoke-test.sh) |
| Smoke test PowerShell | [`scripts/demo/smoke-test.ps1`](../scripts/demo/smoke-test.ps1) |
| Bảng testcase đầy đủ (78 case, 5 nhóm) | [`docs/testcases.md`](testcases.md) |
| Chuẩn field audit + curl mẫu | [`docs/audit-testcases.md`](audit-testcases.md) |
