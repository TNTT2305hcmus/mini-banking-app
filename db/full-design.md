# CHI TIẾT THIẾT KẾ CƠ SỞ DỮ LIỆU (DATABASE DESIGN)

## 📌 Ghi chú các bổ sung so với mô hình cũ
Để hệ thống vận hành theo tiêu chuẩn Ngân hàng số hiện đại, mô hình này đã được mở rộng với các điểm mới quan trọng sau:
1.  **Quản lý Danh bạ (`Recipients`):** Tách riêng thông tin người nhận giúp người dùng thực hiện giao dịch nhanh hơn và hệ thống quản lý rủi ro tốt hơn.
2.  **Bảo mật & Quản lý phiên (`Device Sessions`, `Security Settings`):** Theo dõi thiết bị đăng nhập, địa chỉ IP và cơ chế khóa tài khoản tự động khi đăng nhập sai.
3.  **Hệ thống Hạn mức & Phí (`Transfer Limits`, `Transaction Fees`):** Tách biệt logic tính toán phí và kiểm soát hạn mức ra khỏi mã nguồn, cho phép thay đổi cấu hình linh hoạt (phí cố định hoặc % theo dải tiền) mà không cần code lại.
4.  **Chuẩn hóa Ngân hàng(`Banks`):** Hỗ trợ chuyển tiền liên ngân hàng

---

## 1. Bảng Users (Người Dùng)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID/BIGINT (PK) | Khóa chính, định danh duy nhất |
| username | VARCHAR(50) | Tên đăng nhập, phải duy nhất |
| email | VARCHAR(120) | Email, dùng để khôi phục mật khẩu & thông báo |
| phone | VARCHAR(20) | Số điện thoại, dùng gửi OTP |
| password_hash | VARCHAR(255) | Hash mật khẩu (bcrypt/argon2) - KHÔNG lưu plain text |
| status | ENUM | Trạng thái (active, locked, deleted) |
| is_verified | BOOLEAN | Đánh dấu email/SĐT đã xác minh |
| created_at | TIMESTAMP | Ngày tạo tài khoản |
| updated_at | TIMESTAMP | Cập nhật lần cuối |

**Điểm đáng chú ý:**
* Lưu `password_hash` chứ không phải mật khẩu gốc.
* `username` và `email` phải có UNIQUE constraint.

---

## 2. Bảng Accounts (Tài Khoản Ngân Hàng)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Tham chiếu đến Users |
| account_number | VARCHAR(30) | Số tài khoản (duy nhất) |
| account_type | ENUM | checking, savings, credit |
| balance | DECIMAL(19,2) | Số dư hiện tại |
| currency | VARCHAR(3) | Mã tiền tệ (VND, USD, EUR...) |
| status | ENUM | active, inactive, blocked |
| created_at | TIMESTAMP | Ngày mở tài khoản |
| updated_at | TIMESTAMP | Cập nhật lần cuối |

---

## 3. Bảng Recipients (Người Nhận Tiền)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Người dùng sở hữu danh bạ |
| account_number | VARCHAR(30) | Số tài khoản người nhận |
| recipient_name | VARCHAR(120) | Tên người nhận (để xác nhận) |
| bank_id | UUID (FK) | Ngân hàng nhận (NULL nếu cùng ngân hàng) |
| relationship | VARCHAR(50) | Mối quan hệ |
| is_favorite | BOOLEAN | Đánh dấu người nhận thường xuyên |
| created_at | TIMESTAMP | Ngày lưu vào danh bạ |

---

