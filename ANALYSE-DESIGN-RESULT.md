# Analyse Design Result

## 1. Tổng quan đánh giá

Đây là đồ án demo/sandbox với mục tiêu minh họa các khái niệm bảo mật: OTP/PKI, Kerberos-like ticket, Zero-Knowledge key, replay protection, digital signature, immutable ledger. Phạm vi đã được giới hạn rõ ràng trong `proposal.md` (out-of-scope: HSM thật, KMS production, core banking thật).

**Kết luận tổng thể**: Phần lớn thiết kế là hợp lý và phục vụ đúng mục tiêu giáo dục. Có **3 điểm over-engineering** rõ ràng được phân tích dưới đây.

---

## 2. Các phần over-engineering

### 2.1 K_sub (Subsession Key via HKDF) — Over-engineering rõ nhất

**Thiết kế hiện tại**: Với mỗi AP request/giao dịch, client và Bank Service phải derive thêm một subsession key:
```
K_sub = HKDF(K_{c,v}, nonce || timestamp || request_id)
```
Sau đó dùng `K_sub` để mã hóa payload thay vì dùng trực tiếp `K_{c,v}`.

**Tại sao over-engineering**:

- `K_{c,v}` đã là một session key ngắn hạn (5-10 phút TTL, scoped per `Ticket_v`). Kết hợp với AES-GCM dùng **IV/nonce duy nhất cho mỗi lần mã hóa** là đủ để đảm bảo tính tươi mới của từng message.
- Replay protection đã được xử lý hoàn toàn bởi `nonce + timestamp + request_id` trong Authenticator + Redis replay cache. `K_sub` không bổ sung thêm lớp bảo vệ replay.
- Nếu `K_{c,v}` bị lộ, kẻ tấn công biết `nonce`, `timestamp`, `request_id` có thể derive lại `K_sub`. Tức là `K_sub` không cung cấp forward secrecy thực sự trong kịch bản này.
- Đòi hỏi cả browser (WebCrypto HKDF) và Bank Service (Go) phải implement HKDF đồng nhất với cùng input encoding — một điểm dễ sai và khó debug trong demo.
- Chuẩn Kerberos RFC 4120 có `subkey` trong AP_REQ nhưng đây là optional và dùng để client đề xuất thay thế `K_{c,v}` cho session — khác về mục đích so với HKDF-derive per request như thiết kế hiện tại.

**Thiết kế hợp lý hơn**: Dùng `K_{c,v}` trực tiếp với AES-256-GCM, mỗi lần mã hóa dùng IV 96-bit random duy nhất. Nonce/timestamp/request_id trong Authenticator đã đủ để chống replay và bind request vào identity. Không cần thêm HKDF layer.

---

### 2.2 Full mTLS cho tất cả internal services — Arguable over-engineering

**Thiết kế hiện tại**: ADR-02 yêu cầu `gRPC + mTLS/Auth` cho tất cả giao tiếp nội bộ (Gateway↔CA, Gateway↔KDC, Gateway↔Bank, KDC↔CA, Bank↔CA). Điều này cần:
- Một CA riêng (hoặc self-signed) để cấp service certificate cho mỗi service.
- Mỗi service cần cert + key + trust bundle khi start.
- Cấu hình TLS trong cả gRPC server lẫn client của 5 service.

**Tại sao arguable over-engineering cho demo**:

- Trong môi trường Docker Compose, network isolation đã tách internal services khỏi public network. Không có kẻ tấn công có thể MITM traffic giữa containers trong cùng Docker network.
- Setup mTLS cho demo tốn công provisioning và dễ gây lỗi config (cert mismatch, expired cert, trust chain sai) trước khi demo được test functionality.
- Mục tiêu học thuật của gRPC (contract rõ ràng bằng Protobuf, type-safe) đạt được mà không cần mTLS.

**Lưu ý**: Nếu đây là nội dung cần minh họa thì giữ mTLS là hợp lý. Nếu không, có thể simplify.

**Thiết kế hợp lý hơn**: Dùng gRPC với shared service token trong metadata header (mỗi service có một pre-shared API key để caller proof identity). Đây vẫn minh họa service boundary và caller authorization nhưng không cần quản lý service certificate và TLS handshake. Ghi production note: "thay bằng mTLS trong production."

Hoặc: áp dụng mTLS chỉ cho một cặp service (vd: Gateway↔Bank) để minh họa, còn các cặp khác dùng shared token.

---

### 2.3 Circuit Breaker trong system protection — Over-engineering cho demo

**Thiết kế hiện tại**: Phần "Cơ chế bảo vệ hệ thống" đề cập "circuit breaker" cho service dependency failure.

**Tại sao over-engineering**:

- Circuit breaker (trạng thái Closed/Open/Half-Open, failure threshold, recovery timeout) là production resilience pattern.
- Trong Docker Compose demo với 3-5 services, nếu một service chết thì demo fail — không có traffic volume đủ lớn để cần circuit breaker tự động recover.
- Implement circuit breaker đúng nghĩa cần cấu hình threshold và test failure mode — thêm scope không cần thiết cho đồ án.

