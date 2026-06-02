# Mini-Banking-App - Project Proposal

## Vấn đề

| Vấn đề | Nguyên nhân | Hậu quả |
|---|---|---|
| Xác thực người dùng chưa đủ mạnh | Hệ thống ngân hàng truyền thống thường chỉ dựa vào username/password hoặc một lớp xác thực đơn giản | Dễ bị credential stuffing, phishing, MITM hoặc chiếm quyền tài khoản |
| Không chứng minh được giao dịch do chính chủ thực hiện | Server không có bằng chứng mật mã gắn giao dịch với khóa riêng của người dùng | Người dùng có thể chối bỏ giao dịch; hệ thống khó truy vết và giải quyết tranh chấp |
| Replay Attack trên các request quan trọng | Request thiếu nonce, timestamp và cơ chế lưu dấu request đã dùng | Kẻ tấn công có thể bắt và phát lại request để tạo giao dịch trùng lặp hoặc trái phép |
| Khóa bí mật người dùng có nguy cơ bị lộ | Server lưu trữ hoặc can thiệp vào quá trình tạo, giải mã, sử dụng private key | Mất nguyên tắc Zero-Knowledge; nếu server bị xâm nhập thì khóa người dùng có thể bị đánh cắp |
| Lịch sử giao dịch có thể bị chỉnh sửa | Dữ liệu giao dịch chỉ lưu dạng record thông thường trong database | Admin hoặc attacker có quyền DB có thể sửa dữ liệu quá khứ mà khó bị phát hiện |
| Session key và ticket có thể bị lạm dụng | Ticket tồn tại quá lâu hoặc không gắn scope cụ thể | Kẻ tấn công có thể dùng lại ticket/key bị lộ để thực hiện thao tác ngoài phạm vi hợp lệ |

## Mục tiêu

| Mục tiêu | Mô tả/ Yêu cầu cụ thể |
|---|---|
| Xác thực nhiều lớp | Triển khai luồng OTP -> PKI/X.509 -> Kerberos-like Ticket trước khi cho phép giao dịch |
| Zero-Knowledge Key Generation | Private key của người dùng được sinh ở trình duyệt bằng WebCrypto API, lưu dạng wrapped key và không bao giờ gửi plaintext private key lên server |
| Chống Replay Attack | Mỗi request quan trọng dùng nonce + timestamp; KDC và Bank Service kiểm tra Redis Replay Cache với TTL ngắn |
| Chống chối bỏ giao dịch | Mỗi giao dịch được ký số bằng `privKeyRSA_c`; Bank Service xác minh chữ ký bằng public key trong certificate/ticket |
| Bảo vệ lịch sử giao dịch | Sử dụng Immutable Ledger với Hash Chaining: mỗi giao dịch gắn `previous_hash`, payload và chữ ký |
| Phân quyền theo phạm vi | `Ticket_v` chứa scope cụ thể; Bank Service kiểm tra scope, ownership, daily limit và trạng thái tài khoản trước khi xử lý |
| Bảo vệ dữ liệu nhạy cảm trong RAM | PIN, plaintext private key, session key và ticket được xóa khỏi memory/session state ngay sau khi dùng xong |
| Dễ chạy demo/local | Cấu trúc service rõ ràng, dùng Docker Compose, PostgreSQL, Redis và tài khoản seed phục vụ kiểm thử đồ án |

## Người dùng và nhu cầu

| Người dùng | Hoạt động | Điều quan trọng nhất đối với họ |
|---|---|---|
| Khách hàng | Đăng ký tài khoản bằng email/OTP, tạo khóa, nhận chứng chỉ X.509 | Quy trình đăng ký rõ ràng, khóa riêng được bảo vệ và không bị server sao chép |
| Khách hàng | Đăng nhập, lấy TGT/Ticket_v và thực hiện chuyển tiền | Giao dịch an toàn, không bị giả mạo, không bị replay và có phản hồi xác thực từ Bank Service |
| Khách hàng | Xem số dư và lịch sử giao dịch | Dữ liệu chính xác, lịch sử giao dịch không bị chỉnh sửa trái phép |
| Admin | Truy cập Web App Dashboard để quản lý chứng chỉ X.509 trong hệ thống PKI/CA | Có dữ liệu chứng chỉ đầy đủ, trạng thái rõ ràng và thao tác revoke/tra cứu an toàn |
| Admin | Tra cứu certificate, trạng thái revocation và thông tin liên quan đến người dùng | Dashboard phản ánh đúng dữ liệu từ CA PostgreSQL DB và không làm lộ private key |

