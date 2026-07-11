# Cert TLS giao tiếp nội bộ qua gRPC (CA / KDC / Bank)

Sinh vật liệu **TLS cho gRPC nội bộ** để các service nói chuyện với nhau qua TLS khi chạy local và trong Docker.

Tài liệu này theo kiến trúc CA mới:

```text
Root CA
  └─ gRPC Transport CA
       ├─ ca-server.crt
       ├─ kdc-server.crt
       └─ bank-server.crt
```

`ca-service/certs/intermediate/grpc-ca.crt` là **Intermediate CA do Root CA ký**, không phải self-signed CA độc lập. Nó chỉ dùng để ký cert TLS cho service nội bộ.

Các file `grpc-ca.crt` được phân phối sang Gateway/KDC/Bank là **trust bundle** gồm:

```text
gRPC Transport CA
Root CA
```

Giữ nguyên tên file giúp không phải đổi env `CA_CERT_PATH`, nhưng nội dung phải có đủ Root CA để OpenSSL/Node/Go dựng được chain khi verify service TLS cert.

## Yêu cầu

- Có `openssl` trong `PATH` (OpenSSL 1.1.1+ / 3.x). Kiểm tra bằng `openssl version`.

## Chạy

```powershell
# Windows / PowerShell
./scripts/gen-certs/gen-certs.ps1            # cert hạn 825 ngày
./scripts/gen-certs/gen-certs.ps1 -Days 90
```

```bash
# Linux / macOS / git-bash / Docker / CI
./scripts/gen-certs/gen-certs.sh             # cert hạn 825 ngày
DAYS=90 ./scripts/gen-certs/gen-certs.sh
```

Chạy lại sẽ tái sử dụng gRPC Transport CA sẵn có và chỉ cấp lại cert server cho CA/KDC/Bank. Nếu xoay (rotate) gRPC Transport CA thì phải sinh lại và phân phối lại toàn bộ service cert cùng trust bundle.

Lưu ý chuyển đổi: theo kiến trúc mới, script không được tự tạo một `out/grpc-ca.*` self-signed độc lập và không dùng `ca-server-ca.crt` làm trust anchor riêng cho CA Service. `grpc-ca.crt/key` phải đến từ `ca-service/certs/intermediate`, do Root CA ký trong bước provision CA.

## Sinh ra những gì

| Trust anchor / CA | Dùng cho |
| ----------------- | -------- |
| `ca-service/certs/root-ca/ca.crt` | Root CA cao nhất, ký Intermediate CA |
| `ca-service/certs/intermediate/grpc-ca.crt` | gRPC Transport CA, ký service TLS cert |

Các server cert được đặt đúng nơi từng service load:

| Service | Cert / key | Được đọc bởi |
| ------- | ---------- | ------------ |
| CA   | `ca-service/certs/grpc/ca-server.{crt,key}`   | `GRPC_SERVER_CERT_PATH` / `..._KEY_PATH`; cert này do gRPC Transport CA ký |
| KDC  | `kdc-service/certs/kdc-server.{crt,key}`      | hardcode trong `kdc-service/internal/grpc/server.go` |
| Bank | `banking-service/certs/grpc/bank-server.{crt,key}` | `BANK_TLS_CERT_PATH` / `..._KEY_PATH` |

Cert CA công khai, không có private key, được phân phát tới các bên cần xác thực server:

| Bản sao | Dùng bởi |
| ------- | -------- |
| `api-gateway/certs/grpc-ca.crt` | gateway `CA_CERT_PATH` — trust bundle verify CA/KDC/Bank service cert |
| `kdc-service/certs/grpc-ca.crt` | bootstrap gRPC server của KDC (`server.go`), trust bundle gồm gRPC Transport CA + Root CA |
| `kdc-service/certs/grpc-ca.crt` | trust bundle cho client KDC → CA (`CA_CERT_PATH`) |
| `banking-service/certs/grpc/grpc-ca.crt` | trust bundle cho client Bank → CA (`CA_CERT_PATH`) |

## SAN

Mỗi server cert phủ cả hostname local lẫn hostname Docker để cùng một cert dùng
được ở cả hai môi trường:

| Service | CN / SAN DNS | SAN IP |
| ------- | ------------ | ------ |
| CA   | `ca-service`, `localhost` | `127.0.0.1` |
| KDC  | `kdc-service`, `localhost` | `127.0.0.1` |
| Bank | `banking-service`, `bank-service`, `localhost` | `127.0.0.1` |

Client phải gọi server bằng một trong các tên này. Client KDC → CA dùng `CA_SERVER_NAME=ca-service`; Bank → CA dùng `CA_TLS_SERVER_NAME=ca-service`; gateway thì gọi `localhost:5005x`.

## Lưu ý

- Toàn bộ output đã bị git bỏ qua (`/certs/`, `*.crt`, `*.key`, `*.pem`). Tuyệt
  đối không commit private key.
- Private key của Root CA trong `ca-service/certs/root-ca/ca.key` chỉ ký Intermediate CA.
- Private key của gRPC Transport CA trong `ca-service/certs/intermediate/grpc-ca.key` ký cert TLS cho CA/KDC/Bank.
- Client/user certificate không do gRPC Transport CA ký; chúng do Client CA (`client-ca.crt/key`) ký.
- Giữ các private key ngoài container hoặc mount theo secret; chỉ ship leaf cert/key và cert CA công khai.
