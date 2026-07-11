# Báo Cáo Final - Mini Banking App

> Ngày cập nhật: 11/07/2026  
> Ngôn ngữ: Tiếng Việt  
> Ghi chú: bản rút gọn để viết báo cáo nộp, có sequence diagram cho các luồng chính.

## 0. Thông Tin Nhóm Và Tài Liệu Nộp

| Mục | Nội dung |
|---|---|
| Tên đề tài | Mini Banking App - Secure Banking Workflow with PKI, Kerberos-like Tickets and Audit SOC |
| Môn học | Applied Cryptography |
| Nhóm | G05 |
| Lớp | 23_22 |
| Giảng viên hướng dẫn | Trương Toàn Thịnh, Mai Anh Tuấn |
| Repository | https://github.com/TNTT2305hcmus/mini-banking-app |
| Video demo YouTube | TBD - điền link YouTube demo sau khi upload |
| Hướng dẫn chạy | `README.md`, `RUN.md`, `demo_test_guide/guide/RUN_GUIDE.md` |
| Testcase | `demo_test_guide/tests/` |
| Kết quả runtime | `demo_test_guide/tests/runtime-results.md` |

### Tỷ Lệ Đóng Góp

| Thành viên | Phần việc chính | Tỷ lệ đề xuất |
|---|---|---:|
| Thanh | Lead, PKI registration, CA layerd architecture, Admin CA, register consistency, CA auth flow | 25% |
| Thái | Admin Bank, dashboard, user/account/transaction view, Bank UI/API flow | 25% |
| Thuận | Audit, SOC, trace/operation id, hash-chain testcase, security monitoring | 25% |
| Quang | KDC/Bank integration, compose/env, seed, smoke script, demo/runtime guide | 25% |
| Tổng |  | 100% |

## 1. Mô Tả Đề Tài

Mini Banking App là hệ thống demo ngân hàng thu nhỏ tập trung vào các cơ chế Applied Cryptography:

- Đăng ký người dùng bằng OTP, CSR và chứng thư X.509.
- Lưu private key ở browser, wrap bằng PIN, không gửi plaintext lên server.
- Đăng nhập/gọi dịch vụ theo mô hình Kerberos-like: AS -> TGS -> AP.
- Ký số banking payload bằng RSA-PSS, mã hóa payload bằng AES-GCM.
- Phân quyền bằng certificate role và service ticket scope.
- Quản trị CA, quản trị Bank và SOC audit.
- Audit hash-chain để phát hiện sửa/xóa/đảo event.

Hệ thống là coursework/demo, không xử lý tiền thật và không claim production banking compliance.

## 2. Kiến Trúc Và Mô Hình Tin Cậy

```text
Browser SPA
  | REST/HTTP(S)
  v
API Gateway
  | gRPC/TLS server-auth
  +--> CA Service
  +--> KDC Service
  +--> Banking Service
  |
  +--> Redis + PostgreSQL
```

| Thành phần | Vai trò |
|---|---|
| Frontend | Sinh key/CSR, lưu cert/private key, gọi Gateway, verify cert/KDC response |
| API Gateway | REST boundary, OTP, validation, orchestration, forward gRPC, SOC aggregation |
| CA Service | Root CA, Client CA, issue/verify/revoke cert, CA audit |
| KDC Service | AS/TGS, TGT, service ticket, scope, KDC audit |
| Banking Service | AP auth, balance/history/transfer, idempotency, Bank audit |
| Admin SOC | Timeline, verify hash-chain, summary, export |

Trust model quan trọng:

- Root CA là trust anchor. Frontend nạp Root CA runtime từ same-origin `/trust/root-ca.pem`, không hard-code trong bundle.
- Client xác minh cert được cấp trước khi lưu: chain về Root CA, public key khớp key vừa sinh, CN/email SAN khớp đăng ký.
- Client xác minh AS_REP bằng certificate chain của KDC và `kdc_signature`.
- KDC/Bank không tin public key raw từ request; public key lấy từ CA `VerifyCertificate`.

## 3. Chức Năng Đã Hoàn Thành

