# Sprint 2 — Progress Report

**Ngày:** 2026-06-08  
**Sprint Goal:** Hoàn thiện CA/KDC theo blueprint để các flow PKI và Kerberos-like có nền tảng tích hợp.

---

## Tóm tắt quá trình thực hiện

Sprint 2 tập trung vào CA Service (Thanh), KDC Service (Quang), Gateway PKI/Auth routes (Thuận) và Gateway validation cho auth (Thái). Phần Thái phụ trách đã hoàn thành và verify. Các phần còn lại của nhóm chưa thực hiện — phần lớn là carryover từ Sprint 1 dang dở chưa được unblock.

Hiện tại **phần Gateway middleware của Thái hoàn chỉnh**, nhưng chưa có route nào mount validators này do Thuận chưa tạo `auth.routes.ts`, `pki.routes.ts`, `otp.routes.ts`. CA và KDC vẫn không build được do Sprint 1 chưa fix xong.

---

## Đã hoàn thành

### Thái — Mở rộng `validate.ts` cho auth binary/base64 và scope (Task 1) ✅

`tsc --noEmit` PASS.

| File | Thay đổi |
|---|---|
| `api-gateway/src/middleware/validate.ts` | Thêm `requireBase64Fields` — kiểm tra ký tự base64 hợp lệ cho các bytes fields (`nonce`, `signature`, `tgt`, `authenticator`) |
| `api-gateway/src/middleware/validate.ts` | Thêm `validateScope(allowed)` — kiểm tra `req.body.scope` trong whitelist, trả 400 `INVALID_SCOPE` nếu sai |
| `api-gateway/src/middleware/validate.ts` | Thêm `validateTimestamp` — kiểm tra `req.body.timestamp` là Unix seconds nguyên trong cửa sổ ±5 phút, trả 400 `STALE_REQUEST` nếu lệch |

### Thái — Auth error factories trong `error-envelope.ts` (Task 2) ✅

`tsc --noEmit` PASS.

| Export | HTTP | Code | Khi nào |
|---|---|---|---|
| `certRevokedError()` | 403 | `CERT_REVOKED` | CA trả `CERT_STATUS_REVOKED` |
| `certExpiredError()` | 403 | `CERT_EXPIRED` | CA trả `CERT_STATUS_EXPIRED` |
| `ticketExpiredError()` | 401 | `TICKET_EXPIRED` | KDC/Bank check TTL ticket |
| `replayDetectedError()` | 409 | `REPLAY_DETECTED` | nonce hoặc ticket_id đã dùng |
| `scopeDeniedError(scope)` | 403 | `SCOPE_DENIED` | Scope trong ticket không match |
| `invalidSignatureError()` | 400 | `INVALID_SIGNATURE` | Signature verify fail |

### Thái — Preset rate limiters trong `rate-limit.ts` (Task 3) ✅

`tsc --noEmit` PASS.

| Export | windowMs | max | Route áp dụng |
|---|---|---|---|
| `otpRateLimit` | 15 phút | 5 | `POST /v1/otp/request` |
| `authRateLimit` | 1 phút | 10 | `POST /v1/auth/as-req`, `POST /v1/auth/tgs-req` |

---

## Đang thực hiện

### Quang — Fix KDC enum (carryover Sprint 1, chưa xong)

**Lỗi hiện tại:**
```
kdc-service/internal/kdc/service.go:188:
undefined: capb.CertStatus_CERT_STATUS_VALID
```

**Fix cần làm:** Đổi `capb.CertStatus_CERT_STATUS_VALID` → `capb.CertStatus_CERT_STATUS_ACTIVE` tại dòng 188.

### Thanh — Fix CA handler + stub RPCs (carryover Sprint 1, chưa xong)

**Lỗi hiện tại:**
```
ca-service/internal/grpc/handler.go:59,63,70:
req.UserId undefined (type *capb.RegisterUserRequest has no field or method UserId)
```

