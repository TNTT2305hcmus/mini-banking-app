# Phân tích Task 1: Mở rộng `validate.ts` cho auth binary/base64 và scope

Task này chỉ sửa 1 file: `api-gateway/src/middleware/validate.ts`.  
Thêm 3 validators mới, không đụng vào code cũ.

---

## Hiện trạng `validate.ts`

- `checkField(field, value)` — check UUID v4 hoặc non-empty string
- `requireBodyFields(...fields)` — loop body fields qua `checkField`
- `requireQueryFields(...fields)` — loop query fields qua `checkField`

Thiếu: không check base64, không check scope whitelist, không check timestamp freshness.

---

## Bước 1: Thêm `requireBase64Fields`

**Làm gì:**  
Thêm regex check ký tự base64 hợp lệ (`A-Za-z0-9+/` với padding `=`) và middleware mới vào `validate.ts`. Field phải là string, non-empty, và pass regex — trả 400 `INVALID_REQUEST` nếu sai.

**Field áp dụng (từ kdc.proto):**
- `ASRequest`: `nonce`, `signature`
- `TGSRequest`: `tgt`, `authenticator`

**Verify:** Gọi middleware với body `{ nonce: "!!!not-base64!!!" }` → 400. Body `{ nonce: "YWJj" }` → pass.

---

## Bước 2: Thêm `validateScope`

**Làm gì:**  
Middleware nhận danh sách `allowed: string[]` và kiểm tra `req.body.scope` nằm trong whitelist. Sprint 2 chỉ có `"bank"` là hợp lệ. Trả 400 `INVALID_SCOPE` nếu sai.

**Verify:** Body `{ scope: "admin" }` → 400 `INVALID_SCOPE`. Body `{ scope: "bank" }` → pass.

---

## Bước 3: Thêm `validateTimestamp`

**Làm gì:**  
Middleware kiểm tra `req.body.timestamp` (Unix seconds, số nguyên) nằm trong cửa sổ ±5 phút tính từ server time. Trả 400 `STALE_REQUEST` nếu quá cũ hoặc quá mới.

**Lý do:** Gateway reject replay sớm mà không tốn gRPC round-trip sang KDC.

**Verify:** Body timestamp hiện tại → pass. Body timestamp 6 phút trước → 400 `STALE_REQUEST`.

---

## Bước 4: Verify `tsc --noEmit`

Sau khi thêm 3 validators, chạy `tsc --noEmit` trong `mini-banking-app/api-gateway/` — phải pass không có lỗi type.

---

## Thứ tự thực hiện

```
Bước 1 (requireBase64Fields) → Bước 2 (validateScope) → Bước 3 (validateTimestamp) → Bước 4 (tsc verify)
```

Cả 3 bước độc lập nhau, không có dependency.

---

## File cần tham khảo

| File | Lý do |
|---|---|
| `mini-banking-app/api-gateway/src/middleware/validate.ts` | Code hiện tại — giữ nguyên style, thêm vào cuối file |
| `mini-banking-app/api-gateway/src/middleware/error-envelope.ts` | Import `AppError` — không thay đổi cách dùng |
| `mini-banking-app/proto/kdc.proto` | Xác nhận tên field và kiểu: `nonce`, `signature` (bytes), `timestamp` (int64), `scope` (string) |
| `mini-banking-app/api-gateway/src/proto/kdc.ts` | TypeScript interface `ASRequest`, `TGSRequest` — biết field nào là `Buffer` (gửi qua REST dưới dạng base64) |

## File cần tạo

Không có — chỉ sửa file hiện có.

## File bị ảnh hưởng

| File | Ảnh hưởng |
|---|---|
| `api-gateway/src/middleware/validate.ts` | **Sửa trực tiếp** — thêm 3 exports mới |
| `api-gateway/src/routes/auth.routes.ts` | Do Thuận tạo — import `requireBase64Fields`, `validateScope`, `validateTimestamp` từ file này |
| `api-gateway/src/routes/otp.routes.ts` | Do Thuận tạo — có thể dùng `requireBodyFields` hiện có, không cần thêm gì từ task này |
