# Demo Prep Checklist

Mục tiêu: chuẩn bị trước khi quay để tránh thiếu env, cert, seed, token hoặc browser state.

## 1. Runtime checklist

Chạy từ root runtime:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

Kiểm tra file chính:

- `.env` đã được copy từ `.env.demo.example`.
- `.env` đã thay secret thật.
- `docker-compose.local.yml` hoặc `docker-compose.demo.yml` là compose đang dùng.

Kiểm tra rate-limit mode:

- Rehearsal/quay nhiều lần: `RATE_LIMIT_DISABLED=1`.
- Muốn demo rate-limit thật: `RATE_LIMIT_DISABLED=0`, nhưng cần chuẩn bị tránh tự khóa flow chính.

## 2. Certificate/key checklist

Các file cần tồn tại:

```powershell
Test-Path .\ca-service\certs\root-ca\ca.key
Test-Path .\ca-service\certs\root-ca\ca.crt
Test-Path .\ca-service\certs\intermediate\client-ca.key
Test-Path .\ca-service\certs\intermediate\client-ca.crt
Test-Path .\ca-service\certs\intermediate\grpc-ca.crt
Test-Path .\api-gateway\certs\grpc-ca.crt
Test-Path .\kdc-service\certs\k_tgs.key
Test-Path .\kdc-service\certs\kdc-server.crt
Test-Path .\banking-service\certs\grpc\bank-server.crt
```

Nếu thiếu, chạy lại:

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

.\scripts\gen-certs\gen-certs.ps1
go run .\kdc-service\scripts\provision_kdc_dev.go
```

## 3. Stack checklist

Chạy local compose:

```powershell
docker compose -f docker-compose.local.yml up --build -d
docker compose -f docker-compose.local.yml ps
```

Các service cần healthy/running:

- `bank-postgres`
- `gateway-redis`
- `ca-service`
- `kdc-service`
- `banking-service`
- `api-gateway`
- `frontend`

Nếu Postgres volume cũ chưa có KDC migration, apply thủ công hoặc reset volume theo `COMPOSE_GUIDE.md`.

## 4. Smoke checklist

PowerShell smoke:

```powershell
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

Nếu có token:

```powershell
$env:ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>"
$env:ADMIN_SEC_DEMO_TOKEN="<security-admin-token>"
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

Smoke pass không thay thế functional/security rehearsal. Nó chỉ kiểm stack và các endpoint có thể kiểm bằng HTTP/token.

## 5. Browser checklist

Trước khi quay:

- Dùng browser profile sạch hoặc clear IndexedDB của app.
- Nếu đã từng activate cert cũ, clear IndexedDB để tránh đọc nhầm.
- Chuẩn bị PIN demo thống nhất cho customer/admin nếu được phép.
- Chuẩn bị sẵn mailbox nhận OTP/activation link.

Các route UI:

- Customer register: `http://localhost:5173/register`
- Customer login: `http://localhost:5173/login`
- Customer home: `http://localhost:5173/home`
- Admin CA: `http://localhost:5173/admin-ca`
- Admin CA activate: `http://localhost:5173/admin-ca/activate`
- Admin Bank: `http://localhost:5173/admin-bank`
- Admin SOC: `http://localhost:5173/admin-soc`

## 6. Data checklist

Chuẩn bị:

- Customer demo mới để register.
- Customer/account seed để transfer.
- Receiver account hợp lệ.
- Cert phụ để revoke, không dùng cert chính đang cần cho flow.
- Bank Admin cert/session.
- CA Admin cert/session.
- SOC security-admin credential/token.

Ghi lại trong lúc quay:

- `operation_id` của flow register/login/transfer nếu UI/log hiển thị.
- Serial cert được revoke.
- Transaction id của transfer thành công.
- Export file CSV/JSON từ SOC.

## 7. Backup plan

Nếu OTP/SMTP lỗi:

- Chuyển qua đoạn đã chuẩn bị sẵn user/cert.
- Ghi rõ OTP email là external dependency.

Nếu cert trong browser lỗi:

- Clear IndexedDB.
- Activate lại cert.

Nếu SOC thiếu KDC audit:

- Kiểm tra `DATABASE_URL` của KDC.
- Kiểm tra bảng `kdc_audit_log`.
- Chạy lại login AS/TGS sau khi KDC audit DB đã bật.

Nếu rate-limit chặn:

- Set `RATE_LIMIT_DISABLED=1`.
- Restart API Gateway.
