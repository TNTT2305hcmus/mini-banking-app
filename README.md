# MINI APP BANKING

---

# Overview

Hệ thống triển khai mô hình xác thực bảo mật nhiều lớp kết hợp:

* OTP Verification
* Public Key Infrastructure (PKI)
* X.509 Certificate
* Kerberos-like Authentication Flow
* Digital Signature
* Session Key Encryption
* Scope-based Authorization
* Hash Chaining

## Kiến trúc gồm các thành phần

| Thành phần       | Công nghệ               | Vai trò                  |
| ---------------- | ----------------------- | ------------------------ |
| **Client**       | React + Typescript      | Giao diện người dùng     |
| **Gateway**      | Node.js + Typescript    | API Gateway / DMZ        |
| **CA Service**   | Go                      | Certificate Authority    |
| **KDC**          | Authentication Server   | Cấp Ticket & Session Key |
| **Bank Service** | Backend Banking Service | Xử lý giao dịch          |
| **Redis**        | In-memory DB            | Lưu OTP tạm thời         |
| **PostgreSQL**   | Database                | Lưu dữ liệu giao dịch    |

---

# Storage Lifecycle

Vòng đời và phạm vi lưu trữ của từng loại dữ liệu được hệ thống quy định nghiêm ngặt:

## Phía Trình Duyệt (Client-Side)

* **Mã PIN:**
  Lưu dưới dạng mảng byte có thể thay đổi (`Uint8Array`). Chỉ tồn tại cục bộ trong khối lệnh, bị ghi đè thành số 0 ngay sau khi giải mã xong khóa (*Zeroing / Memory Wiping*).

* **Khóa bí mật Client (`privKeyRSA_c`):**
  Lưu bền vững trong trình duyệt (IndexedDB) dưới dạng Wrapped Key (mã hóa bằng AES dẫn xuất từ PIN). Khi nạp vào RAM, sử dụng WebCrypto API với `extractable: false`, ngăn chặn hoàn toàn việc trích xuất khóa gốc.

* **Khóa công khai của KDC (`pubKeyRSA_KDC`):**
  Được hardcode sẵn trong mã nguồn Client (Public data, chỉ dùng để xác minh, không cần giấu).

* **Registration Token (JWT)**
  Lưu trữ tạm thời trong RAM. Bị hủy bỏ ngay khi Phase 1 (Đăng ký PKI) hoàn tất. Không bao giờ lưu JWT này vào LocalStorage để tránh bị lạm dụng cấp lại chứng chỉ

* **Chứng chỉ X.509 (X.509_pem):**
  Lưu trong IndexedDB/LocalStorage. Đây là dữ liệu công khai (Public Data). Không cần bảo vệ tính bí mật, nhưng cần bảo vệ tính toàn vẹn (tự động verify chữ ký của CA/KDC khi sử dụng).

* **Khóa phiên (K_{c,tgs}, K_{c,v}) và Vé (TGT, Ticket_v):**
  Chỉ lưu trong bộ nhớ RAM tạm thời (Session state của React). Tự động bị dọn dẹp khi đóng tab hoặc khi TGT hết hạn.

## Phía Máy Chủ (Server-Side)
* **Các Khóa Chủ (Master Keys: privKeyRSA_ca, privKeyRSA_KDC, K_tgs, K_v):**
  Lưu trữ trong Dịch vụ Quản lý Khóa (KMS - Key Management Service)

* **Bộ đệm In-memory (Redis):**
  * **OTP:** Ràng buộc bởi Time-To-Live (TTL) rất ngắn (2-5 phút).
  * **Replay Cache (KDC):** Lưu `Nonce` của các Request trong 5-10 phút để chặn Replay Attack.
  * **Revocation Cache:** Bank Service cache kết quả OCSP/Revocation của CA Service (TTL ~3 phút) để giảm tải cho CA.

* **Nhật ký Kiểm toán & Lịch sử Giao dịch (Audit Logs & Ledger)**
  Lưu trong PostgreSQL (Banking DB). Thiết kế theo chuẩn **Immutable Ledger** bằng cơ chế **Hash Chaining**. Mỗi bản ghi chứa `previous_hash`, gán chặt với chữ ký số của Client, ngăn chặn hoàn toàn việc Admin hoặc kẻ tấn công can thiệp chỉnh sửa dữ liệu quá khứ.
  
