# Mini Banking App

Mini Banking App is an applied cryptography project that demonstrates a secure
digital banking flow built around OTP enrollment, PKI/X.509 identity,
Kerberos-like service tickets, signed transactions, replay protection, and an
append-only ledger.

The system is designed as a sandbox/demo banking platform. It is not connected
to real banking rails, does not process real money, and intentionally keeps
production infrastructure concerns such as HSM/KMS, multi-region deployment, and
full regulatory compliance out of scope.

## What This Project Demonstrates

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
- Layered PKI design: a Root CA signs Intermediate CAs, with a gRPC Transport
  CA for internal service TLS certificates and a Client CA for customer
  certificates.
- Secure transaction processing: Bank Service checks ticket scope, account
  ownership, certificate status, limits, and account state before writing.
- Immutable transaction history: transfer records are appended with hash
  chaining so historical tampering can be detected.
- PKI administration scope: the blueprint includes certificate search, metadata
  viewing, and revocation through an admin dashboard.

## Architecture

The application follows a layered service architecture:

```text
Browser Clients
  - Customer React app
  - Admin React dashboard

API Gateway / DMZ
  - Node.js + TypeScript
  - REST API, rate limiting, validation, audit/error envelope
  - gRPC forwarding to internal services

Internal Services
  - CA Service, Go
  - KDC Service, Go
  - Banking Service, Go

PKI Trust Layer
  - Root CA as the highest trust anchor
  - gRPC Transport CA for CA/KDC/Bank service TLS certificates
  - Client CA for user/client enrollment certificates

Data Layer
  - PostgreSQL for CA metadata
  - PostgreSQL for bank accounts, transfers, audit data, and ledger
  - Redis for OTP TTL, replay cache, rate limits, and revocation cache
```

Internal service contracts are defined with Protocol Buffers in
[`mini-banking-app/proto`](mini-banking-app/proto), with generated Go code in
[`mini-banking-app/pkg/pb`](mini-banking-app/pkg/pb). Internal communication is
gRPC over TLS inside the local/demo network boundary.

## PKI / CA Hierarchy

The target CA Service architecture uses a layered PKI hierarchy rather than a
single CA key signing every certificate:

```text
Root CA
  - highest trust anchor
  - signs Intermediate CA certificates only

gRPC Transport CA
  - Intermediate CA signed by the Root CA
  - signs internal service TLS server certificates:
    ca-server.crt, kdc-server.crt, bank-server.crt

Client CA
  - Intermediate CA signed by the Root CA
  - signs user/client certificates issued during registration

CA Repository
  - PostgreSQL or JSON store
  - stores certificate metadata, issuer, status, revocation, and audit data
```

The Admin CA dashboard is not an Intermediate CA. It is a control plane for
viewing, searching, revoking, and auditing certificates managed by the CA
Service.

## Main Security Flow

1. The customer requests and verifies an OTP.
2. The browser generates a key pair and creates a CSR.
3. The API Gateway forwards the CSR to the CA Service.
4. The CA Service issues an X.509 client certificate through the Client CA and
   stores certificate metadata.
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
|-- blueprint/                 # Proposal, technical design, specs, API design
|-- RUN_GUIDE.md               # Detailed local run guide
|-- WORKFLOW.md                # Project workflow notes
|-- PROCESS.md                 # Development/process notes
|-- mini-banking-app/
|   |-- api-gateway/           # Node.js/TypeScript REST gateway
|   |-- ca-service/            # Go X.509 certificate authority service
|   |-- kdc-service/           # Go Kerberos-like KDC service
|   |-- banking-service/       # Go banking transaction service
|   |-- frontend/              # React/Vite customer and admin UI
|   |-- proto/                 # gRPC protobuf contracts
|   |-- pkg/pb/                # Generated protobuf bindings
|   |-- db/                    # PostgreSQL schema/migrations
|   |-- scripts/               # Certificate and demo helper scripts
|   `-- docker-compose.yml     # Partial compose setup for local/demo services
```

## Tech Stack

| Layer | Technology |
|---|---|
| Frontend | React, TypeScript, Vite, WebCrypto API, IndexedDB |
| API Gateway | Node.js, TypeScript, Express, gRPC client, Redis, BullMQ |
| CA Service | Go, gRPC, X.509, PostgreSQL or JSON store |
| KDC Service | Go, gRPC, Redis, AES/RSA-based ticket flow |
| Banking Service | Go, gRPC, PostgreSQL, Redis, hash-chained ledger |
| Infrastructure | Docker, PostgreSQL, Redis, OpenSSL |

## Prerequisites

Install the following tools:

- Go 1.25.x
- Node.js 22.x LTS or 24.x
- npm
- Docker Desktop
- OpenSSL 1.1.1+ or 3.x
- PostgreSQL client tools, especially `psql`

Check your environment:

```powershell
go version
node -v
npm -v
docker version
openssl version
psql --version
```

## Configuration

Most commands below assume you are inside the application folder:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app"
```

For Docker Compose runs, copy the shared template:

```powershell
Copy-Item .env.example .env
```

Update at least these values before a real demo:

```text
SMTP_USER=<email account used for OTP>
SMTP_PASS=<email app password>
JWT_SECRET=<long random secret>
GATEWAY_OTP_SECRET=<long random secret>
ROOT_CA_KEY_PASSWORD=<local CA key password>
CA_DATABASE_URL=<CA database connection string>
```

For local terminal runs, each service can also use its own `.env` file inside
`ca-service`, `kdc-service`, `banking-service`, and `api-gateway`. See
[`RUN_GUIDE.md`](RUN_GUIDE.md) for the full setup path.

## Local Setup

Start Redis and PostgreSQL:

```powershell
docker run --name mini-bank-redis -p 6379:6379 -d redis:7.4-alpine

