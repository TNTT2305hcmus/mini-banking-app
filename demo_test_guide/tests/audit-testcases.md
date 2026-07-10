# Audit Log — Chuẩn, kiến trúc, testcase và curl mẫu

Tài liệu vận hành cho hệ thống audit (CA + KDC + Bank + Gateway).

## 0. Kiến trúc tổng quan

Ba audit store, mỗi service sở hữu domain của mình; Gateway **không** có store riêng.

| Nguồn | Bảng                    | Nội dung                                                                            |
| ----- | ----------------------- | ----------------------------------------------------------------------------------- |
| CA    | `certificate_audit_log` | Vòng đời cert (issued/revoked/verify/chain) **+ sự kiện RA/auth do Gateway đẩy về** |
| KDC   | `kdc_audit_log` _(mới)_ | Cấp khóa: AS/TGS ticket issued/rejected                                             |
| Bank  | `bank_audit_log`        | Truy cập tài nguyên: transfer/replay/ownership…                                     |

**Gateway = RA (Registration Authority)**: OTP, đăng ký, admin-ca login là một phần vòng đời cert nên ghi vào **CA audit** (actor `ra:*` / `admin-ca:*`) qua RPC `AppendAuditEvent` (whitelist action, không giả mạo được event lifecycle).

Trên tầng đọc, Gateway bổ sung:

- **Semantic enrichment**: mỗi event có `category` / `severity` / `outcome` / `actor{type,id,display}` / `description` (suy diễn, không đổi DB).
- **Timeline** theo `request_id` xuyên CA→KDC→Bank.
- **Tamper-evidence**: hash-chain per bảng + endpoint verify.
- **Summary** (dashboard) và **Export** (CSV/JSON).

Nguyên tắc bất biến: audit ghi **best-effort** (insert lỗi chỉ log warning, không làm fail request chính); endpoint đọc **read-only**, không tự ghi audit.

## 1. Chuẩn action (khớp CHECK constraint DB)

### CA — `certificate_audit_log.action`

Lifecycle: `issuer_provisioned`, `issued`, `revoked`, `looked_up`, `verify_certificate`, `chain_verified`.
RA/auth (Gateway đẩy về, migration `003_add_ra_audit_actions.sql`): `ra_otp_requested`, `ra_otp_verified`, `ra_otp_failed`, `ra_registration_approved`, `ra_registration_rejected`, `admin_ca_login_success`, `admin_ca_login_failed`.

### KDC — `kdc_audit_log.action`

`as_ticket_issued`, `as_rejected`, `tgs_ticket_issued`, `tgs_rejected`. Reason lấy từ mã lỗi domain (`replay_detected`, `identity_mismatch`, `cert_revoked`, `auth_invalid`, `request_expired`…).

### Bank — `bank_audit_log.action`

`transfer_completed`, `transfer_rejected`, `replay_detected`, `invalid_signature`, `certificate_rejected`, `forbidden_ownership`, `insufficient_funds`.

## 2. Mô hình semantic (áp lúc đọc)

| Trục       | Giá trị                                                                                                                                                                          |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `category` | `key_issuance` (KDC), `cert_lifecycle` (CA issued/verify/chain/registration), `authentication` (OTP + admin login), `resource_access` (Bank), `admin_action` (revoked/looked_up) |
| `severity` | `critical` = {revoked, certificate_rejected, replay_detected, invalid_signature}; `warning` = mọi `*_rejected`/`*_failed`/forbidden_ownership/insufficient_funds; còn lại `info` |
| `outcome`  | `denied` cho thao tác bị từ chối; `revoked` là `success` (revoke thành công) dù severity critical                                                                                |
| `actor`    | prefix `ra:`→ra, `admin-ca:`/`bank_admin:`→admin, `system:`/`service:`→service, UUID→user                                                                                        |

## 3. Quy ước request id (2 loại — tránh nhầm)

