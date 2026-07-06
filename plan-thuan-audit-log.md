# Kế hoạch triển khai — Thuận: Hoàn thiện Audit Log (CA + Bank)

Mục tiêu: audit log **ghi được, đọc được, filter được, chứng minh được trong demo** cho cả CA và Bank. Kế hoạch bám theo `process.md` mục "Thuận - Audit log còn thiếu" và đã đối chiếu với code hiện tại.

---

## 1. Hiện trạng code (đã xác minh)

### 1.1. Đã có

| Thành phần | File | Ghi chú |
|---|---|---|
| Bank audit write | `mini-banking-app/banking-service/internal/bank/audit.go` | `Service.Audit()` best-effort, nuốt lỗi insert — đúng nguyên tắc "audit fail không crash request". |
| Bank audit insert | `mini-banking-app/banking-service/internal/bank/repository.go:51` | `InsertAudit(ctx, q, e)`. |
| CA audit write | `mini-banking-app/ca-service/internal/ca/service.go` (4 chỗ gọi `AppendAudit`, dòng ~242/275/344/371) | Đã ghi cho issue, verify/revocation check, detail lookup, revoke; dùng `_ =` nên cũng không crash. |
| CA store JSON | `ca-service/internal/ca/store.go` | Có `AppendAudit` + `AuditEvents()` (đọc toàn bộ, không filter). |
| CA store Postgres | `ca-service/internal/ca/postgres_store.go:235` | Chỉ có `AppendAudit`, **chưa có ListAudit**. |
| Schema CA | `mini-banking-app/db/ca/migrations/001_init_ca.sql` | Bảng `certificate_audit_log`, CHECK action IN (`issued`, `revoked`, `looked_up`, `revocation_checked`, `verify_certificate`); revoked bắt buộc có reason. Index theo serial, action, performed_at. |
| Schema Bank | `mini-banking-app/db/bank/migrations/001_init_bank.sql` | Bảng `bank_audit_log`, CHECK action IN 7 giá trị (xem §3.2). Index theo created_at, action, user_id, request_id. |

### 1.2. Còn thiếu (phạm vi của Thuận)

- **Không có API đọc audit** ở cả CA lẫn Bank (CA `Repository` interface chỉ có `AppendAudit`; Bank service chưa có method list audit; Gateway chưa có route `/v1/admin/*`).
- CA proto **chưa có RPC audit** và chưa có field `request_id` trong các RPC admin — muốn lưu request id vào CA audit phải thêm proto field hoặc đọc gRPC metadata.
- Metadata audit chưa chuẩn hóa (request_id / performed_by / ip / user_agent không nhất quán giữa các flow).
- Chưa có test suite / checklist chứng minh audit hoạt động.

---

## 2. Vấn đề ảnh hưởng đến công việc (rủi ro & phụ thuộc)

| # | Vấn đề | Ảnh hưởng | Cách xử lý |
|---|---|---|---|
| 1 | Gateway chưa mount `/v1/admin/*`, chưa có admin auth middleware (Thanh làm) | Endpoint đọc audit không có chỗ mount nếu Thanh chậm | Ngày 1 chốt contract; nếu bị block, tự mount tạm route audit dưới prefix riêng + middleware demo tối thiểu, bàn giao lại sau |
| 2 | Bank admin đi hướng gRPC hay query DB trực tiếp là quyết định của Thái | Endpoint `/v1/admin/audit/bank` phụ thuộc hướng này | Chốt với Thái ngay đầu Ngày 1; audit đi cùng hướng với các endpoint bank admin khác |
| 3 | CA proto không có RPC audit → thêm RPC nghĩa là sửa proto + regenerate Go/TS | Tốn thời gian, đụng generated code | Chọn 1 trong 2 hướng ở §4.2; quyết định trước khi code |
| 4 | Action enum bị CHECK constraint trong DB | Insert action ngoài enum sẽ **fail âm thầm** (bị nuốt lỗi) → mất event mà không ai biết | Mọi action mới phải kèm migration ALTER CHECK; viết test rà enum code ↔ DB |
| 5 | Request id 2 lớp: HTTP `X-Request-ID` (Gateway) vs `request_id` trong body/authenticator (Bank AP flow) | Dễ ghi nhầm nguồn, khó trace xuyên suốt | Chuẩn §3.3 quy định rõ nguồn cho từng loại event |
| 6 | Bank cột `user_id`/`account_id`/`transaction_id` là UUID | Event auth-layer (invalid_signature…) có thể chưa resolve được UUID → insert fail | Cho phép NULL (schema đã cho phép), đẩy thông tin thô vào `metadata` |
| 7 | CA JSON store vs Postgres store là 2 backend | ListAudit phải implement cả 2, filter khác nhau | JSON store: filter in-memory trên `AuditEvents()`; Postgres: SQL WHERE |
| 8 | Repo dùng CRLF, Go files không gofmt-clean | Không bulk-reformat; match style file hiện có | Tuân thủ khi sửa Go |

