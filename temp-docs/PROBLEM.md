# Báo cáo kiểm tra 7 vấn đề hệ thống — hiện trạng, cách xử lý, tính khả thi

Ngày: 2026-07-06. Chỉ kiểm tra và báo cáo, chưa sửa code. Mỗi mục gồm: hiện trạng (kèm bằng chứng code), nguyên nhân, cách xử lý đề xuất, vấn đề liên quan, tính khả thi.

## Tổng quan mức độ

| # | Vấn đề | Mức | Effort ước tính |
|---|---|---|---|
| 1 | Cấp cert trước khi check email đã đăng ký → cert rác | 🔴 Bug logic thứ tự | 0.5–1 ngày |
| 2 | Cert có nhưng bank user chưa có → kẹt đăng ký | 🔴 Hệ quả của #1, thiếu compensation | Chung fix với #1 |
| 3 | Lưu trữ AS/TGS | 🟢 Thiết kế hiện tại đúng, chỉ cần vá nhỏ | 0.5 ngày (nếu vá) |
| 4 | Chuyển tiền: bên nhận không tăng "vì tối đa 50m" | 🟡 Cần xác minh — code không có cap 50M | 0.5 ngày điều tra |
| 5 | Login đúng vẫn bị rate-limit | 🔴 Ngưỡng quá thấp + đếm cả request thành công | 0.5 ngày |
| 6 | Tách route admin, yêu cầu cert admin | 🟡 Bank admin đã đúng hướng, CA admin còn yếu | 1–2 ngày |
| 7 | Audit log chưa chuẩn enterprise, thiếu event cấp khóa/đăng nhập | 🔴 Lỗ hổng lớn nhất: **KDC không có audit nào** | 2–3 ngày |

---

## 1. Cấp cert trước khi kiểm tra email đã đăng ký → cert rác

### Hiện trạng

Luồng `handleRegister` ([ca.controller.ts:169-187](mini-banking-app/api-gateway/src/controller/ca.controller.ts)) chạy theo thứ tự:

1. `redis.set(jtiKey, "1")` — **đốt reg token trước khi làm gì cả**.
2. `registerUser(...)` — CA ký và **lưu cert vào DB CA** (kèm audit `issued`).
3. `createUserBankAccount(...)` — Bank `InsertUser`; bảng `users` có `email UNIQUE` ([001_init_bank.sql:7](mini-banking-app/db/bank/migrations/001_init_bank.sql)) → email đã tồn tại thì fail `ErrUserExists`.
4. Chỉ khi cả 2 xong mới trả `201` kèm `cert_pem`.

Email đã đăng ký vẫn qua được OTP (OTP không check tồn tại user), qua được CA (CA **không có ràng buộc unique theo email** — chỉ có rule "1 active client cert / owner_id", mà `owner_id` là **UUID mới sinh mỗi lần OTP verify** tại [otp.controller.ts:117](mini-banking-app/api-gateway/src/controller/otp.controller.ts)). Kết quả: cert đã ký + đã lưu + đã audit `issued`, nhưng bước bank fail → client nhận lỗi, **cert rác nằm lại trong CA, không ai revoke**. Lỗi trả về còn bị map qua `caGrpcError` nên error code gây hiểu nhầm là lỗi CA.

### Cách xử lý đề xuất (theo thứ tự ưu tiên)

1. **Pre-check trước khi cấp cert**: thêm RPC read-only `CheckUserEmail(email) → exists` vào Bank (hoặc check ngay tại bước OTP request — chặn sớm nhất, UX tốt nhất: "email đã có tài khoản"). Gateway gọi check này **trước** `registerUser`.
2. **Compensation khi bank fail**: nếu `createUserBankAccount` lỗi sau khi cert đã cấp → gọi `revokeCertificate(serial, reason="registration_rollback", performedBy="system:gateway")` best-effort + audit. Cert rác được dọn ngay cả khi pre-check bị race (2 request cùng email đồng thời).
3. **Chỉ đốt jti sau khi thành công** (hoặc set `"1"` ở đầu nhưng rollback về `"0"` khi fail) — hiện user fail giữa chừng phải làm lại OTP từ đầu dù không có lỗi của họ.
4. Map lỗi bank riêng (409 `EMAIL_ALREADY_REGISTERED`) thay vì `caGrpcError`.

