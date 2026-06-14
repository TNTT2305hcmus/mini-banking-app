# Task 2 Report — Auth error factories trong `error-envelope.ts`

**Ngày:** 2026-06-08  
**Status:** ✅ Hoàn thành

---

## Đã làm

Thêm 6 factory functions vào cuối `api-gateway/src/middleware/error-envelope.ts`:

| Factory | HTTP | Code |
|---|---|---|
| `certRevokedError()` | 403 | `CERT_REVOKED` |
| `certExpiredError()` | 403 | `CERT_EXPIRED` |
| `ticketExpiredError()` | 401 | `TICKET_EXPIRED` |
| `replayDetectedError()` | 409 | `REPLAY_DETECTED` |
| `scopeDeniedError(scope)` | 403 | `SCOPE_DENIED` |
| `invalidSignatureError()` | 400 | `INVALID_SIGNATURE` |

`AppError` class, `errorEnvelope` middleware, `grpcToHttp` map không thay đổi.

---

## Verify

- `tsc --noEmit` trong `api-gateway/` — **PASS**, không có lỗi type.

---

## Còn lại

Factories sẵn sàng để Thuận import trong `auth.routes.ts` và `pki.routes.ts`. Chưa có route nào dùng — test thực tế phụ thuộc task 5–7 trong tiền điều kiện Sprint 2.