| Loại                           | Bản chất                                                                          | Đi ở đâu                                                                        |
| ------------------------------ | --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| Trace `X-Request-ID` (Gateway) | Transport concern để correlate xuyên service                                      | **gRPC metadata `x-request-id`**; CA/KDC/Bank đọc từ metadata. CA/KDC ghi vào `metadata.request_id`; Bank ghi vào `metadata.trace_id` và dùng làm **fallback** cho cột `request_id` khi AP request_id chưa tồn tại (auth fail sớm) |
| `request_id` AP flow Bank      | Dữ liệu protocol trong authenticator, persist vào cột `bank_audit_log.request_id` | Body (authenticator)                                                            |
| `request_id` filter đọc audit  | Query domain theo cột đã lưu                                                      | Body/query. CA lưu trong metadata JSONB → filter bằng `metadata->>'request_id'` |

Frontend gọi `X-Request-ID` theo convention `operation_id`: mỗi flow lớn như register, login hoặc transfer dùng một UUID chung cho các HTTP call thuộc cùng flow. AP `request_id` trong Bank authenticator vẫn là UUID riêng cho replay/idempotency và không được dùng thay thế `operation_id`.

## 4. Bề mặt admin & RBAC

Ba danh tính admin tách bạch — không danh tính nào đọc chéo domain:

| Danh tính | Login | Xem gì |
|---|---|---|
| `admin-ca` | activate cert `ca_admin` → `POST /v1/admin-ca/session` | CA audit (cert lifecycle + RA/OTP/đăng ký/admin-ca login) |
| `bank_admin` | activate → `POST /v1/admin/bank/session` (cookie `bank_admin_session`) | Bank audit |
| `security-admin` (SOC) | `POST /v1/admin-sec/auth` (hoặc `ADMIN_SEC_DEMO_TOKEN`) | KDC key-issuance + view xuyên domain (timeline/verify/summary/export) |

Admin CA không còn static demo token. SOC vẫn có thể dùng `ADMIN_SEC_DEMO_TOKEN` như fallback phát triển, scope riêng cho `security-admin`.

## 5. API đọc audit

| Endpoint | Auth | Ghi chú |
|---|---|---|
| `GET /v1/admin-ca/audit?action&serial&performed_by&request_id&from&to&limit&offset` | Bearer `admin-ca` | CA audit, đã enrich |
| `GET /v1/admin-kdc/audit?action&client_id&cert_serial&request_id&from&to&limit&offset` | Bearer **`security-admin`** | KDC key-issuance, đã enrich |
| `POST /v1/admin/bank/audit/query` (body JSON) | Cookie `bank_admin_session` | Bank, đã enrich |
| `GET /v1/admin/audit/timeline?request_id=` | Bearer **`security-admin`** (+ cookie bank để fold Bank) | Hợp nhất CA+KDC(+Bank) theo trace id, sort thời gian |
| `GET /v1/admin/audit/verify` | Bearer **`security-admin`** (+ cookie bank) | Replay hash-chain, báo tampering |
| `GET /v1/admin/audit/summary?window=24h` | Bearer **`security-admin`** (+ cookie bank) | Đếm severity/category/outcome, top reasons, anomalies |
| `GET /v1/admin/audit/export?source=all&from&to&format=csv\|json` | Bearer **`security-admin`** (+ cookie bank) | Tải audit ra CSV/JSON |

Quy tắc chung: `X-Request-ID` bắt buộc; `limit` mặc định 20 max 100; time range nửa mở `[from, to)` ISO 8601; sort mới nhất trước. Response envelope:

```json
{ "success": true, "data": { ... }, "request_id": "<uuid>", "timestamp": "<iso>" }
```

## 6. Tamper-evidence (hash-chain)

Mỗi bảng audit có cột `seq` / `prev_hash` / `hash = SHA256(prev_hash | các field cốt lõi)`. Insert chạy trong transaction + `pg_advisory_xact_lock`. `GET /v1/admin/audit/verify` replay từng chuỗi:

```json
{
  "ok": false,
  "sources": {
    "bank": {
      "checked": true,
      "ok": false,
      "broken_seq": 7,
      "detail": "hash does not match the row contents"
    },
    "ca": { "checked": true, "ok": true, "verified": 12 }
  }
}
```

Field được hash: action + các định danh + reason (loại timestamp/metadata vì không round-trip byte-stable).
**Giới hạn đã biết**: sửa riêng timestamp/metadata hoặc xóa **dòng cuối cùng** không bị phát hiện (cần external anchor); sửa/xóa/đảo dòng giữa hoặc đổi action/reason/actor → chuỗi gãy.