---

## 3. CHUẨN AUDIT LOG (bắt buộc, dùng chung cho cả nhóm)

### 3.1. Nguyên tắc chung

1. **Audit không bao giờ làm fail request chính.** Insert lỗi → log warning ra stderr/service log, request vẫn trả kết quả. (Bank đã đúng; CA dùng `_ =` — bổ sung log warning thay vì nuốt hẳn.)
2. **Mỗi event ghi đúng 1 lần** tại tầng service (không ghi trùng ở cả handler và service).
3. **Action phải nằm trong enum DB.** Thêm action mới = thêm migration ALTER CHECK + cập nhật bảng §3.2 này.
4. **Ghi tại thời điểm quyết định**: event từ chối (replay, invalid_signature…) ghi ngay khi reject; event thành công ghi sau khi commit transaction.
5. **Không ghi dữ liệu nhạy cảm** vào audit: không private key, không OTP, không full signature/ticket — chỉ ghi hash/serial/id.

### 3.2. Chuẩn action enum

**CA — `certificate_audit_log.action`** (khớp CHECK trong `001_init_ca.sql`):

| Action | Khi nào ghi | Bắt buộc kèm |
|---|---|---|
| `issued` | Cấp cert thành công | serial_number, performed_by (`system:register` hoặc email user) |
| `looked_up` | Admin xem detail cert | serial_number, performed_by = `admin:<email>` |
| `revocation_checked` | KDC/Bank check trạng thái revoke | serial_number, performed_by = tên service gọi |
| `verify_certificate` | Verify chữ ký/cert | serial_number, performed_by |
| `revoked` | Revoke thành công | serial_number, performed_by = `admin:<email>`, **reason bắt buộc** (DB constraint) |

**Bank — `bank_audit_log.action`** (khớp CHECK trong `001_init_bank.sql`):

| Action | Khi nào ghi | Field bắt buộc |
|---|---|---|
| `transfer_completed` | Transfer commit thành công | user_id, account_id, transaction_id, cert_serial, request_id |
| `transfer_rejected` | Transfer bị từ chối (lỗi nghiệp vụ khác các loại dưới) | user_id (nếu có), request_id, reason |
| `replay_detected` | Nonce/authenticator bị dùng lại | cert_serial, request_id, reason |
| `invalid_signature` | Chữ ký sai | cert_serial (nếu có), request_id, reason |
| `certificate_rejected` | Cert revoked/expired | cert_serial, request_id, reason |
| `forbidden_ownership` | Account không thuộc user | user_id, account_id, request_id, reason |
| `insufficient_funds` | Không đủ số dư | user_id, account_id, request_id |

### 3.3. Chuẩn field & nguồn dữ liệu

**CA event** (struct `ca.AuditEvent`): `serial_number`, `action`, `performed_by`, `reason`, `performed_at`, `metadata` (JSONB).

**Bank event** (struct `bank.AuditEvent`): `action`, `user_id`, `account_id`, `transaction_id`, `cert_serial`, `request_id`, `reason`, `metadata`, `created_at`.

Quy ước nguồn:

| Field | Nguồn | Quy ước |
|---|---|---|
| `request_id` (Bank user flow) | `request_id` trong body/authenticator của AP flow | Giữ nguyên như hiện tại |
| `request_id` (admin flow, CA) | HTTP `X-Request-ID` — Gateway tự sinh UUID v4 nếu client không gửi | CA: truyền qua gRPC metadata key `x-request-id` (hoặc proto field nếu bổ sung); ghi vào `metadata.request_id` |
| `performed_by` | JWT claim admin ở Gateway | Format: `admin:<email>`; flow tự động: `system:<flow>` (vd `system:register`); service gọi: `service:<name>` (vd `service:kdc`) |
| `metadata.ip`, `metadata.user_agent` | Gateway lấy từ HTTP request, truyền xuống qua metadata | Best-effort, không có thì bỏ qua |
| `metadata.route` | Route/method gây ra event | vd `POST /v1/admin/ca/certificates/:serial/revoke` |
| `reason` | Chuỗi ngắn, tiếng Anh, snake/lower | vd `nonce already used`, `certificate revoked` |

### 3.4. Chuẩn API đọc audit

- `GET /v1/admin/audit/ca?action&serial&performed_by&from&to&limit&offset`
- `GET /v1/admin/audit/bank?action&user_id&cert_serial&request_id&from&to&limit&offset`

Quy tắc chung:

- `limit` mặc định 20, max 100; `offset` >= 0; sort mặc định `performed_at`/`created_at` DESC.
- `from`/`to` là ISO 8601 (`2026-07-04T00:00:00Z`); parse fail → 400.
- `action` ngoài enum → 400 kèm danh sách hợp lệ.
- Response chuẩn (thống nhất với Thái):
  ```json
  { "success": true, "data": { "items": [...], "total": 123, "limit": 20, "offset": 0 },
    "request_id": "<uuid>", "timestamp": "<iso>" }
  ```
- Lỗi: `{ "success": false, "error_code": "...", "message": "...", "request_id": "..." }`.
- Endpoint audit là **read-only** và bản thân nó **không ghi audit** (tránh vòng lặp log).

---

## 4. Nội dung cần audit — rà từng điểm ghi (đã có / còn thiếu / cách bổ sung)

Đã grep toàn bộ call site `AppendAudit` (CA) và `s.Audit(...)`/`h.bank.Audit(...)` (Bank) trong code hiện tại.

### 4.1. CA — `ca-service/internal/ca/service.go`

| Nội dung cần audit | Vị trí code | Hiện trạng | Cách bổ sung |
|---|---|---|---|
| Cấp cert thành công → `issued` | `RegisterUser`, service.go:242 | ✅ Đã có, kèm metadata `request_id`, `owner_id` | Không cần sửa |
| Verify cert / check revocation → `verify_certificate` / `revocation_checked` | `VerifyCertificate`, service.go:275; caller `system:kdc-service`/`system:bank-service` thì ghi `revocation_checked` (dòng 272) | ✅ Đã có, kèm `request_id` trong metadata | Không cần sửa. `GetCertificate`/`CheckRevocation` (dòng 289, 302) đều gọi qua `VerifyCertificate` với caller `legacy:*` nên cũng được audit |
| Admin xem detail → `looked_up` | `GetCertificateDetail`, service.go:344 | ✅ Đã có, `performed_by` fallback `admin:unknown` | Không cần sửa logic; cần Thanh truyền `performedBy` + `requestID` thật từ Gateway (hiện Gateway chưa có route admin nên đang là giá trị fallback) |
| Revoke → `revoked` + reason | `RevokeCertificate`, service.go:371 | ✅ Đã có | Không cần sửa |
| Verify/detail/revoke với **serial không tồn tại** | Các hàm trên return error **trước** khi tới `AppendAudit` | ❌ Chưa ghi | Tùy chọn (P2): ghi event với reason `not_found`. Lưu ý enum không có action riêng — nếu làm thì dùng action hiện có + `metadata.result=not_found`, không thêm enum |
| `ListCertificates` (admin xem danh sách) | service.go:313 | ❌ Không audit | **Chủ đích bỏ qua** — list rất noisy và enum không có action `listed`. Ghi rõ quyết định này vào docs, không thêm migration |
| Lỗi insert audit bị nuốt hẳn (`_ =`) | 4 call site trên | ⚠️ Nuốt lỗi im lặng | Bổ sung: bọc thành helper ghi `fmt.Printf("[CA] warning: audit append failed: %v", err)` (theo style log sẵn có dòng 253), giữ nguyên không return error |

