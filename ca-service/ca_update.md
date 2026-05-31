# CA Service Update

## Fixed Issue 1: Root CA Key Lifecycle

Severity: High

Problem:

- `LoadOrCreate` previously generated a new Root CA automatically when the key or certificate file was missing.
- This is unsafe for a CA because silently changing the trust root can invalidate previously issued certificates and hide deployment/configuration mistakes.
- The generated Root CA private key was written as plaintext PEM on disk.

Solution:

- Changed Root CA loading to fail closed:
  - If both key and certificate exist, load them.
  - If either file is missing, return an error and stop startup.
- Enforced encrypted Root CA private key loading:
  - Plaintext private key PEM is rejected.
  - `ROOT_CA_KEY_PASSWORD` is required to decrypt the key.
- Removed the runtime Root CA generation helper. Root CA provisioning is now an external setup step, not service startup behavior.
- Avoided deprecated RFC 1423 PEM encryption APIs. The key envelope now uses PBKDF2-HMAC-SHA256 plus AES-256-GCM with authenticated ciphertext.
- Updated CA service tests to use an in-memory test Root CA instead of relying on `LoadOrCreate`.

Modified locations:

- `ca-service/internal/ca/rootca.go`
  - `LoadOrCreate`
  - `loadFromDisk`
  - removed `generateAndSave`
  - new helper `decryptPrivateKeyPEM`
- `ca-service/internal/ca/service_test.go`
  - `newTestRootCA`

Operational note:

- CA startup now requires a pre-provisioned Root CA key/certificate pair.
- The Root CA private key must be encrypted and `ROOT_CA_KEY_PASSWORD` must be set before starting the service.

## Fixed Issue 2: Loaded Root CA Validation

Severity: High

Problem:

- `loadFromDisk` parsed the Root CA private key and certificate but did not fully validate that the loaded material was safe and consistent.
- Missing checks included private key/certificate public key match, CA flag, certificate-signing key usage, validity window, and self-signed signature validity.

Solution:

- Added loaded Root CA validation after parsing and before returning the `RootCA`.
- The loader now rejects:
  - RSA private keys that fail internal validation.
  - Certificates whose public key does not match the private key.
  - Certificates that are not valid CA certificates.
  - Certificates missing `KeyUsageCertSign`.
  - Certificates outside their validity window.
  - Certificates whose self-signature cannot be verified.
- Added focused tests for valid loading and each high-risk rejection path.

Modified locations:

- `ca-service/internal/ca/rootca.go`
  - new helper `validateLoadedRootCA`
- `ca-service/internal/ca/rootca_test.go`
  - new tests for loaded Root CA validation

## Fixed Issue 3: CA gRPC mTLS and Revoke Authorization

Severity: Critical

Problem:

- The CA gRPC server used plaintext `grpc.NewServer()` without transport credentials.
- Any reachable client could call `RevokeCertificate` with only a certificate serial number.
- `RevokeCertificate` is an administrative action and must not be exposed to every internal caller.

Solution:

- CA gRPC server now requires mTLS for every connection:
  - Server certificate/key are loaded from `GRPC_SERVER_CERT_PATH` and `GRPC_SERVER_KEY_PATH`.
  - Client certificates are verified against `GRPC_CLIENT_CA_CERT_PATH`.
  - Plaintext clients are rejected at the transport layer.
- Added a unary interceptor for `RevokeCertificate`:
  - Extracts the verified client certificate Common Name from the mTLS peer.
  - Allows revoke only when the Common Name is listed in `REVOKE_ALLOWED_CLIENT_CNS`.
  - Returns `Unauthenticated` if no verified client certificate is present.
  - Returns `PermissionDenied` if the client certificate is valid but not authorized for revoke.
- Updated the KDC CA client to use TLS client credentials instead of `insecure.NewCredentials()`.
- Added tests for revoke authorization allow/deny paths and missing client certificate.

Modified locations:

- `ca-service/internal/grpc/server.go`
  - new `SecurityConfig`
  - new mTLS credential setup
  - new revoke authorization interceptor
- `ca-service/internal/config/config.go`
  - new gRPC mTLS and revoke authz config fields
- `ca-service/cmd/server/main.go`
  - wires security config into the CA gRPC server
- `ca-service/internal/grpc/server_test.go`
  - new tests for revoke authorization
- `kdc-service/cmd/server/main.go`
  - CA client now uses TLS client credentials

Operational note:

- Provision a CA gRPC server certificate/key and a client CA certificate before startup.
- Issue client certificates for internal callers such as KDC and API Gateway.
- Include only administrative callers, for example `api-gateway`, in `REVOKE_ALLOWED_CLIENT_CNS`.

## Fixed Issue 4: Durable Revocation State and Store Race Risk

Severity: Critical / High

Problem:

- Issued certificates and revocation state were kept only in an in-memory map.
- Restarting CA Service lost all issued certificate records and revoked certificates could no longer be answered correctly.
- `Store.Get` returned the internal `*CertRecord` after releasing the lock, so callers could read or mutate the same object while `Revoke` updated it.

Solution:

- Added `NewPersistentStore`, backed by a JSON state file configured by `CA_STORE_STATE_PATH`.
- CA startup now loads persisted certificate records and revocation fields before serving requests.
- `Save` and `Revoke` persist state durably and return errors if persistence fails.
- `Store.Get` now returns a defensive copy of `CertRecord`, including a copied `RevokedAt` pointer.
- Added store tests for restart persistence and pointer/race safety.

Modified locations:

- `ca-service/internal/ca/store.go`
  - new persistent JSON load/save path
  - defensive copies for `Save` and `Get`
  - persistence-aware `Revoke`
- `ca-service/internal/ca/service.go`
  - handles persistence errors from issued certificate save and revoke
- `ca-service/cmd/server/main.go`
  - uses `ca.NewPersistentStore`
- `ca-service/internal/config/config.go`
  - new `CA_STORE_STATE_PATH` config
- `ca-service/internal/ca/store_test.go`
  - new persistence and defensive-copy tests
- `.env.example`
  - documents `CA_STORE_STATE_PATH`
- `ca-service/Dockerfile`
  - creates `/certs/ca-store`

Verification:

```powershell
cd ca-service
$env:GOCACHE = (Join-Path (Get-Location) '.gocache')
go test ./internal/...
go test -race ./internal/...
go test ./...
```

Result: passed.

## Fixed Issue 7: Go Test Folder Layout Cleanup

Severity: Maintenance

Problem:

- CA service tests were placed under `ca-service/internal/tests`, separate from the package they exercise.
- `service_test_design.md` was stale documentation living inside the test folder.
- The extra `internal/tests` package made `go test ./internal/...` less idiomatic for Go package-local tests.

Solution:

- Moved `service_test.go` into `ca-service/internal/ca/service_test.go`.
- Kept it as `package ca_test`, so it still tests the public CA API from an external-package perspective.
- Deleted `ca-service/internal/tests/service_test_design.md`.
- Removed the now-empty `ca-service/internal/tests` folder.

Verification:

```powershell
cd ca-service
$env:GOCACHE = (Join-Path (Get-Location) '.gocache')
go test ./internal/...
go test ./...
```

Result: passed.

## Fixed Issue 5: Certificate Extensions for Issued User Certificates

Severity: Medium / High

Problem:

- `computeSKI` always returned `nil`, so issued certificates lacked a real Subject Key Identifier.
- Issued certificates placed identity only in `Subject.CommonName`.
- Certificates did not include Authority Key Identifier, email/URI SAN, or revocation publication metadata.

Project-scoped solution:

- Implemented `computeSKI` with SHA-1 over the SubjectPublicKey BIT STRING, matching RFC 5280-style SKI generation.
- Added email SAN and URI SAN for the project user ID:
  - email SAN: the plain email `userID`
  - URI SAN: `urn:mini-banking:user:<escaped-userID>`
