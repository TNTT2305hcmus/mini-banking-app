# Mini Banking App

Mini Banking App is an applied cryptography project that demonstrates a secure digital banking flow built around OTP enrollment, PKI/X.509 identity, Kerberos-like service tickets, signed transactions, replay protection, and an append-only ledger.

The system is designed as a sandbox/demo banking platform. It is not connected to real banking rails, does not process real money, and intentionally keeps production infrastructure concerns such as HSM/KMS, multi-region deployment, and full regulatory compliance out of scope.

## What It Demonstrates

- Multi-step authentication: OTP -> PKI/X.509 -> AS Exchange -> TGS Exchange.
- Zero-knowledge client key handling: the user's private key is generated and
  used in the browser with WebCrypto and is never sent to the server in
  plaintext.
- Kerberos-like short-lived tickets: TGT and service tickets carry explicit
  scope and session-key material.
- Replay protection: important requests use nonce, timestamp, request id, and
  Redis-backed replay checks.
- Non-repudiation: banking operations are digitally signed by the client private
  key and verified against the public key bound in the user's certificate.
- Secure transaction processing: Bank Service checks ticket scope, account
  ownership, certificate status, limits, and account state before writing.
- Immutable transaction history: transfer records are appended with hash
  chaining so historical tampering can be detected.
- PKI administration scope: the blueprint includes certificate search, metadata
  viewing, and revocation through an admin dashboard.

## Main Security Flow

1. The customer requests and verifies an OTP.
2. The browser generates a key pair and creates a CSR.
3. The API Gateway forwards the CSR to the CA Service.
4. The CA Service issues an X.509 certificate and stores certificate metadata.
5. The Gateway asks the Banking Service to create the corresponding bank user.
6. The browser signs an AS request with the client private key.
7. The KDC verifies the certificate and signature, then issues a TGT and
   `K_c_tgs`.
8. The browser uses the TGT to request a scoped `Ticket_v` and `K_c_v`.
9. For banking operations, the browser signs the canonical payload, encrypts it
   with `K_c_v`, and sends the encrypted request plus `Ticket_v`.
10. The Banking Service verifies the ticket, authenticator, replay state,
    certificate status, digital signature, authorization rules, and then writes
    the transaction and ledger entry.


## Repository Layout

```text
.
|-- README.md
|-- blueprint/                 # Proposal, design, specs and API design
|-- demo_test_guide/
|   |-- guide/                 # Run guide and seed/account reference
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
---
## Set up this project

### Prerequisites

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

### Docker Compose Local

From a clean clone, go to the runtime root:

```text
git clone https://github.com/TNTT2305hcmus/mini-banking-app.git
cd mini-banking-app\mini-banking-app
```

Create the local demo environment file:

```powershell
Copy-Item .\.env.demo.example .\.env
```

The template is already filled with local demo values for Postgres, Redis,
Gateway, CA, KDC, Bank and SOC. It is enough for a local demo without real
email delivery.

**Optional:** replace these only when you want to test real OTP/admin activation
email delivery:

```env
SMTP_USER=<your-gmail-address>
SMTP_PASS=<your-gmail-app-password>
```

Validate Compose interpolation before generating certificates:

```powershell
docker compose -f docker-compose.local.yml config --quiet
```

Provision local cert/key material:

```powershell
cd .\ca-service
go run .\scripts\provision_ca_dev.go
cd ..

go run .\kdc-service\scripts\provision_kdc_dev.go
powershell -ExecutionPolicy Bypass -File .\scripts\gen-certs\gen-certs.ps1
```

**Important:** `ROOT_CA_KEY_PASSWORD` in `.env` is used to encrypt and later
decrypt `ca-service/certs/root-ca/ca.key`. If you change this password after
provisioning certs, delete generated cert/key folders and run the provision
commands again.

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

### Reset And Rerun From Scratch

Use this when you want to verify the README flow from a clean local state, or
when `.env` values such as `ROOT_CA_KEY_PASSWORD` changed after certs were
generated.

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"

docker compose -f docker-compose.local.yml down -v --remove-orphans

Remove-Item -Recurse -Force .\ca-service\certs -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force .\kdc-service\certs -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force .\banking-service\certs -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force .\api-gateway\certs -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force .\frontend\public\trust -ErrorAction SilentlyContinue

Copy-Item .\.env.demo.example .\.env -Force
```

