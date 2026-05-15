# Tài liệu mô tả CA Service

---

## 1. File `config.go` - Quản lý Cấu hình (Configuration Management)

File này đóng vai trò tập trung hóa toàn bộ các tham số cấu hình của CA Service. Việc sử dụng môi trường (Environment Variables) giúp service dễ dàng được triển khai trên các môi trường khác nhau (Local, Docker, Kubernetes) mà không cần phải thay đổi mã nguồn.

### Cấu trúc dữ liệu (Structs)

#### `Config`

Struct trung tâm chứa toàn bộ các thiết lập cần thiết để CA Service hoạt động.

* `GRPCPort (string)`: Cổng mạng mà gRPC Server sẽ mở ra để lắng nghe các kết nối từ phía Client/Bank/KDC.
* `RootCAKeyPath (string)`: Đường dẫn (có thể là tương đối hoặc tuyệt đối) trỏ đến file chứa Private Key của Root CA.
* `RootCACertPath (string)`: Đường dẫn trỏ đến file chứa Certificate công khai của Root CA.
* `IssuedCertsPath (string)`: Thư mục dùng để lưu trữ/backup các file `.pem` của những chứng chỉ đã cấp phát cho người dùng.
* `CertValidityDays (int)`: Thời hạn sống của một chứng chỉ được cấp (tính bằng số ngày).

### Các hàm chức năng

#### `Load() *Config`

Hàm khởi tạo và nạp cấu hình hệ thống.

* **Luồng hoạt động:** Lần lượt đọc các biến môi trường thông qua các hàm helper (`getEnv`, `getEnvInt`).
* **Đặc điểm thiết kế:** Hàm sử dụng các giá trị mặc định (Default Values) là các đường dẫn tương đối (vd: `certs/root-ca/ca.key`). Điều này cho phép developer có thể chạy thử nghiệm service trực tiếp bằng lệnh `go run main.go` trên máy tính cá nhân một cách dễ dàng mà không bị phụ thuộc vào môi trường Docker.

### Các hàm hỗ trợ (Helpers)

Hai hàm này giúp việc trích xuất dữ liệu từ môi trường trở nên an toàn và gọn gàng hơn.

* **`getEnv(key, defaultVal string) string`**: Đọc một biến môi trường dạng chuỗi (`string`). Nếu biến đó không tồn tại hoặc bị bỏ trống, nó sẽ trả về giá trị an toàn mặc định (`defaultVal`).
* **`getEnvInt(key string, defaultVal int) int`**: Đọc một biến môi trường và ép kiểu sang số nguyên (`int`).
* **Lưu ý an toàn:** Nếu người dùng/hệ thống truyền vào một giá trị không phải là số (ví dụ truyền chữ vào biến `CERT_VALIDITY_DAYS`), hàm `strconv.Atoi` sẽ báo lỗi. Khi đó, hàm này sẽ sử dụng `log.Fatalf` để ngay lập tức **đóng băng (crash)** chương trình kèm theo log cảnh báo. Đây là thiết kế "Fail-fast" (Lỗi sớm, Dừng sớm), giúp phát hiện sai sót cấu hình ngay ở bước khởi động thay vì để service chạy ngầm và sinh ra các lỗi không mong muốn ở runtime.

---

## 2. File `store.go` - Quản lý lưu trữ trạng thái Chứng chỉ (In-Memory)

File này định nghĩa một cơ sở dữ liệu tạm thời (In-memory) để lưu trữ và quản lý vòng đời của các chứng chỉ đã được cấp phát. Trọng tâm của file này là đảm bảo tính toàn vẹn dữ liệu trong môi trường xử lý đồng thời (concurrent) và quản lý trạng thái thu hồi (revocation) một cách an toàn.

### Cấu trúc dữ liệu (Structs)

#### `CertRecord`

Đại diện cho một bản ghi chi tiết của một chứng chỉ đã được cấp phát.

