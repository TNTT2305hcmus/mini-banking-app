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
| 🔴 CRITICAL | C1 | ✅ **ĐÃ FIX** — Endpoint Bank không tồn tại trong gateway đang chạy (`server.ts` không mount Bank router) |
| 🟠 HIGH | H2 | ✅ **ĐÃ FIX** — Read path dùng `GET` với secret (`ticket_v`, `authenticator`) trong query string |
| 🟠 HIGH | H3 | ✅ **ĐÃ FIX** — Hai gRPC client Bank, một cái tắt TLS (`createInsecure`) |
| 🟡 MEDIUM | M4 | `CreateUser` phá vỡ bất biến `ID_c = owner_id = users.id` |
| 🟡 MEDIUM | M5 | Thiếu compensating revoke khi `CreateUser` thất bại |
| 🟡 MEDIUM | M6 | ✅ **ĐÃ FIX** — Mã lỗi/HTTP status lệch error catalog (gộp mapping vào `errorHandler.ts`) |
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

### H3. Hai gRPC client Bank, một cái tắt TLS — ✅ ĐÃ FIX

- `grpc-clients/bank.client.ts:6` → `grpc.credentials.createInsecure()` (dùng cho transfer/balance/history trong `bank.routes.ts`).
- `services/bank.service.ts:6` → `sslCredentials` (TLS, dùng cho `createUser` trong `pki.controller`).

ADR-02 (`design.md`) bắt buộc **gRPC + TLS một chiều**. Client `createInsecure()` vi phạm. Thêm nữa, hai env khác nhau cho cùng địa chỉ Bank: `BANK_SERVICE_ADDR` (bank.client.ts) vs `BANK_GRPC_ADDR` (bank.service.ts) → config drift.

**Đã xử lý:** gộp Bank gRPC client về `api-gateway/src/services/bank.service.ts`, thêm `transferMoney`, `getBalance`, `getHistory` dùng chung `sslCredentials` và `BANK_GRPC_ADDR`; sửa `bank.routes.ts` để dùng service TLS; xóa `api-gateway/src/grpc-clients/bank.client.ts`.

**Bổ sung phía Bank Service:** `banking-service/internal/grpc/server.go` đã bật TLS server credentials; `banking-service/internal/configs/config.go` đọc `BANK_TLS_CERT_PATH` và `BANK_TLS_KEY_PATH`; `cmd/server/main.go` fail sớm nếu cấu hình TLS không hợp lệ.

**Kiểm tra:** `go test ./...` trong `banking-service` pass; compile hẹp TypeScript cho các file Gateway liên quan pass; search không còn `createInsecure()`, `BANK_SERVICE_ADDR`, `bankClient`, hoặc import `grpc-clients/bank.client`.

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

### M6. Mã lỗi/HTTP status lệch error catalog — ✅ ĐÃ FIX

**Vấn đề gốc (2 phần):**

1. `errorHandler.ts` thật (đang chạy trong `server.ts`) map **mọi** lỗi gRPC từ Bank → `500 INTERNAL_ERROR`. Lỗi nghiệp vụ (`INVALID_TICKET`, `WRONG_SCOPE`, `INSUFFICIENT_FUNDS`…) đều biến thành 500.
2. Logic map gRPC→HTTP đúng hơn lại nằm ở `error-envelope.ts` thuộc stack `app.ts` **mồ côi** (không chạy); thêm nữa các factory lệch catalog (`replayDetectedError` 409 thay vì 401, `ticketExpiredError`/`TICKET_EXPIRED` thay vì `INVALID_TICKET`, `scopeDeniedError`/`SCOPE_DENIED` thay vì `WRONG_SCOPE`, `certRevoked/Expired` 403 thay vì 401, `invalidSignature` 400 thay vì 401).

**Đã xử lý:** gộp logic mapping vào `errorHandler.ts` thật bằng helper `bankGrpcError(err)` (cùng cấu trúc `switch` với `asGrpcError`/`tgsGrpcError`). Catch-all `errorHandler` giờ: nếu lỗi có `code` dạng số (gRPC) → gọi `bankGrpcError` trả đúng HTTP status; còn lại → `500 INTERNAL_ERROR`. `bankGrpcError` ưu tiên `error_code` mà BankService đặt trong gRPC trailing metadata (`error-code`), fallback mã generic theo status (không leak lý do nội bộ — đúng `bank-service-flow.md:207`).

→ Mapping đầy đủ + giao kèo Gateway↔Bank: xem **mục "Giao kèo lỗi gRPC Gateway ↔ Bank"** bên dưới.

> `error-envelope.ts` (stack mồ côi) giờ **dư thừa hoàn toàn** — chờ dọn ở L9/cleanup.

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

## Giao kèo lỗi gRPC Gateway ↔ Bank (contract)

Sau khi fix M6, `errorHandler.ts` (gateway) ánh xạ lỗi gRPC từ BankService → HTTP theo `bankGrpcError`. Để client nhận **đúng HTTP status + error_code** như error catalog trong các flow docs, BankService **phải tuân theo giao kèo này** khi trả lỗi gRPC.