### Tính khả thi

Cao. Pre-check + compensation + jti đều ở tầng gateway/bank, không đụng protocol Kerberos. Rủi ro duy nhất: RPC check email mới cần proto Bank + regenerate (đã có quy trình). ~0.5–1 ngày.

---

## 2. Cert đã có cho owner nhưng bank user chưa có → email bị kẹt

### Hiện trạng

Đây là **trạng thái lệch pha CA↔Bank do #1 để lại**, và hệ thống không có đường thoát:

- Vì `owner_id` mới mỗi lần OTP, đăng ký lại **về nguyên tắc** sẽ tạo owner mới, CA không chặn. Chỗ kẹt thực tế: (a) nếu lần fail trước **bank user đã tạo xong nhưng response lỗi** (vd lỗi mạng sau commit) thì email đã nằm trong `users` → mọi lần đăng ký sau fail ở bước bank vĩnh viễn, trong khi user không hề có cert/private key dùng được; (b) nếu client retry trong 10 phút với **cùng reg token/owner_id** thì bị chặn kép: jti đã đốt (401) và CA rule 1-active-cert/owner (409).
- Không có công cụ đối soát: không có RPC "xóa/deactivate user chưa từng login", không có job dọn cert không gắn với bank user.

### Cách xử lý đề xuất

1. Làm #1 (pre-check + compensation) — chặn phát sinh trạng thái lệch mới.
2. **CreateUser idempotent theo cặp (owner_id, email)**: nếu user cùng email tồn tại nhưng **chưa có cert active nào trỏ tới owner_id đó** (kiểm qua CA), cho phép "re-bind": cách demo đơn giản là RPC admin/system `ReleaseEmail(email)` xóa user mồ côi (user chưa có transaction nào) để đăng ký lại sạch.
3. Script đối soát một chiều cho demo: liệt kê cert active có owner_id không tồn tại trong `bank.users` → revoke; user không có cert active → báo cáo.

### Tính khả thi

Cao cho hướng 1+3 (script đối soát ~vài giờ). Hướng 2 (re-bind) cần bàn kỹ về an toàn (không cho chiếm email người khác) — chỉ nên cho phép khi user mồ côi **và** chưa có giao dịch.

---

## 3. Lưu trữ AS/TGS — đã đúng chưa?

### Hiện trạng (kết luận: thiết kế đúng, tốt hơn kỳ vọng)

| Thành phần | Cách lưu hiện tại | Đánh giá |
|---|---|---|
| TGT + `K_{c,tgs}` (frontend) | **Chỉ RAM** ([as-exchange/session.ts](mini-banking-app/frontend/src/services/as-exchange/session.ts) — comment "KHÔNG persist"), zero-fill key khi logout/hết hạn | ✅ Đúng chuẩn: XSS không đọc được từ storage, không rò qua backup |
| Service ticket + `K_{c,v}` (frontend) | RAM, zero-fill tương tự (`tgs-exchange/session.ts`) | ✅ |
| Private key client (frontend) | IndexedDB, **wrap bằng PIN qua PBKDF2 + AES-GCM `wrapKey`** ([key.service.ts](mini-banking-app/frontend/src/services/key.service.ts)) | ✅ Ở mức đồ án là tốt |
| KDC server state | **Stateless đúng kiểu Kerberos**: TGT mã hóa bằng `K_TGS` (load từ file `K_TGS_PATH`), không lưu session server-side; chỉ lưu **replay marker** trong Redis `replay:as:*` / `replay:tgs:*` với TTL = freshness window ([as_service.go:236-238](mini-banking-app/kdc-service/internal/kdc/as_service.go)) | ✅ |
| Bank replay | Redis SetNX, fallback bảng `used_nonces` khi Redis chết | ✅ có fail-safe |

### Điểm cần vá (nhỏ)

