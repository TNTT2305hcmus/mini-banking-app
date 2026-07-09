# Thai jobs

## 1. Full flow Bank Admin

Mục tiêu: chứng minh Bank Admin cert-based chạy được từ đầu đến cuối.

- Chạy lại flow trong runtime `mini-banking-app/mini-banking-app`.
- Provision Bank Admin bằng script:
  - `npm.cmd run provision:bank-admin -- --email <email> --full-name "<name>"`
- Mở link activation trong email.
- Kích hoạt tại `/admin-bank/activate`:
  - nhập token/email/họ tên;
  - đặt PIN 6 số;
  - xác nhận cert role `bank_admin` được lưu vào IndexedDB.
- Đăng nhập tại `/admin-bank/login`:
  - nhập đúng PIN;
  - AS Exchange;
  - TGS Exchange scope `bank-admin:read`;
  - AP session;
  - Gateway set cookie `bank_admin_session`;
  - điều hướng được vào `/admin-bank`.
- Nếu PIN đúng nhưng không vào dashboard, kiểm tra:
  - request `POST /v1/admin/bank/session` có thành công không;
  - response có `Set-Cookie: bank_admin_session=...` không;
  - request dashboard có gửi cookie không;
  - nếu thiếu cookie thì UI sẽ bị đẩy ngược về `/admin-bank/login`.

## 2. Cookie/session và phân quyền Bank Admin

Mục tiêu: chứng minh dashboard Admin Bank chỉ mở cho phiên Bank Admin hợp lệ.

Tình hình hiện tại:

- `bank_admin_session` là cookie HttpOnly do Gateway set sau khi Bank Admin login thành công.
- Cookie này không phải cert, không phải TGT, không phải Ticket_v.
- Cookie là opaque session token; Banking Service lưu hash token trong Redis.
- Các API dashboard bắt buộc có cookie này.

Việc cần kiểm tra:

- Thiếu cookie:
  - gọi các endpoint `/v1/admin/bank/*/query` không gửi cookie;
  - kỳ vọng `ADMIN_SESSION_REQUIRED`.
- Cookie sai format:
  - gửi `Cookie: bank_admin_session=@@@`;
  - kỳ vọng `ADMIN_SESSION_INVALID` từ Gateway.
- Cookie đúng format nhưng fake:
  - gửi một base64url token không tồn tại trong Redis;
  - kỳ vọng `ADMIN_SESSION_INVALID` từ Banking Service.
- Cookie hết hạn:
  - nếu có điều kiện test TTL/session expiry;
  - kỳ vọng `ADMIN_SESSION_EXPIRED`.
- Customer cert xin quyền Bank Admin:
  - dùng customer cert thử login Admin Bank hoặc xin scope `bank-admin:read`;
  - kỳ vọng bị chặn bằng `SCOPE_DENIED` ở KDC hoặc `ADMIN_ROLE_REQUIRED` ở Bank.

## 3. Regression 5 API Admin Bank

Mục tiêu: xác nhận các API dashboard thật sự dùng session hợp lệ và trả dữ liệu đúng.

Endpoint cần test:

- `POST /v1/admin/bank/overview/query`
- `POST /v1/admin/bank/users/query`
- `POST /v1/admin/bank/users/:userId/accounts/query`
- `POST /v1/admin/bank/transactions/query`
- `POST /v1/admin/bank/audit/query`

Yêu cầu test:

- Có cookie hợp lệ thì trả data.
- Không có cookie thì bị chặn.
- Pagination hợp lệ.
- `limit > 100` hoặc `offset < 0` phải trả lỗi phù hợp.
- Filter sai status/action/UUID phải trả `INVALID_FILTER`.
- `from_unix > to_unix` phải trả `INVALID_DATE_RANGE`.
- Users query có empty state nếu không có kết quả.
- User accounts query mở được account theo user đã chọn.

Deliverable:

- Chuẩn bị curl/Postman mẫu cho cả 5 endpoint.
- Ghi request body mẫu, header `X-Request-ID`, cookie cần dùng.

## 4. Ledger completed/failed

Mục tiêu: Admin Bank không chỉ thấy giao dịch thành công, mà phải thấy cả giao dịch thất bại và lý do.

Vấn đề cần giải quyết:

- Trước đó có rủi ro transaction `failed` bị hiểu nhầm như success.
- Admin Bank ledger/audit phải phản ánh đúng trạng thái thực tế.
- Failed reason nằm chủ yếu trong `bank_audit_log.reason`, không phải chỉ ở bảng `transactions`.

Việc cần kiểm tra:

- Filter `completed`:
  - vào `/admin-bank` -> tab `Ledger`;
  - chọn `completed`;
  - phải chỉ thấy giao dịch thành công.
- Filter `failed`:
  - chọn `failed`;
  - phải thấy giao dịch thất bại nếu DB có data;
  - UI phải hiển thị trạng thái failed/thất bại, không như success.
- Overview:
  - `failed_transactions` phải tăng nếu có failed transaction.
- Audit:
  - tab `Security Audit` phải có event/reason tương ứng;
  - ví dụ `insufficient_funds`, `daily_limit_exceeded`, `forbidden_ownership`, `replay_detected`, `certificate_rejected`.
- Nếu DB chưa có failed transaction:
  - tạo một giao dịch lỗi thật;
  - ví dụ chuyển vượt số dư hoặc vượt hạn mức ngày.

Checklist cần ghi:

- Filter all thấy đủ completed/failed.
- Filter completed đúng.
- Filter failed đúng.
- Overview đếm failed đúng.
- Audit có reason.
- Reason hiển thị dễ hiểu.

## 5. Vấn đề 50M ở Client Home/Transfer