* **Lưu ý về Ticket:**
  KDC hoạt động **Stateless**. Các vé `TGT` và `Ticket_v` có vòng đời cực ngắn (5-30 phút), chỉ được truyền qua lại trên mạng và lưu tại RAM của Client, hoàn toàn KHÔNG lưu trong Database của KDC.

---

# Secure Banking Authentication Flow

```mermaid
sequenceDiagram
    autonumber

    box rgb(230,240,255) Client
        participant C as React
    end

    box rgb(255,240,230) DMZ
        participant G as Node(Gateway)
    end

    box rgb(240,255,230) Internal
        participant CA as CA(Go)
        participant KDC
        participant B as Bank
        participant DB as Postgres
    end

    %% ==========================================
    %% PHASE 1
    %% ==========================================

    rect rgb(240,240,240)
    Note over C,CA: 1. OTP & PKI REGISTRATION
    C->>G: POST /otp/request {email}
    G->>G: Gen OTP, Save Redis, Send Email

    C->>G: POST /otp/verify {email, OTP}
    G->>C: Verify OTP in Redis -> Returns Reg_Token (JWT)

    C->>C: Gen RSA(pub_c, priv_c). Gen CSR(pub_c), Sign(CSR, priv_c)

    C->>G: POST /pki/register {CSR, Reg_Token}
    G->>G: Verify Reg_Token (JWT)

    G->>CA: gRPC RegisterUser(CSR)

    CA->>CA: Verify CSR Signature via pub_c. Sign -> X.509
    CA-->>C: Returns X.509 (via G)
    end

    %% ==========================================
    %% PHASE 2 (UPDATED: PRE-AUTHENTICATION)
    %% ==========================================

    rect rgb(255,250,240)
    Note over C,KDC: 2. AS EXCHANGE (PRE-AUTHENTICATION)
    C->>C: PreAuth_Sig = Sign({ID_c, ID_tgs, Nonce1, TS1}, priv_c)
    C->>G: POST /auth/as-req {ID_c, ID_tgs, Nonce1, TS1, cert_sn, PreAuth_Sig}
    G->>KDC: gRPC RequestTGT

    KDC->>CA: gRPC GetCertificate(cert_sn)
    CA-->>KDC: Returns pub_c

    KDC->>KDC: Verify PreAuth_Sig via pub_c. Check Nonce1 against Redis Replay Cache.
    KDC->>KDC: Gen K_{c,tgs}. TGT=E_{K_tgs}[ID_c, K_{c,tgs}, expiry] 

    KDC-->>C: AS_REP=E_{pub_c}[K_{c,tgs}, TGT, Nonce1] (via G)

    C->>C: Decrypt AS_REP via priv_c. Zeroing PIN.
    end

    %% ==========================================
    %% PHASE 3 (UPDATED: ADD SCOPE)
    %% ==========================================

    rect rgb(240,250,255)
    Note over C,KDC: 3. TGS EXCHANGE (EMBED SCOPE & pub_c)
    C->>C: Auth_c=E_{K_{c,tgs}}[ID_c, TS3, Nonce2]

    C->>G: POST /auth/tgs-req {service_id, scope, TGT, Auth_c, cert_sn, Nonce2}
    G->>KDC: gRPC RequestServiceTicket

    KDC->>KDC: Verify TGT/Auth_c. Check Nonce2 in Replay Cache.
    KDC->>KDC: Gen K_{c,v}. Ticket_v=E_{K_v}[ID_c, scope, K_{c,v}, pub_c_pem, expiry]
    
    KDC-->>C: TGS_REP=E_{K_{c,tgs}}[K_{c,v}, Ticket_v, Nonce2] (via G)
    end

    %% ==========================================
    %% PHASE 4 (UPDATED: DEEP AUTHZ & HASH CHAIN)
    %% ==========================================

    rect rgb(255,240,240)
    Note over C,DB: 4. AP EXCHANGE & SECURE TRANSACTION

    C->>C: Payload = {from, to, amount}
    C->>C: Sig_c = Sign(Payload, priv_c)
    C->>C: Auth_c = E_{K_{c,v}}[ID_c, TS5, Nonce3]
    C->>C: Cipher = E_{K_{c,v}}[Payload, Sig_c]

    C->>G: POST /bank/transfer {Ticket_v, Auth_c, Cipher}
    G->>B: gRPC TransferMoney

    B->>B: Decrypt Ticket_v via K_v -> extracts K_{c,v}, pub_c, ID_c, scope
    B->>B: Decrypt Auth_c & Cipher via K_{c,v}
    
    B->>CA: gRPC CheckRevocation(cert_sn) (Strict Revocation Check)
    CA-->>B: Returns status (Valid)

    B->>B: Verify Sig_c via pub_c
    B->>B: Authorization: Check Scope, ID_c Ownership, Daily Limits, Account Status

    B->>DB: Get previous_hash. Hash_n = SHA-256(prev_hash + payload + Sig_c)
    B->>DB: Execute Transaction (ACID) + Append Hash Chaining Log

    B-->>C: AP_REP=E_{K_{c,v}}[TS5+1, Result] (via G)
    end
```

