# Báo cáo kiểm tra tính hợp lý — Phần BANK (proto + api-gateway)

> Đối chiếu mã nguồn với 4 file flow:
> - `z-doc-cua-thai/bank-service-flow.md`
> - `z-doc-cua-thai/history-flow.md`
> - `z-doc-cua-thai/opt-pki-regist-flow.md`
> - `z-doc-cua-thai/transfer-flow.md`
>
> Phạm vi mã nguồn: `mini-banking-app/proto/` + `mini-banking-app/api-gateway/src/` (tập trung Bank).
> Ngày: 2026-06-15.

---

## Kết luận tổng quan

Phần **proto Bank khá hợp lý và khớp flow**, nhưng phần **api-gateway có nhiều mâu thuẫn nghiêm trọng** với cả flow docs lẫn blueprint. Có dấu hiệu **hai codebase song song chồng lên nhau** (`server.ts` vs `app.ts`), dẫn tới lỗi nghiêm trọng nhất: **các endpoint Bank không được wire vào gateway đang chạy**.

| Mức | Mã | Vấn đề |
|---|---|---|
| 🔴 CRITICAL | C1 | Endpoint Bank không tồn tại trong gateway đang chạy (`server.ts` không mount Bank router) |
| 🟠 HIGH | H2 | Read path dùng `GET` với secret (`ticket_v`, `authenticator`) trong query string |
| 🟠 HIGH | H3 | Hai gRPC client Bank, một cái tắt TLS (`createInsecure`) |
| 🟡 MEDIUM | M4 | `CreateUser` phá vỡ bất biến `ID_c = owner_id = users.id` |
| 🟡 MEDIUM | M5 | Thiếu compensating revoke khi `CreateUser` thất bại |
| 🟡 MEDIUM | M6 | Mã lỗi trong `error-envelope.ts` lệch error catalog |
| 🟢 LOW | L7 | `request_id` bắt buộc nhưng không forward sang gRPC |
| 🟢 LOW | L8 | proto có `ap_rep` ở read response nhưng doc nói "không AP_REP" |
| 🟢 LOW | L9 | Ba file rỗng gây nhiễu (`bank.route.ts`, `bank.controller.ts`, `bank.middleware.ts`) |
| 🟢 LOW | L10 | Nhóm `/bank/*` thiếu rate-limit / `validateHeaders` |

---

## 🔴 CRITICAL

### C1. Endpoint Bank không tồn tại trong gateway đang chạy

- Entrypoint thật là `server.ts` (`package.json` → `start: "tsc && node dist/server.js"`, `dev: ts-node-dev ... src/server.ts`). File này chỉ mount `otpRouter`, `pkiRouter`, `authRouter` — **không mount bất kỳ Bank router nào**.
- `bankRouter` (chứa `/bank/transfer`, `/balance/query`, `/transactions/query`) chỉ được mount trong `app.ts:15` (`app.use('/v1', bankRouter)`), mà `app.ts` là **code mồ côi**, không được `server.ts` hay script nào tham chiếu.
- Hệ quả: theo `transfer-flow.md` và `history-flow.md`, client gọi `POST /v1/bank/...` qua Gateway → thực tế các route này **404**. Đường Bank duy nhất đang hoạt động là `createUser` gọi nội bộ trong `pki.controller`.

**Khắc phục:** chọn một stack duy nhất — gộp Bank route vào `server.ts`, xóa `app.ts`.

---

## 🟠 HIGH

### H2. Read path dùng `GET` với secret trong query string

`api-gateway/src/routes/bank.routes.ts:35,57` định nghĩa balance/history là **`GET`** với `ticket_v`, `authenticator` nằm trong **query param**:

```ts
bankRouter.get("/bank/accounts/:id/balance/query",
  requireQueryFields("ticket_v", "authenticator", "request_id"), ...)
```

Mâu thuẫn trực tiếp với:
- `blueprint/api-design/05-bank-balance-history.md` và `base-api.md §1.9`: cả hai endpoint là **`POST .../query`**.
- `history-flow.md` (dòng 25, 35): chủ ý dùng **POST read-action để KHÔNG lộ secret trên URL**.

`GET` đẩy `Ticket_v`/`Authenticator` (đã mã hóa) vào URL → bị ghi vào access log proxy / lịch sử trình duyệt; `GET` mặc định cacheable trong khi không endpoint nào set `Cache-Control: no-store`. **Vi phạm rõ ràng thiết kế.**

**Khắc phục:** chuyển sang `POST`, đọc `ticket_v`/`authenticator` từ body; thêm header `Cache-Control: no-store`.

### H3. Hai gRPC client Bank, một cái tắt TLS

- `grpc-clients/bank.client.ts:6` → `grpc.credentials.createInsecure()` (dùng cho transfer/balance/history trong `bank.routes.ts`).
- `services/bank.service.ts:6` → `sslCredentials` (TLS, dùng cho `createUser` trong `pki.controller`).

ADR-02 (`design.md`) bắt buộc **gRPC + TLS một chiều**. Client `createInsecure()` vi phạm. Thêm nữa, hai env khác nhau cho cùng địa chỉ Bank: `BANK_SERVICE_ADDR` (bank.client.ts) vs `BANK_GRPC_ADDR` (bank.service.ts) → config drift.

