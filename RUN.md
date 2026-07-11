# Hướng Dẫn Cấu Hình Và Chạy Hệ Thống — Mini Banking App

Tài liệu này dành cho người chấm/giáo viên: kéo code về và dựng toàn bộ hệ thống trên máy sạch để kiểm tra. Toàn bộ lệnh viết cho **Windows PowerShell** (kèm ghi chú cho Linux/macOS). Thời gian dựng lần đầu khoảng **15–20 phút** (chủ yếu là Docker build).

Tài liệu chi tiết hơn nằm trong [demo_test_guide/guide/](demo_test_guide/guide/RUN_GUIDE.md) (env, compose, seed, troubleshooting).

---

## 1. Yêu cầu môi trường

| Tool           | Phiên bản                 | Kiểm tra          |
| -------------- | ------------------------- | ----------------- |
| Go             | 1.25.x                    | `go version`      |
| Node.js        | 22.x LTS trở lên          | `node -v`         |
| Docker Desktop | bản mới, engine đang chạy | `docker version`  |
| OpenSSL        | 1.1.1+ hoặc 3.x           | `openssl version` |
| Git            | bất kỳ                    | `git --version`   |

> Go và OpenSSL chỉ cần cho bước sinh certificate/key ban đầu (chạy trên host). Các service chạy trong Docker.

## 2. Kéo code và vào đúng thư mục

```powershell
git clone <URL_REPO>
cd mini-banking\mini-banking-app
```

**Quan trọng:** mọi lệnh từ đây trở đi chạy tại thư mục `mini-banking-app/` bên trong repo (root runtime — nơi có `docker-compose.local.yml`), không phải root repo.

## 3. Cấu hình biến môi trường

Copy template:

```powershell
Copy-Item .\.env.demo.example .\.env
```

Mở `.env` và thay các giá trị bắt buộc (mọi dòng có ghi `⚠️ THAY ĐỔI BẮT BUỘC`):

| Biến                                               | Ý nghĩa                                                                  | Gợi ý giá trị                                                                                           |
| -------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| `ROOT_CA_KEY_PASSWORD`                             | Mật khẩu bảo vệ private key Root CA                                      | Chuỗi bất kỳ, ví dụ tạo bằng `openssl rand -hex 16`                                                     |
| `CA_DATABASE_URL`                                  | Connection string tới CA Postgres                                        | Dùng ca-postgres nội bộ: `postgresql://ca_user:<CA_DB_PASSWORD>@ca-postgres:5432/ca_db?sslmode=disable` |
| `BANK_DB_PASSWORD`                                 | Mật khẩu Postgres của Banking Service                                    | Chuỗi bất kỳ                                                                                            |
| `JWT_SECRET`                                       | Secret ký JWT của Gateway (≥32 ký tự)                                    | `openssl rand -hex 32`                                                                                  |
| `GATEWAY_OTP_SECRET`                               | Secret HMAC cho OTP                                                      | `openssl rand -hex 32`                                                                                  |
| `SMTP_USER` / `SMTP_PASS`                          | Tài khoản Gmail gửi OTP (App Password, không phải mật khẩu Gmail thường) | Xem mục 3.1 nếu không có SMTP                                                                           |
| `ADMIN_SEC_DEMO_PASSWORD` / `ADMIN_SEC_DEMO_TOKEN` | Credential demo cho Admin SOC                                            | Chuỗi bất kỳ (chỉ dùng demo)                                                                            |

Nếu dùng ca-postgres nội bộ (khuyến nghị cho máy chấm, không cần DB ngoài), phải sửa đủ **3 chỗ**:

1. Trong `.env`: uncomment và điền `CA_DB_NAME`, `CA_DB_USER`, `CA_DB_PASSWORD`.
2. Trong `.env`: đặt `CA_DATABASE_URL=postgresql://ca_user:<CA_DB_PASSWORD>@ca-postgres:5432/ca_db?sslmode=disable` (hostname `ca-postgres`, port `5432` vì chạy trong network Docker).
3. Trong `docker-compose.local.yml`: uncomment toàn bộ block service `ca-postgres` **và** dòng `ca_postgres_data:` trong mục `volumes:` ở cuối file. Nếu chỉ uncomment service mà quên volume sẽ gặp lỗi `service "ca-postgres" refers to undefined volume ca_postgres_data`.

Tùy chọn hữu ích khi kiểm tra nhiều lần:

```env
RATE_LIMIT_DISABLED=1
```

### 3.1. Nếu không có tài khoản SMTP

Luồng đăng ký customer cần OTP gửi qua email thật. Nếu không tiện cấu hình Gmail App Password:

- Vẫn dựng được toàn bộ stack và kiểm tra bằng **tài khoản seed** (mục 7) và **smoke test bỏ SMTP**: `.\scripts\demo\smoke-test.ps1 -SkipSmtp` (hoặc `SKIP_SMTP_CHECK=1`).
- Chỉ riêng bước đăng ký user mới qua email là không thực hiện được.

## 4. Sinh certificate và key (chạy một lần, theo đúng thứ tự)

Chạy tại `mini-banking-app/`.

Script provision CA tự đọc `ROOT_CA_KEY_PASSWORD` từ `.env` ở root runtime (đã điền ở mục 3), nên không cần set biến môi trường thủ công. Nếu vẫn gặp `panic: ROOT_CA_KEY_PASSWORD is required`, kiểm tra `.env` đã tồn tại và biến đã được điền, hoặc set tạm trong shell: `$env:ROOT_CA_KEY_PASSWORD = "<giá trị trong .env>"`.

```powershell
# 4.1. Provision Root CA, Client CA, gRPC Transport CA, CA server cert
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

# 4.2. Sinh cert TLS cho KDC/Bank, copy trust bundle, sinh KDC AS_REP signing chain
#      và đặt Root CA anchor cho frontend (cần openssl trên PATH). Tất cả tự động.
# ---- Có thể dùng git bash tại 'mini-baning-app/' với câu lệnh cho Linux/macOS bên dưới
.\scripts\gen-certs\gen-certs.ps1

# 4.3. Sinh riêng K_tgs và K_v (không cần env; set $env:FORCE="1" nếu muốn ghi đè toàn bộ key cũ)
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Script tạo hai khóa độc lập trong `kdc-service/certs/`:

- `k_tgs.key`: chỉ KDC dùng để mã hóa/giải mã TGT.
- `kdc-service/certs/k_v.key`: bản `K_v` riêng mà KDC đọc để cấp `Ticket_v`.
- `banking-service/certs/k_v.key`: bản `K_v` riêng mà Banking Service đọc để mở `Ticket_v`.

Hai file `K_v` chứa **cùng chính xác 32 byte**, nhưng mỗi service chỉ mount file trong thư mục của mình: KDC dùng `/certs/kdc/k_v.key`, Bank dùng `/certs/bank/k_v.key`. Không service nào truy cập thư mục key của service còn lại. Nếu chỉ một bản đã tồn tại, script tạo bản còn thiếu từ bản đó; nếu cả hai tồn tại nhưng khác nhau, script dừng và yêu cầu xử lý hoặc chạy lại với `FORCE=1`.

Các file key được `.gitignore`, vì vậy máy mới bắt buộc phải chạy bước 4.3 trước khi `docker compose up`.

Linux/macOS:

```bash
(cd ca-service && go run ./scripts/provision_ca_dev.go)
./scripts/gen-certs/gen-certs.sh
go run ./kdc-service/scripts/provision_kdc_dev.go
```

Kiểm tra kết quả — tất cả phải trả `True`:

```powershell
Test-Path .\ca-service\certs\root-ca\ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.crt
Test-Path .\api-gateway\certs\grpc-ca.crt
Test-Path .\kdc-service\certs\k_tgs.key
Test-Path .\kdc-service\certs\k_v.key
Test-Path .\banking-service\certs\k_v.key
Test-Path .\kdc-service\certs\kdc-server.crt
Test-Path .\kdc-service\certs\kdc-signing-chain.pem
Test-Path .\banking-service\certs\grpc\bank-server.crt
Test-Path .\frontend\public\trust\root-ca.pem
```

> **Xác minh phía client (bảo mật — bắt buộc có 2 file mới ở trên):** ngoài cert TLS,
> gen-certs còn sinh `kdc-service\certs\kdc-signing-chain.pem` (KDC dùng để **ký AS_REP**)
> và đặt Root CA anchor tại `frontend\public\trust\root-ca.pem` (frontend **nạp runtime**,
> same-origin, để tự xác minh chữ ký KDC ở AS Exchange và certificate do CA cấp lúc đăng ký).
> Cơ chế này **fail-closed**: thiếu 2 file này thì đăng nhập/đăng ký sẽ báo lỗi xác minh
> (không tạo phiên). Không cần thêm biến `.env` — đường dẫn đã cấu hình sẵn trong
> `docker-compose.local.yml` (`KDC_SIGNING_CHAIN_PATH`) và Vite phục vụ file anchor tĩnh.
> Chi tiết: [security-upgrade-report.md](mini-banking-app/security-upgrade-report.md),
> [flows-security-report.md](mini-banking-app/flows-security-report.md).

## 5. Khởi động hệ thống

```powershell
docker compose -f docker-compose.local.yml up --build -d
```

Chờ build xong rồi kiểm tra trạng thái:

```powershell
docker compose -f docker-compose.local.yml ps
```

Các service phải ở trạng thái `running`/`healthy`. Riêng 2 container **một-lần**
`ca-migrate` và `bank-migrate` sẽ ở trạng thái `Exited (0)` sau khi áp xong
migration/seed — **đây là bình thường**, không phải lỗi. Nếu có service lỗi, xem log:

```powershell
docker compose -f docker-compose.local.yml logs -f api-gateway
```

Truy cập:

| Thành phần  | Địa chỉ               |
| ----------- | --------------------- |
| Frontend    | http://localhost:5173 |
| API Gateway | http://localhost:3000 |

> Có thêm `docker-compose.demo.yml` (production-like, không kèm frontend dev server) — chỉ dùng khi frontend được build/serve riêng. Để kiểm tra, nên dùng `docker-compose.local.yml`.

## 6. Xác nhận hệ thống chạy đúng (smoke test)

```powershell
.\scripts\demo\smoke-test.ps1
# hoặc nếu chưa cấu hình SMTP:
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