---

# PHASE 1: OTP & PKI REGISTRATION

## Mục tiêu của giai đoạn

Đảm bảo định danh người dùng thông qua xác thực Email (OTP) và thiết lập định danh an toàn.

Hệ thống áp dụng nguyên tắc **Zero-Knowledge** trong việc khởi tạo khóa:

> Khóa bí mật `privKeyRSA_c` được sinh ra và lưu trữ độc lập tại thiết bị của người dùng thông qua WebCrypto API, máy chủ hoàn toàn không có khả năng can thiệp hay sao chép khóa này.

---

## Quy trình Xử lý Chi tiết

### Bước 1: Yêu cầu mã xác thực (Request OTP)

* **Thao tác:**
  Người dùng nhập Email để tạo tài khoản thông qua xác thực OTP.


* **Xử lý tại Gateway:**
  Node.js sinh mã OTP ngẫu nhiên, lưu vào Redis:

| Thành phần | Giá trị     |
| ---------- | ----------- |
| Key        | Email       |
| Value      | OTP         |
| TTL        | Có thời hạn |

Sau đó gọi API gửi OTP về email của người dùng.

### Bước 2: Xác minh OTP (Verify OTP)

* **Thao tác:**
  Người dùng nhập xác nhận OTP.

* **Xử lý tại Gateway:**
  Node.js truy vấn Redis để:

1. Đối chiếu OTP
2. Kiểm tra thời hạn

Nếu hợp lệ, Gateway trả về Registration Token (JWT).

### Bước 3: Khởi tạo Cặp khóa & Yêu cầu ký chứng chỉ

#### Tạo khóa

Người dùng thiết lập mã PIN cục bộ.

React App dùng Web Crypto API sinh cặp khóa:

* `pubKeyRSA_c`
* `privKeyRSA_c`

#### Tạo và ký CSR

Client tạo:

```text
Certificate Signing Request (CSR)
```

CSR chứa:

* `ID_c (username, email)`
* `pubKeyRSA_c`

Người dùng tiến hành ký CSR để chứng minh tính sở hữu

```text
Sign(CSR, privKeyRSA_c)
```

#### Gửi yêu cầu

Client tiến hành gửi CSR_pem và JWT lên Server để xin cấp chứng chỉ X509.

### Bước 4: Thẩm định và Cấp phát Chứng chỉ X.509

#### Gateway

* Xác thực JWT
* Forward CSR sang CA Service qua gRPC

#### CA Service (Proof of Possession)

CA thực hiện:

1. Bóc tách CSR lấy `pubKeyRSA_c`
2. Verify chữ ký trên CSR
3. Xác minh Client thực sự sở hữu `privKeyRSA_c`

#### Đúc chứng chỉ

CA dùng:

```text
privKeyRSA_ca
```

để ký điện tử và tạo chứng chỉ:

```text
X.509
```

#### Hoàn tất

CA trả:

```text
X.509_pem
```

về Gateway → Gateway forward về Client.

Client lưu:

* `X.509_pem`
* `privKeyRSA_c` (đã mã hóa)

để dùng cho các phiên đăng nhập tiếp theo.

---

# PHASE 2: INITIAL AUTHENTICATION (AS EXCHANGE)

## Mục tiêu của giai đoạn

Đảm bảo Client có thể lấy:

* Ticket-Granting Ticket (`TGT`)
* Session Key (`K_{c,tgs}`)

Hệ thống tận dụng PKI từ Phase 1 để bảo mật việc phân phối khóa.

## Quy trình Xử lý Chi tiết

### Bước 1: Client khởi tạo yêu cầu (AS_REQ)

Client tiến hành gửi as-req với nội dung Payload

Payload:

| Trường    | Ý nghĩa               |
| --------- | --------------------- |
| `ID_c`    | Định danh người dùng  |
| `ID_tgs`  | Định danh dịch vụ TGS |
| `Nonce_1` | Chống Replay Attack   |
| `TS_1`    | Timestamp hiện tại    |

Client tiến hành dùng `privKeyRSA_c` để ký lên AS_REQ (Pre-authentication)


### Bước 2: KDC xử lý và đóng gói AS_REP

Gateway forward request sang KDC qua gRPC.

#### Định danh & Sinh khóa

KDC:

1. Tra cứu `ID_c`
2. Lấy chứng chỉ X.509
3. Trích xuất `pubKeyRSA_c`
4. Xác thực chữ ký của client
5. Kiểm tra Nonce_1 trong Redis (Nếu Nonce_1 tồn tại > 5 phút -> Từ chối)
6. Sinh khóa:

```text
K_{c,tgs}
```

#### Tạo TGT

TGT chứa:

```text
{
  ID_c,
  K_{c,tgs},
  Lifetime
}
```

Được mã hóa:

```text
E_{K_tgs}[ ID_c || K_{c, tgs} || Lifetime]
```

#### Ký số

KDC gom:

```text
{ K_{c,tgs}, TGT, Nonce_1 }
```

và ký bằng:

```text
privKeyRSA_KDC
```

#### Mã hóa bảo mật

Toàn bộ dữ liệu được mã hóa tiếp bằng:

```text
pubKeyRSA_c
```

Kết quả:

```text
AS_REP = E_{pubKeyRSA_c}[ Signed_Data ]
```

### Bước 3: Client giải mã AS_REP & Zeroing Memory

#### Giải mã

Người dùng nhập PIN.

PIN (`Uint8Array`) được dùng để:

* unwrap `privKeyRSA_c`
* giải mã `AS_REP`

#### Xác minh

Client:

1. Dùng `pubKeyRSA_KDC` verify chữ ký
2. Kiểm tra `Nonce_1`
3. Kiểm tra `TS_1`

#### Session & Memory Wiping

Nếu hợp lệ:

* Lưu `K_{c,tgs}`
* Lưu `TGT`

vào Session Memory.

Ngay sau đó:

```text
Memory Zeroing
```

* Ghi đè PIN bằng `0x00`
* Ghi đè plaintext private key bằng `0x00`

---

# PHASE 3: TGS EXCHANGE (XIN VÉ DỊCH VỤ)

## Mục tiêu của giai đoạn

Đây là giai đoạn bản lề trong kiến trúc Kerberos.

Client hiện đang giữ:
1. `K_{c,tgs}`
2. `TGT`

Mục tiêu:
* Xin `Ticket_v` với một **Scope (quyền hạn)** cụ thể.
* Xin `K_{c,v}`
để giao tiếp an toàn với Bank Server.

## Quy trình Xử lý

### Bước 1: Client tạo Authenticator

Client sinh `Auth_c` để:
* Khai báo Scope cụ thể (vd: `transfer:internal`, `balance:read`).
* Chứng minh quyền sở hữu TGT.
* Chống Replay Attack.

#### Sinh nonce

```text
nonce_req = crypto.getRandomValues(16 bytes)
```

---

#### Plaintext

```text
{
  ID_c,
  TS_3,
  nonce_req,
  requested_service = ID_v,
  scope
}

```