## 4. Bảng Transactions (Giao Dịch Chuyển Tiền)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | ID giao dịch |
| source_account_id | UUID (FK) | Tài khoản gửi tiền |
| recipient_id | UUID (FK) | Người nhận (tham chiếu Recipients) |
| amount | DECIMAL(19,2) | Số tiền chuyển |
| currency | VARCHAR(3) | Loại tiền tệ |
| fee | DECIMAL(19,2) | Phí giao dịch |
| total_debit | DECIMAL(19,2) | Tổng trừ (amount + fee) |
| status | ENUM | pending, processing, completed, failed, reversed |
| type | ENUM | same_bank, inter_bank, international |
| reference_number | VARCHAR(50) | Mã tham chiếu (UNIQUE) |
| execution_time | TIMESTAMP | Thời gian thực hiện thực tế |
| description | TEXT | Nội dung chuyển tiền |
| created_at | TIMESTAMP | Lúc tạo giao dịch |
| updated_at | TIMESTAMP | Cập nhật lần cuối |

---

## 5. Bảng Transaction Details (Chi Tiết Lịch Sử Giao Dịch)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| transaction_id | UUID (FK) | Giao dịch liên quan |
| status_before | VARCHAR(50) | Trạng thái trước |
| status_after | VARCHAR(50) | Trạng thái sau |
| changed_by | VARCHAR(100) | Người/hệ thống thay đổi |
| changed_at | TIMESTAMP | Thời gian thay đổi |
| notes | TEXT | Ghi chú (lý do nếu thất bại) |

---

## 6. Bảng Notifications (Thông Báo)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Người nhận thông báo |
| transaction_id | UUID (FK) | Giao dịch liên quan |
| type | ENUM | sms, email, push |
| message | TEXT | Nội dung thông báo |
| status | ENUM | pending, sent, failed |
| retry_count | INT | Số lần thử gửi lại |
| sent_at | TIMESTAMP | Thời gian gửi |

---

## 7. Bảng Device Sessions (Phiên Thiết Bị)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Người dùng |
| device_id | VARCHAR(255) | ID thiết bị (UUID hoặc IMEI) |
| ip_address | VARCHAR(45) | Địa chỉ IP (IPv4 hoặc IPv6) |
| user_agent | TEXT | Thông tin trình duyệt/ứng dụng |
| last_activity | TIMESTAMP | Hoạt động lần cuối |
| status | ENUM | active, inactive, blocked |

---

## 8. Bảng Security Settings (Cài Đặt Bảo Mật)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Người dùng |
| otp_enabled | BOOLEAN | Bật/tắt xác thực OTP |
| two_factor_enabled| BOOLEAN | Bật/tắt 2FA |
| failed_login_attempts| INT | Số lần đăng nhập thất bại |
| locked_until | TIMESTAMP | Khóa tài khoản đến khi nào |

---

## 9. Bảng Transfer Limits (Hạn Mức Chuyển Tiền)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| user_id | UUID (FK) | Người dùng |
| daily_limit | DECIMAL(19,2) | Hạn mức tối đa/ngày |
| daily_used | DECIMAL(19,2) | Đã dùng hôm nay |
| reset_date | DATE | Ngày reset hạn mức |

---

## 10. Bảng Banks (Ngân Hàng)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| code | VARCHAR(10) | Mã ngân hàng (VCB, TCB...) |
| name | VARCHAR(120) | Tên ngân hàng |
| swift_code | VARCHAR(20) | Mã SWIFT |
| status | ENUM | active, inactive |

---

## 11. Bảng Transaction Fees (Phí Giao Dịch)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| transaction_type | ENUM | same_bank, inter_bank... |
| amount_range_min | DECIMAL(19,2) | Mức tiền tối thiểu |
| amount_range_max | DECIMAL(19,2) | Mức tiền tối đa |
| fee_type | ENUM | fixed, percentage |
| fee_amount | DECIMAL(19,2) | Số tiền phí (nếu cố định) |
| fee_percent | DECIMAL(5,2) | % phí (nếu theo %) |

---

## 12. Bảng Currencies (Loại Tiền Tệ)
| Trường | Kiểu Dữ Liệu | Mô Tả |
| :--- | :--- | :--- |
| id | UUID (PK) | Khóa chính |
| code | VARCHAR(3) | Mã ISO (VND, USD, EUR...) |
| symbol | VARCHAR(5) | Ký hiệu (₫, $, €...) |
| is_active | BOOLEAN | Có đang sử dụng? |