Mục tiêu: UI không làm người dùng hiểu nhầm 50M là trần số dư hệ thống.

Tình hình hiện tại:

- Core Bank không có cap số dư tối đa.
- Bank có `daily_transfer_limit`.
- Account mặc định hiện có số dư và daily limit đều có thể là `50,000,000`.
- UI Home có hiển thị số dư và `Hạn mức ngày`.
- Transfer success có gọi refetch balance.
- Transfer failed có modal lỗi.

Thiếu sót/rủi ro UI:

- UI chưa giải thích rõ `Hạn mức ngày` là tổng số tiền được chuyển trong ngày, không phải trần số dư.
- UI chưa hiển thị đã chuyển hôm nay bao nhiêu/còn lại bao nhiêu.
- Transfer form chưa cảnh báo trước nếu amount vượt balance hoặc vượt daily limit.
- Khi daily limit failed, UI chỉ hiện lỗi sau khi đã nhập PIN và gửi request.
- Nếu failed transaction được ghi vào ledger, UI user chưa tự refresh history để người dùng thấy giao dịch failed.

Luồng cần kiểm tra:

- User mới đăng ký:
  - xem số dư ban đầu;
  - xem hạn mức ngày;
  - xác nhận UI không gây hiểu nhầm.
- Chuyển khoản nhỏ dưới hạn mức:
  - phải thành công;
  - balance refetch đúng;
  - history có giao dịch `Hoàn tất`.
- Chuyển vượt số dư:
  - phải hiện lỗi `Số dư tài khoản không đủ`;
  - không được hiện như success.
- Chuyển vượt daily limit:
  - phải hiện lỗi `Giao dịch vượt quá hạn mức chuyển tiền trong ngày`;
  - Admin Bank ledger phải có transaction `failed`;
  - Admin Bank audit phải có reason `daily_limit_exceeded`.

Điều nên đổi nếu còn thời gian:

- Thêm text giải thích cạnh `Hạn mức ngày`.
- Transfer form hiển thị số dư khả dụng và hạn mức ngày.
- Cảnh báo trước khi amount vượt số dư/hạn mức.
- Sau failed transfer, refresh history hoặc hướng dẫn người dùng kiểm tra lịch sử.

## 6. SOC/Bank audit

Mục tiêu: demo đúng quyết định nhóm đã chốt về quyền xem Bank audit.

Tình hình hiện tại:

- SOC timeline/summary/export/verify có thể gom CA, KDC, Bank.
- CA và KDC được đọc theo credential SOC/security-admin.
- Bank audit chỉ được gộp khi request có thêm cookie `bank_admin_session`.
- Nếu không có cookie này, Bank source bị bỏ qua với reason `bank_admin_session_required`.

Điều nhóm muốn:

- Giữ thiết kế cookie-gated Bank audit.
- SOC không tự có quyền đọc Bank audit chỉ bằng security-admin.
- Không mở trusted read path riêng từ SOC sang Bank trong demo.
- Operator muốn thấy Bank audit trong SOC thì phải login Bank Admin trước trong cùng browser/session.

Việc cần kiểm tra:

- Case không có Bank Admin cookie:
  - login `/admin-soc`;
  - gọi timeline/summary/export/verify;
  - kỳ vọng Bank source không được gộp;
  - reason là `bank_admin_session_required`.
- Case có Bank Admin cookie:
  - login `/admin-bank/login` trước;
  - sau đó vào `/admin-soc` cùng browser;
  - gọi timeline/summary/export/verify;
  - kỳ vọng Bank audit được gộp nếu có event phù hợp.

Điều cần ghi trong demo/report:

- Đây là thiết kế cố ý để tách quyền SOC và Bank Admin.
- Bank audit chỉ xuất hiện trong SOC khi browser có `bank_admin_session`.
- Không coi `bank_admin_session_required` là bug nếu operator chưa login Bank Admin.

## 7. Seed data cho dashboard đẹp

Mục tiêu: dashboard có dữ liệu đủ để demo, không bị trống.

Data nên có:

- Ít nhất 2 user/customer active.
- Mỗi user có account và số dư dễ nhìn.
- Ít nhất 1 Bank Admin đã activate.
- Ít nhất 1 transaction `completed`.
- Ít nhất 1 transaction `failed`.
- Audit tương ứng cho:
  - `transfer_completed`;
  - `transfer_rejected` hoặc `insufficient_funds`;
  - `daily_limit_exceeded` nếu test vấn đề 50M;
  - `forbidden_ownership` hoặc negative case khác nếu có thời gian.

Ghi rõ trong report:

- Dữ liệu lấy từ seed hay tạo bằng flow thật.
- Account nào dùng để demo chuyển thành công.
- Account nào dùng để demo failed/revoke/negative.

## 8. Report cá nhân

Cập nhật `temp-docs/thai-report.md`.

Nội dung cần có:

- Việc đã làm.
- File/code liên quan nếu có chỉnh.
- Cách test.
- Kết quả pass/fail.
- Curl/Postman mẫu cho 5 endpoint Admin Bank.
- Seed data cần cho dashboard.
- Blocker còn lại.
- Ảnh hưởng tới demo.

Format gợi ý:

| Hạng mục | Kết quả | Ghi chú |
|---|---|---|
| Bank Admin activate/login | Pass/Fail | ... |
| Cookie missing/invalid/expired | Pass/Fail | ... |
| Customer cert bị chặn | Pass/Fail | ... |
| Overview/users/accounts | Pass/Fail | ... |
| Ledger completed/failed | Pass/Fail | ... |
| Audit reason | Pass/Fail | ... |
| 50M user flow | Pass/Fail | ... |
| SOC Bank audit cookie-gated | Pass/Fail | ... |
