# Hướng Dẫn Chạy Seed Dữ Liệu Bank Demo

Tài liệu này hướng dẫn cách nạp seed dữ liệu mẫu (20 Users, 50 Transactions, 55 Audits) vào database mà không bao giờ bị lỗi font tiếng Việt (biến thành dấu `?`).

## 1. Cách nạp Seed vào Database (Không lỗi font)

**Lưu ý quan trọng**: Không dùng lệnh có ký tự pipe (`|`) trong PowerShell vì PowerShell sẽ tự động chuyển encoding sang ASCII làm hỏng ký tự tiếng Việt.

Hãy đứng ở thư mục gốc dự án và chạy lần lượt 2 lệnh sau:

```powershell
# Bước 1: Copy file seed vào bên trong Docker container
docker cp .\mini-banking-app\db\bank\seed_demo.sql mini-bank-postgres:/tmp/seed_demo.sql

# Bước 2: Thực thi file seed bằng psql trực tiếp trong container
docker exec -i mini-bank-postgres psql -U banking -d banking -f /tmp/seed_demo.sql
```

---

## 2. Cách sinh lại file `seed_demo.sql` (Tùy chọn)

Nếu bạn sửa đổi code sinh dữ liệu mẫu trong file `gen_seed_data.go`, bạn có thể chạy lệnh sau để generate lại file `seed_demo.sql`:

```powershell
# Chạy từ thư mục gốc dự án
go run .\mini-banking-app\db\bank\gen_seed_data.go
```

Sau khi chạy xong, hãy lặp lại **Bước 1 & Bước 2** ở phần trên để nạp dữ liệu mới vào database.
