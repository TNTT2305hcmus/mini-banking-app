# Phân tích Task 2: Scaffold shared validation/error envelope/rate-limit hook

Task 2 hoàn toàn nằm trong **API Gateway (TypeScript)**. Không động vào CA, KDC, Bank Service.

3 middleware cần tạo + 2 file cần cập nhật để wire chúng vào.

---

## Bước 1: Tạo error-envelope.ts

**Làm gì:**

Tạo `api-gateway/src/middleware/error-envelope.ts` — Express error handler middleware (4 tham số: `err, req, res, next`).

Nhiệm vụ:
- Nhận mọi lỗi được `next(err)` từ route handlers
- Map gRPC status code → HTTP status code:

| gRPC code | HTTP |
|---|---|
| INVALID_ARGUMENT | 400 |
| UNAUTHENTICATED | 401 |
| PERMISSION_DENIED | 403 |
| NOT_FOUND | 404 |
| ALREADY_EXISTS | 409 |
| RESOURCE_EXHAUSTED | 429 |
| UNIMPLEMENTED | 501 |
| UNAVAILABLE | 503 |
| mọi lỗi khác | 500 |

- Format response thống nhất:
```json
{ "error": { "code": "INVALID_ARGUMENT", "message": "request_id is required" } }
```
- **Không** trả stack trace, key material, raw gRPC error detail ra ngoài

Cách apply: mount **CUỐI CÙNG** trong `app.ts`, sau tất cả route.

**Tham khảo:**

| Nguồn | Lấy gì |
|---|---|
| `blueprint/design.md` (Error handling security rule, dòng 235–242) | 4 rule: fail closed, không lộ bí mật, audit event, không retry auth failures |
| `blueprint/design.md` (Authorization matrix, dòng 182–191) | HTTP status code kỳ vọng cho từng loại lỗi (`400`, `401`, `403`, `429`) |
| `api-gateway/src/app.ts` | Nơi mount error handler — phải đặt sau `app.use('/v1', ...)` |

**File tạo:**
- `api-gateway/src/middleware/error-envelope.ts`

**File cập nhật:**
- `api-gateway/src/app.ts` — thêm `app.use(errorEnvelope)` ở cuối

---

## Bước 2: Tạo validate.ts

**Làm gì:**

Tạo `api-gateway/src/middleware/validate.ts` — middleware factory validate request fields trước khi forward gRPC.

Export 2 thứ:

**2a. `requireBodyFields(...fields)`** — dùng cho POST (body JSON):
- Kiểm tra field tồn tại và không rỗng
- Base64 fields (`ticket_v`, `authenticator`, `cipher_payload`, `iv`): phải là string, không rỗng sau khi decode
- `request_id`: phải khớp regex UUID v4

**2b. `requireQueryFields(...fields)`** — dùng cho GET (query string):
- Tương tự nhưng đọc từ `req.query`
- `ticket_v`, `authenticator` từ query string

Nếu validate fail → gọi `next(new AppError(400, 'INVALID_REQUEST', '...'))` → error envelope xử lý tiếp.

Cần định nghĩa thêm class `AppError` (hoặc interface) để error envelope nhận diện được.

**Validation cụ thể per route:**

| Route | Fields bắt buộc |
|---|---|
| `POST /bank/transfer` | `ticket_v`, `authenticator`, `cipher_payload`, `iv`, `request_id` (body) |
| `GET /bank/accounts/:id/balance/query` | `ticket_v`, `authenticator`, `request_id` (query) |
| `GET /bank/accounts/:id/transactions/query` | `ticket_v`, `authenticator`, `request_id` (query) |

**Tham khảo:**

| Nguồn | Lấy gì |
|---|---|
| `proto/bank.proto` (dòng 43–49, 56–60, 72–78) | Tên field chính xác của `TransferRequest`, `BalanceRequest`, `HistoryRequest` |
| `api-gateway/src/proto/bank.ts` (dòng 114–184) | TypeScript interface — biết field nào là `bytes` (cần base64 validate) và field nào là `string` |
| `blueprint/design.md` (dòng 230) | "Validate request schema/protobuf, reject unknown/invalid fields" — confirm validate ở Gateway |
| `api-gateway/src/routes/bank.routes.ts` | Nơi inject middleware — mỗi route thêm validator trước handler |

**File tạo:**
- `api-gateway/src/middleware/validate.ts`

**File cập nhật:**
- `api-gateway/src/routes/bank.routes.ts` — inject validator vào từng route, đổi error handling dùng `next(err)`

---

## Bước 3: Tạo rate-limit.ts

**Làm gì:**

Tạo `api-gateway/src/middleware/rate-limit.ts` — middleware factory đếm request theo IP.

```typescript
export function rateLimit(opts: { windowMs: number; max: number }): RequestHandler
```