#### Mã hóa AES-256-GCM

| Thành phần | Giá trị |
| --- | --- |
| Key | `K_{c,tgs}` |
| IV | random 12 bytes |

---

### Bước 2: Client gửi TGS_REQ

Client tiến hành gửi `TGS_REQ` với nội dung Payload:

* `ID_v` (VD: `bank-service`)
* `TGT` (Opaque blob)
* `Auth_c`
* `cert_serial`

### API Gateway xử lý

Gateway:

1. Validate protobuf schema
2. Rate limit
3. Forward gRPC + mTLS vào internal network

### Bước 3: TGS xử lý và kiểm định

#### Kiểm tra TGT

KDC giải mã TGT bằng `K_tgs`. Lấy ra:

* `K_{c,tgs}`
* `ID_c`
* `TS_tgt`
* `Lifetime_tgt`
*(Lưu ý: Không còn Client_address/IP trong cấu trúc TGT)*

#### Kiểm tra trạng thái sơ bộ

* Kiểm tra hạn của TGT (`Lifetime_tgt`).
* (Tùy chọn) KDC tra cứu nhanh danh sách `cert_serial` bị thu hồi khẩn cấp.

#### Xác minh Auth_c

Giải mã `Auth_c` bằng `K_{c,tgs}`. Verify:

```text
ID_{c_auth} == ID_c (trích xuất từ TGT)

```

và:

```text
requested_service == ID_v

```

#### Freshness & Replay Check

Điều kiện thời gian:

```text
|now - TS_3| < 5 phút

```

Replay detection:

```text
hash(nonce_req + ID_c + TS_3)

```

Lưu hash vào Redis (Replay Cache):

```text
SET(key, "1", EX=300, NX=true)

```

Nếu key đã tồn tại → Phát hiện Replay Attack → Reject.

### Bước 4: TGS cấp Ticket_v

#### Sinh tài nguyên mới

```text
K_{c,v} = crypto.getRandomValues(32 bytes)
```

```text
nonce_2 = crypto.getRandomValues(16 bytes)
```

---

#### Tạo Ticket_v

Mã hóa bằng Master Key của Bank Service (`AES-256-GCM` với key `K_v`). Payload:

```text
{
  ID_c,
  sname = ID_v,
  scope,           // Bank Service cần đọc scope này để Authorization
  TS_4,
  Lifetime_v,      // TTL cực ngắn (VD: 5 phút)
  K_{c,v},
  pubKey_c,
  nonce_req
}

```

#### Tạo TGS_REP

Mã hóa bằng Session Key giữa Client và TGS (`K_{c,tgs}`). Payload:

```text
{
  K_{c,v},
  ID_v,
  TS_4,
  nonce_2,
  nonce_req,
  Ticket_v
}

```

### Bước 5: Client nhận Ticket_v

Client:

1. Giải mã `TGS_REP` bằng `K_{c,tgs}` đang lưu trong bộ nhớ.
2. Verify `nonce_req` (Khớp với giá trị sinh ra ở Bước 1).
3. Lưu `K_{c,v}` vào session state.
4. Lưu `Ticket_v` vào session state.

Lúc này Client đã sẵn sàng thực hiện giao dịch ở Phase 4 với Bank Service.

---

# PHASE 4: AP EXCHANGE (GIAO DỊCH BẢO MẬT & XÁC THỰC HAI CHIỀU)

## Mục tiêu của giai đoạn

Kết hợp:

1. **Kerberos Symmetric Encryption** (Tốc độ cao, mutual authentication)
2. **PKI Digital Signature** (Chống chối bỏ - Non-repudiation)
3. **Deep Authorization & Hash Chaining** (Kiểm soát quyền hạn và Sổ cái bất biến)

## Điều kiện đầu vào

### Client

Đang giữ:

* `K_{c,v}`
* `Ticket_v`

trong RAM (từ Session State).
`privKeyRSA_c` đang được lưu dưới dạng wrapped key trong IndexedDB.

### Bank Server

Nắm giữ:

```text
K_v

```

## Quy trình Xử lý Chi tiết