* `UserID (string)`: Định danh của người dùng sở hữu chứng chỉ.
* `Cert (*x509.Certificate)`: Đối tượng chứng chỉ đã được parse để truy xuất các trường dữ liệu (vd: ngày hết hạn, public key).
* `CertPEM (string)`: Chuỗi string định dạng PEM của chứng chỉ để trả về cho client.
* `RevokedAt (*time.Time)`: Thời điểm chứng chỉ bị thu hồi. Trị số `nil` ám chỉ chứng chỉ đang hợp lệ.
* `RevokeReason (string)`: Lý do thu hồi (vd: "key compromise", "user requested").

#### `Store`

Bộ lưu trữ in-memory sử dụng Hash Map.

* `mu (sync.RWMutex)`: Read-Write Mutex giúp đảm bảo luồng (Thread-safe), cho phép nhiều request đọc đồng thời nhưng khóa độc quyền khi ghi (cấp mới/thu hồi).
* `records (map[string]*CertRecord)`: Cấu trúc Map lưu trữ `CertRecord` với Key là số Serial Number dạng chuỗi Hex của chứng chỉ.

### Các hàm chức năng

#### `NewStore() *Store`

Khởi tạo một `Store` với Hash Map rỗng.

* **Lưu ý bảo mật (Security Note):** Theo thiết kế, hệ thống cố tình **KHÔNG** load lại các chứng chỉ từ file `.pem` trên ổ cứng khi khởi động lại. Nguyên nhân là do bản thân định dạng X.509 `.pem` không lưu trữ trạng thái "đã bị thu hồi" (revoked). Việc tự động load lại mù quáng sẽ vô tình "hồi sinh" các chứng chỉ đã bị đánh dấu thu hồi trước đó, gây ra lỗ hổng bảo mật nghiêm trọng (những key đã lộ lọt có thể được sử dụng lại).
* **Hướng phát triển Production:** Để giải quyết triệt để, thành phần `Store` in-memory này cần được thay thế bằng kết nối đến Database (như PostgreSQL), nơi trạng thái revocation được lưu vết vĩnh viễn trong một cột riêng biệt.

#### `Save(serial string, record *CertRecord)`

Lưu trữ một chứng chỉ mới vào store.

* Sử dụng `s.mu.Lock()` để khóa toàn bộ map, tránh race condition khi nhiều goroutine cùng thêm cert mới cùng lúc, sau đó lưu `record` vào map với key là `serial`.

#### `Get(serial string) *CertRecord`

Truy xuất một chứng chỉ từ store thông qua Serial Number.

* Sử dụng `s.mu.RLock()` (Read Lock) để cho phép nhiều luồng đọc thông tin chứng chỉ cùng lúc mà không block lẫn nhau. Trả về `nil` nếu không tìm thấy chứng chỉ.

#### `Revoke(serial, reason string) bool`

Cập nhật trạng thái của một chứng chỉ thành "đã thu hồi".

* **Logic:**
1. Yêu cầu quyền Write Lock (`s.mu.Lock()`).
2. Kiểm tra xem chứng chỉ có tồn tại hay không, và nó đã bị thu hồi trước đó hay chưa (`rec.RevokedAt != nil`). Nếu có, trả về `false`.
3. Nếu chứng chỉ hợp lệ, cập nhật trường `RevokedAt` bằng thời gian hiện tại (`time.Now().UTC()`) và lưu `RevokeReason`.
4. Trả về `true` báo hiệu thao tác thu hồi thành công.

---

## 3. File `rootca.go` - Quản lý Root Certificate Authority

File này chứa logic liên quan đến việc định nghĩa, tạo mới (Generate), tự ký (Self-sign), và tải (Load) Root CA từ ổ cứng.

### Cấu trúc dữ liệu (Structs)

#### `RootCA`

Đại diện cho một Tổ chức phát hành chứng chỉ gốc (Root CA) đang được nạp vào bộ nhớ.