**Khắc phục:** dùng chung một client với `sslCredentials` và một env duy nhất.

---

## 🟡 MEDIUM

### M4. `CreateUser` phá vỡ bất biến `ID_c = owner_id = users.id`

`bank-service-flow.md §1.4` và `opt-pki-regist-flow.md §3` khẳng định: `users.id` (Bank) = `owner_id` (CA DB) = `ID_c` trong ticket. Nhưng:

- proto `CreateUserRequest = {email, full_name}` — **không nhận user_id**; Bank tự sinh và trả về `user_id`.
- `pki.controller.ts:100-109`: gọi CA với `ownerId: idC` (= email) nhưng gọi Bank `createUser({email, fullName})` — **không truyền id chung**.

→ `owner_id` ở CA = email, còn `users.id` ở Bank = UUID Bank tự sinh → **hai giá trị khác nhau**. Sau này ownership check `account.user_id == ID_c` (bước 6/10 trong transfer & read flow) sẽ không bao giờ khớp.

**Khắc phục:** thống nhất `ID_c` xuyên suốt — hoặc Bank nhận `user_id` từ Gateway, hoặc dùng email làm `ID_c` ở mọi service.

### M5. Thiếu compensating revoke khi `CreateUser` thất bại

`bank-service-flow.md §1.3` + diagram `opt-pki-regist-flow.md`: nếu `Bank.CreateUser` fail sau khi CA đã cấp cert → Gateway phải **revoke cert (`enrollment_failed`)** rồi trả `503`, để không tồn tại "cert active mồ côi".

`pki.controller.ts:106` gọi `createUser` trong cùng `try`; nếu nó throw → rơi vào `catch` → `caHttpError(err)` trả `502 CA_SERVICE_UNAVAILABLE` và **không revoke gì cả**. Vừa để lại cert active mồ côi (đúng thứ design cấm), vừa gán nhầm lỗi cho CA.

**Khắc phục:** tách bước; khi `createUser` lỗi → gọi CA revoke với `reason=enrollment_failed`, trả `503 SERVICE_UNAVAILABLE`.

### M6. Mã lỗi trong `error-envelope.ts` lệch error catalog

Các helper lệch so với `base-api.md §1.7` và flow docs:

| Helper | Hiện tại | Catalog yêu cầu |
|---|---|---|
| `replayDetectedError` | `409 REPLAY_DETECTED` | `401 REPLAY_DETECTED` |
| `ticketExpiredError` | `401 TICKET_EXPIRED` | `401 INVALID_TICKET` |
| `scopeDeniedError` | `403 SCOPE_DENIED` | `403 WRONG_SCOPE` |
| `certRevokedError` / `certExpiredError` | `403` | `401` |
| `invalidSignatureError` | `400` | `401` |

(Các helper này thuộc stack `app.ts` mồ côi nên có thể chưa chạy, nhưng nếu giữ thì phải đồng bộ.)

---

## 🟢 LOW / cleanup

- **L7.** `request_id` được `requireBodyFields`/`requireQueryFields` bắt buộc (UUID v4) nhưng **không được forward** sang gRPC (proto `TransferRequest/BalanceRequest/HistoryRequest` không có field này). Validate rồi bỏ đi — nên map vào gRPC metadata `X-Request-ID`, hoặc bỏ bắt buộc.
- **L8.** proto `BalanceResponse`/`HistoryResponse` có `ap_rep bytes`, nhưng `history-flow.md:174` ghi "không AP_REP mã hóa" cho read path. Không phải bug nhưng cần thống nhất doc ↔ proto (đề xuất giữ `ap_rep` cho mutual-auth read, sửa doc).
- **L9.** Ba file rỗng gây nhiễu: `bank.route.ts`, `bank.controller.ts`, `bank.middleware.ts` (bản thật là `bank.routes.ts`). Nên xóa.
- **L10.** Nếu Bank route được wire vào `server.ts`, hiện chưa có rate-limit / `validateHeaders` cho nhóm `/bank/*` như otp/auth đang có.

---

## ✅ Phần khớp tốt (proto Bank)

- `bank.proto` `TransferRequest {ticket_v, authenticator, cipher_payload, iv}` + `TransferResponse {ap_rep, transaction_id}` **khớp chính xác** `transfer-flow.md §2`.
- Gateway forward **opaque bytes** (base64-decode → Buffer), không inspect key material → đúng `proto/description.md` và design.
- Gateway **không nhận `cert_sn` rời** từ REST; Bank tự lấy từ `Ticket_v` → đúng nguyên tắc chống public-key substitution.
- `VerifyCertificate` là fast-path KDC/Bank dùng chung → khớp các flow.

---

## Thứ tự ưu tiên sửa

1. **C1** — wire Bank vào `server.ts` (không có cái này thì luồng Bank không chạy end-to-end).
2. **H2** — POST thay GET cho read path.
3. **H3** — thống nhất TLS + env.
4. **M4 / M5** — định danh user nhất quán + revoke bù trừ.
5. M6, L7–L10 — đồng bộ mã lỗi và dọn dẹp.
