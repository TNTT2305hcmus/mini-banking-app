# Security And Non-Functional Testcases

Mục tiêu: chứng minh các điểm bảo mật/cryptography của dự án.

## 1. PKI / Certificate

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-PKI-01 | Root CA không ký trực tiếp identity cert | Cert đã provision | Kiểm tra issuer/chain cert user/admin | Identity cert do Client CA ký; Root CA ký Client CA | | RUNTIME PENDING | |
| S-PKI-02 | Client CA mount trong compose | Compose local/demo | Start CA container | CA load được `client-ca.key/crt`, issue cert thành công | | RUNTIME PENDING | |
| S-PKI-03 | Private key không rời browser | Register/activate UI | Quan sát flow CSR/key | Browser sinh keypair, chỉ CSR gửi server, private key wrapped bằng PIN | | RUNTIME PENDING | |
| S-PKI-04 | Cert role enforcement | Có cert `customer`, `bank_admin`, `ca_admin` | Thử dùng sai role cho endpoint admin | Bị 401/403, role đúng mới pass | | RUNTIME PENDING | |
| S-PKI-05 | Revoked cert rejected | Có cert phụ đã revoke | Dùng cert revoke login/AS/bank action | Request bị reject, audit có verify/certificate rejected | | RUNTIME PENDING | |

## 2. AS/TGS/AP

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-KRB-01 | AS request success | Cert active, signature đúng | Login user | AS trả TGT + session key, KDC audit `as_ticket_issued` | | RUNTIME PENDING | |
| S-KRB-02 | AS replay | Dùng lại nonce/challenge | Gửi lại AS request | Reject `REPLAY_DETECTED`, KDC audit `as_rejected` | | RUNTIME PENDING | |
| S-KRB-03 | TGS scope success | TGT hợp lệ, scope hợp lệ | Request service ticket | TGS trả ticket scope đúng, audit `tgs_ticket_issued` | | RUNTIME PENDING | |
| S-KRB-04 | TGS wrong scope | Scope không hợp lệ | Request TGS | Reject, audit `tgs_rejected` | | RUNTIME PENDING | |
| S-KRB-05 | AP request replay/idempotency | Có AP request transfer | Replay request hoặc idempotency key | Không tạo transaction mới; audit replay/idempotency phù hợp | | RUNTIME PENDING | |

## 3. Bank Security

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-BANK-01 | Forbidden ownership balance/history | User A, account User B | Query account không thuộc user | 403, Bank audit `forbidden_ownership` | | RUNTIME PENDING | |
| S-BANK-02 | Forbidden ownership transfer | User A chuyển từ account User B | Submit transfer | 403, không đổi balance, audit `forbidden_ownership` | | RUNTIME PENDING | |
| S-BANK-03 | Invalid signature | Payload bị sửa sau ký | Submit request | Reject, audit `invalid_signature` | | RUNTIME PENDING | |
| S-BANK-04 | Insufficient funds | Amount vượt balance | Submit transfer | Reject, audit `insufficient_funds`, balance không đổi | | RUNTIME PENDING | |
| S-BANK-05 | Wrong service scope | Ticket scope `balance:read` dùng cho transfer | Submit transfer | 403 `WRONG_SCOPE` hoặc equivalent, audit reject | | RUNTIME PENDING | |

## 4. Registration Consistency

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-REG-01 | Duplicate email pre-check | Email đã tồn tại | Register lại | `409 EMAIL_ALREADY_REGISTERED`, không gọi CA issue mới | | RUNTIME PENDING | |
| S-REG-02 | Bank fail after CA issue | Môi trường test có thể giả lập Bank fail | Register khi Bank create fail | Cert được revoke best-effort reason `registration_rollback` | | RUNTIME PENDING | |
| S-REG-03 | JTI không mark quá sớm | Lỗi giữa flow register | Retry token khi lỗi hạ tầng có kiểm soát | Không khóa vĩnh viễn token vì lỗi giữa chừng | | RUNTIME PENDING | |

## 5. Audit / SOC / Hash-Chain

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-AUD-01 | SOC auth required | Không có token | Gọi `/v1/admin-kdc/audit` | 401/403 | | RUNTIME PENDING | |
| S-AUD-02 | KDC audit DB enabled | Compose final | Login AS/TGS rồi mở SOC KDC audit | Có `as_ticket_issued`/`tgs_ticket_issued` | | RUNTIME PENDING | |
| S-AUD-03 | Timeline operation_id | Có operation_id từ UI | Mở timeline | Event CA/KDC/(Bank) nối theo cùng operation_id | | RUNTIME PENDING | |
| S-AUD-04 | Verify hash-chain clean | Chưa tamper DB | Gọi `/v1/admin/audit/verify` | `ok:true` cho source checked | | RUNTIME PENDING | |
| S-AUD-05 | Verify hash-chain tampered | DB demo disposable | Sửa action/reason giữa chuỗi rồi verify | `ok:false`, có `broken_seq/detail` | | PENDING_MANUAL_DB | |
| S-AUD-06 | Export evidence | Có audit events | Export CSV/JSON | File tải được, có cột/field audit chính | | RUNTIME PENDING | |

## 6. Rate Limit / Resilience

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| S-NF-01 | Rate limit enabled | `RATE_LIMIT_DISABLED=0` | Gọi OTP/AS/TGS vượt ngưỡng | 429, có `Retry-After` nếu endpoint dùng limiter | | RUNTIME PENDING | |
| S-NF-02 | Demo rate limit disabled | `RATE_LIMIT_DISABLED=1` | Rehearsal nhiều request | Không bị 429 do rehearsal | | RUNTIME PENDING | |
| S-NF-03 | Audit best-effort | Audit DB tạm lỗi có kiểm soát | Gọi nghiệp vụ chính | Request chính không fail chỉ vì audit insert lỗi | | RUNTIME PENDING | |
