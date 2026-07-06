# Flow: OTP & PKI Registration

Sơ đồ luồng cho `blueprint/api-design/01-otp-pki-registration.md` — đăng ký khách hàng mới gồm 3 endpoint: OTP request → OTP verify → PKI register.

---

## 1. Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    actor U as Khách hàng
    participant W as Customer Web App<br/>(browser)
    participant GW as API Gateway
    participant R as Redis
    participant MAIL as Email Provider
    participant CA as CA Service
    participant CADB as CA DB
    participant BK as Bank Service
    participant BKDB as Bank DB

    %% ============ PHA 1: OTP REQUEST ============
    rect rgb(235, 245, 255)
    note over U,MAIL: PHA 1 — OTP Request  (POST /v1/otp/request)
    U->>W: Nhập email
    W->>GW: POST /otp/request {email}
    GW->>R: INCR rate:otp_request:{ip} (EXPIRE 60s)
    alt > 5 request/phút
        GW-->>W: 429 RATE_LIMITED
    else trong giới hạn
        GW->>GW: Sinh OTP 6 chữ số
        GW->>R: SET otp:{email} <otp> EX 300
        GW->>MAIL: Gửi email chứa OTP
        GW-->>W: 200 {message, expires_in:300}
    end
    end

    %% ============ PHA 2: OTP VERIFY ============
    rect rgb(235, 255, 240)
    note over U,GW: PHA 2 — OTP Verify  (POST /v1/otp/verify)
    U->>W: Nhập OTP
    W->>GW: POST /otp/verify {email, otp}
    GW->>R: GET otp:{email}
    alt OTP sai / hết hạn
        GW-->>W: 400 INVALID_OTP
    else OTP khớp
        GW->>R: DEL otp:{email}  (one-time use)
        GW->>GW: Tạo registration_token (JWT, 1 lần, TTL 10')
        GW-->>W: 200 {registration_token, expires_in:600}
    end
    end

    %% ============ PHA 3: PKI ENROLLMENT ============
    rect rgb(255, 248, 235)
    note over U,BKDB: PHA 3 — PKI Enrollment  (POST /v1/pki/register)

    note over W: Sinh RSA key pair (WebCrypto, private key extractable:false)<br/>Tạo CSR + ký proof-of-possession<br/>Lưu WRAPPED private key vào IndexedDB
    U->>W: Đặt PIN (để wrap private key)
    W->>W: Tạo csr_pem, wrap privKey, lưu IndexedDB
    W->>GW: POST /pki/register {csr_pem, registration_token}

    GW->>GW: Verify registration_token<br/>(chữ ký, chưa dùng, chưa hết hạn)
    alt token sai / đã dùng
        GW-->>W: 401 INVALID_REGISTRATION_TOKEN
    else token hợp lệ
        GW->>CA: gRPC RegisterUser(csr_pem, user_id)
        CA->>CA: Verify CSR proof-of-possession
        alt CSR PoP sai
            CA-->>GW: INVALID_CSR
            GW-->>W: 400 INVALID_CSR
        else đã có active cert
            CA-->>GW: ErrActiveCertificateExists
            GW-->>W: 409 ACTIVE_CERT_EXISTS
        else hợp lệ
            CA->>CA: Client CA ký X.509 từ CSR
            CA->>CADB: INSERT certificates (status=active)
            CA->>CADB: INSERT certificate_audit_log (action=issued)
            CA-->>GW: {certificate_pem, serial_number, not_after_unix}

            GW->>BK: gRPC CreateUser(user_id, email, full_name)
            alt CreateUser thành công
                BK->>BKDB: INSERT users (status=active, email UNIQUE)
                BK-->>GW: ok
                GW-->>W: 201 {certificate_pem, serial_number, not_after}
                W->>W: Lưu certificate_pem vào IndexedDB<br/>(cạnh wrapped private key)
                W->>W: Xóa plaintext private key + PIN khỏi RAM
            else CreateUser thất bại
                BK-->>GW: error
                GW->>CA: revoke/mark cert (reason=enrollment_failed)
                CA->>CADB: UPDATE certificates status=revoked
                GW-->>W: 503 SERVICE_UNAVAILABLE
            end
        end
    end
    end
```

---

## 2. Chú thích Key & Cert — nội dung chứa gì

### 2.1. Khóa sinh ở browser (không bao giờ rời browser dạng plaintext)

| Thành phần | Loại | Nội dung / Cấu trúc | Lưu ở đâu |
|---|---|---|---|
| `privKeyRSA_c` | RSA private key (client) | Khóa bí mật của khách hàng. Sinh bằng WebCrypto với `extractable: false`. **Không bao giờ gửi lên server.** Dùng để: (1) ký CSR (proof-of-possession), (2) sau này ký AS_REQ và ký giao dịch | **Wrapped** trong IndexedDB (bọc bằng khóa dẫn xuất từ PIN) |
| `pubKeyRSA_c` | RSA public key (client) | Khóa công khai tương ứng. Được nhúng trong CSR và sau đó trong X.509 certificate do Client CA ký. CA/KDC/Bank dùng để verify chữ ký của client | Công khai — trong CSR & certificate |
| PIN | Secret người dùng | Dùng để **wrap/unwrap** `privKeyRSA_c`. Không lưu — chỉ tồn tại tạm trong RAM, xóa sau khi dùng | RAM (tạm thời) |

### 2.2. CSR — Certificate Signing Request (`csr_pem`)

Định dạng PEM (`-----BEGIN CERTIFICATE REQUEST-----`). Chứa:

| Trường | Ý nghĩa |
|---|---|
| `Subject` | Thông tin định danh: CN (full name/email), email |
| `Public Key` | `pubKeyRSA_c` — khóa công khai của khách hàng |
| `Signature` | **Proof-of-Possession**: chữ ký trên chính CSR bằng `privKeyRSA_c`, chứng minh client sở hữu private key tương ứng với public key trong CSR |

CA verify chữ ký này khớp public key trong CSR trước khi cấp cert.

### 2.3. X.509 Certificate (`certificate_pem`) — Client CA cấp

Định dạng PEM (`-----BEGIN CERTIFICATE-----`), được Client CA ký bằng `client-ca.key`. Client CA là Intermediate CA do Root CA ký. Chứa:

| Trường | Ý nghĩa |
|---|---|
| `serial_number` | Hex-encoded serial X.509 — **key chính** để KDC/Bank lookup & revocation check |
| `Subject` (CN, email) | Định danh khách hàng (`subject_cn`, `subject_email`) |
| `Public Key` | `pubKeyRSA_c` — public key của khách hàng |
| `Issuer` | Client CA của hệ thống |
| `not_before` / `not_after` | Cửa sổ hiệu lực certificate |
| `Signature` | Chữ ký của Client CA đảm bảo tính toàn vẹn và chain về Root CA |
| `fingerprint_sha256` | SHA-256 fingerprint (lưu ở CA DB) cho tra cứu nhanh |

> `owner_id` trong CA DB = `users.id` trong Bank DB = `ID_c` dùng trong ticket sau này. Liên kết lỏng (VARCHAR), không FK cứng.

### 2.4. Root CA và Client CA

| Thành phần | Nội dung |
|---|---|
| `root-ca.key` | Khóa bí mật của Root CA, là trust anchor cao nhất. Chỉ dùng để ký Intermediate CA, không ký trực tiếp user cert trong runtime bình thường |
| `client-ca.key` | Khóa bí mật của Client CA, nằm trong CA Service hoặc secret store. Dùng để ký user/client certificate từ CSR |
| `client-ca.crt` | Intermediate CA certificate do Root CA ký. KDC/Bank tin user cert vì chain là Root CA → Client CA → `X.509_c` |

### 2.5. `registration_token`

| Thành phần | Nội dung |
|---|---|
| `registration_token` | JWT ngắn hạn (TTL 10 phút), **dùng 1 lần**. Phát hành sau khi verify OTP thành công; bị vô hiệu hóa ngay sau khi `/pki/register` thành công. Liên kết phiên OTP với phiên enrollment (chứa email/user_id đã xác minh) |

---

## 3. Ràng buộc bảo mật chính

- **Private key không rời browser** ở bất kỳ bước nào — server chỉ nhận CSR (chứa public key + chữ ký PoP).
- **`registration_token` dùng 1 lần** — vô hiệu ngay sau khi register thành công.
- **OTP TTL 5 phút**, xóa khỏi Redis ngay sau verify thành công.
- **Một user tối đa 1 cert `active`** tại một thời điểm (partial unique index `owner_id WHERE status=active` trong CA DB).
- **User record Bank DB chỉ tạo sau khi CA xác nhận cấp cert thành công**; nếu `CreateUser` fail → revoke cert vừa cấp (`enrollment_failed`) để tránh cert active mồ côi.
- **Không information leakage**: lỗi OTP không phân biệt "sai" vs "hết hạn" — chỉ trả `INVALID_OTP`.
