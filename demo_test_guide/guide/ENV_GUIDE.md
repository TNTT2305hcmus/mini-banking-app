# Environment Guide

Nguồn env chính cho demo final là:

```text
mini-banking-app/.env.demo.example
mini-banking-app/.env
```

Copy `.env.demo.example` thành `.env`, rồi thay secret thật trước khi chạy.

## 1. Nguyên tắc

- Không commit `.env` chứa secret thật.
- Không dùng root `.env.example` cũ; file đó đã bị loại khỏi hướng final.
- Khi dùng Docker Compose, service nhận env từ `.env` và `docker-compose.local.yml` hoặc `docker-compose.demo.yml`.
- Khi chạy terminal riêng, mỗi service có thể đọc `.env` riêng trong folder service. Các file này cần được rà ở Phase 6 để khớp cấu hình final.

## 2. Shared / Compose

```env
API_GATEWAY_PORT=3000
CA_GRPC_PORT=50051
KDC_GRPC_PORT=50052
BANK_GRPC_PORT=50053
BANK_POSTGRES_PORT=5432
REDIS_PORT=6379
FRONTEND_PORT=5173
```

## 3. CA Service

Biến quan trọng:

```env
ROOT_CA_KEY_PASSWORD=<secret>
CA_DATABASE_URL=<postgres-url>
CERT_VALIDITY_DAYS=365
CA_CRL_DISTRIBUTION_POINTS=
CA_OCSP_SERVERS=
```

Trong compose, CA container được set thêm:

```env
ROOT_CA_KEY_PATH=/certs/root-ca/ca.key
ROOT_CA_CERT_PATH=/certs/root-ca/ca.crt
CLIENT_CA_KEY_PATH=/certs/intermediate/client-ca.key
CLIENT_CA_CERT_PATH=/certs/intermediate/client-ca.crt
GRPC_SERVER_CERT_PATH=/certs/grpc/ca-server.crt
GRPC_SERVER_KEY_PATH=/certs/grpc/ca-server.key
```

CA cần mount đủ:

- `./ca-service/certs/root-ca`
- `./ca-service/certs/intermediate`
- `./ca-service/certs/grpc`

## 4. KDC Service

```env
TGT_EXP=10m
```

Trong compose, KDC được set:

```env
DATABASE_URL=postgres://banking:<BANK_DB_PASSWORD>@bank-postgres:5432/banking?sslmode=disable
REDIS_URI=redis://gateway-redis:6379/0
CA_PORT=ca-service:50051
CA_CERT_PATH=/certs/grpc/grpc-ca.crt
CA_SERVER_NAME=ca-service
K_TGS_PATH=/certs/kdc/k_tgs.key
KDC_PRIVATE_KEY_PATH=/certs/kdc/kdc-private.pem
KDC_SERVER_CERT_PATH=/certs/kdc/kdc-server.crt
KDC_SERVER_KEY_PATH=/certs/kdc/kdc-server.key
KDC_SERVER_CA_CERT_PATH=/certs/grpc/grpc-ca.crt
```

`DATABASE_URL` là điểm quan trọng cho SOC. Nếu thiếu, KDC vẫn cấp AS/TGS nhưng KDC audit no-op, làm SOC timeline thiếu AS/TGS.

## 5. Banking Service

```env
BANK_DB_NAME=banking
BANK_DB_USER=banking
BANK_DB_PASSWORD=<secret>
BANK_KEY=<64-character-hex-key>
```

Trong compose:

```env
DATABASE_URL=postgres://banking:<BANK_DB_PASSWORD>@bank-postgres:5432/banking?sslmode=disable
REDIS_URI=redis://gateway-redis:6379/0
CA_SERVICE_ADDRESS=ca-service:50051
CA_CERT_PATH=/certs/grpc/grpc-ca.crt
CA_TLS_SERVER_NAME=ca-service
BANK_CERT_PATH=/certs/grpc/bank-server.crt
BANK_KEY_PATH=/certs/grpc/bank-server.key
```

## 6. API Gateway

```env
FRONTEND_BASE_URL=http://localhost:5173
GATEWAY_REDIS_URL=redis://gateway-redis:6379/0
JWT_SECRET=<at-least-32-chars>
GATEWAY_OTP_SECRET=<secret>
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=<gmail>
SMTP_PASS=<gmail-app-password>
OTP_MAX_ATTEMPTS=5
OTP_EXPIRES_IN=300
OTP_COOLDOWN=60
ADMIN_ACTIVATION_TTL_SECONDS=900
KDC_GRPC_ADDR=kdc-service:50052
BANK_GRPC_ADDR=banking-service:50053
```

Rate-limit:

```env
RATE_LIMIT_DISABLED=0
RATE_LIMIT_AS_WINDOW_SECONDS=300
RATE_LIMIT_AS_MAX=10
RATE_LIMIT_TGS_WINDOW_SECONDS=300
RATE_LIMIT_TGS_MAX=10
RATE_LIMIT_BANK_WINDOW_SECONDS=60
RATE_LIMIT_BANK_MAX=20
RATE_LIMIT_OTP_WINDOW_SECONDS=600
RATE_LIMIT_OTP_MAX=3
```

Set `RATE_LIMIT_DISABLED=1` khi rehearsal nhiều lần và muốn tránh 429.

## 7. Admin / SOC

Admin CA dùng cert-backed session. Không dùng password/static-token cũ.

```env
ADMIN_CA_TOKEN=
```

`ADMIN_CA_TOKEN` chỉ dùng cho smoke/curl sau khi đã login `/admin-ca` bằng cert/PIN và lấy session token.

SOC:

```env
ADMIN_SEC_DEMO_EMAIL=security@minibanking.local
ADMIN_SEC_DEMO_PASSWORD=<secret>
ADMIN_SEC_DEMO_TOKEN=<dev-static-token>
```

Nếu `ADMIN_SEC_DEMO_*` trống, `/v1/admin-sec/auth` fail-closed.

## 8. Frontend

Compose local set:

```env
VITE_API_BASE_URL=http://localhost:3000
```

Khi chạy frontend dev riêng, `frontend/.env` có thể để `VITE_API_BASE_URL=` để dùng Vite proxy, hoặc set absolute API URL.

## 9. Local terminal URLs

Nếu chạy từng service trên host:

```env
LOCAL_BANK_DATABASE_URL=postgres://banking:<BANK_DB_PASSWORD>@localhost:5432/banking?sslmode=disable
LOCAL_CA_DATABASE_URL=postgres://ca_user:<CA_DB_PASSWORD>@localhost:5433/ca_db?sslmode=disable
LOCAL_KDC_DATABASE_URL=postgres://banking:<BANK_DB_PASSWORD>@localhost:5432/banking?sslmode=disable
```

## 10. Phase 6 cần rà lại

Các file env riêng cần được sửa/đồng bộ ở Phase 6:

- `api-gateway/.env.example`
- `ca-service/.env.example`
- `kdc-service/.env.example`
- `banking-service/.env.example`

Mục tiêu: không còn biến Admin CA password/static-token cũ, có đủ `RATE_LIMIT_*`, SOC env, Client CA paths và KDC audit `DATABASE_URL`.