| Nhóm chức năng | Mô tả | Đánh giá |
|---|---|---|
| Customer registration | OTP, CSR, cấp cert, tạo Bank user/account, rollback/revoke khi Bank fail | Tốt |
| Customer login | PIN/cert, AS exchange, verify AS_REP, lưu TGT trong RAM | Tốt |
| TGS/AP | Xin service ticket, AP authenticator, scoped access | Tốt |
| Banking | Profile, balance, history, transfer, failed transaction evidence | Tốt |
| Admin CA | Activate, cert role `ca_admin`, challenge-signature login, list/detail/revoke/audit | Tốt |
| Admin Bank | Activate, AS/TGS/AP login, admin session, dashboard, audit | Tốt |
| Admin SOC | KDC audit, timeline, verify hash-chain, summary, export | Tốt |
| DevOps/demo | Compose, env template, cert scripts, seed, smoke scripts, guide | Khá |

## 4. Luồng Đăng Ký Customer

```mermaid
sequenceDiagram
    autonumber
    participant U as Browser
    participant G as API Gateway / RA
    participant R as Redis
    participant CA as CA Service
    participant B as Banking Service

    U->>G: POST /v1/otp/request(email)
    G->>R: Lưu OTP TTL
    G-->>U: OTP sent
    U->>G: POST /v1/otp/verify(email, otp)
    G->>R: Verify + xóa OTP
    G-->>U: reg_token(JWT, jti, owner_id)
    U->>U: Sinh RSA keypair + CSR
    U->>G: POST /v1/auth/register(CSR, Bearer reg_token)
    G->>R: Kiểm tra jti chưa dùng
    G->>B: CheckUserEmail(email)
    B-->>G: exists=false
    G->>CA: RegisterUser(CSR, owner_id, email, role=customer)
    CA-->>G: certificate + chain metadata
    G->>B: CreateUser(owner_id, email, fullName)
    B-->>G: user/account created
    G->>R: Mark jti used
    G-->>U: cert_pem, cert_serial
    U->>U: Verify cert chain/key/CN/SAN rồi lưu IndexedDB
```

Control chính:

- OTP TTL, single-use registration token.
- CSR proof-of-possession.
- `owner_id` do Gateway sinh, không lấy từ client.
- Pre-check email trước khi cấp cert.
- Nếu Bank create user fail sau khi CA cấp cert, Gateway revoke cert với reason `registration_rollback`.
- Client verify certificate trước khi lưu để chống Gateway/MITM trả cert giả hoặc cert của key khác.

## 5. Luồng Đăng Nhập AS Exchange

```mermaid
sequenceDiagram
    autonumber
    participant U as Browser
    participant G as API Gateway
    participant K as KDC Service
    participant CA as CA Service
    participant R as Redis

    U->>U: Mở private key bằng PIN
    U->>U: Tạo AS_REQ(owner_id, cert_sn, nonce, timestamp)
    U->>U: Ký AS_REQ bằng private key
    U->>G: POST /v1/auth/as-req
    G->>K: RequestTGT + x-request-id
    K->>R: Replay check nonce/timestamp
    K->>CA: VerifyCertificate(cert_sn)
    CA-->>K: public key, owner_id, role, status
    K->>K: Verify signature + bind owner_id
    K->>K: Sinh K_c_tgs, TGT
    K-->>G: AS_REP hybrid encrypted + kdc_signature + KDC cert chain
    G-->>U: AS_REP
    U->>U: Verify KDC chain về Root CA
    U->>U: Verify kdc_signature
    U->>U: Giải mã, lưu TGT + K_c_tgs trong RAM
```

Control chính:

- AS request có nonce/timestamp và replay marker.
- KDC verify chữ ký client bằng public key từ CA.
- `owner_id` phải khớp certificate owner.
- AS_REP dùng hybrid encryption: AES-GCM payload + RSA-OAEP wrap AES key.
- Client verify `kdc_signature` và certificate chain của KDC. Đây là điểm mới từ `review-v3`, đóng lỗ hổng client nhận AS_REP mà không xác minh KDC.