### Checkpoint anchor thủ công (giảm nhẹ rủi ro xóa tail)

Sau rehearsal/trước demo, chốt đuôi chuỗi của từng bảng ra nơi lưu ngoài DB (file/commit/note):

```sql
SELECT 'ca'   AS source, seq, hash FROM certificate_audit_log ORDER BY seq DESC LIMIT 1;
SELECT 'kdc'  AS source, seq, hash FROM kdc_audit_log          ORDER BY seq DESC LIMIT 1;
SELECT 'bank' AS source, seq, hash FROM bank_audit_log         ORDER BY seq DESC LIMIT 1;
```

Khi cần chứng minh: chạy lại query — nếu `seq` hiện tại nhỏ hơn checkpoint hoặc `hash` tại `seq` checkpoint đổi giá trị thì tail đã bị cắt/sửa. Đây là anchor tối thiểu cho demo; external anchor tự động (ghi định kỳ ra hệ thống ngoài) vẫn là hạng mục tương lai.

## 7. Testcase: event → cách kích hoạt → nơi kiểm tra

Trạng thái cập nhật giai đoạn 5: hiện chưa có compose/runtime stack đang chạy, nên không đánh dấu runtime `PASS` khi chưa curl/UI thật. Cột `Pass/Fail` dùng quy ước:

- `CODE PASS / RUNTIME PENDING`: code path, route hoặc test local đã có bằng chứng tĩnh; cần chạy stack thật để chốt demo.
- `RUNTIME PENDING`: chỉ có thể kết luận bằng thao tác runtime/manual.
- `PENDING_MANUAL_DB`: cần thao tác DB có kiểm soát trên môi trường demo.