* `PrivateKey (*rsa.PrivateKey)`: Khóa riêng tư RSA dùng để ký các chứng chỉ cho người dùng.
* `Certificate (*x509.Certificate)`: Chứng chỉ gốc (đã được parse sang định dạng x509 của Go).
* `CertPEM ([]byte)`: Dữ liệu của chứng chỉ ở định dạng PEM (văn bản) để tiện cho việc truyền tải.

### Các hàm chức năng

#### `LoadOrCreate(keyPath, certPath string) (*RootCA, error)`

Hàm giao tiếp chính với bên ngoài.

* **Logic:** Kiểm tra xem thư mục chứa key đã tồn tại chưa (nếu chưa thì tạo mới với quyền `0700`). Tiếp đó, kiểm tra sự tồn tại của cả hai file `key` và `cert`.
* Nếu cả hai đều tồn tại -> Chuyển hướng gọi hàm `loadFromDisk()`.
* Nếu thiếu một trong hai hoặc cả hai -> Chuyển hướng gọi hàm `generateAndSave()` để tạo mới để đảm bảo tính đồng bộ.

#### `loadFromDisk(keyPath, certPath string) (*RootCA, error)`

Đọc Root CA có sẵn từ file.

* **Logic:**
1. Đọc nội dung file private key, giải mã khối PEM.
2. Parse private key theo chuẩn PKCS8. Nếu lỗi, có cơ chế fallback thử parse theo chuẩn cũ PKCS1. Đảm bảo key parse ra phải là RSA.
3. Đọc nội dung file certificate, giải mã PEM và parse thành đối tượng `x509.Certificate`.
4. Trả về struct `RootCA` chứa key và cert đã parse.



#### `generateAndSave(keyPath, certPath string) (*RootCA, error)`

Tạo mới một Root CA (Self-signed) và lưu xuống đĩa cứng.

* **Logic:**
1. Sinh khóa RSA độ dài **4096-bit** (ưu tiên bảo mật cao nhất cho Root CA).
2. Sinh ngẫu nhiên một Serial Number lớn hơn 0 để tuân thủ chuẩn X.509.
3. Khởi tạo template cho Certificate với các thông tin (Subject): Quốc gia (VN), Tổ chức (Mini_App_Banking), Tên chung (Mini_App_Banking Root CA), hiệu lực 10 năm.
4. Cấu hình các Constraint đặc biệt bắt buộc của CA: `IsCA = true`, `KeyUsage = CertSign | CRLSign` (Chỉ dùng key này để ký cert khác hoặc ký danh sách thu hồi cert). Không cho phép tạo CA trung gian (`MaxPathLen = 0`).
5. Dùng chính Private Key vừa tạo để ký lên template này (`x509.CreateCertificate`).
6. Encode khóa (chuẩn PKCS8) và chứng chỉ ra định dạng PEM.
7. Ghi ra đĩa. Rất quan trọng: file Private Key được phân quyền `0600` (chỉ người dùng tạo file mới đọc/ghi được), file Cert phân quyền `0644`.



#### Hàm phụ trợ (Helpers)

* **`parseCertPEM(pemBytes []byte) (*x509.Certificate, error)`:** Hàm tiện ích giúp giải mã một chuỗi byte PEM và parse nó thành `x509.Certificate`.
* **`fileExists(path string) bool`:** Hàm tiện ích kiểm tra xem một đường dẫn file/thư mục có tồn tại hay không thông qua hàm `os.Stat`.

---

## 4. File `service.go` - Lõi Xử lý Nghiệp vụ (Core Business Logic)

File này chứa toàn bộ quy trình nghiệp vụ (Business Logic) của Certificate Authority. Cấu trúc `Service` được thiết kế hoàn toàn độc lập với giao thức mạng (Transport Layer), nghĩa là nó không biết dữ liệu đến từ gRPC hay REST API, nó chỉ nhận dữ liệu thuần và xử lý.

### Cấu trúc dữ liệu (Structs)

#### `Service`