## 6. Luồng TGS Exchange

```mermaid
sequenceDiagram
    autonumber
    participant U as Browser
    participant G as API Gateway
    participant K as KDC Service
    participant CA as CA Service
    participant R as Redis

    U->>U: Tạo Authenticator = E_K_c_tgs(client, nonce2, ts, request_id, scope)
    U->>G: POST /v1/auth/tgs-req(TGT, authenticator, scope)
    G->>K: RequestServiceTicket + x-request-id
    K->>K: Decrypt TGT bằng K_tgs
    K->>K: Decrypt authenticator bằng K_c_tgs
    K->>R: Replay check
    K->>CA: VerifyCertificate(cert_sn)
    CA-->>K: status, role, chain metadata
    K->>K: Check role -> scope + service allowlist
    K->>K: Sinh K_c_v và Ticket_v
    K-->>G: TGS_REP = E_K_c_tgs(K_c_v, Ticket_v, nonce2, scope)
    G-->>U: TGS_REP
    U->>U: Decrypt, check nonce2/scope, lưu Ticket_v RAM
```

Control chính:

- TGT chỉ KDC đọc được vì mã hóa bằng `K_tgs`.
- Authenticator chỉ client/KDC đọc được vì mã hóa bằng `K_c_tgs`.
- Role/scope check:
  - `customer`: `balance:read`, `history:read`, `transfer:create`.
  - `bank_admin`: `bank-admin:read`.
- Service ticket `Ticket_v` chỉ Bank đọc được vì mã hóa bằng `K_v`.

## 7. Luồng AP / Banking Transfer

```mermaid
sequenceDiagram
    autonumber
    participant U as Browser
    participant G as API Gateway
    participant B as Banking Service
    participant CA as CA Service
    participant DB as Bank DB
    participant R as Redis

    U->>U: Tạo payload transfer canonical
    U->>U: Ký payload bằng RSA-PSS
    U->>U: Authenticator = E_K_c_v(client, nonce3, ts, request_id)
    U->>U: CipherPayload = E_K_c_v(payload + signature)
    U->>G: POST /v1/bank/transfer(Ticket_v, Authenticator, CipherPayload)
    G->>B: TransferMoney + x-request-id
    B->>B: Decrypt Ticket_v bằng K_v
    B->>B: Decrypt Authenticator bằng K_c_v
    B->>CA: VerifyCertificate(cert_sn)
    CA-->>B: public key, status, owner, issuer, chain
    B->>B: Check cert active + owner + chain metadata
    B->>R: Replay check nonce/request_id
    B->>B: Verify RSA-PSS signature
    B->>DB: Serializable transaction, lock accounts
    B->>DB: Validate balance, ownership, daily limit, idempotency
    B->>DB: Insert completed/failed transaction + ledger hash
    B-->>G: AP_REP = E_K_c_v(result, nonce, tx_id)
    G-->>U: Result
```

Control chính:

- Payload được mã hóa bằng AES-GCM với `K_c_v`.
- Payload được ký bằng RSA-PSS để chống sửa lệnh/chối bỏ.
- Bank verify cert lại với CA, không tin public key từ ticket/request nếu không khớp CA.
- Replay protection bằng nonce/timestamp/request id, Redis marker và DB fallback.
- Idempotency key tránh thực thi lại giao dịch đã completed.
- Failed transfer cũng ghi transaction/audit để có evidence.

Lưu ý từ `review-v3`: AP_REP hiện là phản hồi đối xứng mã hóa bằng `K_c_v`, client chưa verify chữ ký Bank. Sau khi AS_REP verify đã được bổ sung, rủi ro giảm đáng kể, nhưng hướng phát triển nên cấp signing cert cho Bank và ký AP_REP để có mutual authentication đầy đủ ở chiều Bank -> client.

## 8. Luồng Admin CA Và Admin Bank

