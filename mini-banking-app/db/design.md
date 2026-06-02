# Database Design Note

This folder now follows the blueprint split:

- CA schema: `ca/migrations/001_init_ca.sql`
- Bank schema: `bank/migrations/001_init_bank.sql`

The current source of truth is `../../blueprint/database-design.md`.

Important boundaries:

- CA DB owns certificate metadata and certificate audit log.
- Bank DB owns users, accounts, transactions, replay fallback, bank audit log, and ledger state.
- Bank DB must not contain certificate tables. Services use CA gRPC `VerifyCertificate` for certificate status/public key lookup.
