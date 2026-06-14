# Phân tích Task 2: Cập nhật `error-envelope.ts` với auth error factories

Task này chỉ sửa 1 file: `api-gateway/src/middleware/error-envelope.ts`.  
Thêm 6 factory functions export mới. Không sửa `AppError` class, không sửa `errorEnvelope` middleware.

---

## Hiện trạng `error-envelope.ts`

- `AppError` class — dùng lại được, không cần sửa
- `errorEnvelope` Express error handler — đủ cho Bank errors, không cần sửa
- `grpcToHttp` map — cover các gRPC status chung, không cần sửa

Thiếu: khi Thuận xử lý CA/KDC response, cần throw lỗi với code cụ thể hơn thay vì chỉ `PERMISSION_DENIED` generic.

---

## Các factory cần thêm

| Factory | HTTP | Code | Khi nào dùng |
|---|---|---|---|
| `certRevokedError()` | 403 | `CERT_REVOKED` | CA trả `CERT_STATUS_REVOKED` |
| `certExpiredError()` | 403 | `CERT_EXPIRED` | CA trả `CERT_STATUS_EXPIRED` |
| `ticketExpiredError()` | 401 | `TICKET_EXPIRED` | KDC/Bank kiểm tra TTL của ticket |
| `replayDetectedError()` | 409 | `REPLAY_DETECTED` | nonce hoặc ticket_id đã được dùng |
| `scopeDeniedError(scope)` | 403 | `SCOPE_DENIED` | Scope trong ticket không match service |
| `invalidSignatureError()` | 400 | `INVALID_SIGNATURE` | Verify signature thất bại |

Mỗi factory là một hàm trả `new AppError(httpStatus, code, message)` — pattern giống nhau, message phải generic (không lộ lý do nội bộ).

---

## Cách Thuận sẽ dùng

Thuận import factory rồi truyền vào `next()` sau khi check CA/KDC response status. Frontend nhận response chuẩn format `{ "error": { "code": "...", "message": "..." } }`.

---

## File cần tham khảo

| File | Lý do |
|---|---|
| `mini-banking-app/api-gateway/src/middleware/error-envelope.ts` | `AppError` class — factory chỉ gọi `new AppError(...)`, không import thêm gì |
| `mini-banking-app/proto/ca.proto` (dòng 37–42) | `CertStatus` enum — biết khi nào CA trả `REVOKED` vs `EXPIRED` để chọn đúng factory |
| `mini-banking-app/blueprint/design.md` (phần Error handling security rule) | Quy tắc: message phải đủ cho frontend hiểu, không lộ nội bộ |

## File cần tạo

Không có — chỉ sửa file hiện có.

## File bị ảnh hưởng

| File | Ảnh hưởng |
|---|---|
| `api-gateway/src/middleware/error-envelope.ts` | **Sửa trực tiếp** — thêm 6 factory exports |
| `api-gateway/src/routes/auth.routes.ts` | Do Thuận tạo — import `certRevokedError`, `certExpiredError`, `invalidSignatureError` |
| `api-gateway/src/routes/pki.routes.ts` | Do Thuận tạo — import `certRevokedError`, `replayDetectedError` |
