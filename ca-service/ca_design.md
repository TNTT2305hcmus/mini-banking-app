# CA Service — Implementation Description

## Tổng quan

CA Service là service nội bộ (Internal network) viết bằng Go, đóng vai trò **nguồn tin cậy duy nhất** cho toàn hệ thống. Nhiệm vụ cốt lõi: cấp và quản lý vòng đời X.509 certificate cho client. KDC và Bank Service đều phụ thuộc vào CA — mọi quyết định xác thực cuối cùng đều tra về CA.

Giao tiếp hoàn toàn qua **gRPC** (port 50051), không expose HTTP. Gateway không gọi CA trực tiếp — chỉ KDC và Bank Service mới có quyền truy cập.

---

## Cấu trúc thư mục

```
ca-service/
├── cmd/server/
│   └── main.go                  # Entry point, wiring toàn bộ
├── internal/
│   ├── ca/
│   │   ├── rootca.go            # Root CA: load từ disk hoặc tự sinh
│   │   ├── service.go           # Business logic: RegisterUser, GetCert, ...
│   │   ├── store.go             # In-memory cert store (thread-safe)
│   │   └── service_test.go      # Unit tests
│   ├── config/
│   │   └── config.go            # Đọc environment variables
│   └── grpc/
│       ├── handler.go           # Map gRPC ↔ ca.Service
│       └── server.go            # gRPC server + health check + reflection
├── Dockerfile                   # Multi-stage: builder / dev / prod
└── go.mod
```

**Nguyên tắc phân layer:**
- `internal/ca/` — business logic thuần túy, không biết gRPC tồn tại
- `internal/grpc/` — transport layer, không chứa business logic
- `cmd/server/` — wiring: tạo các struct và inject dependency

---

## Layer 1: Config (`internal/config/config.go`)

Đọc toàn bộ config từ environment variable, có giá trị mặc định cho dev:

| Biến môi trường | Mặc định | Mô tả |
|---|---|---|
| `GRPC_PORT` | `50051` | Port gRPC lắng nghe |
| `ROOT_CA_KEY_PATH` | `/certs/root-ca/ca.key` | Private key của Root CA |
| `ROOT_CA_CERT_PATH` | `/certs/root-ca/ca.crt` | Certificate của Root CA |
| `ISSUED_CERTS_PATH` | `/certs/issued` | Thư mục lưu cert đã cấp |
| `CERT_VALIDITY_DAYS` | `365` | Số ngày hiệu lực cert user |

Giá trị này được inject qua `docker-compose.yml` và Docker volume `ca_certs`.

---

## Layer 2: Root CA (`internal/ca/rootca.go`)

### Struct `RootCA`

```go
type RootCA struct {
    PrivateKey  *rsa.PrivateKey
    Certificate *x509.Certificate
    CertPEM     []byte
}
```

Giữ private key và certificate của Root CA trong memory suốt vòng đời process.

### `LoadOrCreate(keyPath, certPath string) (*RootCA, error)`

Đây là bước **đầu tiên** khi CA Service khởi động. Logic:

```
Cả hai file tồn tại → loadFromDisk()
Một trong hai thiếu → generateAndSave()
```

Tạo lại cả bộ nếu một trong hai thiếu để đảm bảo key và cert luôn khớp nhau.

**`generateAndSave()`** — tự sinh Root CA:
1. Sinh RSA-4096 private key (`crypto/rand`) — 4096-bit vì Root CA ưu tiên bảo mật hơn tốc độ
2. Sinh serial number 128-bit ngẫu nhiên
3. Tạo certificate template với:
   - `IsCA: true`, `BasicConstraintsValid: true`
   - `MaxPathLen: 0` — không cho phép intermediate CA
   - `KeyUsage: KeyUsageCertSign | KeyUsageCRLSign` — chỉ ký cert và CRL
   - Validity: 10 năm
4. Self-sign: `x509.CreateCertificate(rand, template, template, pubKey, privKey)`
5. Lưu key với permission **0600** (chỉ owner đọc), cert với 0644

---

## Layer 3: Cert Store (`internal/ca/store.go`)

### Struct `Store`

```go
type Store struct {
    mu      sync.RWMutex
    records map[string]*CertRecord  // key: serial hex string
}

type CertRecord struct {
    UserID       string
    Cert         *x509.Certificate
    CertPEM      string
    RevokedAt    *time.Time   // nil = chưa revoke
    RevokeReason string
}
```

**In-memory store** với `sync.RWMutex` cho thread-safety:
- Read operations (`Get`, `CheckRevocation`): dùng `RLock` — nhiều goroutine đọc đồng thời được
- Write operations (`Save`, `Revoke`): dùng `Lock` — exclusive

**Giới hạn và lý do chấp nhận:** Store reset khi restart. Acceptable cho scope đồ án vì cert được backup trên disk (`/certs/issued/<serial>.pem`). Production thay bằng Postgres query vào bảng `user_certificates`.

---

## Layer 4: CA Service (`internal/ca/service.go`)

### `RegisterUser(csrPEM, userID string)`

5 bước theo thứ tự bắt buộc:

**Bước 1 — Decode PEM:**
```
pem.Decode(csrPEM) → kiểm tra block != nil và Type == "CERTIFICATE REQUEST"
```

**Bước 2 — Parse và verify chữ ký CSR:**
```
x509.ParseCertificateRequest(block.Bytes)
csr.CheckSignature()  ← QUAN TRỌNG NHẤT
```
`CheckSignature()` xác minh rằng client thực sự sở hữu private key tương ứng với public key trong CSR. Đây là bước ngăn attacker dùng `cert_sn` của người khác.

