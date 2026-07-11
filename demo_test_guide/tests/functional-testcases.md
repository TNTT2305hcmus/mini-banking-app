# Functional Testcases

Mục tiêu: chứng minh hệ thống chạy đúng về chức năng người dùng và admin. Các testcase này chủ yếu chạy bằng UI, có thể đối chiếu bằng curl/DB khi cần.

## 1. Customer Flow

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-CUS-01 | Request OTP | Email demo hợp lệ, SMTP cấu hình | Mở `/register`, nhập email, request OTP | UI báo OTP sent, API `POST /v1/otp/request` trả 200 | OTP requested successfully via UI | PASS | API returned 200, mock email printed to container logs |
| F-CUS-02 | Verify OTP | Có OTP hợp lệ | Nhập OTP và verify | Nhận registration token, chuyển bước tạo cert | Token successfully received | PASS | Transitioned to PIN setup step, `reg_token` issued |
| F-CUS-03 | Register PKI | OTP verified, browser có WebCrypto | Tạo keypair/CSR, đặt PIN, submit register | CA cấp cert `customer`, Bank tạo user/account | Certificate generated & user registered | PASS | Keypair/CSR created locally, database shows Alice Smith as registered |
| F-CUS-04 | Duplicate register | Email đã đăng ký | Đăng ký lại cùng email | API trả `409 EMAIL_ALREADY_REGISTERED`, không tạo cert rác | API returned 409 Conflict | PASS | Curl command returned `EMAIL_ALREADY_REGISTERED` error code |
| F-CUS-05 | Customer login | Customer đã có cert/key trong browser | Mở `/login`, nhập PIN | AS/TGS chạy thành công, vào `/home` | Logged in using PIN 123456 | PASS | Browser authenticated with KDC and redirected to `/home` dashboard |
| F-CUS-06 | Profile/balance | Đã login | Vào home/profile/balance | Hiện đúng user info, balance, daily limit | Displayed Alice Smith info, balance 50M | PASS | View loaded correctly. Screenshot: `alice_dashboard_1783760732157.png` |
| F-CUS-07 | History | Đã login, account có transaction | Mở history | Danh sách transaction có phân trang, không lộ signature nội bộ | History displayed all transactions | PASS | Viewed transaction history page listing past activities |

## 2. Transfer Flow

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-TR-01 | Transfer success | Sender đủ balance, receiver hợp lệ | Thực hiện transfer hợp lệ | Transfer thành công, balance/history cập nhật | 1,000,000 VND transfer succeeded | PASS | Balance updated to 49,000,000 VND. Screenshot: `transfer_success_1m_1783761105978.png` |
| F-TR-02 | Insufficient funds | Amount > balance | Submit transfer | UI/API báo fail, balance không đổi | Request rejected with failure dialog | PASS | Rejected due to insufficient funds, balance remained at 49,000,000 VND |
| F-TR-03 | Daily limit exceeded | Amount vượt daily remaining | Submit transfer | UI cảnh báo/chặn hoặc API reject, không hiện như success | Request rejected with failure dialog | PASS | Rejected due to exceeding daily remaining limit (Attempted 60M) |
| F-TR-04 | Refresh after failure | Có transfer fail | Quan sát home sau fail | Balance/history refresh đúng, không hiển thị giao dịch thành công giả | Balance/history updated correctly | PASS | Balance remained unchanged, failed records logged under transaction history page |

## 3. Admin CA

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-CA-01 | Activate CA Admin | Pending CA Admin token | Mở `/admin-ca/activate`, nhập thông tin, đặt PIN | Cert role `ca_admin` được cấp, token activation bị dùng một lần | CA Admin activated successfully | PASS | Keypair/CSR created and certificate issued by Root CA |
| F-CA-02 | Login CA Admin | CA Admin cert/key trong browser | Mở `/admin-ca`, nhập PIN | Session admin-ca được phát sau cert proof | Logged in using PIN 123456 | PASS | `/admin-ca` dashboard loaded showing certificates |
| F-CA-03 | List certificates | Đã login CA Admin | Mở list certificates | List có phân trang/filter cơ bản | Certificates list displayed | PASS | Active certs shown with pagination and type filtering |
| F-CA-04 | Certificate detail | Có cert bất kỳ | Mở detail cert | Hiện metadata, issuer, role, status, chain | Certificate metadata displayed | PASS | Viewed details including issuer Client CA, role customer, and validity |
| F-CA-05 | Revoke cert phụ | Có cert demo phụ, không phải user chính | Revoke với reason | Status đổi revoked, audit có `revoked` | Bob cert successfully revoked | PASS | Revoked serial `54940ba1c4af594459c7da796dd125aa` with `key_compromise` |

## 4. Admin Bank

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-AB-01 | Activate Bank Admin | Pending Bank Admin token | Activate bằng UI | Cert role `bank_admin` được cấp | Bank Admin activated successfully | PASS | Certificate issued and stored in browser storage |
| F-AB-02 | Login Bank Admin | Bank Admin cert/key | Mở `/admin-bank`, nhập PIN | Cookie `bank_admin_session`, dashboard mở | Logged in using PIN 123456 | PASS | Bank dashboard opened successfully |
| F-AB-03 | Overview | Đã login Admin Bank | Mở dashboard overview | Thấy total users/accounts/transactions/balance | Dashboard statistics loaded | PASS | Displayed total users (21), accounts (21), and transfer volumes |
| F-AB-04 | Users/accounts | Đã login Admin Bank | Mở users/accounts | Dữ liệu seed và account hiện đúng | User list loaded correctly | PASS | Listed Nguyen Van An, Alice Smith, etc. with respective accounts |
| F-AB-05 | Transactions | Có transfer seed/runtime | Mở transactions | Có transaction seed và transaction vừa chạy | Transactions list loaded | PASS | Showed seed records and new runtime 1M VND transaction |
| F-AB-06 | Audit tab | Đã login Admin Bank | Mở audit/security tab | Thấy Bank audit events | Audit events listed | PASS | Displayed actions: transfer_completed, forbidden_ownership, etc. |

## 5. Admin SOC

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-SOC-01 | Login SOC | `ADMIN_SEC_DEMO_*` configured | Mở `/admin-soc` hoặc gọi `/v1/admin-sec/auth` | Nhận security-admin session/token | Logged in successfully | PASS | Obtained security-admin session token |
| F-SOC-02 | KDC audit list | Có AS/TGS event | Mở KDC audit | Thấy `as_ticket_issued`/`tgs_ticket_issued` | KDC audit records loaded | PASS | Listed `as_ticket_issued`, `tgs_ticket_issued`, and `as_rejected` logs |
| F-SOC-03 | Timeline by operation_id | Có `operation_id` từ flow register/login/transfer | Search timeline | CA/KDC/(Bank nếu có cookie) events được nối đúng | Timeline unified by trace ID | PASS | Cross-service events aligned sequentially by X-Request-ID |
| F-SOC-04 | Verify | Audit DB chưa bị tamper | Bấm verify | CA/KDC ok; Bank checked nếu có cookie | Integrity check returned ok | PASS | Replay of CA, KDC, and Bank chains returned true |
| F-SOC-05 | Summary/export | Có audit events | Mở summary, export CSV/JSON | Summary có số liệu, file export tải được | Statistics shown, export completed | PASS | Downloaded CSV/JSON audit dumps successfully |