Đóng gói các dependency cần thiết để thực hiện nghiệp vụ cấp phát/thu hồi chứng chỉ.

* `rootCA`: Thể hiện của Root CA (chứa private key dùng để ký).
* `store`: Tham chiếu đến In-Memory Store dùng để lưu/truy xuất trạng thái chứng chỉ.
* `issuedCertsPath`: Thư mục dùng để backup các chứng chỉ (dưới dạng file `.pem`).
* `certValidityDays`: Số ngày hiệu lực mặc định của chứng chỉ được cấp.

### Các hàm chức năng chính

#### `RegisterUser(csrPEM, userID string) (certPEM string, serialHex string, notAfter int64, err error)`

Đây là hàm phức tạp và quan trọng nhất trong hệ thống, thực hiện quy trình tiếp nhận Yêu cầu ký chứng chỉ (CSR - Certificate Signing Request) từ Client và trả về chứng chỉ đã được ký.

**Luồng xử lý 6 bước:**

1. **Decode CSR:** Nhận chuỗi `csrPEM` từ client, giải mã khối PEM để lấy dữ liệu thô.
2. **Xác minh chữ ký CSR (Verify Signature):** Đây là rào chắn bảo mật cốt lõi. Hàm gọi `csr.CheckSignature()` để đảm bảo người gửi CSR thực sự sở hữu *Private Key* tương ứng với *Public Key* đính kèm trong CSR (tránh bị kẻ xấu giả mạo tạo CSR hộ). Hệ thống cũng kiểm tra bắt buộc key phải là RSA tối thiểu 2048-bit.
3. **Sinh Serial Number ngẫu nhiên:** Sử dụng Cryptographically Secure Pseudo-Random Number Generator (CSPRNG) để tạo số Serial Number duy nhất (>0).
4. **Tạo Template cho X.509:** Dựng khung chứng chỉ mới (không phải CA) với CommonName là `userID`. Xác định `KeyUsage` cho phép dùng để ký điện tử (DigitalSignature) và mã hóa (KeyEncipherment). `ExtKeyUsage` là ClientAuth để xác thực.
5. **Ký Chứng chỉ:** Sử dụng hàm `x509.CreateCertificate`, lấy Private Key của Root CA để ký lên template vừa tạo cùng với Public Key của Client.
6. **Lưu trữ và Trả kết quả:** Sinh chuỗi PEM, lưu chứng chỉ vào In-Memory Store, ghi file dự phòng ra thư mục `issuedCertsPath` trên ổ cứng và trả kết quả về cho Controller.

#### `GetCertificate(serialHex string)`

Lấy thông tin chi tiết và trạng thái của một chứng chỉ dựa trên mã Serial Number của nó.

#### `CheckRevocation(serialHex string)`

Hàm chuyên biệt dùng để kiểm tra tính hợp lệ của chứng chỉ.

* **Ứng dụng:** Được sử dụng bởi thành phần Key Distribution Center (KDC) trong quá trình TGS Exchange và bởi phía Ngân Hàng (Bank) trước khi cho phép bắt đầu bất kỳ giao dịch (Transaction) nào.

#### `RevokeCertificate(serialHex, reason string)`

Thực hiện thu hồi (Revoke) một chứng chỉ.

* **Logic:** Kiểm tra trước xem chứng chỉ có tồn tại và đã bị thu hồi hay chưa. Sau đó tiến hành gọi hàm thu hồi vào bộ nhớ (`Store.Revoke`). Nếu gặp lỗi đồng bộ hóa (do xử lý đa luồng), hàm sẽ báo lỗi.

### Các hàm hỗ trợ (Helpers)

* **`resolveStatus`:** Hàm nội bộ xác định trạng thái thực sự của một chứng chỉ theo độ ưu tiên: Đã thu hồi (REVOKED) > Đã hết hạn (EXPIRED) > Hợp lệ (VALID).
* **`computeSKI`:** Tính toán định danh khóa (Subject Key Identifier) bằng thuật toán băm SHA-1 từ chuỗi Public Key theo chuẩn RFC 5280.
* **`saveCertToDisk`:** Ghi một file backup `.pem` ra ổ cứng dựa vào số Serial của chứng chỉ.