| #   | Tình huống kích hoạt                                                   | Event mong đợi                                                                                                             | Nơi kiểm tra                                                      | Pass/Fail | Owner | Note |
| --- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- | --------- | ----- | ---- |
| 1   | OTP request                                                            | CA `ra_otp_requested`, actor `ra:otp`                                                                                      | `GET /v1/admin-ca/audit?action=ra_otp_requested`                  | CODE PASS / RUNTIME PENDING | Thuận | `otp.controller.ts` gọi `recordRaAudit`; cần curl OTP thật rồi xem Admin CA Audit Log |
| 2   | OTP verify đúng                                                        | CA `ra_otp_verified` (metadata.owner_id)                                                                                   | `GET /v1/admin-ca/audit?action=ra_otp_verified`                   | CODE PASS / RUNTIME PENDING | Thuận | Verify OTP đúng phải dùng cùng `operation_id` của flow register |
| 3   | OTP verify sai/hết lượt                                                | CA `ra_otp_failed` reason `otp_mismatch`/`too_many_attempts`, severity warning                                             | `GET /v1/admin-ca/audit?action=ra_otp_failed`                     | CODE PASS / RUNTIME PENDING | Thuận | Case âm cần OTP sai hoặc quá lượt trong stack thật |
| 4   | PKI register thành công                                                | CA `ra_registration_approved` + `issued` (cùng request_id)                                                                 | `GET /v1/admin-ca/audit?request_id=<operation_id>`                | CODE PASS / RUNTIME PENDING | Thuận | Giai đoạn 4 đã truyền `operation_id`; cần đăng ký user mới để chốt runtime |
| 5   | Register fail (email trùng…)                                           | CA `ra_registration_rejected`                                                                                              | `GET /v1/admin-ca/audit?action=ra_registration_rejected`          | CODE PASS / RUNTIME PENDING | Thuận | Sau sửa rollback, email trùng phải dừng trước CA issue và ghi reject |
| 6   | Admin CA login đúng/sai                                                | CA `admin_ca_login_success` / `admin_ca_login_failed`                                                                      | `GET /v1/admin-ca/audit?action=admin_ca_login_success`            | CODE PASS / RUNTIME PENDING | Thuận | Đường chính là cert-backed `/v1/admin-ca/session`, không còn password/static token |
| 7   | Mở detail cert                                                         | CA `looked_up`, category admin_action                                                                                      | `GET /v1/admin-ca/audit?action=looked_up`                         | CODE PASS / RUNTIME PENDING | Thuận | Kích hoạt từ UI Admin CA detail drawer hoặc `GET /v1/admin-ca/certificates/{serial}` |
| 8   | Revoke cert (có reason)                                                | CA `revoked` severity critical                                                                                             | `GET /v1/admin-ca/audit?action=revoked`                           | CODE PASS / RUNTIME PENDING | Thuận | Chỉ revoke cert `client`; không dùng tài khoản demo chính để tránh phá luồng |
| 9   | AS login thành công                                                    | KDC `as_ticket_issued`                                                                                                     | `GET /v1/admin-kdc/audit?action=as_ticket_issued`                 | CODE PASS / RUNTIME PENDING | Thuận | Cần `kdc-service` có `DATABASE_URL`; nếu unset thì KDC audit no-op |
| 10  | AS với cert revoked / pre-auth sai                                     | KDC `as_rejected` reason `cert_revoked`/`auth_invalid`, severity warning                                                   | `GET /v1/admin-kdc/audit?action=as_rejected`                      | CODE PASS / RUNTIME PENDING | Thuận | Dùng cert phụ đã revoke hoặc chữ ký pre-auth sai |
| 11  | TGS cấp service ticket                                                 | KDC `tgs_ticket_issued` (scope)                                                                                            | `GET /v1/admin-kdc/audit?action=tgs_ticket_issued`                | CODE PASS / RUNTIME PENDING | Thuận | Kiểm tra scope trong metadata/detail event |
| 12  | TGS sai scope/authenticator                                            | KDC `tgs_rejected`                                                                                                         | `GET /v1/admin-kdc/audit?action=tgs_rejected`                     | CODE PASS / RUNTIME PENDING | Thuận | Dùng scope không hợp lệ hoặc authenticator sai |
| 13  | Transfer thành công                                                    | Bank `transfer_completed`                                                                                                  | `POST /v1/admin/bank/audit/query {"action":"transfer_completed"}` | CODE PASS / RUNTIME PENDING | Thuận | Cần cookie `bank_admin_session` để đọc Bank audit |
| 14  | Gửi lại nonce/request                                                  | Bank `replay_detected` severity critical                                                                                   | `POST /v1/admin/bank/audit/query {"action":"replay_detected"}`    | CODE PASS / RUNTIME PENDING | Thuận | Replay AP request hoặc cùng nonce/request_id |
| 15  | Transfer/balance account không thuộc user                              | Bank `forbidden_ownership`                                                                                                 | `POST /v1/admin/bank/audit/query {"action":"forbidden_ownership"}` | CODE PASS / RUNTIME PENDING | Thuận | Có thể dùng account seed của user khác |
| 16  | Chữ ký payload sai                                                     | Bank `invalid_signature` severity critical                                                                                 | `POST /v1/admin/bank/audit/query {"action":"invalid_signature"}`  | CODE PASS / RUNTIME PENDING | Thuận | Cần request transfer bị sửa payload/signature |
| 17  | Vượt số dư                                                             | Bank `insufficient_funds`                                                                                                  | `POST /v1/admin/bank/audit/query {"action":"insufficient_funds"}` | CODE PASS / RUNTIME PENDING | Thuận | Dùng số tiền lớn hơn balance nhưng dưới test kiểm soát |
| 18  | Bank flow cert revoked/expired                                         | Bank `certificate_rejected` severity critical                                                                              | `POST /v1/admin/bank/audit/query {"action":"certificate_rejected"}` | CODE PASS / RUNTIME PENDING | Thuận | Cần cert phụ revoked/expired; không dùng user demo chính |
| 19  | **Timeline** theo `operation_id` 1 flow register/login/transfer               | Register: `ra_otp_*→ra_registration_*→issued` (CA). Login: `as_ticket_issued` + profile TGS/AP theo cùng id. Transfer mới: `tgs_ticket_issued` + Bank event theo cùng id khi cần ticket mới. | `GET /v1/admin/audit/timeline?request_id=<operation_id>`                    | CODE PASS / RUNTIME PENDING | Thuận | Frontend truyền `operation_id` qua `X-Request-ID`; Bank AP `request_id` vẫn riêng |
| 20  | **Verify** khi chưa sửa gì                                             | `ok:true` mọi source                                                                                                       | `GET /v1/admin/audit/verify`                                      | CODE PASS / RUNTIME PENDING | Thuận | Bank source trả `checked:false` nếu thiếu cookie `bank_admin_session` |
| 21  | **Verify** sau khi sửa tay 1 dòng `bank_audit_log` (đổi action/reason) | `bank.ok:false` + `broken_seq`                                                                                             | `GET /v1/admin/audit/verify`                                      | PENDING_MANUAL_DB | Thuận | Chỉ chạy trên DB demo disposable; cần backup/rollback sau khi tamper |
| 22  | **Summary** 24h sau vài event denied                                   | `security_events`>0, `by_severity`, `top_reasons` có dữ liệu                                                               | `GET /v1/admin/audit/summary?window=24h`                          | CODE PASS / RUNTIME PENDING | Thuận | CA+KDC luôn gộp; Bank chỉ gộp khi có cookie |
| 23  | **Anomaly** ≥5 event denied cùng danh tính                             | Xuất hiện trong `summary.anomalies`                                                                                        | `GET /v1/admin/audit/summary?window=24h`                          | RUNTIME PENDING | Thuận | Phải tạo >=5 denied event cùng actor/cert trong runtime |
| 24  | **Export** CSV                                                         | File CSV mở được bằng Excel, đủ cột                                                                                        | `GET /v1/admin/audit/export?format=csv`                           | CODE PASS / RUNTIME PENDING | Thuận | Kiểm thêm `source=all|ca|kdc|bank` và JSON nếu cần |
| 25  | Query audit action rác / `limit=101` / thiếu `X-Request-ID`            | HTTP 400                                                                                                                   | curl                                                              | CODE PASS / RUNTIME PENDING | Thuận | Gateway validators/rate header đã có; cần curl để chốt response code |
| 26  | Query không token / sai role                                           | HTTP 401 / 403                                                                                                             | curl                                                              | CODE PASS / RUNTIME PENDING | Thuận | Kiểm role tách biệt `admin-ca`, `security-admin`, `bank_admin` |
| 27  | Tắt Postgres audit rồi chạy request chính                              | Request chính vẫn OK, log `warning: cannot insert/append audit`                                                            | service log                                                       | RUNTIME PENDING | Thuận | Phải chạy stack thật; audit insert là best-effort nên request chính không fail |

