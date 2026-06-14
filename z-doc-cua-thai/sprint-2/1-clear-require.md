# Sprint 2 - Nhiệm vụ của Thái

## Bối cảnh từ Sprint 1

Middleware nền đã có từ Sprint 1:
- `api-gateway/src/middleware/error-envelope.ts` — `AppError` + Express error handler, map gRPC → HTTP
- `api-gateway/src/middleware/validate.ts` — `requireBodyFields` / `requireQueryFields`, kiểm tra UUID v4 và non-empty string
- `api-gateway/src/middleware/rate-limit.ts` — In-memory Map theo IP, 60 req/phút

Sprint 2 Thuận sẽ tạo 3 route mới: `/v1/pki/register`, `/v1/auth/as-req`, `/v1/auth/tgs-req`.  
Thái cần cung cấp validators chuyên biệt cho các route này **trước khi Thuận mount** để Thuận chỉ cần import và dùng.

---

## Tiền điều kiện — việc của nhóm cần xong trước

Thái có thể code Task 1–3 ngay lập tức (middleware độc lập, không phụ thuộc ai). Tuy nhiên để **verify và test** validators trong context thật, các task dưới đây của nhóm phải xong trước.

### Từ Sprint 1 còn dang dở

| # | Owner | Task | Unblock gì cho Thái |
|---|---|---|---|
| 1 | Quang | Fix `CERT_STATUS_VALID` → `CERT_STATUS_ACTIVE` tại `kdc-service/internal/kdc/service.go:188` | KDC build được → smoke test AS/TGS qua Gateway |
| 2 | Thanh | Fix `req.UserId` → `req.OwnerId` tại `ca-service/internal/grpc/handler.go:59,63,70` | CA build được → PKI register không crash |
| 3 | Thanh | Stub các RPC CA còn thiếu: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail` | CA build hoàn chỉnh → `certRevokedError` / `certExpiredError` factories có nơi test |
| 4 | Thuận | Tạo `ca.client.ts` và `kdc.client.ts` trong Gateway | Auth routes có gRPC client để gọi |
| 5 | Thuận | Tạo `otp.routes.ts`, `pki.routes.ts`, `auth.routes.ts` và mount vào `app.ts` | Validators của Thái có route để được áp dụng |

### Từ Sprint 2 của nhóm (cần trước khi test end-to-end)

| # | Owner | Task | Unblock gì cho Thái |
|---|---|---|---|
| 6 | Thanh | Implement `VerifyCertificate` trả status, validity, public key theo flags | Xác nhận `certRevokedError` / `certExpiredError` đúng với response CA thật |
| 7 | Thuận | Implement Gateway `/v1/pki/register`, `/v1/auth/as-req`, `/v1/auth/tgs-req` | Mount validators của Thái vào route thật, chạy smoke test |
| 8 | Quang | Chuẩn hóa `ASRequest` fields: nonce, timestamp, signature | Xác nhận `requireBase64Fields('nonce', 'signature')` và `validateTimestamp` đúng contract KDC |

---

## Tổng quan nhiệm vụ Sprint 2

> "Hỗ trợ Gateway validation cho auth binary/base64 fields, scope và error envelope. Auth routes reject input sai schema trước khi forward."

Cụ thể hơn: `validate.ts`, `error-envelope.ts`, `rate-limit.ts` hiện tại chỉ đủ cho Bank routes. Sprint 2 cần mở rộng 3 file này để auth routes (PKI/AS/TGS) cũng có validation đúng schema trước khi gọi gRPC.

---

## Task 1: Mở rộng `validate.ts` cho auth binary/base64 và scope

**Yêu cầu:**  
Thêm các validator mới vào `api-gateway/src/middleware/validate.ts`:

### 1a. `requireBase64Fields(...fields)` — middleware cho bytes/base64 fields

Các field bytes trong KDC proto (khi frontend gửi qua REST sẽ encode base64):
- `ASRequest`: `nonce`, `signature`
- `TGSRequest`: `tgt`, `authenticator`

Yêu cầu: field phải là string, non-empty, **và là base64 hợp lệ** (chỉ chứa ký tự base64).  
Trả 400 `INVALID_REQUEST` nếu sai.

Lý do cần tách riêng (không dùng `requireBodyFields`): `requireBodyFields` chỉ check non-empty, không check base64 format.

### 1b. `validateScope(allowed: string[])` — middleware kiểm tra scope

`TGSRequest.scope` phải nằm trong whitelist. Sprint 2 chỉ có `"bank"` là hợp lệ, nhưng về sau có thể mở rộng.  
Trả 400 `INVALID_SCOPE` nếu scope không hợp lệ.

### 1c. `validateTimestamp` — middleware kiểm tra freshness

`ASRequest.timestamp` là Unix seconds, phải nằm trong cửa sổ ±5 phút tính từ server time.  
Trả 400 `STALE_REQUEST` nếu quá cũ hoặc quá mới (có thể là replay).

Lý do: Gateway có thể reject replay sớm mà không tốn gRPC round-trip sang KDC.

---

## Task 2: Cập nhật `error-envelope.ts` với auth-specific error codes

**Yêu cầu:**  
Thêm các `AppError` convenience factories cho auth failures để Thuận dùng trong route handlers.

Không thêm vào middleware `errorEnvelope` (logic đó đã đủ). Chỉ cần export thêm các error factories:

| Factory | HTTP | Code | Khi nào |
|---|---|---|---|
| `certRevokedError()` | 403 | `CERT_REVOKED` | CA trả REVOKED status |
| `certExpiredError()` | 403 | `CERT_EXPIRED` | CA trả EXPIRED status |
| `ticketExpiredError()` | 401 | `TICKET_EXPIRED` | KDC/Bank check TTL |
| `replayDetectedError()` | 409 | `REPLAY_DETECTED` | nonce/ticket_id đã dùng |
| `scopeDeniedError()` | 403 | `SCOPE_DENIED` | scope không match |
| `invalidSignatureError()` | 400 | `INVALID_SIGNATURE` | signature verify fail |

Lý do: Thuận cần throw lỗi đúng format khi gRPC trả PERMISSION_DENIED/UNAUTHENTICATED nhưng muốn expose code cụ thể hơn cho frontend debug.

---

## Task 3: Điều chỉnh `rate-limit.ts` cho OTP/Auth endpoints

**Yêu cầu:**  
Hiện tại chỉ có 1 limiter global (60 req/phút mọi route). Auth routes cần giới hạn chặt hơn.

Thêm khả năng tạo nhiều limiter độc lập với config khác nhau (factory đã hỗ trợ, nhưng cần export thêm 2 preset):

| Preset | Route áp dụng | Limit |
|---|---|---|
| `otpRateLimit` | `POST /v1/otp/request` | 5 req / 15 phút / IP |
| `authRateLimit` | `POST /v1/auth/as-req`, `POST /v1/auth/tgs-req` | 10 req / phút / IP |

Lý do: OTP brute-force sẽ bị exploit nếu giữ 60 req/phút. AS_REQ cũng cần chặt hơn Bank queries.

Export các preset này từ `rate-limit.ts` để Thuận import trực tiếp khi tạo route.

---

## Output mong đợi

Sau Sprint 2, Thuận khi tạo route chỉ cần viết:

```typescript
// Ví dụ trong auth.routes.ts (Thuận viết):
import { requireBodyFields, requireBase64Fields, validateScope, validateTimestamp } from '../middleware/validate'
import { authRateLimit } from '../middleware/rate-limit'