Linux/macOS/Git Bash:

```bash
./scripts/demo/smoke-test.sh
```

Smoke test tự động kiểm các endpoint kiểm được bằng HTTP/token. Các flow cần private key trong browser (đăng ký đầy đủ, AS/TGS/AP signed request, admin cert session) kiểm bằng UI theo mục 7.

## 7. Kịch bản kiểm tra qua UI

Dữ liệu seed (nạp tự động lần đầu khởi tạo Postgres): 20 customer, 20 account, 50 transaction — chi tiết trong [SEED_AND_ACCOUNTS.md](demo_test_guide/guide/SEED_AND_ACCOUNTS.md). Email seed mặc định: `alice@demo.minibanking.local`.

### 7.1. Luồng customer (cần SMTP để đăng ký mới)

1. Mở `http://localhost:5173/register` → nhập email thật → nhận OTP → đặt PIN → browser sinh keypair/CSR → đăng ký thành công (CA cấp cert, Bank tạo account).
2. `http://localhost:5173/login` → đăng nhập bằng cert/PIN (AS/TGS Exchange chạy ngầm).
3. Tại Home: chuyển tiền tới account seed → kiểm tra số dư và lịch sử cập nhật.

### 7.2. Admin CA — `http://localhost:5173/admin-ca`

Activate/login bằng certificate role `ca_admin` (provision theo hướng dẫn admin trong `demo_test_guide/guide/`), sau đó: list/detail certificate, revoke cert, xem audit CA. Sau khi revoke cert của một customer, customer đó login/giao dịch sẽ bị từ chối — đây là điểm kiểm tra revocation.

### 7.3. Admin Bank — `http://localhost:5173/admin-bank`

Login cert-based role `bank_admin` → dashboard overview/users/accounts/transactions/audit.

### 7.4. Admin SOC — `http://localhost:5173/admin-soc`

Login bằng `ADMIN_SEC_DEMO_EMAIL` + `ADMIN_SEC_DEMO_PASSWORD` đã đặt trong `.env`, sau đó kiểm:

- KDC audit list (event AS/TGS);
- Timeline cross-service theo `operation_id` của giao dịch vừa thực hiện;
- Verify hash-chain (kỳ vọng PASS; nếu sửa tay 1 record audit trong DB rồi verify lại sẽ FAIL — chứng minh tamper-evidence);
- Summary và export JSON/CSV.

Bộ testcase đầy đủ (functional/security/audit) để đối chiếu: [demo_test_guide/tests/](demo_test_guide/tests/testcases.md).

## 8. Lỗi thường gặp

