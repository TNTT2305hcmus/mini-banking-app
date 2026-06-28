# 1 - DONE
- Hãy đọc @frontend/src/services/pki-regist/opt-pki-regist-flow.md để biết frontend của user cần làm: Sinh RSA key pair (WebCrypto, private key extractable:false), Tạo CSR + ký proof-of-possession, Lưu WRAPPED private key vào IndexedDB, Lưu certificate_pem vào IndexedDB

- Do logic của IndexDB và Key sẽ được sử dụng chung nên sẽ được đặt ở db.service.ts và key.service.ts

- Các logic mà frontend cần thì được đặt trong pki-registration/

- Trước hết cần hiểu rõ thật đầy đủ luồng từ otp-pki-regist-flow.md (chính), 01-otp-pki-registration.md trong api-design/specs và design.md trong blueprint và thư mục api-gateway/proto/   

# 2 - DONE
Hiện tại tôi đang ko biết cho USER nhập thông tin về full name ở đâu.
  1. Cùng lúc với nhập Email và gửi đến API gateway của server
  2. Sau khi nhập được mã OTP và xác thực thành công.
  3. Một thời điểm nào đó khác

Hãy xác định tại cái folder bên server ở api-gateway/src/proto và api-gateway/src/route


# 3 
- Hãy đọc as-exchange-flow.md, 02-as-exchange.md ở spec và  api-design để biết frontend cần xử lý những thao tác.
- Đọc API gateway route, proto để có thể tích hợp đúng.
- Đọc sơ về src/services/ của frontend để biết ta đã dùng indexedDB để lưu Cert và private key (đã mã bằng PIN)
- Trường ClientID sẽ được lấy từ trong Cert lưu ở indexedDB
- Thực thi mã nguồn trong as-exchange của frontend

# 4
- 