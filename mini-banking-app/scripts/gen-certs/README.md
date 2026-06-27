# Cert TLS giao tiếp nội bộ qua gRPC (ca / kdc / bank)

Sinh toàn bộ vật liệu **TLS cho gRPC nội bộ** để các service nói chuyện với nhau
qua TLS khi chạy local và trong Docker.

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

Chạy lại sẽ tái sử dụng CA sẵn có trong `out/` và chỉ cấp lại các leaf server
cert. Xoá `out/grpc-ca.*` nếu muốn xoay (rotate) CA — khi đó phải sinh lại và
phân phối lại toàn bộ leaf cert.

## Sinh ra những gì

| Trust anchor (một CA duy nhất) | `scripts/gen-certs/out/grpc-ca.{crt,key}` |
| ------------------------------ | ----------------------------------------- |

Các server cert, ký bởi CA trên, được đặt đúng nơi từng service load:

| Service | Cert / key | Được đọc bởi |
| ------- | ---------- | ------------ |
| CA   | `ca-service/certs/grpc/ca-server.{crt,key}`   | `GRPC_SERVER_CERT_PATH` / `..._KEY_PATH` |
| KDC  | `kdc-service/certs/kdc-server.{crt,key}`      | hardcode trong `kdc-service/internal/grpc/server.go` |
| Bank | `banking-service/certs/grpc/bank-server.{crt,key}` | `BANK_TLS_CERT_PATH` / `..._KEY_PATH` |

Cert của CA (công khai, không có key) được phân phát tới mọi bên cần xác thực server:

| Bản sao `grpc-ca.crt` | Dùng bởi |
| --------------------- | -------- |
| `api-gateway/certs/grpc-ca.crt` | gateway `CA_CERT_PATH` — xác thực CA, KDC và Bank |
| `kdc-service/certs/grpc-ca.crt` | bootstrap gRPC server của KDC (`server.go`) |
| `kdc-service/certs/grpc/ca-server-ca.crt` | trust anchor cho client KDC → CA (`CA_TLS_CA_CERT_PATH`) |

## SAN

Mỗi server cert phủ cả hostname local lẫn hostname Docker để cùng một cert dùng
được ở cả hai môi trường:

| Service | CN / SAN DNS | SAN IP |
| ------- | ------------ | ------ |
| CA   | `ca-service`, `localhost` | `127.0.0.1` |
| KDC  | `kdc-service`, `localhost` | `127.0.0.1` |
| Bank | `banking-service`, `bank-service`, `localhost` | `127.0.0.1` |

Client phải gọi server bằng một trong các tên này. Client KDC → CA đã dùng sẵn
`CA_TLS_SERVER_NAME=ca-service`; gateway thì gọi `localhost:5005x`.

## Lưu ý

- Toàn bộ output đã bị git bỏ qua (`/certs/`, `*.crt`, `*.key`, `*.pem`). Tuyệt
  đối không commit private key.
- Private key của CA (`out/grpc-ca.key`) là bí mật duy nhất ký nên niềm tin —
  giữ nó ngoài container; chỉ ship leaf cert/key và cert CA công khai.