- Added `AuthorityKeyId` from the Root CA Subject Key Identifier, or computed from the Root CA public key when the loaded Root CA cert lacks SKI.
- Added optional certificate endpoint metadata:
  - `CA_CRL_DISTRIBUTION_POINTS`
  - `CA_OCSP_SERVERS`
- Kept revocation enforcement within the project architecture: KDC/Bank still call CA gRPC `CheckRevocation`, backed by the persistent store. The CRL/OCSP fields are embedded metadata for standards compatibility and future publication, not a newly introduced CRL/OCSP service.

Modified locations:

- `ca-service/internal/ca/service.go`
  - fixed `computeSKI`
  - added SAN, AKI, CRL Distribution Points, and OCSP Server fields to issued certs
  - added `CertificateExtensionConfig` and `NewServiceWithExtensionConfig`
- `ca-service/internal/config/config.go`
  - added CRL/OCSP endpoint environment config
- `ca-service/cmd/server/main.go`
  - wires extension config into CA service
- `ca-service/internal/ca/service_test.go`
  - added assertions for SAN, SKI, AKI, CRL, and OCSP extensions
- `.env.example`
  - documents the new CA extension endpoint variables

Verification:

```powershell
cd ca-service
$env:GOCACHE = (Join-Path (Get-Location) '.gocache')
go test ./internal/...
go test -race ./internal/...
go test ./...
```

Result: passed.

## Fixed Issue 6: Issuance Policy Binding and Duplicate Active Certificates

Severity: Medium

Problem:

- CA accepted `user_id` from the Gateway request and wrote it directly into the issued certificate.
- The CA proto contract does not carry the registration JWT/claims, so CA cannot independently verify the OTP/JWT token without changing the documented Gateway -> CA flow.
- CSR identity was not bound to the Gateway-verified `user_id`.
- The store did not prevent issuing multiple active certificates for the same user.

Project-scoped solution:

- Kept the documented design flow unchanged:
  - API Gateway verifies OTP/JWT and consumes the single-use registration token.
  - Gateway calls CA with `user_id = JWT.sub`.
  - CA treats Gateway as the authenticated policy enforcement caller over mTLS.
- Added CA-side issuance guardrails that fit the existing proto:
  - CSR `Subject.CommonName` must exactly match `user_id`.
  - If CSR email SANs are present, every email SAN must match `user_id`.
  - If CSR URI SANs are present, every URI SAN must match the project URI identity.
- Added atomic duplicate active certificate prevention in `Store.SaveIssued`:
  - A user can have only one not-revoked, not-expired certificate.
  - A replacement cert can be issued after the old cert is revoked or expired.
  - Persistent store load fails closed if duplicate active certs already exist in state.
- Handler maps issuance policy failures to gRPC client errors:
  - CSR identity mismatch -> `InvalidArgument`
  - active certificate already exists -> `AlreadyExists`

Modified locations:

- `ca-service/internal/ca/service.go`
  - added `ErrCSRIdentityMismatch`
  - validates CSR identity before signing
  - saves issued certs through `Store.SaveIssued`
- `ca-service/internal/ca/store.go`
  - added `ErrActiveCertificateExists`
  - added atomic `SaveIssued`
  - validates persisted state for duplicate active certificates
- `ca-service/internal/grpc/handler.go`
  - maps policy errors to appropriate gRPC status codes
- `ca-service/internal/ca/service_test.go`
  - updated CSR generation to bind CSR identity to `user_id`
  - added tests for CSR identity mismatch and duplicate active cert rejection
- `ca-service/internal/ca/store_test.go`
  - added duplicate active certificate policy test

Verification:

```powershell
cd ca-service
$env:GOCACHE = (Join-Path (Get-Location) '.gocache')
go test ./internal/...
go test -race ./internal/...
go test ./...
```

Result: passed.
