# Mini Banking App

Mini Banking App is an applied cryptography demo for a secure banking workflow:
OTP enrollment, PKI/X.509 identity, Kerberos-like AS/TGS/AP tickets, signed
banking requests, replay protection, certificate administration and SOC audit
monitoring.

This is a coursework/demo system. It does not process real money and does not
claim production-grade banking compliance.

## What It Demonstrates

- OTP + PKI registration with browser-generated key pairs.
- Root CA, Client CA and gRPC Transport CA separation.
- Certificate roles: `customer`, `bank_admin`, `ca_admin`.
- Kerberos-like AS/TGS/AP flow with scoped service tickets.
- Signed banking requests and replay/idempotency protection.
- Admin CA certificate management and revocation.
- Admin Bank dashboard and audit view.
- Admin SOC views for KDC audit, cross-service timeline, hash-chain verify,
  summary and export.
- CA/KDC/Bank audit logs with tamper-evident hash-chain design.

## Repository Layout

```text
.
|-- README.md
|-- blueprint/                 # Proposal, design, specs and API design
|-- demo_test_guide/
|   |-- guide/                 # Final run/config/compose guides
|   |-- tests/                 # Final testcase and runtime result templates
|   `-- demo/                  # Functional and security video scripts
|-- mini-banking-app/
|   |-- api-gateway/           # Node.js/TypeScript REST gateway
|   |-- ca-service/            # Go X.509 CA service
|   |-- kdc-service/           # Go Kerberos-like KDC service
|   |-- banking-service/       # Go banking service
|   |-- frontend/              # React/Vite UI
|   |-- proto/                 # gRPC protobuf contracts
|   |-- pkg/pb/                # Generated protobuf bindings
|   |-- db/                    # PostgreSQL migrations and seed data
|   |-- scripts/               # Cert and demo helper scripts
|   |-- .env.demo.example      # Final shared env template
|   |-- docker-compose.local.yml
|   `-- docker-compose.demo.yml
```

## Prerequisites

- Go 1.25.x
- Node.js 22.x LTS or newer
- npm
- Docker Desktop
- OpenSSL 1.1.1+ or 3.x

Check:

```powershell
go version
node -v
npm.cmd -v
docker version
openssl version
```

## Quick Start: Docker Compose Local

From the runtime root:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
Copy-Item .\.env.demo.example .\.env
```

Edit `.env` and replace required secrets such as:

- `ROOT_CA_KEY_PASSWORD`
- `CA_DATABASE_URL`
- `BANK_DB_PASSWORD`
- `BANK_KEY`
- `JWT_SECRET`
- `GATEWAY_OTP_SECRET`
- `SMTP_USER`
- `SMTP_PASS`
- `ADMIN_SEC_DEMO_PASSWORD`
- `ADMIN_SEC_DEMO_TOKEN`

Provision local cert/key material:

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

.\scripts\gen-certs\gen-certs.ps1
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Start the local stack:

```powershell
docker compose -f docker-compose.local.yml up --build -d
docker compose -f docker-compose.local.yml ps
```

Open:

```text
Frontend     http://localhost:5173
API Gateway  http://localhost:3000
```

## Quick Start: Demo Compose

The demo compose file is production-like and does not run the Vite frontend dev
server.

```powershell
docker compose -f docker-compose.demo.yml up --build -d
```

Use it when the frontend is built/served separately.

## Smoke Test

PowerShell:

```powershell
.\scripts\demo\smoke-test.ps1
```

Skip SMTP during rehearsal:

```powershell
.\scripts\demo\smoke-test.ps1 -SkipSmtp
```

With admin tokens:

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

## Demo Routes

Frontend:

- Customer register: `http://localhost:5173/register`
- Customer login: `http://localhost:5173/login`
- Customer home: `http://localhost:5173/home`
- Admin CA: `http://localhost:5173/admin-ca`
- Admin CA activate: `http://localhost:5173/admin-ca/activate`
- Admin Bank: `http://localhost:5173/admin-bank`
- Admin SOC: `http://localhost:5173/admin-soc`

Key API groups:

- User OTP/auth: `/v1/otp/*`, `/v1/auth/*`
- Bank: `/v1/bank/*`
- Admin CA: `/v1/admin-ca/*`
- Admin Bank: `/v1/admin/bank/*`
- SOC auth: `/v1/admin-sec/auth`
- KDC audit: `/v1/admin-kdc/audit`
- SOC cross-service audit: `/v1/admin/audit/*`

## Final Documentation

Run/config guides:

- [Run Guide](demo_test_guide/guide/RUN_GUIDE.md)
- [Environment Guide](demo_test_guide/guide/ENV_GUIDE.md)
- [Docker Compose Guide](demo_test_guide/guide/COMPOSE_GUIDE.md)
- [Terminal Guide](demo_test_guide/guide/TERMINAL_GUIDE.md)
- [Seed And Accounts](demo_test_guide/guide/SEED_AND_ACCOUNTS.md)
- [Troubleshooting](demo_test_guide/guide/TROUBLESHOOTING.md)

Testcases:

- [Testcase Index](demo_test_guide/tests/testcases.md)
- [Functional Testcases](demo_test_guide/tests/functional-testcases.md)
- [Security Testcases](demo_test_guide/tests/security-testcases.md)
- [Smoke Testcases](demo_test_guide/tests/smoke-testcases.md)
- [Audit Testcases](demo_test_guide/tests/audit-testcases.md)
- [Runtime Results](demo_test_guide/tests/runtime-results.md)

Video scripts:

- [Demo Prep](demo_test_guide/demo/00-demo-prep.md)
- [Functional Demo Script](demo_test_guide/demo/01-functional-demo-script.md)
- [Non-Functional/Security Demo Script](demo_test_guide/demo/02-non-functional-security-demo-script.md)

## Known Limitations

- Hash-chain audit does not yet have an automatic external anchor.
- Tail truncation is not detected without an external checkpoint.
- Timestamp/metadata are not part of the core hash-chain fields.
- Audit insert is best-effort and should not fail primary business requests.
- `RATE_LIMIT_DISABLED=1` is for rehearsal/demo stability, not production mode.
- Admin CA cert login is verified directly at the Gateway; it does not go
  through AS/TGS/AP like Bank Admin.

## Notes For Maintainers

- Root `mini-banking-app/.env.example` and root `mini-banking-app/docker-compose.yml`
  are no longer part of the final run path.
- Use `.env.demo.example`, `.env`, `docker-compose.local.yml` and
  `docker-compose.demo.yml`.
- `temp-docs` is for internal handoff/planning only and is not final
  documentation.
