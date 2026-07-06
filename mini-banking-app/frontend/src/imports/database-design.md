# MINI BANKING DATABASE DESIGN

This frontend import is intentionally kept as a pointer instead of duplicating the
database blueprint.

Current source of truth:

- `../../../../blueprint/database-design.md`

Important boundaries:

- CA DB owns `ca_issuers`, `certificates`, and `certificate_audit_log`.
- CA DB stores issuer/certificate metadata, status, revocation, audit, and chain metadata.
- CA DB does not store Root CA, gRPC Transport CA, or Client CA private keys.
- Bank DB owns users, accounts, transactions, replay fallback, bank audit, and ledger state.
- Bank DB must not contain certificate tables. KDC and Bank call CA gRPC `VerifyCertificate` for certificate status, public key, and issuer/chain metadata.
