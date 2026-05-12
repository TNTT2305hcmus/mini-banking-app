# Database (Postgree)

## 1. Users table 
- id: UUID (PK)
- email: VARCHAR(120) (UNIQUE)
- full_name: VARCHAR(100)
- phone: VARCHAR(20)
- status: ENUM('active', 'locked')
- created_at: TIMESTAMP
- updated_at: TIMESTAMP

## 2. Accounts table 
*Chức năng: Quản lý số dư và thiết lập các giới hạn bảo mật (Authorization).*
- id: UUID (PK)
- user_id: UUID (FK) -> Users.id
- account_number: VARCHAR(30) (UNIQUE)
- balance: DECIMAL(19, 2)
- daily_transfer_limit: DECIMAL(19, 2) - Hạn mức chuyển khoản trong ngày (Phục vụ Domain Validation)
- status: ENUM('active', 'locked', 'frozen')
- created_at: TIMESTAMP
- updated_at: TIMESTAMP

## 3. User Certificates table 
- id: UUID (PK)
- user_id: UUID (FK) -> Users.id
- serial_number: VARCHAR(255) (UNIQUE)
- public_key_pem: TEXT
- status: ENUM('active', 'revoked', 'expired')
- not_after: TIMESTAMP
- created_at: TIMESTAMP

## 4. Transactions table 
*Chức năng: Lưu trữ Sổ cái bất biến (Immutable Ledger).*
- id: UUID (PK)
- source_account_number: VARCHAR(30) (FK)
- destination_account_number: VARCHAR(30) (FK)
- amount: DECIMAL(19, 2)
- status: ENUM('pending', 'completed', 'failed')
- description: TEXT
- payload_hash: VARCHAR(255) - Hash nội dung giao dịch (từ Client)
- client_signature: TEXT - Chữ ký số bằng privKey_c (từ Client)
- previous_hash: VARCHAR(255) - Hash của bản ghi giao dịch liền kề trước đó (Phục vụ Hash Chain)
- created_at: TIMESTAMP

# Redis
**Lưu trữ Tạm thời Session Key để Verify Request**
* Khi KDC cấp TGT ở Phase 2, nó sinh ra Session Key K_{c,tgs} và gửi cho Client. Khi Client gọi Phase 3 (TGS-REQ), Client sẽ dùng K_{c,tgs} để mã hóa Authenticator.
* KDC nhận được TGS-REQ, giải mã TGT để lấy K_{c,tgs}. Tuy nhiên, KDC cũng cần nhớ danh sách các Nonce (từ Authenticator) đã được sử dụng trong vài phút qua để chặn Replay Attack (Kẻ gian bắt gói tin cũ và gửi lại).
* **Cách làm:** Lưu Nonce vào Redis với TTL = 5 phút. Nếu thấy Nonce lặp lại, KDC từ chối ngay.

**Cơ chế Thu hồi Ticket khẩn cấp**
* Mặc dù TGT là stateless, nhưng nếu Client phát hiện lộ thiết bị và báo mất, chứng chỉ X.509 bị thu hồi. Lúc này, TGT (vẫn còn hạn) có thể bị kẻ gian lợi dụng.
* **Cách làm:** Khi KDC giải mã TGT ở Phase 3, trước khi cấp Service Ticket, KDC phải kiểm tra xem ID của TGT này có nằm trong "Blacklist" trên Redis không (hoặc gọi sang CA để check trạng thái chứng chỉ). Nếu có, từ chối cấp Ticket.