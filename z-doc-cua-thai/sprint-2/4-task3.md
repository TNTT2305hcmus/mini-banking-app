# Phân tích Task 3: Export preset rate limiters cho OTP/Auth

Task này chỉ sửa 1 file: `api-gateway/src/middleware/rate-limit.ts`.  
Thêm 2 exported constants là preset limiter. Không sửa `rateLimit` factory.

---

## Hiện trạng `rate-limit.ts`

- `rateLimit(opts)` factory — đã hoạt động, không cần sửa
- `app.ts` đang dùng global limit 60 req/phút cho tất cả routes

Thiếu: OTP và auth routes cần limits chặt hơn, nhưng không có preset sẵn để Thuận import.

---

## Các preset cần thêm

| Preset | windowMs | max | Lý do |
|---|---|---|---|
| `otpRateLimit` | 15 phút | 5 | OTP 6 chữ số: 5 lần đủ cho user nhập sai, không đủ để brute-force |
| `authRateLimit` | 1 phút | 10 | AS/TGS crypto-heavy: 10/phút đủ dùng bình thường, cản bot |

Thêm 2 dòng `export const` này vào cuối file, sau khi `rateLimit` function kết thúc, gọi lại chính `rateLimit` factory.

---

## Cách Thuận dùng

- `otp.routes.ts` — import `otpRateLimit`, mount trên `POST /otp/request`
- `auth.routes.ts` — import `authRateLimit`, mount trên `POST /auth/as-req` và `POST /auth/tgs-req`

---

## File cần tham khảo

| File | Lý do |
|---|---|
| `mini-banking-app/api-gateway/src/middleware/rate-limit.ts` | `rateLimit` factory — preset gọi lại factory này, không viết thêm logic |
| `mini-banking-app/api-gateway/src/app.ts` | Xem cách global limiter được mount — preset route-level mount tương tự |

## File cần tạo

Không có — chỉ sửa file hiện có.

## File bị ảnh hưởng

| File | Ảnh hưởng |
|---|---|
| `api-gateway/src/middleware/rate-limit.ts` | **Sửa trực tiếp** — thêm 2 preset exports |
| `api-gateway/src/routes/otp.routes.ts` | Do Thuận tạo — import `otpRateLimit` |
| `api-gateway/src/routes/auth.routes.ts` | Do Thuận tạo — import `authRateLimit` |