**Bước 3 — Validate public key:**
```
pubKey phải là *rsa.PublicKey
BitLen() >= 2048
```

**Bước 4 — Tạo certificate template:**

| Field | Giá trị | Lý do |
|---|---|---|
| `SerialNumber` | 128-bit random | Đủ lớn để không collision |
| `Subject.CommonName` | userID | Dễ identify cert thuộc về ai |
| `IsCA` | `false` | End-entity cert |
| `KeyUsage` | `DigitalSignature \| KeyEncipherment` | Ký payload + encrypt session key |
| `ExtKeyUsage` | `ClientAuth` | Kerberos PKINIT |

**Bước 5 — Ký và lưu:**
```
x509.CreateCertificate(rand, template, rootCA.Certificate, csr.PublicKey, rootCA.PrivateKey)
store.Save(serialHex, record)
saveCertToDisk(serialHex, pemBytes)  // backup
```

### `GetCertificate(serialHex string)`

Lookup từ store → gọi `resolveStatus()` để tính trạng thái thực tế.

### `CheckRevocation(serialHex string)`

Tương tự `GetCertificate` nhưng tập trung vào trạng thái revocation. Được gọi với tần suất cao (mỗi giao dịch Bank đều gọi) — lý do store dùng `RLock` thay `Lock`.

### `resolveStatus(rec *CertRecord) CertStatusVal`

```
RevokedAt != nil  →  REVOKED   (ưu tiên cao nhất)
now > NotAfter    →  EXPIRED
else              →  VALID
```

Thứ tự ưu tiên quan trọng: cert đã revoke dù chưa hết hạn vẫn phải trả REVOKED.

### `RevokeCertificate(serialHex, reason string)`

Kiểm tra: tồn tại → chưa revoke → gọi `store.Revoke()`. Trả lỗi có prefix (`not_found:`, `already_revoked:`) để handler map sang đúng gRPC status code.

---

## Layer 5: gRPC Handler (`internal/grpc/handler.go`)

Implement interface `CAServiceServer` được generate từ `ca.proto`. Không chứa business logic — chỉ:

1. **Validate input** — kiểm tra field required
2. **Gọi `ca.Service`**
3. **Map lỗi → gRPC Status Code:**

| Loại lỗi | gRPC Code |
|---|---|
| CSR signature invalid | `InvalidArgument` |
| Serial not found | `NotFound` |
| Already revoked | `AlreadyExists` |
| Unexpected | `Internal` |

Lý do không dùng `bool success` trong response: gRPC đã có cơ chế lỗi chuẩn, nhồi thêm flag vào payload gây ambiguity (thành công hay thất bại khi `success=false` kèm data?).

---

## Layer 6: gRPC Server (`internal/grpc/server.go`)

### Health Check

```go
healthSrv.SetServingStatus("ca.CAService", SERVING)
healthSrv.SetServingStatus("", SERVING)  // overall
```

KDC và Bank Service dùng `grpc_health_v1.HealthCheck` để xác nhận CA ready trước khi gửi request. Docker Compose `depends_on` + `condition: service_healthy` dựa vào endpoint này.

### Reflection

```go
reflection.Register(grpcSrv)
```

Cho phép debug không cần proto file:
```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 ca.CAService/CheckRevocation \
  '{"serial_number": "1a2b3c..."}'
```

---

## Unit Tests (`internal/ca/service_test.go`)

10 test cases phân thành 4 nhóm:

### RegisterUser (5 tests)

| Test | Kịch bản | Expect |
|---|---|---|
| `ValidCSR` | CSR đúng, key RSA-2048 | cert PEM hợp lệ, CN = userID, IsCA = false |
| `TamperedCSR` | Flip bit cuối của signature | **error** — bảo vệ cốt lõi |
| `EmptyCSR` | `csrPEM = ""` | error |
| `InvalidPEM` | Chuỗi random không phải PEM | error |
| `WrongPEMType` | PEM type = "CERTIFICATE" | error |

### GetCertificate (2 tests)

| Test | Kịch bản | Expect |
|---|---|---|
| `AfterRegister` | Register → GetCert ngay | status = VALID, userID khớp |
| `NotFound` | Serial không tồn tại | error |

### CheckRevocation (3 tests)

| Test | Kịch bản | Expect |
|---|---|---|
| `ValidCert` | Cert chưa revoke | status = VALID, revokedAt = 0 |
| `AfterRevoke` | Register → Revoke → Check | status = REVOKED, reason khớp |
| `AlreadyRevoked` | Revoke 2 lần | lần 2 trả error |

### Isolation (1 test)

| Test | Kịch bản | Expect |
|---|---|---|
| `MultipleUsers` | 3 user, revoke user1 | serials unique, user2 vẫn VALID |

**Chạy test:**
```bash
cd ca-service
go test ./internal/ca/... -v
go test ./internal/ca/... -v -run TestRegisterUser_TamperedCSR  # test cụ thể
```

---

## Khởi động (`cmd/server/main.go`)

Thứ tự wiring:

```
Config.Load()
  └─► ca.LoadOrCreate(keyPath, certPath)       # Root CA
        └─► ca.NewStore()                       # Cert store
              └─► ca.NewService(rootCA, store)  # Business logic
                    └─► grpc.NewHandler(svc)    # Transport mapping
                          └─► grpc.NewServer(handler, port)
                                └─► server.Start()  # Block
```

Graceful shutdown: bắt `SIGINT` / `SIGTERM` → `grpcServer.GracefulStop()` chờ request đang xử lý hoàn thành trước khi tắt.

---