docker run --name mini-bank-postgres `
  -e POSTGRES_DB=banking `
  -e POSTGRES_USER=banking `
  -e POSTGRES_PASSWORD=MiniBankingDev123! `
  -p 5432:5432 `
  -d postgres:17-alpine
```

Import the bank schema:

```powershell
psql "postgres://banking:MiniBankingDev123!@localhost:5432/banking?sslmode=disable" `
  -f .\db\bank\migrations\001_init_bank.sql
```

Optionally run CA PostgreSQL locally:

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

Generate local CA hierarchy material once. In the target CA architecture this
provisions the Root CA plus the gRPC Transport CA and Client CA intermediates:

```powershell
cd .\ca-service
$env:ROOT_CA_KEY_PASSWORD = "dev-root-ca-password-change-me"
go run .\scripts\provision_ca_dev.go
cd ..
```

Generate gRPC TLS certificates:

```powershell
.\scripts\gen-certs\gen-certs.ps1
```

Generate KDC demo keys:

```powershell
go run .\kdc-service\scripts\provision_kdc_dev.go
```

Install JavaScript dependencies:

```powershell
cd .\api-gateway
npm install
cd ..

cd .\frontend
npm install
cd ..
```

Go modules are downloaded automatically by `go run`. To download them ahead of
time:

```powershell
cd .\ca-service; go mod download; cd ..
cd .\kdc-service; go mod download; cd ..
cd .\banking-service; go mod download; cd ..
```

## Running the Application

Run the main services in separate terminals.

Terminal 1, CA Service:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\ca-service"
go run .\cmd\server
```

Terminal 2, KDC Service:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\kdc-service"
go run .\cmd\server
```

Terminal 3, Banking Service:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\banking-service"
go run .\cmd\server
```

Terminal 4, API Gateway:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\api-gateway"
npm run dev
```

Terminal 5, Frontend:

```powershell
cd "D:\U\Y3\S2\Applied Cryptography\mini-banking-app\mini-banking-app\frontend"
npm run dev
```

Default local URLs:

| Component | URL |
|---|---|
| API Gateway | `http://localhost:3000` |
| Frontend | `http://localhost:5173` |
| User login | `http://localhost:5173/login` |
| User registration | `http://localhost:5173/register` |
| User banking home | `http://localhost:5173/home` |
| Admin CA page | `http://localhost:5173/admin-ca` |
| Admin Bank page | `http://localhost:5173/admin-bank` |

## Current REST Surface

The API Gateway currently mounts these route groups:

| Method | Endpoint | Purpose |
|---|---|---|
| `POST` | `/v1/otp/request` | Request an OTP for registration |
| `POST` | `/v1/otp/verify` | Verify OTP and issue a registration token |
| `POST` | `/v1/auth/register` | Register CSR, issue certificate, create bank user |
| `POST` | `/v1/auth/as-req` | Request TGT from KDC |
| `POST` | `/v1/auth/tgs-req` | Request scoped service ticket |
| `POST` | `/v1/auth/me` | Resolve current user/account using a bank ticket |
| `POST` | `/v1/bank/transfer` | Submit encrypted, signed transfer request |
| `POST` | `/v1/bank/accounts/:id/balance/query` | Query account balance |
| `POST` | `/v1/bank/accounts/:id/transactions/query` | Query transaction history |

The blueprint also specifies admin certificate APIs for listing, inspecting, and
revoking certificates. The frontend contains admin pages, but the current
gateway entrypoint does not yet mount a dedicated `/v1/admin/*` route group.

## Testing and Verification

Run Go tests per service:

```powershell
cd .\ca-service; go test ./...; cd ..
cd .\kdc-service; go test ./...; cd ..
cd .\banking-service; go test ./...; cd ..
```

Build the frontend:

```powershell
cd .\frontend
npm run build
cd ..
```

Check local infrastructure:

```powershell
docker exec mini-bank-redis redis-cli ping
netstat -ano | findstr ":3000 :50051 :50052 :50053 :6379 :5432"
```

## Documentation Map

- [`blueprint/proposal.md`](blueprint/proposal.md): problem statement, goals,
  users, scope, and risks.
- [`blueprint/design.md`](blueprint/design.md): technical design, key model,
  cryptographic choices, C4 diagrams, and authorization pipelines.
- [`blueprint/database-design.md`](blueprint/database-design.md): logical data
  model, constraints, indexes, and Redis keys.
- [`blueprint/specs`](blueprint/specs): per-flow business and security specs.
- [`blueprint/api-design`](blueprint/api-design): public REST API design.
- [`RUN_GUIDE.md`](RUN_GUIDE.md): detailed local setup and run instructions.

## Scope and Limitations

- This is a local/demo project for applied cryptography coursework.
- Secrets and demo passwords in guides are development placeholders.
- Private CA/KDC/Bank keys are provisioned locally for demo purposes.
- Docker Compose currently covers a partial service setup; the most reliable
  local workflow is the terminal-based flow in `RUN_GUIDE.md`.
- The project does not provide production HSM/KMS integration, real payment
  settlement, AML/fraud detection, or legal banking compliance.