| Triệu chứng                                                         | Nguyên nhân / cách xử lý                                                                                                                                                                           |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `panic: ROOT_CA_KEY_PASSWORD is required` khi provision CA          | `.env` ở root runtime chưa tồn tại hoặc biến chưa điền (mục 3). Kiểm tra lại `.env`, hoặc set tạm `$env:ROOT_CA_KEY_PASSWORD = "<giá trị trong .env>"` rồi chạy lại.                               |
| `gen-certs.ps1` báo `Missing provisioned gRPC Transport CA`         | Bước 4.1 chưa chạy hoặc chạy fail. Chạy lại provision CA trước, xác nhận các file trong `ca-service\certs\` tồn tại.                                                                               |
| KDC/Bank báo thiếu `k_v.key`                                       | Chưa chạy bước 4.3 tại root runtime. Chạy provisioning, xác nhận `k_v.key` tồn tại trong cả `kdc-service/certs/` và `banking-service/certs/`, rồi recreate hai service.                              |
| `service "ca-postgres" refers to undefined volume ca_postgres_data` | Đã uncomment service `ca-postgres` nhưng quên uncomment dòng `ca_postgres_data:` trong mục `volumes:` cuối `docker-compose.local.yml` (xem mục 3).                                                 |
| Seed/migration không có dữ liệu, SOC không thấy event               | Migration/seed nay tự chạy mỗi lần `up` qua 2 service một-lần `ca-migrate`/`bank-migrate` (idempotent, áp cả lên volume cũ) nên lỗi kiểu `column ... does not exist` không còn. Nếu vẫn muốn reset sạch: `docker compose -f docker-compose.local.yml down -v` rồi `up --build -d`. |
| Đăng nhập lỗi `AS_REP thiếu kdc_cert_chain` / `Không tải được Root CA từ /trust/root-ca.pem` | Mục 4.2 chưa chạy (thiếu `kdc-service\certs\kdc-signing-chain.pem` hoặc `frontend\public\trust\root-ca.pem`). Chạy lại 4.2; nếu vừa đổi code KDC thì `docker compose -f docker-compose.local.yml build kdc-service` rồi `up -d`. |
| Đăng ký lỗi `Public key trong cert không khớp` / `cert không chain về Root` | Root CA anchor (`frontend\public\trust\root-ca.pem`) không khớp CA đang cấp cert — thường do đổi/sinh lại Root CA mà chưa chạy lại gen-certs để cập nhật anchor. Chạy lại **cả mục 4** để đồng bộ, và xóa site data của `localhost:5173`. |
| Không nhận được OTP                                                 | SMTP chưa cấu hình đúng (`SMTP_PASS` phải là Gmail **App Password**). Tạm thời kiểm tra bằng seed account + smoke `-SkipSmtp`.                                                                     |
| Bị 429 khi thao tác nhiều lần                                       | Rate-limit. Set `RATE_LIMIT_DISABLED=1` trong `.env` rồi restart gateway.                                                                                                                          |
| Login dùng nhầm cert/key cũ, hành vi lạ                             | Browser giữ IndexedDB từ lần chạy trước. Dùng browser profile mới hoặc xóa site data của `localhost:5173`.                                                                                         |
| Container Go service không healthy                                  | Thường do thiếu cert/key (mục 4 chưa chạy đủ hoặc sai thứ tự) hoặc `CA_DATABASE_URL` sai. Xem `docker compose logs <service>`.                                                                     |
| Cảnh báo Docker config khi `up`                                     | Không phải lỗi YAML — xem [TROUBLESHOOTING.md](demo_test_guide/guide/TROUBLESHOOTING.md).                                                                                                          |

## 9. Dừng và dọn dẹp

```powershell
# Dừng stack, giữ dữ liệu
docker compose -f docker-compose.local.yml down

# Dừng và xóa volume (chạy lại initdb/migration/seed từ đầu ở lần up sau)
docker compose -f docker-compose.local.yml down -v
```

---

## Tóm tắt trình tự cho người chấm

```powershell
git clone <URL_REPO>
cd mini-banking\mini-banking-app
Copy-Item .\.env.demo.example .\.env        # rồi điền secret theo mục 3
cd .\ca-service; go run .\scripts\provision_ca_dev.go; cd ..
.\scripts\gen-certs\gen-certs.ps1
go run .\kdc-service\scripts\provision_kdc_dev.go
docker compose -f docker-compose.local.yml up --build -d
.\scripts\demo\smoke-test.ps1 -SkipSmtp
# Mở http://localhost:5173 và kiểm tra theo mục 7
```