```mermaid
sequenceDiagram
    autonumber
    participant A as Admin Browser
    participant G as API Gateway
    participant R as Redis
    participant CA as CA Service
    participant K as KDC Service
    participant B as Banking Service

    A->>A: Sinh keypair + CSR
    A->>G: POST /v1/admin-ca/activate hoặc /v1/admin/bank/activate
    G->>R: Kiểm tra activation token
    G->>CA: RegisterUser(role=ca_admin hoặc bank_admin)
    CA-->>G: Admin certificate
    G-->>A: cert_pem, cert_serial

    alt Admin CA login
        A->>A: Ký challenge admin-ca-login bằng private key
        A->>G: POST /v1/admin-ca/session
        G->>CA: VerifyCertificate(cert_serial)
        CA-->>G: active + role=ca_admin + public key
        G->>G: Verify challenge signature
        G-->>A: JWT admin-ca session
    else Admin Bank login
        A->>K: AS/TGS với scope bank-admin:read qua Gateway
        K-->>A: Ticket_v cho Bank
        A->>B: CreateAdminSession bằng AP exchange qua Gateway
        B-->>A: Bank admin session cookie
    end
```

Control chính:

- Admin certificate có role riêng: `ca_admin`, `bank_admin`.
- Admin CA login dùng challenge-signature, cert phải active và đúng role.
- Admin Bank đi qua AS/TGS/AP với scope `bank-admin:read`.
- Dashboard Admin Bank và SOC chỉ đọc dữ liệu qua session/token hợp lệ.

Lưu ý từ `review-v3`: client-side verify cert trước khi lưu đã áp dụng cho customer registration; với admin activation nên đồng bộ cùng cơ chế để giảm rủi ro nhận cert sai trước khi lưu.

## 9. Luồng SOC Audit Và Hash-Chain

```mermaid
sequenceDiagram
    autonumber
    participant S as Admin SOC
    participant G as API Gateway
    participant CA as CA Service
    participant K as KDC Service
    participant B as Banking Service

    S->>G: GET /v1/admin/audit/timeline?request_id=operation_id
    G->>CA: List CA audit by request_id
    G->>K: List KDC audit by request_id
    opt có bank_admin_session
        G->>B: List Bank audit by request_id
    end
    G->>G: Merge + sort + enrich severity/category
    G-->>S: Cross-service timeline

    S->>G: GET /v1/admin/audit/verify
    G->>CA: VerifyAuditChain
    G->>K: VerifyAuditChain
    opt có bank_admin_session
        G->>B: VerifyAuditChain
    end
    G-->>S: ok/broken_seq/broken_id/detail
```

Control chính:

- CA/KDC/Bank audit đều có hash-chain `prev_hash` -> `hash`.
- SOC verify phát hiện sửa/xóa/đảo event ở giữa chuỗi.
- Timeline dùng `operation_id` / `X-Request-ID` để nối sự kiện cross-service.

Giới hạn:

- Hash-chain chưa có external anchor tự động.
- Chưa chống tail truncation tuyệt đối nếu attacker xóa đoạn cuối và không có checkpoint ngoài.
- Audit insert là best-effort.

## 10. Tổng Kết Theo Checklist Tiêu Chí

### Cơ Bản

| Tiêu chí | Cơ chế hiện tại | Đánh giá |
|---|---|---|
| Mã hóa dữ liệu bằng symmetric encryption | AES-256-GCM cho TGT, Ticket_v, authenticator, AS/TGS/AP payload | Tốt |
| Hybrid encryption hoặc KDC cơ bản | AS_REP hybrid encryption; KDC sinh `K_c_tgs`, `K_c_v` | Tốt |
| Key lifecycle | Browser sinh keypair; CA cấp cert có hạn; KDC session key có TTL; cert/key provisioning scripts | Tốt |
| Identification + verification | OTP, cert serial, owner_id, CSR PoP, AS signature, PIN mở private key | Tốt |
| Chống replay | nonce/timestamp/challenge-response, Redis SET NX, DB fallback | Tốt |
| Xác thực nguồn public key | Public key lấy từ CA/certificate chain, không lấy raw từ request | Tốt |

### Mức Khá