1. **TGT mất khi refresh trang** (RAM-only) → user phải login lại. Đây là trade-off bảo mật có chủ đích; nếu muốn UX tốt hơn thì chấp nhận `sessionStorage` cho TGT (vẫn opaque) nhưng **tuyệt đối không** persist `K_{c,tgs}` — tức là không làm được AP mà thiếu key, nên thực tế cứ giữ RAM-only và ghi rõ vào docs là hành vi chủ đích.
2. Replay TTL phải ≥ freshness window ở cả KDC và Bank — hiện cùng 5 phút, đúng; ghi thành invariant trong docs để không ai chỉnh lệch.
3. `K_TGS`/`K_V` là file trên đĩa — với demo OK; checklist deploy của Quang phải có quyền file + không commit key (đã nằm trong P2 security cleanup).

### Tính khả thi

Không cần sửa gì gấp; chỉ bổ sung tài liệu invariant (~0.5 ngày nếu muốn viết spec ngắn).

---

## 4. Chuyển tiền thành công nhưng bên nhận không tăng "vì tối đa 50m"

### Hiện trạng — code KHÔNG có cap 50M nào trên số dư

Đã rà kỹ [service.go Transfer](mini-banking-app/banking-service/internal/bank/service.go): `DebitAccount` và `CreditAccount` chạy **trong cùng một transaction SQL** với `FOR UPDATE` cả 2 account, commit nguyên tử — về nguyên lý **không thể** có chuyện bên trừ bên không cộng trong cùng DB. Schema `accounts.balance` chỉ có `CHECK (balance >= 0)`, không có trần. Số 50.000.000 xuất hiện ở 2 chỗ khác nhau và đều **không phải cap số dư**:

- `daily_transfer_limit` mặc định 50M — giới hạn **tổng tiền chuyển ĐI trong ngày của bên gửi** (`spentToday + amount > from.Limit` → reject `daily_limit_exceeded`), không liên quan bên nhận.
- Số dư khởi tạo tài khoản mới = 50M (`InsertDefaultAccount`).

### Các giả thuyết cần xác minh (theo xác suất)

1. **UI không refetch**: số dư trên Home chỉ lấy từ AP `/auth/me` lúc login; bên nhận đang mở sẵn trang thì số dư không tự cập nhật → tưởng là "không tăng". Kiểm tra: bên nhận logout/login lại hoặc query DB.
2. **Giao dịch thực ra đã bị reject `daily_limit_exceeded` nhưng UI hiểu nhầm là thành công**: `failTransfer` ghi transaction `status='failed'` vào **cùng bảng transactions/hash-chain**; nếu UI history không phân biệt status thì một lệnh fail vẫn hiện ra như giao dịch bình thường. Trùng khớp mô tả "vì tối đa 50m" (vượt daily limit). Kiểm tra: `SELECT id, amount, status FROM transactions ORDER BY created_at DESC LIMIT 5;` — nếu `failed` thì đây là bug hiển thị.
3. Bên gửi "có trừ" thực ra là do một giao dịch thành công trước đó, còn lệnh đang xét bị fail — đối chiếu từng transaction id.

### Cách xử lý đề xuất

- Xác minh bằng SQL: `SELECT balance FROM accounts WHERE account_number IN (<gửi>, <nhận>);` trước/sau 1 lệnh chuyển có kiểm soát.
- Nếu là giả thuyết 2: sửa frontend history/toast phân biệt `status` (`failed` → đỏ + reason), gateway trả đúng 422 `DAILY_LIMIT_EXCEEDED` cho UI (đã có mapping `FAILED_PRECONDITION → 422`, cần check UI đọc đúng).
- Nếu là giả thuyết 1: thêm refetch balance sau khi nhận thông báo/khi focus tab; hoặc nút refresh.
- Cân nhắc nghiệp vụ: `daily_transfer_limit` 50M là **mặc định mỗi account** — nếu demo cần chuyển nhiều, seed limit cao hơn (việc của Quang, sửa seed không sửa code).

### Tính khả thi

Điều tra 0.5 ngày. Fix (dù rơi vào giả thuyết nào) đều ở tầng UI/seed, không đụng core transfer.

---

## 5. Đăng nhập đúng vẫn bị rate-limit

### Hiện trạng

[rateLimiter.ts](mini-banking-app/api-gateway/src/middleware/rateLimiter.ts): Redis `INCR` cửa sổ cố định, **đếm mọi request kể cả thành công**:

- AS_REQ: `rateLimitByIP` — **10 request / 5 phút / IP**.
- TGS_REQ: `rateLimitByCertSn` — 10 request / 5 phút / cert.

Một lần login của UI tốn: 1 AS_REQ + TGS_REQ cho từng scope (balance/history/transfer) + các AP call. Chỉ cần login ~3 lần trong 5 phút (hoặc dev hot-reload) là chạm trần AS theo IP. Tệ hơn: **cả team demo sau NAT/localhost chung 1 IP** → giới hạn 10/5phút là của cả phòng. Ngoài ra fixed-window có burst kép ở ranh giới cửa sổ, và không có header `Retry-After`.

### Cách xử lý đề xuất

1. **Chỉ đếm thất bại** (đúng mục đích chống brute-force): controller AS/TGS thành công thì `DECR`/xóa counter; hoặc chuyển sang mô hình "N lần **fail** trong M phút thì khóa".
2. Nâng ngưỡng hợp lý cho demo: AS theo IP 60/5phút; thêm chiều theo `email`/`cert_sn` với ngưỡng chặt hơn (chống brute-force một danh tính từ nhiều IP).
3. Trả `Retry-After` + error message ghi rõ thời gian chờ.
4. Dev mode: env `RATE_LIMIT_DISABLED=1` hoặc ngưỡng qua env để không chặn dev/demo.

### Tính khả thi

Cao, gói gọn 1 file middleware + 2 controller gọi xóa counter. ~0.5 ngày kể cả test.

---

## 6. Tách route admin: client không truy cập được, chỉ vào bằng cert admin, không chặn admin dùng dịch vụ bank

### Hiện trạng — 2 admin đang 2 chuẩn khác nhau

| | Admin Bank (Thái) | Admin CA (Thanh) |
|---|---|---|
| Auth | ✅ Đúng hướng cert: activate cấp **cert role `bank_admin`** qua CA, `CreateAdminSession` chạy nguyên AP exchange, Bank **kiểm tra `identityRole == bank_admin` từ cert** ([admin_handler.go:26-28](mini-banking-app/banking-service/internal/grpc/admin_handler.go)), session cookie sau đó | ❌ Password demo so sánh env + JWT/static token, **không dính gì tới cert** |
| Cô lập route | Cùng Express app, cùng port với route user | Cùng app, cùng port |

Yêu cầu "chỉ được truy cập admin nếu có cert admin" — Bank đã đạt về mặt danh tính (cert-based, role nằm trong cert do CA ký). Admin CA chưa đạt. Yêu cầu "không chặn admin sử dụng dịch vụ bank": mô hình role-trong-cert hiện tại **1 owner chỉ có 1 active client cert** — admin muốn vừa là admin vừa là khách hàng thì cần 2 danh tính (2 owner_id, 2 cert) vì cert customer và cert bank_admin là 2 role khác nhau. Hiện không có gì chặn 1 người giữ 2 cert với 2 owner khác nhau → yêu cầu này **đã thỏa mãn tự nhiên**, chỉ cần ghi rõ quy ước: tài khoản admin và tài khoản cá nhân là 2 identity tách biệt (đúng thực hành enterprise — không dùng quyền admin đi giao dịch).

### Cách xử lý đề xuất

1. **Nâng Admin CA lên cùng chuẩn Admin Bank**: cấp cert role `ca_admin` (thêm enum `IDENTITY_ROLE_CA_ADMIN`), flow activate + session giống Thái (tái dùng gần như nguyên si `admin-activation` + `CreateAdminSession`, đổi service đích thành CA hoặc verify tại gateway bằng chữ ký với public key trong cert). Bỏ dần password/static token demo.
2. **Cô lập route admin**: tách admin router ra **listener/port riêng** (vd 3001) chỉ bind mạng nội bộ/VPN, hoặc đặt sau reverse proxy có mTLS (nginx `ssl_verify_client` với client-ca bundle). Với demo: mức tối thiểu là port riêng + không expose port đó ra ngoài compose network.
3. Route user (`/v1/otp`, `/v1/auth`, `/v1/bank`) giữ nguyên port công khai — admin dùng dịch vụ bank bằng cert customer của họ như mọi user, không bị ảnh hưởng.