## Phạm vi đồ án

| In Scope | Out Scope |
|---|---|
| Thiết kế và cài đặt 4 phase: OTP & PKI Registration, AS Exchange, TGS Exchange, AP Exchange & Transaction | Tích hợp core banking thật hoặc payment gateway live |
| Client React + TypeScript xử lý WebCrypto API, IndexedDB, session state và chữ ký số | Hỗ trợ đầy đủ mọi nghiệp vụ ngân hàng thực tế như vay, tiết kiệm, liên ngân hàng production |
| API Gateway Node.js + TypeScript làm DMZ, rate limiting, audit logging và forward gRPC + mTLS/Auth vào internal services | Triển khai hạ tầng production đa vùng, autoscaling, observability đầy đủ |
| Admin Web App Dashboard để quản lý, tra cứu và revoke chứng chỉ X.509 | Dashboard quản trị production đầy đủ cho mọi nghiệp vụ vận hành ngoài phạm vi PKI/CA |
| CA Service Go cấp phát, tra cứu và thu hồi chứng chỉ X.509; có PostgreSQL DB riêng để lưu certificate metadata | HSM thật cho private key của CA/KDC/Bank Service |
| KDC Go cấp TGT và Ticket_v theo mô hình Kerberos-like, stateless ticket | KMS production thật; trong đồ án dùng env/file key local hoặc cấu hình demo |
| Bank Service Go xác thực AP Exchange, kiểm tra authorization và xử lý giao dịch ACID | Kết nối dữ liệu ngân hàng thật hoặc xử lý tiền thật |
| Redis cho OTP TTL, replay cache và revocation cache | Đảm bảo tuân thủ pháp lý/ngân hàng ở mức production |
| PostgreSQL lưu giao dịch và Immutable Ledger bằng Hash Chaining | Xây dựng hệ thống fraud detection/AML hoàn chỉnh |
| README, setup guide, seed data và demo flow phục vụ trình bày đồ án | Mobile app native hoặc đa nền tảng ngoài web client |

## Rủi ro và ràng buộc

| Loại | Mô tả | Hướng giảm thiểu |
|---|---|---|
| Kỹ thuật | Kiến trúc nhiều service, gRPC, mTLS, PKI và Kerberos-like flow có độ phức tạp cao | Chia implementation theo phase, viết spec/API trước, test từng service và từng exchange độc lập |
| Bảo mật | Private key, PIN, session key hoặc ticket có thể tồn tại trong RAM lâu hơn cần thiết | Dùng WebCrypto `extractable: false`, memory zeroing, TTL ngắn và xóa session state sau giao dịch |
| Bảo mật | Replay cache hoặc timestamp validation sai có thể làm request hợp lệ bị từ chối hoặc request cũ được chấp nhận | Chuẩn hóa window thời gian, hash nonce theo user/timestamp, test case cho replay và clock skew |
| Bảo mật | Certificate revocation không được kiểm tra nghiêm ngặt trước giao dịch | Bank Service bắt buộc gọi CA Service hoặc revocation cache với TTL ngắn trước khi xử lý transfer |
| Dữ liệu | Hash chain sai logic có thể làm ledger mất khả năng chứng minh toàn vẹn | Định nghĩa payload canonical, test hash chain, chỉ append record và dùng reversal transaction thay vì update lịch sử |
| Vận hành | Redis/PostgreSQL hoặc service nội bộ lỗi làm gián đoạn flow xác thực/giao dịch | Có health check, retry có kiểm soát, circuit breaker và thông báo lỗi rõ ràng cho client |
| Phạm vi | Dễ mở rộng quá mức sang core banking hoặc KMS/HSM production | Giữ phạm vi sandbox/demo, ghi rõ out scope trong proposal và workflow |
| Trải nghiệm người dùng | Luồng nhiều phase có thể gây khó hiểu hoặc chậm đối với người dùng | Thiết kế UI theo từng bước rõ ràng, cache session ngắn hạn hợp lý và phản hồi trạng thái đầy đủ |
| Ràng buộc đồ án | Thời gian triển khai giới hạn, trong khi yêu cầu bảo mật nhiều lớp | Ưu tiên luồng chính end-to-end, sau đó mới bổ sung hardening và test mở rộng |