Cơ chế:
- Key: `req.ip` hoặc header `X-Forwarded-For` (phòng trường hợp đứng sau proxy)
- Store: in-memory `Map<string, { count: number; resetAt: number }>`
- Nếu `count >= max` và chưa hết window → gọi `next(new AppError(429, 'RATE_LIMITED', 'Too many requests'))`
- Nếu đã hết window → reset counter
- Cleanup: `setInterval` mỗi windowMs xóa entry đã expire (tránh memory leak)

Cách dùng — 2 level:
- **Strict** (`max: 5/phút`): cho OTP routes (Thuận implement sau)
- **General** (`max: 60/phút`): apply toàn bộ `/v1` để chặn bot

Sprint 1 chỉ cần general rate limit apply global. OTP strict limit để Sprint 2.

**Tham khảo:**

| Nguồn | Lấy gì |
|---|---|
| `blueprint/design.md` (dòng 221) | "Rate limiting theo IP/email, OTP TTL ngắn" — confirm cần rate limit ở Gateway |
| `blueprint/design.md` (Authorization matrix, dòng 184) | `/otp/request`, `/otp/verify` → `429 Too Many Requests` khi quá limit |
| `api-gateway/src/middleware/error-envelope.ts` (vừa tạo ở Bước 1) | Import `AppError` để throw đúng format |
| `api-gateway/src/app.ts` | Nơi apply global rate limit — `app.use(rateLimit({ windowMs: 60_000, max: 60 }))` |

**File tạo:**
- `api-gateway/src/middleware/rate-limit.ts`

**File cập nhật:**
- `api-gateway/src/app.ts` — thêm `app.use(rateLimit(...))` trước các routes

---

## Bước 4: Wire middleware vào app.ts và bank.routes.ts

**Làm gì:**

Thứ tự mount trong `app.ts` phải đúng — Express xử lý middleware theo thứ tự khai báo:

```
app.use(express.json())           // 1. parse body
app.use(rateLimit(...))           // 2. rate limit (trước mọi route)
app.get('/health', ...)           // 3. health — không cần rate limit riêng
app.use('/v1', bankRouter)        // 4. routes (đã có validator per-route bên trong)
app.use(errorEnvelope)            // 5. error handler — PHẢI ở cuối
```

Trong `bank.routes.ts`, inject validator trước handler:
```typescript
// Trước (Task 1):
bankRouter.post('/bank/transfer', (req, res) => { ... })

// Sau (Task 2):
bankRouter.post('/bank/transfer', requireBodyFields('ticket_v', 'authenticator', 'cipher_payload', 'iv', 'request_id'), (req, res, next) => { ... })
```

Đồng thời đổi error handling trong route handlers từ:
```typescript
if (err) return res.status(500).json({ error: err.message })
```
thành:
```typescript
if (err) return next(err)
```
Để error envelope xử lý thống nhất.

**Tham khảo:**

| Nguồn | Lấy gì |
|---|---|
| `api-gateway/src/app.ts` | File cần sửa — thứ tự mount |
| `api-gateway/src/routes/bank.routes.ts` | File cần sửa — inject validator, đổi `next(err)` |
| `api-gateway/src/middleware/error-envelope.ts` | Import vào app.ts |
| `api-gateway/src/middleware/validate.ts` | Import vào bank.routes.ts |
| `api-gateway/src/middleware/rate-limit.ts` | Import vào app.ts |

**File cập nhật:**
- `api-gateway/src/app.ts`
- `api-gateway/src/routes/bank.routes.ts`

---

## Bước 5: Verify build

**Làm gì:**
- Chạy `tsc --noEmit` — đảm bảo toàn bộ TypeScript compile sạch
- Không có `any` implicit, không có unused import

**Verify:** `tsc --noEmit` pass, không còn type error.

---

## Thứ tự thực hiện

```
Bước 1 (error-envelope) 
  → Bước 2 (validate — import AppError từ error-envelope)
  → Bước 3 (rate-limit — import AppError từ error-envelope)
  → Bước 4 (wire vào app.ts + bank.routes.ts)
  → Bước 5 (verify)
```

Bước 1 phải làm trước vì Bước 2 và 3 cần import `AppError` từ đó.

---

## File summary

| File | Trạng thái |
|---|---|
| `api-gateway/src/middleware/error-envelope.ts` | Tạo mới |
| `api-gateway/src/middleware/validate.ts` | Tạo mới |
| `api-gateway/src/middleware/rate-limit.ts` | Tạo mới |
| `api-gateway/src/app.ts` | Cập nhật — thêm rate limit + error envelope |
| `api-gateway/src/routes/bank.routes.ts` | Cập nhật — inject validator, đổi `next(err)` |

---

## Definition of Done cho Task 2

- `tsc --noEmit` pass
- Request thiếu `ticket_v` vào `POST /v1/bank/transfer` → trả `400 { error: { code: "INVALID_REQUEST", ... } }`
- Request với `request_id` không phải UUID → trả `400`
- Gọi quá 60 request/phút từ cùng IP → trả `429 { error: { code: "RATE_LIMITED", ... } }`
- Lỗi gRPC từ Bank Service (Unimplemented) → trả `501`, không lộ stack trace
- Response lỗi không chứa stack trace, key material hay internal error detail