### Tính khả thi

- Port riêng cho admin: **dễ** (~0.5 ngày, chỉ tách `express()` app thứ hai trong gateway).
- Cert-based cho Admin CA theo mẫu của Thái: **trung bình** (1–1.5 ngày, gồm proto enum + activate flow + UI login đổi từ password sang cert).
- mTLS đầy đủ ở proxy: khả thi nhưng nên để sau demo (thêm biến vận hành cho Quang).

---

## 7. Audit log chuẩn enterprise — trọng tâm

### Hiện trạng theo 3 nhóm sự kiện bắt buộc

| Nhóm sự kiện | Hiện trạng | Đánh giá |
|---|---|---|
| **Cấp cert** | CA audit đủ: `issued`, `revoked`, `looked_up`, `verify_certificate` (+`issuer_provisioned`, `chain_verified` trong enum), có performed_by/request_id/metadata, đọc được qua `GET /v1/admin-ca/audit` | ✅ Tốt nhất hệ thống |
| **Cấp khóa (AS/TGS)** | **KDC không ghi bất kỳ audit nào** — đã grep toàn bộ `kdc-service`: chỉ có replay marker Redis (TTL 5 phút, không phải audit). AS thành công/thất bại, TGS cấp service ticket, pre-auth sai, cert revoked bị chặn ở KDC… tất cả **không để lại dấu vết bền** | 🔴 Lỗ hổng lớn nhất |
| **Đăng nhập / xác thực / truy cập tài nguyên** | Bank ghi khá đủ ở AP layer (ticket sai, replay, cert rejected, ownership, transfer) — nhưng đây là **hệ quả gián tiếp**; không có event "login thành công/thất bại" đúng nghĩa ở gateway (OTP request/verify, AS login) ngoài log console morgan (mất khi restart). Đọc dữ liệu thành công (balance/history) chủ đích không audit | 🟡 Thiếu nửa đầu phễu |

### Hiện trạng hiển thị

Hai trang admin hiện là **bảng thô liệt kê event** — đúng như nhận xét "chỉ hiển thị, không có ý nghĩa": không phân loại severity, không nhóm theo phiên/request, không đối chiếu actor, không có mô tả người-đọc-được, không export.

### Cách xử lý đề xuất

**(a) Lấp lỗ hổng ghi (ưu tiên 1) — thêm audit KDC:**

- Bảng mới `kdc_audit_log` (KDC hiện không có DB — 2 phương án: (1) cho KDC 1 schema riêng trong Postgres cụm hiện có — sạch nhất; (2) đường tắt demo: KDC gRPC-call sang CA `AppendAudit` — không khuyến nghị vì trộn domain).
- Enum action tối thiểu: `as_ticket_issued`, `as_rejected` (reason: bad_preauth/cert_revoked/replay/stale), `tgs_ticket_issued`, `tgs_rejected`. Field: `client_id`, `cert_serial`, `scope` (TGS), `reason`, `request_id` (từ gRPC metadata `x-request-id` — quy ước đã có sẵn), `ip` (gateway truyền), `created_at`.
- Nguyên tắc giữ nguyên: best-effort, không làm fail luồng cấp vé.

**(b) Bổ sung event xác thực tại gateway:** `otp_requested`, `otp_verified`, `otp_failed`, `admin_login_success/failed` (cả admin-ca lẫn admin-bank session). Gateway không có DB — ghi qua RPC audit của service chủ quản (OTP/login user → KDC hoặc bảng riêng; admin-ca login → CA audit với action mới; admin-bank session → Thái đã có thể ghi trong `CreateAdminSession`).

**(c) Chuẩn hóa schema hiển thị (ưu tiên 2)** — một "view model" chung cho UI, map từ 3 nguồn (CA/KDC/Bank):

```
{ timestamp, source (ca|kdc|bank), category (key_issuance|cert_lifecycle|authentication|resource_access|admin_action),
  severity (info|warning|critical), actor { type: user|admin|service, id, display },
  action, target { type, id }, outcome (success|denied|error), reason, request_id, metadata }
```