---

## 5. File `handler.go` - Tầng Giao tiếp gRPC (Transport Layer)

File này đóng vai trò là "Cánh cửa" giao tiếp (Controller) giữa thế giới bên ngoài (các service khác thông qua mạng) và lõi xử lý nghiệp vụ của hệ thống (`ca.Service`). Thiết kế này tuân thủ chặt chẽ nguyên lý **Separation of Concerns (Phân tách mối quan tâm)**: gRPC Handler chỉ lo việc nhận request, kiểm tra tính hợp lệ cơ bản, gọi xuống tầng nghiệp vụ, và đóng gói dữ liệu/lỗi thành các mã trạng thái gRPC chuẩn để trả về. Nó **TUYỆT ĐỐI KHÔNG** chứa bất kỳ logic cấp chứng chỉ hay xử lý 암호 (cryptography) nào.

### Cấu trúc dữ liệu (Structs)

#### `Handler`

Struct này triển khai (implement) giao diện (interface) `CAServiceServer` được tự động sinh ra từ file `ca.proto`.

* `pb.UnimplementedCAServiceServer`: Việc nhúng (embed) struct này là bắt buộc trong gRPC Go để đảm bảo tính tương thích ngược (Forward-compatible). Nếu sau này file `.proto` có thêm API mới mà bạn chưa kịp code trong `handler.go`, server sẽ không bị lỗi biên dịch mà chỉ trả về lỗi `Unimplemented` cho API mới đó.
* `svc (*ca.Service)`: Dependency Injection tiêm đối tượng xử lý nghiệp vụ (Business Logic) vào Handler.

### Các API Endpoints (gRPC Methods)

#### `RegisterUser(ctx, req)`

Tiếp nhận yêu cầu đăng ký người dùng mới và cấp chứng chỉ.

* **Quy trình:**
1. Kiểm tra đầu vào (Validation): Đảm bảo `req.CsrPem` và `req.UserId` không bị rỗng. Nếu rỗng, ném lỗi `codes.InvalidArgument` (HTTP 400).
2. Ủy quyền xuống tầng Service: Gọi `h.svc.RegisterUser()`.
3. Xử lý lỗi: Nếu gặp lỗi liên quan đến chữ ký CSR không hợp lệ, phân loại nó thành `InvalidArgument`. Các lỗi khác đưa về `codes.Internal` (Lỗi Server).
4. Trả kết quả: Đóng gói chứng chỉ (.pem), số Serial và ngày hết hạn thành object `RegisterUserResponse`.



#### `GetCertificate(ctx, req)`

Truy vấn thông tin chi tiết và trạng thái của chứng chỉ.

* Trả về thông tin chứng chỉ cùng enum `Status` (Valid, Revoked, Expired, Unknown).
* Nếu chứng chỉ không tồn tại trong hệ thống, bắt lỗi `isNotFoundError` và chủ động trả về chuẩn gRPC status `codes.NotFound` (HTTP 404).

#### `CheckRevocation(ctx, req)`

Hàm chuyên trách phục vụ việc kiểm tra bảo mật từ các Service khác.

* KDC (Key Distribution Center) sẽ gọi hàm này trong quá trình xác thực Kerberos (TGS Exchange) để chặn các Client dùng key đã lộ.
* Bank Service sẽ gọi trước khi bắt đầu bất kỳ transaction nào liên quan đến tiền bạc.
* Cấu trúc trả về `CheckRevocationResponse` gọn nhẹ hơn `GetCertificate`, chỉ tập trung vào Status, lý do (Reason) và thời điểm thu hồi (RevokedAt).

#### `RevokeCertificate(ctx, req)`

API thực hiện việc thu hồi khẩn cấp chứng chỉ.