| Tiêu chí | Cơ chế hiện tại | Đánh giá |
|---|---|---|
| Tách master key/session key | `K_tgs`, `K_v` tách với `K_c_tgs`, `K_c_v` | Tốt |
| KDC/KMS tập trung | KDC quản lý AS/TGS; CA quản lý public key/cert repository | Tốt |
| Mutual authentication client-server | Client chứng minh bằng cert/private key; KDC response có signature; Bank AP_REP đối xứng. Chưa full mTLS/AP_REP signature | Khá |
| Phân quyền dựa trên identity | Certificate role + KDC scope + Bank ownership + Gateway middleware | Tốt |
| X.509 certificate | Customer, `bank_admin`, `ca_admin`, service TLS cert | Tốt |
| Revocation | CA revoke/status; KDC/Bank reject cert không active | Tốt |
| Chống MITM khi trao đổi public key | Trust anchor Root CA, cert chain, AS_REP KDC signature, gRPC TLS trust bundle | Tốt |

### Nâng Cao

| Tiêu chí | Cơ chế hiện tại | Đánh giá |
|---|---|---|
| PKI có CA, RA, repository, cấp/thu hồi cert | CA Service + Gateway RA + CA repository + Admin CA revoke/audit | Tốt |
| Certificate chain validation | Client verify issued cert; Bank check cert type/issuer/chain metadata | Tốt |
| Kerberos-like ticketing/SSO | AS/TGS/AP cho customer và Bank Admin | Tốt |
| Audit log cho cấp khóa/cert/auth/resource access | CA/KDC/Bank audit + SOC timeline/verify/export | Tốt |

## 11. Tình Huống Tấn Công Và Cơ Chế Bảo Vệ

| Tấn công/rủi ro | Cơ chế bảo vệ hiện tại | Ghi chú |
|---|---|---|
| Replay AS/TGS/AP | nonce, timestamp, request id, Redis SET NX, DB fallback | Đã có trong code |
| MITM/public-key substitution | Root CA trust anchor, cert chain, CA `VerifyCertificate`, AS_REP KDC signature | Cập nhật quan trọng từ `review-v3` |
| Giả mạo `owner_id` | AS bind owner_id request với owner trong cert | Fail-closed |
| Cert bị revoke vẫn dùng | CA status revoked; KDC/Bank reject cert không active | Cơ chế tương đương CRL trong demo |
| Sai role/scope | KDC role-to-scope, service ticket scoped, Gateway middleware | Chặn customer vào admin |
| Unauthorized account access | Bank ownership check bằng `caller.ClientID` | Ghi `forbidden_ownership` audit |
| Sửa/xóa audit log | Hash-chain verify CA/KDC/Bank | Chưa có external anchor |
| Duplicate registration | Bank `CheckUserEmail`, rollback revoke nếu Bank fail | Giảm lệch CA/Bank |
| Brute-force OTP | OTP TTL, max attempts, cooldown, rate limit | Demo có thể disable rate limit |
| Stolen browser private key/PIN | Private key wrap bằng PIN, không gửi server | Chưa hardware-backed |
| Gateway HTTP khi deploy thật | Cần HTTPS/HSTS | Demo local đang HTTP |
| Internal caller spoofing | gRPC TLS server-auth + network isolation | Chưa full mTLS |
| XSS lạm dụng phiên browser | Cần CSP/security headers | Khuyến nghị thêm |

## 12. Giới Hạn Và Hướng Phát Triển

| Mức ưu tiên | Nội dung | Đánh giá |
|---|---|---|
| P0 | Bật HTTPS/HSTS cho Gateway khi deploy ngoài local | Yếu |
| P1 | Đồng bộ client-side verify cert cho Admin CA/Bank activation | Khá |
| P1 | Ký AP_REP bằng Bank signing cert | Khá |
| P2 | CRL/OCSP khi verify chain runtime | Yếu |
| P2 | mTLS nội bộ giữa Gateway và services | Khá |
| P2 | CSP/security headers cho frontend/gateway | Yếu |
| P3 | Chính sách độ mạnh PIN | Khá |
| P3 | External audit anchor/checkpoint | Khá |