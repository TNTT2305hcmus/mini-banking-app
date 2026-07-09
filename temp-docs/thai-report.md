# Thai report - Bank Admin, seed data, ledger/audit

## 1. Seed data và cách nạp

File seed chính: `mini-banking-app/db/bank/seed_demo.sql`.

Nội dung seed:

- 20 customer active, email dạng `*.demo.local`.
- 20 account, mỗi user 1 account, daily limit `50,000,000 VND`.
- 50 transaction có hash-chain, gồm cả `completed` và `failed`.
- 55 audit event có hash-chain, gồm `transfer_completed`, `transfer_rejected`, `insufficient_funds`, `daily_limit_exceeded`, `replay_detected`, `invalid_signature`, `certificate_rejected`, `forbidden_ownership`.

Chạy seed như bên dưới, do nếu chạy trực tiếp bằng seed_demo.sql nó bị lỗi font tiếng việt (Tránh dùng pipe PowerShell để đẩy SQL có tiếng Việt vào `psql`, vì dễ làm hỏng encoding).

```powershell
docker cp .\mini-banking-app\db\bank\seed_demo.sql mini-bank-postgres:/tmp/seed_demo.sql
docker exec -i mini-bank-postgres psql -U banking -d banking -f /tmp/seed_demo.sql
```

Sinh lại seed nếu sửa generator:

```powershell
go run .\mini-banking-app\db\bank\gen_seed_data.go
```

# 2. Đã hoàn thành
- Tạo seed
- UI sẽ cập nhật hạn mức khi user giao dịch xong
- Một số chỗ râu ria khác
- Cũng đã chạy test tay
  - Cho admin bank vào URL của client -> báo không có quyền -> ok
  - Cho client vào URL của BankAdmin -> báo không có quyền -> ok

# 3. Chưa hoàn thành

- Chưa có Postman test cụ thể, do cần Cert phức tạp nên khó tạo


# 4. Lưu ý
- 1 trình duyệt chỉ có thể chứa account của 1 user thôi
- Muốn chứa nhiều cũng được chỉ là đổi cách lưu cert ở frontend
- Chưa có chặn brute force mã PIN