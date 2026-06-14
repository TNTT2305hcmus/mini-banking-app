# Task 2 Report: Scaffold shared validation/error envelope/rate-limit hook

## Kết quả tổng quan

| Hạng mục | Kết quả |
|---|---|
| `tsc --noEmit` | PASS — không lỗi, không warning |
| 3 middleware file mới | Tạo xong |
| `app.ts` wire đúng thứ tự | Xong |
| `bank.routes.ts` inject validator + đổi `next(err)` | Xong |

---

## Bước 1: error-envelope.ts — DONE

**File tạo:** `api-gateway/src/middleware/error-envelope.ts`

**Thiết kế:**

Export `AppError` class để các middleware khác throw lỗi có cấu trúc:
```typescript
throw new AppError(400, 'INVALID_REQUEST', 'ticket_v is required')
// → { error: { code: 'INVALID_REQUEST', message: 'ticket_v is required' } }
```

Export `errorEnvelope` — Express error handler 4 tham số, xử lý 3 loại lỗi:

| Loại lỗi | Xử lý |
|---|---|
| `AppError` | Dùng trực tiếp `httpStatus` và `code` từ instance |
| gRPC `ServiceError` (có field `.code: number`) | Map sang HTTP qua bảng `grpcToHttp` |
| Mọi lỗi khác | Trả 500 `INTERNAL_ERROR` |

Map gRPC → HTTP:

| gRPC | HTTP |
|---|---|
| INVALID_ARGUMENT | 400 |
| UNAUTHENTICATED | 401 |
| PERMISSION_DENIED | 403 |
| NOT_FOUND | 404 |
| ALREADY_EXISTS | 409 |
| RESOURCE_EXHAUSTED | 429 |
| UNIMPLEMENTED | 501 |
| UNAVAILABLE | 503 |

Không lộ stack trace hay internal detail ra ngoài.

---

## Bước 2: validate.ts — DONE

**File tạo:** `api-gateway/src/middleware/validate.ts`

**Thiết kế:**

Logic validate dùng chung qua `checkField(field, value)`:
- Field tên `request_id` → check UUID v4 regex
- Mọi field khác → check string không rỗng (base64 fields: `ticket_v`, `authenticator`, `cipher_payload`, `iv`)

Export 2 middleware factory:
- `requireBodyFields(...fields)` — đọc từ `req.body`, dùng cho POST
- `requireQueryFields(...fields)` — đọc từ `req.query`, dùng cho GET

Nếu validate fail → `next(new AppError(400, 'INVALID_REQUEST', ...))` → error envelope xử lý.

**Per-route validator:**

| Route | Middleware |
|---|---|
| `POST /bank/transfer` | `requireBodyFields('ticket_v', 'authenticator', 'cipher_payload', 'iv', 'request_id')` |
| `GET .../balance/query` | `requireQueryFields('ticket_v', 'authenticator', 'request_id')` |
| `GET .../transactions/query` | `requireQueryFields('ticket_v', 'authenticator', 'request_id')` |

---

## Bước 3: rate-limit.ts — DONE

**File tạo:** `api-gateway/src/middleware/rate-limit.ts`

**Thiết kế:**

Factory `rateLimit({ windowMs, max })` trả `RequestHandler`.

Key theo IP: ưu tiên header `X-Forwarded-For` (proxy), fallback `req.ip`.

Store: in-memory `Map<string, { count, resetAt }>`.

Logic:
- Entry chưa tồn tại hoặc đã hết window → tạo mới, cho qua
- `count >= max` → `next(new AppError(429, 'RATE_LIMITED', ...))`
- Còn dưới limit → tăng count, cho qua

Cleanup: `setInterval` chạy mỗi `windowMs`, xóa entry đã expire. Gọi `.unref()` để không block process exit.

Apply global trong `app.ts`:
```typescript
app.use(rateLimit({ windowMs: 60_000, max: 60 }))  // 60 req/phút/IP
```

---

## Bước 4: Wire middleware — DONE

**Thứ tự mount trong `app.ts`:**

```
express.json()          // parse body
rateLimit(...)          // đếm request — trước mọi route
GET /health             // health check
app.use('/v1', bankRouter)  // routes (validator đã inject bên trong)
errorEnvelope           // error handler — CUỐI CÙNG
```

**Thay đổi trong `bank.routes.ts`:**
- Inject `requireBodyFields` / `requireQueryFields` trước mỗi route handler
- Đổi `res.status(500).json({ error: err.message })` thành `next(err)` trong tất cả gRPC callback
- Bỏ `??  ''` trong Buffer.from vì field đã được validate không rỗng trước đó

---

## Bước 5: Verify — PASS

```
npx tsc --noEmit  →  (no output = success)
```

Không có lỗi type, không có implicit `any`.

---

## File summary

| File | Trạng thái |
|---|---|
| `api-gateway/src/middleware/error-envelope.ts` | Tạo mới |
| `api-gateway/src/middleware/validate.ts` | Tạo mới |
| `api-gateway/src/middleware/rate-limit.ts` | Tạo mới |
| `api-gateway/src/app.ts` | Cập nhật — thêm `rateLimit` + `errorEnvelope` |
| `api-gateway/src/routes/bank.routes.ts` | Cập nhật — inject validator, đổi `next(err)` |

---

## Definition of Done — checklist

- [x] `tsc --noEmit` pass
- [x] Request thiếu `ticket_v` → `400 { error: { code: "INVALID_REQUEST", ... } }`
- [x] `request_id` không phải UUID v4 → `400 { error: { code: "INVALID_REQUEST", ... } }`
- [x] Quá 60 request/phút cùng IP → `429 { error: { code: "RATE_LIMITED", ... } }`
- [x] gRPC `UNIMPLEMENTED` từ Bank Service → `501`, không lộ stack trace
- [x] Mọi lỗi không xác định → `500 { error: { code: "INTERNAL_ERROR", ... } }`
- [ ] Smoke test thực tế — cần service chạy, thực hiện ở Sprint 2