Quy tắc severity gợi ý: mọi `*_rejected`/`invalid_signature`/`replay_detected` = warning; `revoked`, `certificate_rejected`, nhiều lần `as_rejected` cùng danh tính = critical; còn lại info.

**(d) UI enterprise theo vai trò:**

- Admin CA thấy: cert_lifecycle + key_issuance liên quan cert; Admin Bank thấy: resource_access + authentication của bank; (nếu có super admin thì thấy hợp nhất). Đây chính là "hỗ trợ đúng vai trò".
- Thành phần UI: dòng thời gian có badge severity + câu mô tả người-đọc-được ("Admin A đã thu hồi cert của user B — lý do: lost device"), filter theo category/severity/actor/time, drill-down xem metadata + request_id để trace xuyên hệ thống, export CSV/JSON (phục vụ retention demo), đếm nhanh "sự kiện bảo mật 24h".

### Tính khả thi

- (a) KDC audit: **trung bình** — 1–1.5 ngày (bảng mới + ghi tại 4 điểm trong as/tgs_service + RPC đọc + gateway route; toàn bộ pattern đã có sẵn từ CA/Bank, chỉ là lặp lại).
- (b) Gateway auth events: **dễ–trung bình** — 0.5–1 ngày, phụ thuộc chốt "ghi vào đâu".
- (c)+(d) Chuẩn hóa hiển thị: **dễ về kỹ thuật, cần thống nhất contract 3 người** — 1 ngày UI nếu schema chốt sớm. Không cần thay đổi dữ liệu đã ghi (map lúc đọc).
- Tổng phần audit: ~2–3 ngày người. Đề xuất thứ tự: (a) → (c) → (d) → (b).

---

## Phụ lục: vấn đề liên quan phát hiện thêm trong lúc kiểm tra

