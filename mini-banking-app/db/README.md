# Database Layout

Database schema is split by service ownership.

```text
db/
├── ca/
│   └── migrations/
│       └── 001_init_ca.sql
├── bank/
│   └── migrations/
│       └── 001_init_bank.sql
└── design.md
```

Rules:

- CA DB owns certificate lifecycle and issuer metadata: `ca_issuers`, `certificates`, and `certificate_audit_log`.
- Bank DB owns users, accounts, transactions, replay fallback, bank audit, and ledger state.
- Bank DB must not contain a certificate table. KDC and Bank call CA gRPC `VerifyCertificate`.
- `design.md` in this folder is legacy implementation context. The current source of truth is `../../blueprint/database-design.md`.
