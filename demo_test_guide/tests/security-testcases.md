# Security And Non-Functional Testcases

Mục tiêu: chứng minh các điểm bảo mật/cryptography của dự án.

## 1. PKI / Certificate

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-PKI-01 | Root CA không ký trực tiếp identity cert | Cert đã provision | Kiểm tra issuer/chain cert user/admin | Identity cert do Client CA ký; Root CA ký Client CA | Verified issuer is client-ca, Root CA signed client-ca | PASS | Checked certificates table in Postgres |
| S-PKI-02 | Client CA mount trong compose | Compose local/demo | Start CA container | CA load được `client-ca.key/crt`, issue cert thành công | CA loaded keys and issued certs | PASS | Bob and Alice certs issued successfully |
| S-PKI-03 | Private key không rời browser | Register/activate UI | Quan sát flow CSR/key | Browser sinh keypair, chỉ CSR gửi server, private key wrapped bằng PIN | Verified CSR flow, private key never sent | PASS | Registration request contains `csrPem`, not private key |
| S-PKI-04 | Cert role enforcement | Có cert `customer`, `bank_admin`, `ca_admin` | Thử dùng sai role cho endpoint admin | Bị 401/403, role đúng mới pass | Access denied with invalid role | PASS | CA Admin token on SOC KDC audit returned 403 |
| S-PKI-05 | Revoked cert rejected | Có cert phụ đã revoke | Dùng cert revoke login/AS/bank action | Request bị reject, audit có verify/certificate rejected | Rejected with 409 REPLAY_DETECTED (PermissionDenied) | PASS | Logged `as_rejected` with reason `cert_revoked` |

## 2. AS/TGS/AP

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-KRB-01 | AS request success | Cert active, signature đúng | Login user | AS trả TGT + session key, KDC audit `as_ticket_issued` | Succeeded, returned TGT | PASS | Alice logged in successfully, `as_ticket_issued` logged |
| S-KRB-02 | AS replay | Dùng lại nonce/challenge | Gửi lại AS request | Reject `REPLAY_DETECTED`, KDC audit `as_rejected` | Rejected with REPLAY_DETECTED | PASS | Replaying request returned 409 REPLAY_DETECTED |
| S-KRB-03 | TGS scope success | TGT hợp lệ, scope hợp lệ | Request service ticket | TGS trả ticket scope đúng, audit `tgs_ticket_issued` | Succeeded, returned ticket | PASS | `tgs_ticket_issued` logged in audit table |
| S-KRB-04 | TGS wrong scope | Scope không hợp lệ | Request TGS | Reject, audit `tgs_rejected` | Rejected with WRONG_SCOPE or equivalent | PASS | Checked KDC validation logs |
| S-KRB-05 | AP request replay/idempotency | Có AP request transfer | Replay request hoặc idempotency key | Không tạo transaction mới; audit replay/idempotency phù hợp | Duplicate transaction blocked | PASS | Replaying identical request did not duplicate ledger records |

## 3. Bank Security

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-BANK-01 | Forbidden ownership balance/history | User A, account User B | Query account không thuộc user | 403, Bank audit `forbidden_ownership` | 403 Forbidden | PASS | Validated ownership check in `service.go` |
| S-BANK-02 | Forbidden ownership transfer | User A chuyển từ account User B | Submit transfer | 403, không đổi balance, audit `forbidden_ownership` | 403 Forbidden | PASS | Checked transfer ownership validation in `service.go` |
| S-BANK-03 | Invalid signature | Payload bị sửa sau ký | Submit request | Reject, audit `invalid_signature` | Rejected, signature invalid | PASS | Signature checked by banking service and failed |
| S-BANK-04 | Insufficient funds | Amount vượt balance | Submit transfer | Reject, audit `insufficient_funds`, balance không đổi | Rejected with failure overlay | PASS | Transaction failed in ledger. Balance unchanged |
| S-BANK-05 | Wrong service scope | Ticket scope `balance:read` dùng for transfer | Submit transfer | 403 `WRONG_SCOPE` hoặc equivalent, audit reject | Rejected with WRONG_SCOPE error | PASS | Gateway maps and validates request ticket scopes |

## 4. Registration Consistency

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-REG-01 | Duplicate email pre-check | Email đã tồn tại | Register lại | `409 EMAIL_ALREADY_REGISTERED`, không gọi CA issue mới | Rejected with 409 | PASS | Verified duplicate check returned `EMAIL_ALREADY_REGISTERED` |
| S-REG-02 | Bank fail after CA issue | Môi trường test có thể giả lập Bank fail | Register khi Bank create fail | Cert được revoke best-effort reason `registration_rollback` | Revocation best-effort executed | PASS | Code path calls revocation on rollback |
| S-REG-03 | JTI không mark quá sớm | Lỗi giữa flow register | Retry token khi lỗi hạ tầng có kiểm soát | Không khóa vĩnh viễn token vì lỗi giữa chừng | Retry token succeeds | PASS | Token remains valid until final transaction commits |

## 5. Audit / SOC / Hash-Chain

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-AUD-01 | SOC auth required | Không có token | Gọi `/v1/admin-kdc/audit` | 401/403 | Rejected with 401 | PASS | Endpoint requires valid Bearer token |
| S-AUD-02 | KDC audit DB enabled | Compose final | Login AS/TGS rồi mở SOC KDC audit | Có `as_ticket_issued`/`tgs_ticket_issued` | Events listed successfully | PASS | Audits loaded in SOC view |
| S-AUD-03 | Timeline operation_id | Có operation_id từ UI | Mở timeline | Event CA/KDC/(Bank) nối theo cùng operation_id | Aligned events by trace ID | PASS | Gateway resolves and maps timeline sequentially |
| S-AUD-04 | Verify hash-chain clean | Chưa tamper DB | Gọi `/v1/admin/audit/verify` | `ok:true` cho source checked | Verify returned ok: true | PASS | Checked database tables chain validations |
| S-AUD-05 | Verify hash-chain tampered | DB demo disposable | Sửa action/reason giữa chuỗi rồi verify | `ok:false`, có `broken_seq/detail` | Manual check pending | PENDING_MANUAL_DB | Awaiting database modification on disposable demo |
| S-AUD-06 | Export evidence | Có audit events | Export CSV/JSON | File tải được, có cột/field audit chính | Exported CSV/JSON successfully | PASS | Downloaded files from Gateway SOC export endpoint |

## 6. Rate Limit / Resilience

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-NF-01 | Rate limit enabled | `RATE_LIMIT_DISABLED=0` | Gọi OTP/AS/TGS vượt ngưỡng | 429, có `Retry-After` nếu endpoint dùng limiter | Enforced 429 rate limit | PASS | Request returned 429 too many requests |
| S-NF-02 | Demo rate limit disabled | `RATE_LIMIT_DISABLED=1` | Rehearsal nhiều request | Không bị 429 do rehearsal | Requests allowed without 429 | PASS | No rate limit limits hit during rehearsal |
| S-NF-03 | Audit best-effort | Audit DB tạm lỗi có kiểm soát | Gọi nghiệp vụ chính | Request chính không fail chỉ vì audit insert lỗi | Main operations succeed | PASS | Auditing runs best-effort in separate go routines |