### 4.2. Bank — `banking-service/internal/bank/service.go` + `internal/grpc/auth.go`, `handler.go`

| Nội dung cần audit | Vị trí code | Hiện trạng | Cách bổ sung |
|---|---|---|---|
| Transfer thành công → `transfer_completed` | service.go:347 (đủ user/account/transaction/cert/request_id) | ✅ Đã có | Không cần sửa |
| Transfer rejected (validate payload, business) → `transfer_rejected` | service.go:214, 251; handler.go:107, 111, 115 | ✅ Đã có | Không cần sửa |
| Replay → `replay_detected` | auth.go:213 (`redis_replay`), auth.go:223 (`db_replay`) | ✅ Đã có | Không cần sửa |
| Chữ ký payload transfer sai → `invalid_signature` | handler.go:119 | ✅ Đã có | Không cần sửa |
| Cert revoked/expired/owner mismatch/CA down → `certificate_rejected` | auth.go:88, 96, 100 | ✅ Đã có | Không cần sửa |
| Ownership sai → `forbidden_ownership` | service.go:123 (balance), 170 (history), 272 (transfer from-account) | ✅ Đã có | Không cần sửa |
| Thiếu tiền → `insufficient_funds` | service.go:284 | ✅ Đã có | Không cần sửa |
| Auth-layer fail (invalid_ticket, ticket_expired, wrong_scope, invalid_authenticator, stale_request) | auth.go:41–77 — ghi action `transfer_rejected` **cho mọi scope**, kể cả khi request là balance/history/profile | ⚠️ Action gây hiểu nhầm | Không đổi enum (tránh migration + vỡ contract với Thái). Bổ sung: truyền scope vào `AuditEvent.Metadata` (vd `metadata: map[string]any{"scope": scope}`) tại `authorize()`/`markReplay()` để phân biệt khi đọc. Ghi rõ quy ước này vào docs |
| Event sớm thiếu `RequestID` (invalid_ticket → invalid_authenticator, auth.go:41–66) | Authenticator chưa giải mã được nên chưa có request_id | ⚠️ Chấp nhận được | Không sửa — document là hạn chế đã biết; các event này vẫn trace được qua `created_at` + cert_serial |
| `UserID` = `caller.ClientID` nhưng cột DB là UUID | service.go/auth.go, mọi event | ⚠️ Cần xác minh | Kiểm tra ClientID thực tế là UUID hay email; nếu là email thì insert **fail âm thầm** (rủi ro #4). Nếu vậy: để `user_id` NULL, đẩy giá trị thô vào `metadata.client_id` |
| `CreateUser` (handler.go:59) | Không có audit | ❌ Chưa ghi | **Chủ đích bỏ qua** — enum không có action user-creation, ngoài scope 3 ngày. Ghi note vào docs |
| Balance/history/profile **thành công** | Không có audit | ❌ Chưa ghi | **Chủ đích bỏ qua** — chỉ audit event bảo mật/nghiệp vụ tiền, đọc dữ liệu thành công không cần audit (noisy). Ghi note |

### 4.3. Kết luận rà soát

- **Phần ghi (write) đã đủ ~95%** cho demo — cả CA và Bank đều cover hết các action trong enum DB. Việc phải làm ở tầng write chỉ là 3 việc nhỏ: (1) log warning khi CA append fail, (2) thêm `metadata.scope` cho event auth-layer Bank, (3) xác minh `ClientID` là UUID.
- **Phần đọc (read) là 0%** — không có `ListAudit` ở cả hai service, không có endpoint. Đây là trọng tâm của Ngày 2 (§5, Bước 5–7).

---

## 5. Các bước thực hiện (step-by-step)

### Ngày 1 — Rà soát, chốt contract, viết testcase

**Bước 1. Chốt contract & phân giới (sáng, ~1h, cùng Thanh + Thái)**
- Với Thái: chốt hướng Bank admin (gRPC vs query DB trực tiếp) → endpoint `/v1/admin/audit/bank` đi cùng hướng. Chốt response envelope §3.4.
- Với Thanh: chốt admin middleware dùng chung, cách truyền `performed_by` + `X-Request-ID` xuống CA gRPC.
- Chốt hướng CA audit read (Bước 4) — khuyến nghị **Hướng A**.

**Bước 2. Sửa các điểm ghi audit CA theo kết quả rà soát §4.1** (`ca-service/internal/ca/service.go`)
- Kết quả rà soát: 4 call `AppendAudit` (dòng 242, 275, 344, 371) đã map đúng bảng §3.2 — không cần thêm điểm ghi.
- Việc phải làm: đổi `_ = s.repository.AppendAudit(...)` thành helper ghi kèm log warning khi lỗi (không return error).
- Ghi note cho Thanh: `performedBy`/`requestID` của detail/revoke hiện đang rơi về fallback `admin:unknown` vì Gateway chưa có route admin — Thanh phải truyền giá trị thật khi wrap REST.

**Bước 3. Sửa các điểm ghi audit Bank theo kết quả rà soát §4.2** (`banking-service/internal/`)
- Kết quả rà soát: cả 7 action §3.2 đều đã có điểm ghi — không cần thêm điểm ghi mới.
- Việc phải làm: thêm `metadata.scope` vào các event auth-layer trong `grpc/auth.go` (hiện balance/history fail đều ghi `transfer_rejected` không phân biệt scope).
- Xác minh `caller.ClientID` là UUID; nếu là email thì insert audit fail âm thầm — chuyển giá trị vào `metadata.client_id`, để `user_id` NULL.
- Viết test nhỏ (hoặc rà tay) đối chiếu action string trong code khớp CHECK constraint DB — tránh rủi ro #4.

**Bước 4. Viết trước testcase audit checklist** — file `mini-banking-app/docs/audit-testcases.md` theo bảng §6.

### Ngày 2 — Implement API đọc audit

**Bước 5. CA — ListAudit ở repository**

Chọn hướng (quyết định ở Bước 1):
- **Hướng A (khuyến nghị): thêm gRPC `ListAuditEvents` vào CA proto.** Đúng kiến trúc, Thanh dùng lại được cho Audit tab.
- Hướng B (fallback nếu thiếu thời gian): Gateway query thẳng `certificate_audit_log` bằng connection read-only — ghi rõ trong docs là đường tắt demo.

Việc cụ thể cho Hướng A:
1. Thêm vào `Repository` interface (`service.go:108`): `ListAudit(ctx, AuditFilter) ([]AuditEvent, int, error)` với `AuditFilter{Serial, Action, PerformedBy, From, To, Limit, Offset}`.
2. `postgres_store.go`: implement bằng SQL `SELECT ... FROM certificate_audit_log WHERE ... ORDER BY performed_at DESC LIMIT $ OFFSET $` + `COUNT(*)` cùng filter (index đã sẵn cho serial/action/performed_at).
3. `store.go` (JSON): filter in-memory trên `AuditEvents()`, sort DESC, cắt limit/offset.
4. Proto: thêm `ListAuditEvents(ListAuditRequest) returns (ListAuditResponse)`; regenerate Go server + TS client; implement gRPC handler gọi service.
5. Nhân tiện thêm field/metadata `request_id` cho các RPC admin (đọc gRPC metadata `x-request-id` ở handler, đưa vào `metadata` của event).

**Bước 6. Bank — list audit**
- Theo hướng của Thái: nếu gRPC → thêm `ListAuditEvents` vào Bank proto + repository method `ListAudit(ctx, filter)`; nếu query DB → cung cấp cho Thái SQL chuẩn:
  ```sql
  SELECT id, action, user_id, account_id, transaction_id, cert_serial,
         request_id, reason, metadata, created_at
  FROM bank_audit_log
  WHERE ($1 = '' OR action = $1)
    AND ($2::uuid IS NULL OR user_id = $2)
    AND ($3 = '' OR cert_serial = $3)
    AND ($4 = '' OR request_id = $4)
    AND created_at >= COALESCE($5, '-infinity'::timestamptz)
    AND created_at <  COALESCE($6, 'infinity'::timestamptz)
  ORDER BY created_at DESC
  LIMIT $7 OFFSET $8;
  ```
- Chốt JSON shape trả về đúng §3.4 để UI Admin Bank của Thái dùng thẳng.

**Bước 7. Gateway — route + controller audit**
- Tạo `api-gateway/src/routes/admin-audit.route.ts`, `controller/admin-audit.controller.ts`; mount dưới `/v1/admin/audit` trong `server.ts` (reuse admin middleware của Thanh; nếu chưa có, viết middleware demo check Bearer token + role).
- Validate query theo §3.4 (limit/offset/enum/date) trước khi gọi service.
- Map lỗi: invalid argument→400, service unavailable→502, còn lại→500.
- Tự sinh `X-Request-ID` nếu thiếu, echo vào response envelope.

**Bước 8. Build & test kỹ thuật**
- `go build ./...` + `go test ./...` trong `ca-service` và `banking-service`.
- `npx.cmd tsc --noEmit` trong `api-gateway`.
- Curl từng endpoint: no filter, filter action, filter serial/user, date range, limit=101 (→400), action rác (→400).

### Ngày 3 — Regression, seed tình huống, tài liệu

**Bước 9. Chạy audit regression theo checklist §6** trên stack full local; điền pass/fail.

**Bước 10. Xác nhận UI** — Audit tab của Thanh (Admin CA) và Security Audit tab của Thái (Admin Bank) hiển thị data thật từ endpoint; fix contract mismatch nếu có.

**Bước 11. Tài liệu bàn giao** — hoàn thiện `docs/audit-testcases.md`:
- Bảng mapping `event → cách kích hoạt → nơi kiểm tra` (§6).
- Chuẩn field audit (§3.3) + enum (§3.2).
- 6–8 curl mẫu cho Quang đưa vào demo script.
- Ghi chú retention: trước demo backup DB (`pg_dump`) hoặc export audit ra CSV/JSON; note những gì còn thiếu.

---

## 6. Testcase audit (mapping event → kích hoạt → kiểm tra)

| # | Tình huống kích hoạt | Event mong đợi | Nơi kiểm tra |
|---|---|---|---|
| 1 | Đăng ký user mới (OTP → PKI register) | CA `issued` | `GET /v1/admin/audit/ca?action=issued` hoặc `certificate_audit_log` |
| 2 | Mở detail cert trong Admin CA | CA `looked_up`, performed_by=`admin:<email>` | audit CA filter serial |
| 3 | Revoke cert (có reason) | CA `revoked` + reason | audit CA filter action=revoked |
| 4 | Login/AS/TGS bằng cert đã revoke | CA `revocation_checked`/`verify_certificate` + flow bị reject | audit CA + response lỗi flow |
| 5 | Transfer thành công | Bank `transfer_completed` có transaction_id, request_id | `GET /v1/admin/audit/bank?action=transfer_completed` |
| 6 | Gửi lại cùng request/nonce | Bank `replay_detected` (hoặc idempotency response đúng) | audit Bank filter request_id |
| 7 | Transfer từ account không thuộc user | Bank `forbidden_ownership` | audit Bank |
| 8 | Payload chữ ký sai (nếu dựng được payload test) | Bank `invalid_signature` | audit Bank |
| 9 | Transfer vượt số dư | Bank `insufficient_funds` | audit Bank |
| 10 | Transfer với cert revoked | Bank `certificate_rejected` | audit Bank |
| 11 | Query audit với action rác / limit>100 / date sai | HTTP 400, message rõ | curl endpoint audit |
| 12 | Tắt Postgres audit rồi thực hiện request chính | Request chính vẫn thành công, có warning log | service log |

Mỗi dòng có cột kết quả: **pass/fail/owner/note** khi chạy regression Ngày 3.

---

## 7. Deliverable

1. CA `ListAudit` (JSON + Postgres store) + gRPC/REST đọc audit CA qua `GET /v1/admin/audit/ca`.
2. Bank audit đọc được qua `GET /v1/admin/audit/bank` (tự implement hoặc SQL/contract bàn giao cho Thái).
3. Chuẩn audit (§3) được cả nhóm dùng thống nhất.
4. `docs/audit-testcases.md` với checklist pass/fail + curl mẫu cho Quang.
5. UI Admin CA/Bank có data audit thật để hiển thị.