### 1. Bảng ánh xạ gRPC status → HTTP (gateway đã cài)

| gRPC status (Bank trả) | HTTP | error_code generic (khi không có metadata) | Nhóm lỗi catalog tương ứng |
|---|---|---|---|
| `INVALID_ARGUMENT` | 400 | `BAD_REQUEST` | `BAD_REQUEST` (sai format request) |
| `UNAUTHENTICATED` | 401 | `UNAUTHENTICATED` | `INVALID_TICKET`, `STALE_REQUEST`, `REPLAY_DETECTED`, `CERT_REVOKED`, `CERT_EXPIRED`, `INVALID_SIGNATURE` |
| `PERMISSION_DENIED` | 403 | `FORBIDDEN` | `WRONG_SCOPE`, `FORBIDDEN` |
| `NOT_FOUND` | 404 | `NOT_FOUND` | account_id không tồn tại |
| `ALREADY_EXISTS` | 409 | `ALREADY_EXISTS` | (enrollment: `ACTIVE_CERT_EXISTS`) |
| `FAILED_PRECONDITION` | 422 | `UNPROCESSABLE_ENTITY` | `ACCOUNT_NOT_ACTIVE`, `INSUFFICIENT_FUNDS`, `DAILY_LIMIT_EXCEEDED` |
| `RESOURCE_EXHAUSTED` | 429 | `RATE_LIMITED` | (dự phòng) |
| `UNAVAILABLE` / `DEADLINE_EXCEEDED` | 503 | `SERVICE_UNAVAILABLE` | `SERVICE_UNAVAILABLE` (fail-closed: Bank/CA down hoặc timeout) |
| còn lại | 500 | `INTERNAL_ERROR` | lỗi nội bộ ngoài dự kiến |

> `DEADLINE_EXCEEDED` → 503 (không phải 500) để giữ đúng tinh thần **fail-closed**: timeout tới Bank/CA là "service không khả dụng", không phải lỗi gateway.

### 2. Hai điều kiện BankService phải đảm bảo

**(a) Dùng đúng gRPC status cho từng nhóm lỗi** — nếu không, HTTP status sẽ sai:

| Nhóm lỗi (theo flow docs) | gRPC status Bank PHẢI dùng | → HTTP |
|---|---|---|
| Ticket/auth/replay/cert/signature | `UNAUTHENTICATED` | 401 |
| Scope sai / ownership sai | `PERMISSION_DENIED` | 403 |
| Account không tồn tại | `NOT_FOUND` | 404 |
| Business rules (số dư, hạn mức, account status) | `FAILED_PRECONDITION` | 422 |
| Bank/CA down hoặc timeout | `UNAVAILABLE` (hoặc để timeout → `DEADLINE_EXCEEDED`) | 503 |

**(b) Đặt error_code domain vào gRPC trailing metadata key `error-code`** — để client nhận đúng mã chi tiết thay vì generic. Ví dụ Bank trả `FAILED_PRECONDITION` + `metadata: { "error-code": "INSUFFICIENT_FUNDS" }` → gateway xuất `422 { error_code: "INSUFFICIENT_FUNDS" }`.

- Nếu **không** set metadata → client chỉ nhận mã generic (HTTP status vẫn đúng). Với nhóm 401 việc gom chung này **đúng tinh thần** `bank-service-flow.md:207` ("auth failure không phân biệt nguyên nhân" — chống information leakage); BankService tự quyết lộ mã chi tiết tới đâu.

### 3. Lưu ý phạm vi

- Giao kèo này áp dụng cho **transfer/balance/history** (đi qua catch-all `errorHandler` bằng `next(err)`).
- Luồng **AS-req/TGS-req** (KDC) vẫn dùng helper riêng `asGrpcError`/`tgsGrpcError` gọi thủ công trong `kdc.controller` — **không** đổi.
- Luồng **CreateUser** (enrollment) đi qua `pki.controller` với error handler riêng của nó — liên quan M4/M5, chưa nằm trong giao kèo này.

---

## Thứ tự ưu tiên sửa

1. ~~**C1** — wire Bank vào `server.ts`~~ ✅ đã fix (mount `bankRouter` trong `server.ts`).
2. ~~**H2** — POST thay GET cho read path~~ ✅ đã fix (POST + secret trong body + `Cache-Control: no-store`).
3. ~~**M6** — đồng bộ mã lỗi/HTTP status~~ ✅ đã fix (`bankGrpcError` trong `errorHandler.ts` + giao kèo Gateway↔Bank).
4. ~~**H3** — thống nhất TLS + env~~ ✅ đã fix (Gateway dùng một Bank client TLS; Bank gRPC server bật TLS).
5. **M4 / M5** — định danh user nhất quán + revoke bù trừ (còn lại).
6. L7–L10 — dọn dẹp (gồm xóa stack mồ côi `app.ts`/`error-envelope.ts`/`rate-limit.ts`/`validate.ts` và các file rỗng).
