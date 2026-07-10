# Mini-Banking-App - Structure Proposal

## Cấu trúc tài liệu 

```text
blueprint/
├── proposal.md
├── design.md
├── database-design.md
├── structure.md
├── specs/
│   ├── 01-otp-pki-registration.md
│   ├── 02-as-exchange.md
│   ├── 03-tgs-exchange.md
│   ├── 04-bank-transfer.md
│   ├── 05-bank-balance-history.md
│   └── 06-admin-certificate-management.md
└── api-design/
    ├── base-api.md
    ├── 01-otp-pki-registration.md
    ├── 02-as-exchange.md
    ├── 03-tgs-exchange.md
    ├── 04-bank-transfer.md
    ├── 05-bank-balance-history.md
    └── 06-admin-certificate-management.md
mini-banking-app/
├── api-gateway/
│   └── src/
│       ├── config/
│       ├── middleware/
│       │   ├── request-id.ts
│       │   ├── rate-limit.ts
│       │   ├── admin-auth.ts
│       │   └── error-envelope.ts
│       ├── routes/
│       │   ├── otp.routes.ts
│       │   ├── pki.routes.ts
│       │   ├── auth.routes.ts
│       │   ├── bank.routes.ts
│       │   └── admin.routes.ts
│       ├── grpc-clients/
│       │   ├── ca.client.ts
│       │   ├── kdc.client.ts
│       │   └── bank.client.ts
│       ├── services/
│       │   ├── otp.service.ts
│       │   ├── registration-token.service.ts
│       │   └── admin-token.service.ts
│       └── validators/
├── ca-service/
│   └── internal/
│       ├── ca/
│       │   ├── service.go
│       │   ├── rootca.go
│       │   ├── store.go
│       │   └── audit.go
│       ├── grpc/
│       └── config/
├── kdc-service/
│   └── internal/
│       ├── kdc/
│       │   ├── as_service.go
│       │   ├── tgs_service.go
│       │   ├── ticket.go
│       │   ├── replay.go
│       │   └── crypto.go
│       ├── grpc/
│       └── config/
├── banking-service/
│   └── internal/
│       ├── bank/
│       │   ├── service.go
│       │   ├── auth_pipeline.go
│       │   ├── transfer.go
│       │   ├── balance.go
│       │   ├── history.go
│       │   ├── ledger.go
│       │   ├── idempotency.go
│       │   └── replay.go
│       ├── grpc/
│       ├── store/
│       └── config/
├── proto/
│   ├── ca.proto
│   ├── kdc.proto
│   └── bank.proto
├── pkg/pb/
└── db/
    ├── ca/
    │   └── migrations/
    └── bank/
        └── migrations/
```

Quy ước:

- `specs/` mô tả nghiệp vụ và flow, không lặp quá sâu request/response JSON.
- `api-design/` mô tả REST contract public, error code và payload shape.
- `database-design.md` mô tả schema logical, index, constraint và Redis key.
- `structure.md` mô tả mapping triển khai, thứ tự ưu tiên, service boundary và điểm cần chuẩn hóa.

Ghi chú:

- Gateway chỉ nên giữ logic HTTP, token ngắn hạn, rate limit và orchestration. Không ghi CA DB hoặc Bank DB trực tiếp.
- CA Service sở hữu toàn bộ certificate lifecycle.
- KDC Service sở hữu TGT, `Ticket_v`, session key và replay cho AS/TGS.
- Bank Service sở hữu tài khoản, giao dịch, idempotency, ledger và authorization nghiệp vụ.
- `pkg/pb/` là generated code; không sửa tay.

---

## REST -> gRPC -> Data mapping 

| REST endpoint | Gateway xử lý | gRPC nội bộ | Data owner |
|---|---|---|---|
| `POST /v1/otp/request` | Validate email, rate limit, sinh OTP, gửi email | Không bắt buộc | Redis |
| `POST /v1/otp/verify` | Verify OTP, cấp registration token | Không bắt buộc | Redis |
| `POST /v1/pki/register` | Verify registration token, forward CSR | `CA.RegisterUser`; sau đó `Bank.CreateUser` nếu enrollment thành công | CA DB, Bank DB |
| `POST /v1/auth/as-req` | Validate JSON, forward request | `KDC.RequestTGT`; KDC gọi `CA.VerifyCertificate` | CA DB, Redis |
| `POST /v1/auth/tgs-req` | Validate JSON, forward request | `KDC.RequestServiceTicket` | Redis |
| `POST /v1/bank/transfer` | Validate envelope, forward opaque crypto payload | `Bank.TransferMoney`; Bank gọi CA verify cert | Bank DB, CA DB, Redis |
| `POST /v1/bank/accounts/{id}/balance/query` | Validate account id, forward ticket/authenticator | `Bank.GetBalance`; Bank gọi CA `VerifyCertificate` | Bank DB, CA DB, Redis |
| `POST /v1/bank/accounts/{id}/transactions/query` | Validate pagination, forward ticket/authenticator | `Bank.GetHistory`; Bank gọi CA `VerifyCertificate` | Bank DB, CA DB, Redis |
| `POST /v1/admin-ca/session` | Verify CA Admin cert proof, cấp session | `CA.VerifyCertificate` | CA DB |
| `GET /v1/admin-ca/certificates` | Verify Admin CA session | `CA.ListCertificates` | CA DB |
| `GET /v1/admin-ca/certificates/{serial}` | Verify Admin CA session | `CA.GetCertificateDetail` | CA DB |
| `POST /v1/admin-ca/certificates/{serial}/revoke` | Verify Admin CA session, validate reason | `CA.RevokeCertificate` | CA DB, Redis |
