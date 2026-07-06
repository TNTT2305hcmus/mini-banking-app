# Audit Log — Chuẩn field, testcase và curl mẫu

Tài liệu bàn giao của Thuận cho nhóm (Thanh/Thái nối UI, Quang đưa vào demo script).

## 1. Chuẩn field audit

### CA — `certificate_audit_log`

| Field | Ý nghĩa | Ghi chú |
|---|---|---|
| `serial_number` | Serial cert liên quan | |
| `action` | `issuer_provisioned` / `issued` / `revoked` / `looked_up` / `verify_certificate` / `chain_verified` | CHECK constraint trong DB — action mới phải có migration. Revocation check của KDC/Bank ghi chung `verify_certificate`, phân biệt bằng `performed_by` |
| `cert_type`, `issuer_id` | Loại cert (`client`/`service_tls`/…) và issuer trong chain CA mới | Cột mới sau khi CA chuyển kiến trúc issuer chain |
| `performed_by` | `admin:<email>` / `system:<flow>` / tên service caller | Admin identity do Gateway truyền qua gRPC field `performed_by` |
| `reason` | Bắt buộc với `revoked` | |
| `performed_at` | UTC | |
| `metadata` | JSONB: `request_id`, `owner_id`, … | Trace `request_id` do Gateway truyền qua **gRPC metadata `x-request-id`** (không phải field trong message) |

### Bank — `bank_audit_log`

| Field | Ý nghĩa | Ghi chú |
|---|---|---|
| `action` | 7 giá trị: `transfer_completed`, `transfer_rejected`, `replay_detected`, `invalid_signature`, `certificate_rejected`, `forbidden_ownership`, `insufficient_funds` | CHECK constraint trong DB |
| `user_id` | UUID user (ClientID từ ticket) | NULL nếu chưa xác định được (ví dụ invalid_ticket) |
| `account_id`, `transaction_id` | UUID, nullable | |
| `cert_serial` | Serial cert trong ticket | |
| `request_id` | Từ `request_id` trong authenticator/body của AP flow | Các event fail trước khi giải mã authenticator sẽ không có |
| `reason` | Chuỗi ngắn lower_snake, vd `redis_replay`, `certificate_not_active` | |
| `metadata` | JSONB; event auth-layer có `scope` để phân biệt transfer/balance/history | |

### Quy ước request id (quan trọng, tránh nhầm 2 loại)

| Loại | Bản chất | Đi ở đâu |
|---|---|---|
| Trace id của Gateway (`X-Request-ID`) | Cross-cutting transport concern, chỉ để trace/correlate | **gRPC metadata key `x-request-id`** — không bao giờ là field trong proto message. CA đọc bằng `metadata.FromIncomingContext` và ghi vào `metadata.request_id` của audit event (`issued`, `looked_up`, `revoked`) |
| `request_id` trong AP flow của Bank | Dữ liệu protocol, nằm trong authenticator được mã hóa/ký, được persist vào cột `bank_audit_log.request_id` | **Body** (trong authenticator) — giữ nguyên |
| `request_id` filter của API đọc audit Bank | Tham số query domain: tìm event theo giá trị cột đã lưu | **Body** (`ListAuditEventsRequest.request_id`) |
| `performed_by` | Dữ liệu domain được persist vào audit | **Body** (field proto) |

Nguyên tắc: audit ghi best-effort — insert lỗi chỉ log warning, không làm fail request chính. Endpoint đọc audit là read-only và không tự ghi audit.

## 2. API đọc audit

- **CA**: `GET /v1/admin-ca/audit?action&serial&performed_by&from&to&limit&offset` — auth bằng `Authorization: Bearer <token>` (JWT role `admin-ca` từ `POST /v1/admin-ca/auth`, hoặc static token `ADMIN_CA_DEMO_TOKEN`) + header `X-Request-ID` bắt buộc.
- **Bank**: `POST /v1/admin/bank/audit/query` (body JSON: `action`, `user_id`, `cert_serial`, `request_id`, `from_unix`, `to_unix`, `limit`, `offset`) — auth bằng **session cookie** của Admin Bank (`POST /v1/admin/bank/activate` → `POST /v1/admin/bank/session`). Do Thái phụ trách, contract chi tiết xem `admin-bank.middleware.ts`.

Quy tắc chung: `limit` mặc định 20 max 100; time range là khoảng nửa mở `[from, to)`; sort mới nhất trước. Response CA:

```json
{ "success": true,
  "data": { "items": [...], "total": 12, "limit": 20, "offset": 0 },
  "request_id": "<uuid>", "timestamp": "<iso>" }
```

Lỗi: `{ "success": false, "error_code": "...", "message": "...", "request_id": "..." }`.

## 3. Testcase: event → cách kích hoạt → nơi kiểm tra