Then run the Quick Start commands again.

---

## Demo Accounts And Login

The app uses browser-held private keys and X.509 certificates, so customer and
admin users are not password-seeded like a normal CRUD app. After Docker is up,
use the identities below to create local browser credentials.

### Admin CA Activation Link

Run this after `docker compose up`:

```powershell
docker compose -f docker-compose.local.yml exec api-gateway `
  node dist/scripts/provision-ca-admin.js `
  --email "ca.admin@demo.minibanking.local" `
  --full-name "CA Administrator" `
  --print-only
```

Copy the printed `activation_url`, open it in the browser, confirm the same
email/full name, set PIN `123456`, then login at:

```text
http://localhost:5173/admin-ca
```

### Bank Admin Activation Link

Run this after `docker compose up`:

```powershell
docker compose -f docker-compose.local.yml exec api-gateway `
  node dist/scripts/provision-bank-admin.js `
  --email "bank.admin@demo.minibanking.local" `
  --full-name "Bank Administrator" `
  --print-only
```

Copy the printed `activation_url`, open it in the browser, confirm the same
email/full name, set PIN `123456`, then login at:

```text
http://localhost:5173/admin-bank
```

### Security Admin

The credential is set up in `.env`:

```text
ADMIN_SEC_DEMO_EMAIL=security@minibanking.local
ADMIN_SEC_DEMO_PASSWORD=dev-security-admin-password-change-me
```

Go to `http://localhost:5173/admin-soc`

### Customer Registration With Demo Or Real OTP

Customer login requires a customer certificate/private key in the browser.
Register once at:

```text
http://localhost:5173/register
```

Use the demo customer email:

```text
customer.demo@demo.minibanking.local
```

For the local demo, `.env.demo.example` sets:

```text
DEMO_OTP=123456
```

So after entering `customer.demo@demo.minibanking.local`, type OTP `123456`,
continue registration, and set PIN `123456`. This keeps the normal browser
PKI enrollment flow but avoids needing a real mailbox during grading.

If SMTP is configured with real `SMTP_USER`/`SMTP_PASS` and `DEMO_OTP` is
removed or left empty, the OTP is generated randomly and sent by email. Use
that only when you want to test the real OTP email path.

Seeded receiver accounts for transfer testing are already loaded into Bank DB.
For example, transfer to account number: `110000000001`

### Account Demo Table

| Role | Demo email | Route | How to enter |
|---|---|---|---|
Customer | `customer.demo@demo.minibanking.local` | `http://localhost:5173/register` then `/login` | Register once with demo OTP `123456`, set PIN `123456`, then login on the same browser profile. |
| CA Admin | `ca.admin@demo.minibanking.local` | `/admin-ca/activate` then `/admin-ca` | Generate an activation URL with the command below, activate, set PIN, then login. |
| Bank Admin | `bank.admin@demo.minibanking.local` | `/admin-bank/activate` then `/admin-bank` | Generate an activation URL with the command below, activate, set PIN, then login. |
| SOC Admin | `security@minibanking.local` | `http://localhost:5173/admin-soc` | Login with password `dev-security-admin-password-change-me` from `.env`. |

---

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

Detail Run/config:

- [RUN_GUIDE.md](demo_test_guide/guide/RUN_GUIDE.md)
- [Seed And Accounts](demo_test_guide/guide/SEED_AND_ACCOUNTS.md)

Testcases:

- [Testcase Index](demo_test_guide/tests/testcases.md)
- [Functional Testcases](demo_test_guide/tests/functional-testcases.md)
- [Security Testcases](demo_test_guide/tests/security-testcases.md)
- [Smoke Testcases](demo_test_guide/tests/smoke-testcases.md)
- [Audit Testcases](demo_test_guide/tests/audit-testcases.md)
- [Runtime Results](demo_test_guide/tests/runtime-results.md)

Video scripts:

- [Demo Preperation](demo_test_guide/demo/00-demo-prep.md)
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