## 7.1. SOC demo checklist

| Hạng mục | Endpoint/UI | Trạng thái | Ghi chú |
|---|---|---|---|
| Login SOC bằng `security-admin` | `POST /v1/admin-sec/auth`, UI `/admin-soc` | CODE PASS / RUNTIME PENDING | Dùng token SOC cho KDC/timeline/verify/summary/export |
| KDC audit list | `GET /v1/admin-kdc/audit?action=as_ticket_issued` | CODE PASS / RUNTIME PENDING | Phụ thuộc `DATABASE_URL` của `kdc-service` trong compose |
| Timeline theo `operation_id` | `GET /v1/admin/audit/timeline?request_id=<operation_id>` | CODE PASS / RUNTIME PENDING | Kỳ vọng CA + KDC, thêm Bank nếu có cookie Bank Admin |
| Verify hash-chain | `GET /v1/admin/audit/verify` | CODE PASS / RUNTIME PENDING | Trạng thái Bank `checked:false` là hợp lệ nếu thiếu cookie |
| Summary 24h | `GET /v1/admin/audit/summary?window=24h` | CODE PASS / RUNTIME PENDING | Cần tạo vài denied event để summary đẹp |
| Export CSV/JSON | `GET /v1/admin/audit/export?source=all&format=csv\|json` | CODE PASS / RUNTIME PENDING | Lưu file làm bằng chứng rehearsal |
| Bank folded view | Các endpoint SOC + cookie `bank_admin_session` | CODE PASS / RUNTIME PENDING | Thiết kế chủ đích: SOC không tự đọc Bank nếu không có credential Bank Admin |

## 8. Curl mẫu