| # | Tình huống kích hoạt | Event mong đợi | Nơi kiểm tra | Pass/Fail | Owner | Note |
|---|---|---|---|---|---|---|
| 1 | Đăng ký user mới (OTP → PKI register) | CA `issued` | `GET /v1/admin-ca/audit?action=issued` | | | |
| 2 | Mở detail cert trong Admin CA | CA `looked_up`, performed_by=`admin:<email>` | audit CA filter `serial` | | | |
| 3 | Revoke cert (có reason) | CA `revoked` + reason | audit CA `action=revoked` | | | |
| 4 | Login/AS/TGS hoặc bank flow với cert đã revoke | CA `verify_certificate` (performed_by = service caller) + flow bị reject | audit CA + response lỗi | | | |
| 5 | Transfer thành công | Bank `transfer_completed` có transaction_id, request_id | `POST /v1/admin/bank/audit/query` body `{"action":"transfer_completed"}` | | | |
| 6 | Gửi lại cùng nonce/request | Bank `replay_detected` (`redis_replay`/`db_replay`) | audit Bank filter `request_id` | | | |
| 7 | Query balance/history của account không thuộc user | Bank `forbidden_ownership` | audit Bank | | | |
| 8 | Transfer từ account không thuộc user | Bank `forbidden_ownership` reason `from_account_owner_mismatch` | audit Bank | | | |
| 9 | Payload transfer chữ ký sai | Bank `invalid_signature` | audit Bank | | | |
| 10 | Transfer vượt số dư | Bank `insufficient_funds` | audit Bank | | | |
| 11 | Bank flow với cert revoked/expired | Bank `certificate_rejected` | audit Bank | | | |
| 12 | Balance request với ticket sai scope | Bank `transfer_rejected` reason `wrong_scope`, metadata.scope=`balance:read` | audit Bank | | | |
| 13 | Query audit với action rác | HTTP 400 `INVALID_REQUEST` | curl endpoint | | | |
| 14 | Query audit `limit=101` | HTTP 400 | curl endpoint | | | |
| 15 | Query audit không có token / sai role | HTTP 401 / 403 | curl endpoint | | | |
| 16 | Tắt Postgres audit rồi chạy request chính | Request chính vẫn OK, service log có `warning: cannot append/insert audit` | service log | | | |

## 4. Curl mẫu (cho demo script)

```bash
GW="http://localhost:3000"
RID() { python -c "import uuid;print(uuid.uuid4())"; }   # hoặc uuidgen

# 0a. Lấy token Admin CA (JWT role admin-ca)
TOKEN=$(curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(RID)" \
  -d '{"email":"<ADMIN_CA_DEMO_EMAIL>","password":"<ADMIN_CA_DEMO_PASSWORD>"}' \
  "$GW/v1/admin-ca/auth" | jq -r .data.token)

# 1. CA audit — tất cả event mới nhất
curl -s -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: $(RID)" "$GW/v1/admin-ca/audit"

# 2. CA audit — cert vừa cấp
curl -s -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: $(RID)" \
  "$GW/v1/admin-ca/audit?action=issued&limit=5"

# 3. CA audit — lịch sử một serial, do admin thao tác
curl -s -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: $(RID)" \
  "$GW/v1/admin-ca/audit?serial=<serial>&performed_by=admin-ca"

# 4. CA audit — theo khoảng thời gian
curl -s -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: $(RID)" \
  "$GW/v1/admin-ca/audit?from=2026-07-05T00:00:00Z&to=2026-07-07T00:00:00Z"

# 5. Bank audit — cần session cookie Admin Bank (activate → session trước),
#    sau đó query qua body JSON:
curl -s -b cookies.txt -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(RID)" \
  -d '{"action":"transfer_completed","limit":20,"offset":0}' \
  "$GW/v1/admin/bank/audit/query"

# 6. Bank audit — trace một request id của AP flow
curl -s -b cookies.txt -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(RID)" \
  -d '{"request_id":"<request-id>"}' "$GW/v1/admin/bank/audit/query"

# 7. Negative — action rác phải trả 400
curl -s -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: $(RID)" "$GW/v1/admin-ca/audit?action=hack"

# 8. Negative — không token phải trả 401
curl -s -H "X-Request-ID: $(RID)" "$GW/v1/admin-ca/audit"

# 9. Negative — thiếu X-Request-ID phải trả 400
curl -s -H "Authorization: Bearer $TOKEN" "$GW/v1/admin-ca/audit"
```

## 5. Quyết định chủ đích (không phải thiếu sót)

- `ListCertificates` và balance/history/profile **thành công** không ghi audit (noisy, enum không có action tương ứng).
- `CreateUser` bank không ghi audit (ngoài scope, enum không có action).
- Event auth-layer fail trước khi giải mã authenticator (`invalid_ticket`, `ticket_expired`, …) không có `request_id` — trace bằng `created_at` + `cert_serial`.
- Retention khi demo: backup bằng `pg_dump` trước demo, hoặc export `COPY (SELECT * FROM bank_audit_log) TO ... CSV`.