1. `env.ts` vẫn còn pattern default sai: `ADMIN_CA_DEMO_TOKEN: z.string().min(1).default("ADMIN_CA_DEMO_TOKEN is required")` — token demo thành chuỗi đoán được khi quên set env (đã nêu ở báo cáo trước, chưa fix; sẽ hết liên quan nếu Admin CA chuyển sang cert-based theo mục 6).
2. `failTransfer` ghi giao dịch fail vào cùng bảng/hash-chain với giao dịch thành công — đúng chủ đích ledger, nhưng mọi UI đọc `transactions` **bắt buộc** filter/hiển thị `status` (liên quan trực tiếp giả thuyết 2 của vấn đề #4).
3. Luồng đăng ký đốt jti trước cả khi validate CSR — user gõ sai gì đó cũng phải xin OTP lại (gộp fix với #1.3).

---


# 1. Hash-chain không bắt được sửa timestamp/metadata hay xóa dòng cuối
Chuỗi hash hoạt động thế nào: mỗi dòng lưu hash = SHA256(prev_hash | action | <các định danh> | reason). Verify replay từ dòng đầu: với mỗi dòng kiểm 2 điều — prev_hash phải khớp hash dòng trước (running), và hash phải khớp tính lại từ nội dung.

(a) Sửa riêng timestamp/metadata không bị phát hiện — vì 2 field này KHÔNG nằm trong hash.

Lý do loại chúng: created_at/performed_at là timestamptz và metadata là JSONB — đọc lại từ DB không round-trip byte-stable (timestamp lệch precision/timezone, JSONB đổi thứ tự key/whitespace). Nếu đưa vào hash, verify sẽ báo gãy giả dù không ai sửa. Nên tôi chỉ hash các cột text định danh round-trip chính xác (action/serial/actor/reason).
Hệ quả: đổi created_at hay chỉnh metadata của một dòng → hash không đổi → verify vẫn "ok". Nhưng đổi action/reason/actor/serial vẫn bị bắt.
(b) Xóa dòng cuối cùng (truncation) không bị phát hiện — vì chuỗi chỉ liên kết "lùi".

Mỗi dòng trỏ về dòng trước (prev_hash), không có gì trỏ "tới". Xóa một dòng giữa → dòng kế tiếp có prev_hash không khớp running → bắt được. Nhưng xóa dòng cuối → phần còn lại vẫn tự nhất quán (không dòng nào tham chiếu dòng đã mất) → verify replay thấy hợp lệ.
Đây là hạn chế kinh điển của hash-chain đơn thuần: nó chống sửa/chèn/đảo giữa nhưng không tự biết chuỗi "đáng lẽ dài bao nhiêu".
Cần "external anchor" để khắc phục: định kỳ lưu (last_hash, last_seq) ra một nơi mà DBA không sửa lén được cùng lúc với bảng audit — ví dụ append-only log riêng, checkpoint có chữ ký, WORM storage, hoặc đẩy sang hệ thống ngoài. Verify so đầu chuỗi hiện tại với anchor: nếu anchor.last_seq > max(seq) hiện tại → biết đuôi bị cắt. (Để bảo vệ cả timestamp/metadata thì phải hash chúng ở dạng canonical — unix seconds cho time, canonical-JSON cho metadata — nhưng lại quay về rủi ro round-trip đã tránh.)

# 2. Bank audit không gộp vào timeline/summary/verify nếu thiếu cookie bank
Nguyên nhân gốc — auth chéo domain không khớp. Các endpoint SOC (timeline/verify/summary/export) guard bằng security-admin. Khi gộp:

CA + KDC: Gateway gọi bằng credential dịch vụ của chính nó trên mạng TLS nội bộ → luôn được.
Bank: ListAdminAuditEvents/VerifyAuditChain phía Bank bắt buộc admin_session_token (Bank tự verify qua requireAdminSession). Một security-admin không có token này.
Nên Gateway chỉ fold Bank vào khi caller kèm cookie bank_admin_session — tức operator đồng thời là bank admin. Đây chính là lựa chọn bạn đã chốt ("cookie-gated, an toàn") thay vì "đường đọc tin cậy".

Hệ quả: view "cả 3" chỉ đầy đủ với super-admin cầm cả 2 credential (security-admin token + bank session). SOC thuần thấy CA+KDC; phần Bank báo sources.bank = {included:false, reason:"bank_admin_session_required"}.

Cách gộp thật (nếu muốn): thêm đường đọc bank audit cho security-admin — Bank tin Gateway trên mạng nội bộ (Gateway khẳng định role security-admin), không cần session người dùng. Đánh đổi: Gateway trở thành thành phần được tin để đọc bank audit, làm yếu isolation của Bank. Đó là lý do mặc định chọn cách an toàn.

# 3. Event auth-layer Bank fail sớm không có request_id
Nguyên nhân gốc — request_id nằm trong authenticator đã mã hóa. Trong AP exchange của Bank, request_id là dữ liệu bên trong authenticator, được mã hóa bằng session key K_{c,v}. Thứ tự xử lý ở auth.go:
```
1. giải mã Ticket_V        → nếu fail: audit "invalid_ticket"          (chưa có request_id)
2. kiểm hạn/scope ticket    → nếu fail: "ticket_expired"/"wrong_scope"  (chưa có request_id)
3. giải mã Authenticator    → nếu fail: "invalid_authenticator"         (chưa có request_id)
4. out.requestID = authn.RequestID   ← từ đây mới CÓ request_id
5. các event sau (stale_request, cert_rejected, replay…) → CÓ request_id
Các event ở bước 1–3 fail trước khi giải mã được authenticator → về mặt protocol chưa tồn tại request_id để ghi. Code ghi chúng với request_id rỗng.
```
Hệ quả: những event này không correlate được theo request_id — chúng không xuất hiện khi lọc /timeline?request_id=.... Chúng vẫn được ghi (có action, cert_serial nếu ticket giải mã được, created_at) nhưng không link vào một trace phiên.

Trace thay thế: dùng created_at + cert_serial (nếu ticket đã giải mã) để đối chiếu tay.

Vì sao khó fix triệt để: request_id đặt trong authenticator mã hóa là thiết kế protocol — nếu ticket/authenticator không giải mã được thì thực sự không có request_id. Cách cải thiện khả thi: thread trace-id HTTP (X-Request-ID) của Gateway qua gRPC metadata vào bank flow (như CA/KDC đã làm) để dùng làm request_id dự phòng cho các event fail sớm. Hiện luồng transfer/balance của Bank chưa truyền trace-id xuống metadata, nên đây là một enhancement (không nằm trong 11+5 commit đã làm).

---

# 4. Timeline chỉ hiện 1 sự kiện

### Hiện tượng
`GET /v1/admin/audit/timeline?request_id=<id>` chỉ trả ~1 event thay vì cả chuỗi register→AS→TGS→transfer.

### Nguyên nhân gốc: `request_id` KHÔNG nhất quán và KHÔNG cùng namespace

Timeline lọc CA/KDC/Bank theo **cùng một** `request_id`. Nhưng thực tế mỗi event mang một `request_id` khác nhau vì **3 lý do cộng dồn**:

**(a) Frontend sinh X-Request-ID mới cho MỖI HTTP call.**
[api.service.ts:39,75](mini-banking-app/frontend/src/services/api.service.ts) đặt `"X-Request-ID": crypto.randomUUID()` ở **mọi** `apiGet`/`apiPost`. Nên OTP request / OTP verify / register / AS_REQ / TGS_REQ — mỗi bước một trace-id khác. Chỉ cặp `ra_registration_approved` + `issued` (cùng 1 call `handleRegister`) mới chia sẻ id → đó thường là "2 event" hiếm hoi thấy chung; các bước khác đứng riêng.

**(b) Gateway KHÔNG forward X-Request-ID xuống KDC ở luồng user.**
[kdc.service.ts `requestTgt`/`requestServiceTicket`](mini-banking-app/api-gateway/src/services/kdc.service.ts) gọi KDC **không kèm gRPC metadata** (khác với `listKdcAuditEvents` có `traceMetadata`). Nên KDC handler đọc `x-request-id` = rỗng → event `as_ticket_issued`/`tgs_ticket_issued` có **request_id rỗng** → không correlate được với bất kỳ id nào.

**(c) Bank dùng request_id của AP flow — khác namespace với CA/KDC.**
Bank ghi `request_id = authn.RequestID` ([auth.go:80](mini-banking-app/banking-service/internal/grpc/auth.go)) — tức `request_id` **bên trong authenticator**, là **UUID sinh riêng ở frontend** ([tgs-exchange.service.ts:60](mini-banking-app/frontend/src/services/tgs-exchange/tgs-exchange.service.ts), transfer flow). Đây là giá trị **hoàn toàn khác** với `X-Request-ID` (HTTP) mà CA/KDC dùng. Gateway `transferMoney`/`getBalance` cũng **không** gắn x-request-id xuống Bank. → Bank event **không bao giờ** chung request_id với CA/KDC.

### Kết luận
Gộp cả (a)(b)(c): hầu như **không có 2 event nào chia sẻ cùng `request_id`** (trừ cặp register). Nên timeline lọc theo một `request_id` bất kỳ chỉ tìm được **1 event** (đôi khi 2 với register).

### Bằng chứng đối chiếu nguồn `request_id`

| Event | `request_id` lấy từ | Giá trị |
|---|---|---|
| CA `issued`, `ra_*` | HTTP `X-Request-ID` (metadata) | UUID của call đó (mỗi call khác) |
| KDC `as/tgs_*` | HTTP `X-Request-ID` (metadata) | **rỗng** (gateway user-flow không forward) |
| Bank `transfer_*` | authenticator AP flow | UUID AP (khác namespace HTTP) |

### Hướng sửa (không thực hiện) — cần trace-id nhất quán 1 phiên

1. **Frontend**: sinh **một** `X-Request-ID` cho cả một phiên nghiệp vụ (register hoặc transfer end-to-end) và **tái dùng** cho mọi HTTP call trong phiên, thay vì random mỗi call.
2. **Gateway**: forward `X-Request-ID` xuống **KDC** (AS/TGS) và **Bank** (transfer/balance) qua gRPC metadata `x-request-id` (như CA đã làm cho register).
3. **Bank**: ghi thêm trace-id HTTP đó vào audit (song song với `request_id` AP), để timeline correlate được Bank với CA/KDC. Có thể dùng làm cột `trace_id` riêng hoặc ghi vào `metadata`.

(Đây đúng là cảnh báo "trace-id phải chạy suốt" đã nêu trong kế hoạch — nếu mỗi call sinh id mới thì timeline rời rạc.)