authRouter.post('/auth/as-req',
  authRateLimit,
  requireBodyFields('id_c', 'cert_sn', 'request_id'),
  requireBase64Fields('nonce', 'signature'),
  validateTimestamp,
  handler
)

authRouter.post('/auth/tgs-req',
  authRateLimit,
  requireBodyFields('scope', 'service_id'),
  requireBase64Fields('tgt', 'authenticator'),
  validateScope(['bank']),
  handler
)
```

---

## Definition of Done cho Thái — Sprint 2

| Tiêu chí | Verify |
|---|---|
| `requireBase64Fields` reject field không phải base64 hợp lệ | Unit test hoặc manual curl |
| `validateScope` reject scope không trong whitelist | Trả đúng 400 `INVALID_SCOPE` |
| `validateTimestamp` reject timestamp quá cũ / quá mới | Test với timestamp ±6 phút |
| Auth error factories export đúng HTTP code và error code | Kiểm tra response format |
| `otpRateLimit` và `authRateLimit` export từ `rate-limit.ts` | Thuận import được, không lỗi TS |
| `tsc --noEmit` pass sau khi thêm validators | Build không có type error |

---

## File cần đọc trước khi code

| File | Lý do |
|---|---|
| `mini-banking-app/api-gateway/src/middleware/validate.ts` | Code hiện tại — thêm vào đây, không tạo file mới |
| `mini-banking-app/api-gateway/src/middleware/error-envelope.ts` | Code hiện tại — thêm error factories |
| `mini-banking-app/api-gateway/src/middleware/rate-limit.ts` | Code hiện tại — thêm preset exporters |
| `mini-banking-app/proto/kdc.proto` | Field names của ASRequest / TGSRequest |
| `mini-banking-app/proto/ca.proto` | Field names của RegisterUserRequest |
| `mini-banking-app/api-gateway/src/proto/kdc.ts` | TypeScript interface — type ASRequest, TGSRequest |

## File cần sửa (không tạo file mới)

| File | Thay đổi |
|---|---|
| `api-gateway/src/middleware/validate.ts` | Thêm `requireBase64Fields`, `validateScope`, `validateTimestamp` |
| `api-gateway/src/middleware/error-envelope.ts` | Thêm auth error factories |
| `api-gateway/src/middleware/rate-limit.ts` | Export `otpRateLimit`, `authRateLimit` preset |
