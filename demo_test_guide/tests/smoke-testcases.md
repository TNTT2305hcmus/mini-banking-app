# Smoke Testcases

Mục tiêu: kiểm nhanh stack trước khi rehearsal/quay demo. Bộ này bám theo:

- `mini-banking-app/scripts/demo/smoke-test.ps1`
- `mini-banking-app/scripts/demo/smoke-test.sh`

## 1. Cách chạy

PowerShell:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
.\scripts\demo\smoke-test.ps1
```

Bỏ SMTP:

```powershell
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

Với token admin:

```powershell
$env:ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>"
$env:ADMIN_SEC_DEMO_TOKEN="<security-admin-token>"
.\scripts\demo\smoke-test.ps1
```

Bash:

```bash
ADMIN_CA_TOKEN="<cert-backed-admin-ca-session-token>" \
ADMIN_SEC_DEMO_TOKEN="<security-admin-token>" \
./scripts/demo/smoke-test.sh
```

## 2. Testcases

| ID | Script step | Input / Env | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|
| SMK-01 | Docker daemon | Docker Desktop/dockerd running | `docker info` OK | | RUNTIME PENDING | |
| SMK-02 | Port listening | Compose stack up | Ports 3000/50051/50052/50053/6379/5432 listening | | RUNTIME PENDING | |
| SMK-03 | Redis ping | `gateway-redis` container | `redis-cli ping` returns `PONG` | | RUNTIME PENDING | |
| SMK-04 | Redis flush | `gateway-redis` container | `FLUSHDB` returns `OK` | | RUNTIME PENDING | |
| SMK-05 | Gateway health | `GW=http://localhost:3000` | Gateway returns any HTTP status, not connection failure | | RUNTIME PENDING | |
| SMK-06 | OTP request | SMTP configured, no skip | `/v1/otp/request` returns 200 | | RUNTIME PENDING | |
| SMK-07 | OTP skipped | `-SkipSmtp` or `SKIP_SMTP_CHECK=1` | Script records SKIP, not FAIL | | RUNTIME PENDING | |
| SMK-08 | OTP verify optional | `DEMO_OTP` or `-DemoOtp` | `/v1/otp/verify` returns token | | RUNTIME PENDING | |
| SMK-09 | Admin CA token present | `ADMIN_CA_TOKEN` | Script records Admin CA token PASS | | RUNTIME PENDING | |
| SMK-10 | Admin CA list | `ADMIN_CA_TOKEN` | `/v1/admin-ca/certificates?limit=5` returns 200 | | RUNTIME PENDING | |
| SMK-11 | Admin CA detail | Cert list non-empty | Detail endpoint returns 200 | | RUNTIME PENDING | |
| SMK-12 | Admin CA invalid token | N/A | Invalid token returns 401/403 | | RUNTIME PENDING | |
| SMK-13 | Admin Bank no session negative | No bank cookie | `/v1/admin/bank/audit/query` returns 401/403 | | RUNTIME PENDING | |
| SMK-14 | SOC token/login | `ADMIN_SEC_DEMO_TOKEN` or credentials | Security-admin token available | | RUNTIME PENDING | |
| SMK-15 | KDC audit list | SOC token | `/v1/admin-kdc/audit?limit=5` returns 200 | | RUNTIME PENDING | |
| SMK-16 | SOC verify | SOC token | `/v1/admin/audit/verify` returns 200 | | RUNTIME PENDING | |
| SMK-17 | SOC summary | SOC token | `/v1/admin/audit/summary?window=24h` returns 200 | | RUNTIME PENDING | |
| SMK-18 | SOC export | SOC token | `/v1/admin/audit/export?source=all&format=json` returns 200 | | RUNTIME PENDING | |
| SMK-19 | SOC timeline empty trace | SOC token | `/v1/admin/audit/timeline?request_id=<uuid>` returns 200, count may be 0 | | RUNTIME PENDING | |
| SMK-20 | SOC no token negative | No token | `/v1/admin-kdc/audit?limit=1` returns 401/403 | | RUNTIME PENDING | |
| SMK-21 | Duplicate register note | OTP/CSR not automated | Script records SKIP with reason | | RUNTIME PENDING | |

## 3. Không thuộc smoke tự động

Các flow sau không nên ép vào smoke tự động vì cần private key/CSR/browser state:

- Full customer register từ CSR thật.
- AS/TGS/AP request có chữ ký private key.
- Transfer success/fail đầy đủ.
- Admin CA activation bằng browser.
- Admin Bank activation/session bằng browser.

Các flow này được kiểm trong `functional-testcases.md` và `security-testcases.md`.