```bash
GW="http://localhost:3000"
RID() { python -c "import uuid;print(uuid.uuid4())"; }   # hoặc uuidgen

# Token Admin CA (chỉ dùng cho CA audit). Lấy từ cert-backed session:
# frontend /admin-ca hoặc POST /v1/admin-ca/session với cert_serial/challenge/signature.
CA="<ADMIN_CA_CERT_SESSION_TOKEN>"

# Token SOC (dùng cho KDC + view xuyên domain)
SEC=$(curl -s -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(RID)" \
  -d '{"email":"<ADMIN_SEC_DEMO_EMAIL>","password":"<ADMIN_SEC_DEMO_PASSWORD>"}' \
  "$GW/v1/admin-sec/auth" | jq -r .data.token)

HCA=(-H "Authorization: Bearer $CA"  -H "X-Request-ID: $(RID)")
HSC=(-H "Authorization: Bearer $SEC" -H "X-Request-ID: $(RID)")

# CA audit — token admin-ca
curl -s "${HCA[@]}" "$GW/v1/admin-ca/audit?action=issued&limit=5"

# KDC + cross-service — token security-admin
curl -s "${HSC[@]}" "$GW/v1/admin-kdc/audit?action=as_ticket_issued"
curl -s "${HSC[@]}" "$GW/v1/admin/audit/timeline?request_id=<operation-id>"
curl -s "${HSC[@]}" "$GW/v1/admin/audit/verify"
curl -s "${HSC[@]}" "$GW/v1/admin/audit/summary?window=24h"
curl -s "${HSC[@]}" "$GW/v1/admin/audit/export?source=all&format=csv&from=2026-07-01T00:00:00Z" -o audit.csv

# Bank audit (cần activate→session trước để có cookie)
curl -s -b cookies.txt -X POST -H "Content-Type: application/json" -H "X-Request-ID: $(RID)" \
  -d '{"action":"transfer_completed"}' "$GW/v1/admin/bank/audit/query"

# Negative — token admin-ca gọi route SOC phải 403 (tách vai trò)
curl -s "${HCA[@]}" "$GW/v1/admin-kdc/audit"
# Negative — action rác → 400; không token → 401
curl -s "${HCA[@]}" "$GW/v1/admin-ca/audit?action=hack"
curl -s -H "X-Request-ID: $(RID)" "$GW/v1/admin-ca/audit"
```

## 9. Demo trên UI (3 bề mặt tách biệt)

- **Admin CA** (`/admin-ca` → tab **Audit Log**): chỉ CA audit (cert lifecycle + RA/OTP/đăng ký/admin-ca login), timeline có severity badge + filter. Không còn summary/verify/session-drawer.
- **Admin Bank** (`/admin-bank` → tab **Security Audit**): Bank audit qua `<AuditTimeline>`.
- **Security Operations** (`/admin-soc`, login `security-admin`):
  - Tab **Key Issuance (KDC)**: list KDC + drill-down "View session".
  - Tab **Cross-service**: summary cards 24h, **Verify integrity**, anomalies, **Export CSV/JSON**, ô tra cứu `request_id → Open timeline`.
- Kịch bản demo tamper-evidence: đăng nhập SOC → `UPDATE bank_audit_log SET reason='x' WHERE seq=<n>` (kèm cookie bank) → bấm Verify → banner đỏ `broken @<n>`.

## 10. Quyết định chủ đích & giới hạn

- `ListCertificates`, balance/history/profile **thành công**, `CreateUser` bank: không ghi audit (noisy / ngoài enum).
- Event auth-layer Bank fail trước khi giải mã authenticator (`invalid_ticket`…) không có AP `request_id` — cột `request_id` fallback bằng trace `X-Request-ID` của Gateway (gRPC metadata); nếu caller không gửi header thì vẫn trace bằng `created_at` + `cert_serial`.
- KDC audit là **optional theo `DATABASE_URL`**: không cấu hình DB thì KDC vẫn cấp vé, chỉ mất audit.
- Bank timeline/verify/summary chỉ gộp khi có cookie `bank_admin_session` (super-admin cầm cả 2 credential); admin-ca đơn thuần thấy CA+KDC.
- Retention: `pg_dump` trước demo hoặc dùng `GET .../export?format=csv|json`.
- Hash-chain không phát hiện sửa timestamp/metadata hay xóa dòng cuối (xem §5).