### Bước 1: Ký số và Mã hóa tại Client

Người dùng nhập:

* Payload chuyển tiền
* PIN

#### Unwrap private key

PIN được dùng để unwrap:

```text
privKeyRSA_c

```

Khóa được đánh dấu:

```text
extractable: false

```

#### Tạo chữ ký số

Client:

1. Hash Payload
2. Ký bằng:

```text
privKeyRSA_c

```

#### Tạo Auth_v

```text
Auth_v = E_{K_{c,v}}[
    ID_c +
    TS_5 +
    Nonce_3
]

```

#### Tạo CipherPayload

```text
CipherPayload = E_{K_{c,v}}[
    Payload +
    Signature
]

```

#### Memory Zeroing

Ngay sau khi ký:

* overwrite PIN
* overwrite plaintext private key

bằng `0x00` (Xóa hoàn toàn khỏi RAM).

#### Gửi request

Client gửi:

```text
{
  Ticket_v,
  Auth_v,
  CipherPayload
}

```

### Bước 2: API Gateway định tuyến

Gateway thực hiện:

* Rate Limiting
* Audit Logging
* Forward sang Bank Service qua gRPC.

### Bước 3: Bank Server xác thực danh tính

#### Giải mã Ticket_v

Bank Server dùng `K_v` để lấy:

* `K_{c,v}`
* `ID_c`
* `pubKey_c`
* `scope`
* ticket lifetime

#### Verify Auth_v & Replay Check

Bank Server:

1. Giải mã `Auth_v` bằng `K_{c,v}`
2. Verify `ID_c` khớp với Ticket
3. Verify `TS_5` (Check độ trễ thời gian)
4. Verify `Nonce_3` (Check Redis Cache để chặn Replay Attack)

#### Strict Revocation Check (Kiểm tra thu hồi chứng chỉ)

Bank Server gọi gRPC sang CA Service (hoặc tra Redis Cache):

* Kiểm tra `pubKey_c` (hoặc `cert_sn`) có đang bị `revoked` hay không.
* Nếu đã bị thu hồi khẩn cấp → Block giao dịch.

#### Verify chữ ký số

Bank Server:

1. Giải mã `CipherPayload` bằng `K_{c,v}`
2. Lấy Payload và Signature
3. Verify bằng `pubKey_c`

Nếu hợp lệ: Giao dịch được xác nhận là do chính chủ thực hiện (Không thể chối bỏ).

### Bước 4: Deep Authorization & Thực thi giao dịch

#### Scope & Domain Validation

Bank Server kiểm tra nghiệp vụ:

1. **Ticket Scope:** Có chứa quyền `transfer` không?
2. **Ownership:** `ID_c` (trong Ticket) `==` Chủ sở hữu của `from_account` (trong Database)?
3. **Limits & Status:** Kiểm tra tài khoản không bị khóa, số dư (`balance`) đủ, và chưa vượt quá `daily_transfer_limit`.

#### Xử lý nghiệp vụ (ACID + Hash Chaining)

Bank Server bắt đầu Transaction:

1. Trừ tiền `from_account`, cộng tiền `to_account`.
2. Lấy `previous_hash` của giao dịch cuối cùng trong DB.
3. Tính toán băm bất biến: `Hash_n = SHA-256(previous_hash + Payload + Signature)`
4. Lưu bản ghi vào bảng `Transactions` kèm `Hash_n`.
5. Commit ACID Transaction.

### Bước 5: Mutual Authentication

Bank Server chuẩn bị:

```text
AP_REP = E_{K_{c,v}}[
    Result +
    TS_5 + 1
]

```

và trả về cho Gateway để forward về Client.

### Bước 6: Client xác minh phản hồi

Client:

1. Giải mã `AP_REP` bằng `K_{c,v}`
2. Verify `TS_5 + 1` (Xác nhận Bank Server là thật vì chỉ Server mới có `K_{c,v}` để tính toán phép cộng này).
3. Xác nhận Mutual Authentication thành công.
4. Xóa `K_{c,v}` và `Ticket_v` khỏi RAM (Session kết thúc).
5. Hiển thị:

```text
"Giao dịch thành công"
```

---