**Thiết kế hợp lý hơn**: Chỉ cần context timeout + retry tối đa 1 lần với lỗi network/transient + trả lỗi rõ ràng cho client. Ghi note "production dùng circuit breaker" trong ADR là đủ.

---

## 3. Các phần thiết kế hợp lý (không over-engineering)

| Thành phần | Lý do hợp lý |
|---|---|
| Kiến trúc 3 internal services (CA, KDC, Bank) | CA, KDC, Bank là 3 trust domain khác nhau trong PKI + Kerberos; tách service phản ánh đúng trách nhiệm mật mã — đây là nội dung học thuật cần minh họa |
| AS Exchange + TGS Exchange (2-phase Kerberos-like) | Đây là mục tiêu chính của đồ án; độ phức tạp là có chủ đích |
| X.509 PKI + CA Service + CA DB riêng | Quản lý certificate là một trong các mục tiêu chính; CA DB riêng giúp CA phục vụ Admin Dashboard độc lập với Bank data |
| Redis replay cache (`SET NX EX`) | Cơ chế chống replay đơn giản, hiệu quả và dễ test |
| Idempotency key cho transfer | Phân biệt malicious replay (dùng nonce cache) với legitimate retry (dùng idempotency key) — đây là hai vấn đề khác nhau, cần xử lý riêng |
| Hash chaining ledger | Explicit objective trong proposal; không thể bỏ |
| Zero-Knowledge key (WebCrypto + IndexedDB wrapped key) | Explicit objective; cần để minh họa nguyên tắc private key không rời browser |
| Scope trong Ticket_v (`balance:read`, `transfer:create`) | Minh họa scoped authorization, hợp lý và không phức tạp |
| OTP + rate limiting | Không phức tạp và cần thiết để bảo vệ registration endpoint |
| Revocation check trước giao dịch + revocation cache | Bắt buộc về logic bảo mật; cache TTL ngắn là pattern hợp lý |
| Admin Dashboard (list/detail/revoke X.509) | Trong scope đồ án; cần để demo PKI admin flow |
| Strict fail-closed error handling | Nguyên tắc bảo mật cơ bản, không thêm phức tạp implementation |

---

## 4. Thiết kế đề xuất sau khi loại bỏ over-engineering

### Thay đổi 1: Bỏ K_sub, dùng K_{c,v} trực tiếp với random IV

```
AP_REQ gửi:
  - Ticket_v (mã hóa bằng K_v)
  - Authenticator = AES-256-GCM_{K_{c,v}}[ID_c, nonce, timestamp, request_id, IV_auth]
  - CipherPayload = AES-256-GCM_{K_{c,v}}[canonical_payload || digital_signature, IV_payload]

Bank Service verify:
  1. Decrypt Ticket_v với K_v → lấy K_{c,v}
  2. Decrypt Authenticator với K_{c,v} → check nonce/timestamp freshness + replay cache
  3. Decrypt CipherPayload với K_{c,v}
  4. Verify chữ ký số trên canonical_payload bằng pubKeyRSA_c từ certificate
```

Đơn giản hơn, gần với Kerberos standard hơn, dễ debug khi demo.

---

### Thay đổi 2: gRPC + shared service token thay full mTLS (nếu mTLS không là learning objective)

```
Mỗi service được cấp một SERVICE_TOKEN (pre-shared secret, đặt trong env/config).
Caller đặt token vào gRPC metadata: "x-service-token: <token>"
Callee verify token trước khi xử lý — reject nếu token không khớp service identity.
```

Nếu vẫn muốn minh họa mTLS: chỉ áp dụng cho cặp Gateway↔Bank (cặp quan trọng nhất với giao dịch), các cặp còn lại dùng shared token.

---

### Thay đổi 3: Bỏ circuit breaker, giữ timeout + retry có giới hạn

```
- Mỗi gRPC call có context timeout (đề xuất 5s cho lookup, 10s cho transaction)
- Gateway retry tối đa 1 lần với lỗi network/timeout
- Không retry với lỗi auth, replay, authorization (fail closed)
- Trả lỗi có mã cụ thể cho client — không leak internal detail
```

---

## 5. Tóm tắt

| Điểm | Đánh giá | Đề xuất |
|---|---|---|
| K_sub (HKDF subsession key) | **Over-engineering** — không tăng security thực sự cho demo, tăng implementation complexity và điểm dễ sai | Dùng K_{c,v} + AES-GCM với random IV |
| Full mTLS cho tất cả internal services | **Arguable** — heavy setup cho demo; gRPC đã đủ cho contract clarity | Shared service token, hoặc mTLS chỉ 1 cặp nếu muốn minh họa |
| Circuit breaker | **Over-engineering** — production pattern không cần cho Docker Compose demo | Timeout + retry có giới hạn là đủ |
| Phần còn lại của thiết kế | **Hợp lý** — phức tạp là có chủ đích vì phục vụ mục tiêu học thuật | Giữ nguyên |