* **Xử lý mã lỗi cực kỳ chi tiết:** Dựa vào tiền tố (prefix) của câu lỗi trả ra từ Service, hàm dùng `switch case` để map ra đúng chuẩn gRPC:
* Lỗi `"not_found..."` -> `codes.NotFound`
* Lỗi `"already_revoked..."` -> `codes.AlreadyExists` (Báo cho client biết thao tác này thừa, chứng chỉ vốn dĩ đã chết rồi).



### Các hàm phân loại lỗi (Error Helpers)

Đây là các hàm private tiện ích nằm cuối file giúp chuẩn hóa quy trình phân tích lỗi.

* **`isCSRSignatureError(err error) bool`**: Quét nội dung của thông báo lỗi. Nếu chứa các từ khóa như "signature", "verification failed" hoặc "invalid CSR", hệ thống nhận diện đây là lỗi dữ liệu từ phía Client truyền lên.
* **`isNotFoundError(err error) bool`**: Quét từ khóa "not found" để xác định xem tài nguyên có tồn tại hay không. Việc dùng String Matching thay vì tạo Custom Error Types là một cách xử lý nhanh chóng gọn gàng, khá phổ biến trong các microservices nhỏ.

---

## 6. File `server.go` - Khởi tạo và Quản lý gRPC Server (Server Management)

File này chịu trách nhiệm đóng gói (wrap) thư viện `google.golang.org/grpc` tiêu chuẩn thành một đối tượng `Server` dễ quản lý hơn. Nó không chỉ dùng để cấu hình Port hay gắn Handler, mà còn tích hợp các module quản trị hệ thống quan trọng dành cho môi trường Microservices (Docker/Kubernetes).

### Cấu trúc dữ liệu (Structs)

#### `Server`

Bao bọc (wrapper) cho server gRPC gốc.

* `grpcServer (*grpc.Server)`: Đối tượng server cốt lõi của thư viện gRPC thực thi nhiệm vụ quản lý kết nối HTTP/2.
* `port (string)`: Lưu trữ cổng mạng mà server sẽ mở ra để lắng nghe.

### Các hàm chức năng chính

#### `NewServer(handler *Handler, port string) *Server`

Hàm thiết lập (Bootstrap) toàn bộ cấu hình mạng và các tiện ích đi kèm cho Server trước khi nó thực sự chạy.

**Các tính năng cốt lõi được cấu hình trong hàm này:**

1. **Khởi tạo Server gốc:** Gọi `grpc.NewServer()`. Tại đây, hệ thống có để ngỏ (comment) sẵn chỗ để chèn các Middleware (Interceptors) sau này, ví dụ như Logging, Authentication, hay Rate Limiting.
2. **Đăng ký Handler:** Sử dụng hàm `pb.RegisterCAServiceServer()` được code-gen tự động để gắn bộ xử lý `Handler` (đã khai báo ở file `handler.go`) vào Server.
3. **Kích hoạt Health Check Protocol:**
* Đăng ký dịch vụ kiểm tra sức khỏe chuẩn của gRPC (`grpc.health.v1`).
* Hệ thống báo cáo trạng thái `SERVING` (Sẵn sàng phục vụ) cho cả tên định danh cụ thể `"ca.CAService"` lẫn trạng thái tổng thể của toàn Server (`""`).
* **Tầm quan trọng:** Các thành phần nội bộ khác (KDC, Bank Service) và các công cụ điều phối container (Docker Healthcheck, Kubernetes Liveness/Readiness Probes) sẽ gọi vào endpoint ẩn này để biết liệu CA Service đã khởi động xong và sẵn sàng nhận request hay chưa, từ đó quyết định có gửi traffic tới hay không.


4. **Kích hoạt gRPC Reflection:**
* Tính năng này phơi bày cấu trúc của các API (schema) ra ngoài.
* **Tầm quan trọng:** Nó giúp ích cực lớn cho việc Debug. Developer (hoặc Q/A) có thể sử dụng các công cụ như Postman hoặc dòng lệnh `grpcurl` để gọi test trực tiếp vào Server mà không cần phải có/nạp file `ca.proto` gốc.



