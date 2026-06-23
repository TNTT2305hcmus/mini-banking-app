# KDC flow helper (otp → register → as-req → tgs-req)

Generates the client-side crypto and request bodies for the full mini-banking
auth flow and drives it against the api-gateway. Use it to test end-to-end, or
to produce bodies you fire from Postman.

## Why a script (and not pure Postman)

The flow needs real client crypto that the Postman sandbox cannot do natively:

| Step      | Crypto the client must do                                              |
| --------- | ---------------------------------------------------------------------- |
| register  | RSA-2048 keypair + **PKCS#10 CSR** (CommonName = `full_name`, email SAN) |
| as-req    | **RSA sign** the canonical `{cert_sn, owner_id, nonce, timestamp}`       |
| as-rep    | **RSA-OAEP** unwrap an AES key, **AES-256-GCM** decrypt the payload      |
| tgs-req   | **AES-256-GCM** encrypt the authenticator with `K_c_tgs`                |

This script does all of that with Node's `crypto` (+ `node-forge` for the CSR),
so the bodies it emits drop straight into Postman.

## Prerequisites

- Node.js **18+** (uses global `fetch`).
- The stack running (CA + KDC + gateway + Redis). From `mini-banking-app/`:
  `docker compose up` — the gateway listens on **:3000** by default.
- Access to the OTP email (the gateway emails a 6-digit code). Point `EMAIL` at a
  mailbox you can read, or your local mail catcher.

```bash
cd mini-banking-app/scripts/kdc-flow
npm install        # installs node-forge
npm run selftest   # offline crypto check, no server needed
```

## Config (env vars, all optional)

| Var         | Default                     | Notes                                        |
| ----------- | --------------------------- | -------------------------------------------- |
| `API_BASE`  | `http://localhost:3000`     | gateway base URL                             |
| `EMAIL`     | `kdc.tester@example.com`    | registration email (OTP is sent here)        |
| `FULL_NAME` | `KDC Flow Tester`           | CSR CommonName **and** register `fullName`   |
| `SCOPE`     | `transfer:internal`         | or `account:read`                            |
| `KEY_FILE`  | `./out/client.key`          | client RSA key (created on first run, reused)|

## Usage

### A. Full end-to-end (quickest test)

```bash
npm run flow
# or: API_BASE=http://localhost:3000 EMAIL=you@example.com node flow.mjs run
```

Fires all 5 requests, pausing once for you to paste the OTP from the email.
Every request body + response is written to `./out/`.

### B. Drive the crypto steps from Postman

Postman can't build the AS-REQ signature or read the AS_REP, so let the script do
the crypto and hand Postman the bodies:

```bash
node flow.mjs gen          # runs otp -> verify -> register live, prints AS-REQ body
```

This also writes `out/postman_environment.generated.json`. Then:

1. In Postman, **Import** `postman/kdc-flow.postman_collection.json` and
   `out/postman_environment.generated.json`; select that environment.
2. Run **AS-REQ** (its body is `{{as_req_body}}`). Copy the JSON response.
3. Turn the AS_REP into the TGS-REQ body:
   ```bash
   node flow.mjs tgs out/04_as_req.response.json
   # or paste the Postman response into a file and pass its path
   ```
   Copy the printed body into the **TGS-REQ** request (or re-import the updated
   environment so `{{tgs_req_body}}` is filled), then run **TGS-REQ**.

> The OTP / register requests in the collection are fully reusable. The **AS-REQ
> and TGS-REQ bodies are single-use** (fresh nonce + timestamp, and the KDC
> rejects replays), so regenerate them with the script for each attempt.

## Outputs (`./out/`)

- `0X_*.body.json` / `0X_*.response.json` — each request and response.
- `state.json` — owner_id / cert_sn / scope carried between `gen` and `tgs`.
- `postman_environment.generated.json` — import into Postman.
- `client.key` — the reused client RSA private key.

## How identity binding works (so the bodies pass)

- `owner_id` (a UUID) is minted by the gateway at **OTP verify** and is the
  authoritative identity. The KDC binds `owner_id == cert.OwnerID`.
- The CA sets the certificate `SubjectCN = full_name` (a display name) — it is
  **not** an identity key, so `FULL_NAME` can be any human name.
- The CSR's CommonName must equal `full_name`, and its email SAN must equal
  `EMAIL` (enforced by the CA). The script handles both.
