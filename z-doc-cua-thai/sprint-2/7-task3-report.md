# Task 3 Report — Preset rate limiters trong `rate-limit.ts`

**Ngày:** 2026-06-08  
**Status:** ✅ Hoàn thành

---

## Đã làm

Thêm 2 preset exports vào cuối `api-gateway/src/middleware/rate-limit.ts`:

| Export | windowMs | max | Route áp dụng |
|---|---|---|---|
| `otpRateLimit` | 15 phút | 5 | `POST /v1/otp/request` |
| `authRateLimit` | 1 phút | 10 | `POST /v1/auth/as-req`, `POST /v1/auth/tgs-req` |

`rateLimit` factory không thay đổi. Cả 2 preset đều gọi lại factory, không có logic mới.

---

## Verify

- `tsc --noEmit` trong `api-gateway/` — **PASS**, không có lỗi type.

---

## Còn lại

Presets sẵn sàng để Thuận import. Chưa có route nào mount — test thực tế phụ thuộc task 5 trong tiền điều kiện Sprint 2.
