# MUST-REPORT.md

## Đồng bộ định danh `ID_c` giữa Gateway, CA, KDC và Bank

### Mục tiêu

Toàn bộ hệ thống phải sử dụng **một định danh duy nhất (`ID_c`)** xuyên suốt quá trình đăng ký, xác thực và truy cập tài nguyên.

```text
ID_c
 ├─ API Gateway sinh
 ├─ CA certificates.owner_id
 ├─ Bank users.id
 ├─ KDC Ticket_v.id_c
 └─ Bank accounts.user_id
````

---

## 1. CA Service

### Hiện trạng

`ca.proto` đã hỗ trợ truyền và trả về `owner_id`.

```proto
message RegisterUserRequest {
  string csr_pem = 1;
  string owner_id = 2;
}
```

```proto
message VerifyCertificateResponse {
  string owner_id = 2;
}
```

### Kết luận

Không cần thay đổi contract lớn.

Gateway chỉ cần truyền:

```text
owner_id = ID_c
```

thay vì:

```text
owner_id = email
```

---

## 2. Bank Service

### Hiện trạng

`CreateUserRequest` chưa hỗ trợ nhận `ID_c`.

```proto
message CreateUserRequest {
  string email = 1;
  string full_name = 2;
}
```

### Thay đổi bắt buộc

Bổ sung trường `user_id`.

```proto
message CreateUserRequest {
  string email = 1;
  string full_name = 2;
  string user_id = 3;
}
```

### Gateway

```ts
createUser({
  email,
  fullName,
  userId: idC,
});
```

### Bank Service

Thay vì:

```text
users.id = UUID tự sinh
```

phải:

```text
users.id = user_id từ Gateway
```

### Yêu cầu

* Không tự sinh UUID mới.
* `users.id` phải trùng với `ID_c` của toàn hệ thống.

---

## 3. KDC Service

### Hiện trạng

AS Request đã chứa:

```proto
message ASRequest {
  string id_c = 1;
  string cert_sn = 2;
}
```

Ticket cũng đã chứa:

```proto
message TicketPayload {
  string id_c = 1;
}
```

KDC hiện gọi:

```text
CA.VerifyCertificate(cert_sn)
```

và CA có khả năng trả về:

```text
owner_id
```

Tuy nhiên chưa có kiểm tra bắt buộc:

```text
req.id_c == owner_id
```

### Rủi ro

Client có thể gửi:

```text
id_c = User_B
cert_sn = Certificate của User_A
```

Nếu KDC không đối chiếu:

```text
req.id_c
==
CA.owner_id
```

thì có thể phát hành ticket với định danh sai.

### Thay đổi bắt buộc

Sau khi verify certificate:

```go
if req.IdC != cert.OwnerId {
    return error
}
```

### Khuyến nghị

Tốt hơn nên sử dụng trực tiếp:

```text
CA.owner_id
```

làm canonical identity thay vì tin tưởng giá trị `id_c` do client gửi.

---

## 4. Registration Flow

### Thiết kế chuẩn

#### Bước 1

API Gateway sinh:

```text
ID_c = UUID
```

duy nhất cho người dùng.

---

#### Bước 2

Registration Token phải chứa:

```text
ID_c
Email
```

Ví dụ:

```json
{
  "id_c": "uuid",
  "email": "user@example.com"
}
```

### Lưu ý

Không tin tưởng:

```text
id_c từ client request
```

---

#### Bước 3

Gateway gọi CA:

```text
owner_id = ID_c
```

---

#### Bước 4

Gateway gọi Bank:

```text
user_id = ID_c
```

---

#### Bước 5

Bank lưu:

```text
users.id = ID_c
```

---

#### Bước 6

KDC nhận AS_REQ:

```text
id_c
cert_sn
```

và thực hiện:

```text
VerifyCertificate(cert_sn)
```

Sau đó kiểm tra:

```text
req.id_c == owner_id
```

---

#### Bước 7

KDC phát:

```text
TGT
Ticket_v
```

với:

```text
ID_c đã được xác thực
```

---

#### Bước 8

Bank xác thực quyền sở hữu tài khoản:

```text
Ticket_v.ID_c
==
accounts.user_id
```

---

## Checklist Bắt Buộc

* [ ] API Gateway sinh `ID_c` một lần duy nhất trong registration flow.
* [ ] Registration Token chứa `ID_c` và `email`.
* [ ] Gateway truyền `owner_id = ID_c` cho CA.
* [ ] Gateway truyền `user_id = ID_c` cho Bank.
* [ ] Bank lưu `users.id = ID_c`.
* [ ] KDC verify `req.id_c == CA.owner_id`.
* [ ] KDC phát hành `TGT/Ticket_v` với `ID_c` đã xác thực.
* [ ] Bank dùng `Ticket_v.ID_c` để kiểm tra `accounts.user_id`.

---

## Kết luận

Việc bổ sung `ID_c` vào `CreateUserRequest` là cần thiết nhưng **chưa đủ**.

Để đảm bảo tính nhất quán định danh trên toàn hệ thống:

1. CA phải lưu `owner_id = ID_c`.
2. Bank phải lưu `users.id = ID_c`.
3. KDC phải đối chiếu:

```text
req.id_c == CA.owner_id
```

trước khi phát hành ticket.

Nếu thiếu bước xác thực này, hệ thống vẫn có nguy cơ xảy ra **identity mismatch** giữa Client, Certificate, Ticket và Account.

```
