# Functional Testcases

Mục tiêu: chứng minh hệ thống chạy đúng về chức năng người dùng và admin. Các testcase này chủ yếu chạy bằng UI, có thể đối chiếu bằng curl/DB khi cần.

## 1. Customer Flow

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-CUS-01 | Request OTP | Email demo hợp lệ, SMTP cấu hình | Mở `/register`, nhập email, request OTP | UI báo OTP sent, API `POST /v1/otp/request` trả 200 | | RUNTIME PENDING | |
| F-CUS-02 | Verify OTP | Có OTP hợp lệ | Nhập OTP và verify | Nhận registration token, chuyển bước tạo cert | | RUNTIME PENDING | |
| F-CUS-03 | Register PKI | OTP verified, browser có WebCrypto | Tạo keypair/CSR, đặt PIN, submit register | CA cấp cert `customer`, Bank tạo user/account | | RUNTIME PENDING | |
| F-CUS-04 | Duplicate register | Email đã đăng ký | Đăng ký lại cùng email | API trả `409 EMAIL_ALREADY_REGISTERED`, không tạo cert rác | | RUNTIME PENDING | |
| F-CUS-05 | Customer login | Customer đã có cert/key trong browser | Mở `/login`, nhập PIN | AS/TGS chạy thành công, vào `/home` | | RUNTIME PENDING | |
| F-CUS-06 | Profile/balance | Đã login | Vào home/profile/balance | Hiện đúng user info, balance, daily limit | | RUNTIME PENDING | |
| F-CUS-07 | History | Đã login, account có transaction | Mở history | Danh sách transaction có phân trang, không lộ signature nội bộ | | RUNTIME PENDING | |

## 2. Transfer Flow

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-TR-01 | Transfer success | Sender đủ balance, receiver hợp lệ | Thực hiện transfer hợp lệ | Transfer thành công, balance/history cập nhật | | RUNTIME PENDING | |
| F-TR-02 | Insufficient funds | Amount > balance | Submit transfer | UI/API báo fail, balance không đổi | | RUNTIME PENDING | |
| F-TR-03 | Daily limit exceeded | Amount vượt daily remaining | Submit transfer | UI cảnh báo/chặn hoặc API reject, không hiện như success | | RUNTIME PENDING | |
| F-TR-04 | Refresh after failure | Có transfer fail | Quan sát home sau fail | Balance/history refresh đúng, không hiển thị giao dịch thành công giả | | RUNTIME PENDING | |

## 3. Admin CA

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-CA-01 | Activate CA Admin | Pending CA Admin token | Mở `/admin-ca/activate`, nhập thông tin, đặt PIN | Cert role `ca_admin` được cấp, token activation bị dùng một lần | | RUNTIME PENDING | |
| F-CA-02 | Login CA Admin | CA Admin cert/key trong browser | Mở `/admin-ca`, nhập PIN | Session admin-ca được phát sau cert proof | | RUNTIME PENDING | |
| F-CA-03 | List certificates | Đã login CA Admin | Mở list certificates | List có phân trang/filter cơ bản | | RUNTIME PENDING | |
| F-CA-04 | Certificate detail | Có cert bất kỳ | Mở detail cert | Hiện metadata, issuer, role, status, chain | | RUNTIME PENDING | |
| F-CA-05 | Revoke cert phụ | Có cert demo phụ, không phải user chính | Revoke với reason | Status đổi revoked, audit có `revoked` | | RUNTIME PENDING | |

## 4. Admin Bank

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-AB-01 | Activate Bank Admin | Pending Bank Admin token | Activate bằng UI | Cert role `bank_admin` được cấp | | RUNTIME PENDING | |
| F-AB-02 | Login Bank Admin | Bank Admin cert/key | Mở `/admin-bank`, nhập PIN | Cookie `bank_admin_session`, dashboard mở | | RUNTIME PENDING | |
| F-AB-03 | Overview | Đã login Admin Bank | Mở dashboard overview | Thấy total users/accounts/transactions/balance | | RUNTIME PENDING | |
| F-AB-04 | Users/accounts | Đã login Admin Bank | Mở users/accounts | Dữ liệu seed và account hiện đúng | | RUNTIME PENDING | |
| F-AB-05 | Transactions | Có transfer seed/runtime | Mở transactions | Có transaction seed và transaction vừa chạy | | RUNTIME PENDING | |
| F-AB-06 | Audit tab | Đã login Admin Bank | Mở audit/security tab | Thấy Bank audit events | | RUNTIME PENDING | |

## 5. Admin SOC

| ID | Testcase | Input / Tiền điều kiện | Bước chạy | Expected | Actual | Status | Evidence |
|---|---|---|---|---|---|---|---|
| F-SOC-01 | Login SOC | `ADMIN_SEC_DEMO_*` configured | Mở `/admin-soc` hoặc gọi `/v1/admin-sec/auth` | Nhận security-admin session/token | | RUNTIME PENDING | |
| F-SOC-02 | KDC audit list | Có AS/TGS event | Mở KDC audit | Thấy `as_ticket_issued`/`tgs_ticket_issued` | | RUNTIME PENDING | |
| F-SOC-03 | Timeline by operation_id | Có `operation_id` từ flow register/login/transfer | Search timeline | CA/KDC/(Bank nếu có cookie) events được nối đúng | | RUNTIME PENDING | |
| F-SOC-04 | Verify | Audit DB chưa bị tamper | Bấm verify | CA/KDC ok; Bank checked nếu có cookie | | RUNTIME PENDING | |
| F-SOC-05 | Summary/export | Có audit events | Mở summary, export CSV/JSON | Summary có số liệu, file export tải được | | RUNTIME PENDING | |
