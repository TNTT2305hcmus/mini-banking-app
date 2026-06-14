# Task 1 Report — Mở rộng `validate.ts`

**Ngày:** 2026-06-08  
**Status:** ✅ Hoàn thành

---

## Đã làm

Thêm 3 exports mới vào `api-gateway/src/middleware/validate.ts`:

| Export | Mô tả |
|---|---|
| `requireBase64Fields(...fields)` | Middleware kiểm tra các field bytes (nonce, signature, tgt, authenticator) là string non-empty và pass regex base64 `[A-Za-z0-9+/]*={0,2}` |
| `validateScope(allowed)` | Middleware kiểm tra `req.body.scope` nằm trong whitelist; trả 400 `INVALID_SCOPE` nếu sai |
| `validateTimestamp` | Middleware kiểm tra `req.body.timestamp` là số nguyên Unix seconds trong cửa sổ ±5 phút; trả 400 `STALE_REQUEST` nếu lệch |

Code cũ (`requireBodyFields`, `requireQueryFields`, `checkField`) không thay đổi.

---

## Verify

- `tsc --noEmit` trong `api-gateway/` — **PASS**, không có lỗi type.

---

## Còn lại

Validators sẵn sàng để Thuận import khi tạo `auth.routes.ts` và `otp.routes.ts`. Chưa có route nào mount — test smoke phụ thuộc Sprint 1 dang dở của Thuận (task 5 trong tiền điều kiện).