**Fix cần làm:** Đổi `req.UserId` → `req.OwnerId` tại 3 dòng. Stub thêm: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail`.

### Thuận — Gateway CA/KDC clients + Auth/OTP routes (carryover Sprint 1, chưa bắt đầu)

Các task Thuận chưa thực hiện:
- Khởi tạo `ca.client.ts` và `kdc.client.ts`
- Tạo `src/routes/otp.routes.ts` — `POST /otp/request`, `POST /otp/verify`
- Tạo `src/routes/pki.routes.ts` — `POST /pki/register`
- Tạo `src/routes/auth.routes.ts` — `POST /auth/as-req`, `POST /auth/tgs-req`
- Mount vào `app.ts`

### Thanh — CA Service core Sprint 2 (chưa bắt đầu)

- Thiết kế CA repository/store theo schema `certificates` và `certificate_audit_log`
- Implement lưu certificate metadata đầy đủ cho `RegisterUser`
- Implement `VerifyCertificate` trả status, validity, public key theo flags
- Implement admin list/detail/revoke và audit event

### Quang — KDC Service core Sprint 2 (chưa bắt đầu)

- Đổi AS pre-auth sang gọi `CA.VerifyCertificate` thay vì legacy `GetCertificate`
- Chuẩn hóa `AS_REQ`: nonce, timestamp, request_id, signature verification
- Chuẩn hóa `TGS_REQ`: TGT, authenticator, scope, service_id, replay check

---

## Chưa hoàn thành

Theo thứ tự ưu tiên để unblock Sprint 3:

| # | Task | Owner | Unblock |
|---|---|---|---|
| 1 | Fix KDC `CERT_STATUS_VALID` → `CERT_STATUS_ACTIVE` (1 dòng) | Quang | KDC build |
| 2 | Fix CA `req.UserId` → `req.OwnerId` (3 dòng) | Thanh | CA build |
| 3 | Stub CA: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail` | Thanh | CA build hoàn chỉnh |
| 4 | Tạo `ca.client.ts`, `kdc.client.ts` trong Gateway | Thuận | PKI/Auth routes |
| 5 | Tạo `otp.routes.ts`, `pki.routes.ts`, `auth.routes.ts` + mount | Thuận | Validators của Thái có nơi chạy, Gateway smoke test |
| 6 | Implement CA `VerifyCertificate` thật (trả status, key, validity) | Thanh | KDC/Bank trust cert, test `certRevokedError` factory |
| 7 | Implement Gateway `/v1/pki/register`, `/v1/auth/as-req`, `/v1/auth/tgs-req` | Thuận | End-to-end PKI → AS → TGS |
| 8 | Chuẩn hóa AS/TGS fields và replay check trong KDC | Quang | KDC contract ổn định, confirm validators đúng field |

---

## Vấn đề còn tồn tại

### Bug / Build blocker

| Vấn đề | File | Severity |
|---|---|---|
| `CertStatus_CERT_STATUS_VALID` không tồn tại | `kdc-service/internal/kdc/service.go:188` | HIGH — KDC không build |
| `req.UserId` không tồn tại | `ca-service/internal/grpc/handler.go:59,63,70` | HIGH — CA không build |
| CA handler thiếu `VerifyCertificate` stub | `ca-service/internal/grpc/handler.go` | HIGH — KDC gọi CA fail runtime |

### Technical debt

| Vấn đề | Mô tả |
|---|---|
| `otpRateLimit` / `authRateLimit` chưa được mount | Preset đã có nhưng Thuận chưa tạo route để dùng — giới hạn OTP chưa có hiệu lực |
| Auth error factories chưa được test trong context thật | `certRevokedError` v.v. chỉ pass tsc, chưa verify với CA response thật |
| Rate limit vẫn in-memory | Vẫn là `Map` in-memory như Sprint 1 — chưa dùng Redis, chưa share state giữa nhiều instance |
| Module naming inconsistent | `ca-service` dùng `mini-banking/pkg` (hyphen), `kdc-service` dùng `mini_banking/pkg` (underscore) — chưa thống nhất |

### Hạn chế hiện tại

| Hạn chế | Tác động |
|---|---|
| Chưa có OTP/PKI/Auth routes trong Gateway | Không thể test đăng ký hay đăng nhập end-to-end |
| CA/KDC không build | Smoke test PKI register → AS_REQ → TGS_REQ không chạy được |
| `validateTimestamp` chưa được verify với KDC thật | Cửa sổ ±5 phút giả định đúng contract KDC, cần Quang xác nhận |
| Bank Service 4 RPC vẫn trả `Unimplemented` | Chưa có core implementation — carryover sang Sprint 3 |

---

## Sprint 2 Definition of Done — checklist

| Tiêu chí | Trạng thái |
|---|---|
| `go test ./...` CA/KDC pass hoặc lỗi được ghi rõ | ❌ CA/KDC không build — lỗi từ Sprint 1 chưa fix |
| `VerifyCertificate` là RPC chính cho KDC | ❌ Thanh chưa implement |
| Gateway gọi được CA/KDC qua PKI/Auth routes | ❌ Thuận chưa tạo routes |
| Gateway validation cho auth binary/base64 fields và scope | ✅ `requireBase64Fields`, `validateScope`, `validateTimestamp` hoàn chỉnh |
| Auth error factories chuẩn format cho frontend | ✅ 6 factories export từ `error-envelope.ts` |
| OTP/Auth rate limit chặt hơn global | ✅ `otpRateLimit`, `authRateLimit` sẵn sàng, chờ route mount |
| Demo nội bộ chạy PKI register → AS_REQ → TGS_REQ ở mức smoke | ❌ Phụ thuộc CA/KDC build + Gateway routes |