#### `Start() error`

Khởi động việc lắng nghe kết nối.

* Mở một TCP listener trên port đã cấu hình (`net.Listen`).
* Chuyển quyền điều khiển TCP này cho gRPC Server thông qua `s.grpcServer.Serve(lis)`.
* **Lưu ý vòng đời:** Lệnh `Serve()` là một thao tác **Blocking** (chặn luồng thực thi). Nó sẽ treo ở đó liên tục để hứng request. Đó là lý do tại sao trong `main.go`, hàm `Start()` phải được đặt bên trong một goroutine riêng (`go func() {...}`) để tiến trình chính tiếp tục lắng nghe tín hiệu tắt máy (SIGINT/SIGTERM) từ hệ điều hành.

#### `Stop()`

Tắt server một cách an toàn.

* Không gọi lệnh tắt ngay lập tức (force stop/kill) mà sử dụng cơ chế `GracefulStop()`.
* Chức năng này sẽ ngừng nhận các kết nối mạng mới, nhưng vẫn kiên nhẫn chờ các request đang xử lý dở dang (ví dụ đang ký dở một chứng chỉ) hoàn thành xong rồi mới đóng hoàn toàn Server, tránh làm hỏng dữ liệu hoặc rớt kết nối đột ngột của client.

---

## 7. File `main.go` - Entry Point

File `main.go` đóng vai trò là nhạc trưởng, chịu trách nhiệm khởi tạo các thành phần cần thiết và ghép nối chúng lại với nhau để tạo thành một service hoàn chỉnh có khả năng phục vụ qua gRPC.

### Hàm `main()`

Đây là hàm duy nhất trong file, thực thi tuần tự các bước khởi động (Bootstrapping) của CA Service.

**Luồng xử lý chi tiết:**

1. **Load Configuration (Cấu hình):**
* Gọi `config.Load()` để nạp các biến môi trường (port, số ngày hiệu lực của cert, đường dẫn lưu file...).


2. **Khởi tạo Root CA:**
* Gọi hàm `ca.LoadOrCreate()` để lấy Root CA. Nếu hệ thống chưa có Root CA, nó sẽ tự động sinh mới. Nếu có lỗi xảy ra ở bước này, chương trình sẽ log lỗi (FATAL) và thoát ngay lập tức (Exit 1).


3. **Khởi tạo Certificate Store (Bộ nhớ lưu trữ Chứng chỉ):**
* Gọi `ca.NewStore()` để tạo một in-memory store lưu trữ các chứng chỉ đã cấp phát. *Lưu ý: Trong phiên bản hiện tại, store sẽ rỗng sau khi restart (phù hợp cho scope đồ án).*


4. **Khởi tạo Core CA Service:**
* Gọi `ca.NewService()` với các tham số phụ thuộc đã tạo ở trên (RootCA, store, config) để đóng gói business logic xử lý việc cấp phát và thu hồi chứng chỉ.


5. **Khởi tạo gRPC Server:**
* Bọc `svc` (business logic) vào `cagrpc.NewHandler`, sau đó khởi tạo gRPC server bằng `cagrpc.NewServer`.


6. **Start Server và Graceful Shutdown:**
* Giao việc chạy server cho một goroutine riêng biệt `go func() {...}` để không chặn luồng chính.
* Sử dụng `os/signal` để lắng nghe các tín hiệu hệ điều hành như `SIGINT` (Ctrl+C) hoặc `SIGTERM` (lệnh stop từ Docker).
* Dùng `select` để chờ: Nếu server lỗi, văng lỗi ra `errCh`. Nếu nhận được tín hiệu dừng hệ thống, gọi `server.Stop()` để tắt dịch vụ một cách an toàn (Graceful Shutdown) và giải phóng tài nguyên.


---

