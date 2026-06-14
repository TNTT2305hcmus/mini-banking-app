# Tóm tắt: Khóa và Ticket

## Các khóa

| Khóa | Chủ | Loại | Tác dụng |
|---|---|---|---|
| `K_tgs` | KDC | Dài hạn, bí mật | Mã hóa/giải mã TGT — client giữ TGT như hộp đen, chỉ KDC mở được |
| `K_v` | Bank Service | Dài hạn, bí mật | Mã hóa/giải mã `Ticket_v` — client giữ ticket nhưng không đọc được |
| `K_{c,tgs}` | Client + KDC | Session, ngắn hạn | Session key sau AS Exchange; dùng để mã hóa Authenticator trong TGS_REQ và giải mã TGS_REP |
| `K_{c,v}` | Client + Bank Service | Session, ngắn hạn | Session key sau TGS Exchange; dùng để mã hóa payload giao dịch và giải mã AP_REP |

---

## Ticket

### TGT (Ticket Granting Ticket)

```
TGT = E_{K_tgs}[ID_c, cert_sn, K_{c,tgs}, issued_at, expires_at]
```

- KDC tạo sau AS Exchange, mã hóa bằng `K_tgs`
- Client giữ nhưng không đọc được
- Dùng để xin `Ticket_v` trong TGS Exchange
- TTL: 15–30 phút

### `Ticket_v` (Service Ticket)

```
Ticket_v = E_{K_v}[ID_c, scope, K_{c,v}, issued_at, expires_at]
```

- KDC tạo sau TGS Exchange, mã hóa bằng `K_v` của Bank Service
- Client giữ nhưng không đọc được
- Gửi cho Bank Service để xác thực giao dịch
- TTL: 5–10 phút

---

## Luồng tóm tắt

```
AS Exchange:
  Client ký AS_REQ bằng private key
  → KDC verify certificate qua CA, verify chữ ký
  → KDC sinh K_{c,tgs} + TGT
  → AS_REP = E_{pubKeyRSA_c}[K_{c,tgs}, TGT]
  → Client giải mã → lưu TGT + K_{c,tgs} vào RAM

TGS Exchange:
  Client gửi TGT + Authenticator = E_{K_{c,tgs}}[nonce, ts, scope]
  → KDC giải mã TGT bằng K_tgs → lấy K_{c,tgs}
  → KDC giải mã Authenticator bằng K_{c,tgs} → verify
  → KDC sinh K_{c,v} + Ticket_v = E_{K_v}[ID_c, scope, K_{c,v}, ...]
  → TGS_REP = E_{K_{c,tgs}}[Ticket_v, K_{c,v}]   ← mã hóa bằng K_{c,tgs}, không phải K_tgs
  → Client giải mã → lưu Ticket_v + K_{c,v} vào RAM

AP Exchange (giao dịch):
  Client gửi Ticket_v + CipherPayload = E_{K_{c,v}}[payload + signature]
  → Bank Service giải mã Ticket_v bằng K_v → lấy K_{c,v}, scope, ID_c
  → Bank Service giải mã payload bằng K_{c,v} → verify chữ ký
  → Xử lý giao dịch → AP_REP = E_{K_{c,v}}[result]
```

---

## Điểm dễ nhầm

| Câu hỏi | Đáp án |
|---|---|
| TGS_REP mã hóa bằng `K_tgs` hay `K_{c,tgs}`? | **`K_{c,tgs}`** — client có key này mới giải mã được |
| `K_tgs` client có biết không? | **Không** — chỉ KDC giữ |
| `K_v` client có biết không? | **Không** — chỉ Bank Service giữ (và KDC dùng lúc tạo ticket) |
| KDC có cần giữ `K_v` không? | Cần để tạo `Ticket_v`; trong demo chấp nhận được, production nên dùng HSM hoặc key exchange |
