# Báo cáo bảo mật toàn bộ luồng — mini-banking-app

> Ngày: 2026-07-11. Ngôn ngữ: tiếng Việt.
> Tài liệu liên quan: [report.md](report.md) (phân tích lỗ hổng ban đầu),
> [plan.md](plan.md) (kế hoạch), [security-upgrade-report.md](security-upgrade-report.md)
> (chi tiết triển khai AS_REP verify).
>
> Báo cáo này rà **tất cả các luồng** của hệ thống, nêu control bảo mật của từng
> luồng, trạng thái **sau** đợt nâng cấp (AS_REP verify + client cert verify), và
> rủi ro còn lại.

## 0. Kiến trúc & mô hình tin cậy

```
Browser (SPA) ──HTTPS*──► API Gateway ──gRPC/TLS1.3──► CA / KDC / Bank services
                                                        └── Postgres (CA, Bank) + Redis
```

- **PKI 3 tầng:** Root CA (offline, 10 năm) → {Client CA, gRPC Transport CA} → leaf.
- **Kerberos-style:** AS Exchange (TGT) → TGS Exchange (Ticket_v) → AP Exchange (giao dịch).
- **Trust anchor phía client:** **Root CA nạp runtime từ file same-origin**
  (`/trust/root-ca.pem`, [trust-anchors.ts](frontend/src/config/trust-anchors.ts)) —
  KHÔNG hard-code trong bundle; phục vụ bởi chính origin SPA (không qua gateway).
  Nền tảng để client tự xác minh chữ ký KDC và cert do CA cấp.
- **Khóa riêng client:** sinh trong browser (WebCrypto, non-extractable sau khi
  wrap), **wrap bằng PIN** (PBKDF2 210k vòng + AES-GCM), lưu IndexedDB. Không bao
  giờ rời browser dạng plaintext.

\* HTTPS: **bắt buộc khi deploy**; hiện demo chạy `http://localhost:3000` (xem §7).

---

## 1. Luồng đăng ký khách hàng (Customer Registration)

**Các bước:**
1. `POST /v1/otp/request {email}` → gateway gửi OTP qua email (Redis, TTL 5').
2. `POST /v1/otp/verify {email, otp}` → trả **reg_token** (JWT, TTL 10', **dùng 1
   lần** qua Redis jti) + `owner_id` (server sinh, client không kiểm soát).
3. Browser: sinh RSA keypair → dựng **CSR + proof-of-possession** (ký CSR bằng
   private key) → `POST /v1/auth/register` (Bearer reg_token, CSR).
4. Gateway: verify JWT + jti chưa dùng + email chưa tồn tại → CA ký cert (role
   customer, CN=fullName, SAN email + owner URI) → tạo tài khoản bank (rollback +
   revoke cert nếu tạo tài khoản lỗi) → đánh dấu jti đã dùng.
5. Browser: **xác minh cert TRƯỚC KHI lưu** ([verifyIssuedCertificate](frontend/src/services/pki-registration/pki-registration.service.ts)) → lưu wrapped key + cert vào IndexedDB.

**Control bảo mật:**
| Control | Cơ chế |
| --- | --- |
| Xác thực chủ email | OTP một lần, TTL 5', xóa sau verify |
| Chống dùng lại token | reg_token JWT ký + jti single-use (Redis), TTL 10' |
| Chống giả danh owner_id | owner_id do gateway đặt từ token đã vetting, **không** từ client |
| Proof-of-possession | CA verify chữ ký CSR khớp public key trước khi cấp |
| Chống email trùng | Kiểm tra bank trước khi cấp; rollback + revoke nếu lỗi |
| **Chống tráo cert (MỚI)** | Client verify: (1) cert **chain về Root nhúng**, (2) public key trong cert **trùng khóa vừa sinh**, (3) CN + SAN email khớp đăng ký. Fail-closed. |

**Trạng thái sau nâng cấp:** ✅ Đã đóng lỗ "nhận cert không kiểm tra" mô tả trong
[report.md](report.md) §Pha 1. Nếu MITM/gateway trả cert giả (CA khác) hoặc cert
của khóa khác → client **từ chối lưu**. Đã kiểm chứng bằng interop test (cert do
Client CA cấp: chain OK, khớp/không-khớp key, khớp SAN email, thiếu intermediate bị
từ chối).

**Rủi ro còn lại:** chưa kiểm thu hồi (CRL/OCSP) tại thời điểm nhận cert — nhưng cert
vừa cấp nên không liên quan; thu hồi quan trọng hơn ở phía verify runtime (§8).

---

## 2. Luồng đăng nhập — AS Exchange (Kerberos Phase 2)

**Các bước:** Browser đọc cert + wrapped key (mở bằng PIN) → dựng AS_REQ (owner_id,
cert_sn, nonce, timestamp) → **ký RSA** → `POST /v1/auth/as-req` → KDC trả AS_REP
(mã hóa lai + **chữ ký KDC** + **chuỗi cert KDC**) → browser giải mã, **verify**,
lưu TGT + K_c,tgs trong RAM.

**Control bảo mật:**
| Control | Cơ chế |
| --- | --- |
| Xác thực client | KDC verify chữ ký AS_REQ bằng public key trong cert (CA-authoritative) |
| Binding danh tính | owner_id phải khớp owner của cert (fail-closed, chống giả owner) |
| Chống replay | nonce+timestamp, cửa sổ 5', Redis SET NX |
| Bảo mật session key | K_c,tgs mã hóa lai: AES-GCM + RSA-OAEP tới public key client |
| **Xác thực KDC (MỚI)** | Browser verify **chuỗi cert KDC** chain về Root nhúng, kiểm CN=kdc-service, rồi **verify `kdc_signature`** (RSA-PSS/SHA-256) trên đúng plaintext AS_REP. Fail-closed. |

**Trạng thái sau nâng cấp:** ✅ **Đây là lỗ hổng gốc đã đóng.** Trước đây client bỏ
qua `kdc_signature` → gateway bị chiếm hoặc MITM có thể giả AS_REP và ép session key.
Nay client xác minh end-to-end danh tính KDC bằng trust anchor độc lập với gateway.
Đã kiểm chứng interop (chữ ký thật verify được; chữ ký/payload/chain bị sửa → từ chối).

**Rủi ro còn lại:** phụ thuộc `KDC_SIGNING_CHAIN_PATH` được cấu hình (fail-closed nếu
thiếu). Chưa kiểm thu hồi cert KDC (§8).

---

## 3. Luồng TGS Exchange (Kerberos Phase 3)

**Các bước:** Browser dựng Authenticator = E_{K_c,tgs}[client_id, ts, nonce2,
request_id, scope] → `POST /v1/auth/tgs-req` (kèm TGT) → KDC giải mã TGT bằng K_tgs,
giải Authenticator bằng K_c,tgs, cấp Ticket_v + K_c,v → browser giải mã, kiểm nonce2
+ scope, lưu Ticket_v (RAM).

**Control bảo mật:** đối xứng — tin cậy đến từ bí mật chia sẻ `K_c,tgs`. KDC kiểm
scope (allowlist theo service), replay (nonce2, Redis), thời hạn TGT, khớp cert_sn.
Client kiểm nonce2 + scope khớp yêu cầu (chống replay/nhầm vé).

**Trạng thái:** ✅ Đúng thiết kế Kerberos. An toàn của TGS **kế thừa** từ AS
(§2): vì K_c,tgs chỉ client và KDC biết (nay đã được bảo đảm bởi verify AS_REP),
giải mã được + nonce khớp là đủ xác thực. Không cần chữ ký bất đối xứng ở tầng này.

**Rủi ro còn lại:** không có điểm yếu độc lập; phụ thuộc AS Exchange an toàn.

---

## 4. Luồng giao dịch — AP Exchange / Transfer (Kerberos Phase 4)

**Các bước:** Đảm bảo có Ticket_v (scope `transfer:create`) → dựng Authenticator
(E_{K_c,v}) + **payload canonical ký RSA-PSS** bằng private key client → CipherPayload
= E_{K_c,v}[payload + chữ ký] → `POST /v1/bank/transfer` → Bank verify ticket +
Authenticator + **chữ ký RSA-PSS** trên payload tái-canonical → thực hiện chuyển tiền
→ AP_REP (E_{K_c,v}[result, tx_id, nonce]) → browser kiểm nonce + result.

**Control bảo mật:**
| Control | Cơ chế |
| --- | --- |
| Toàn vẹn + chống chối bỏ lệnh | Client **ký RSA-PSS** payload; Bank verify — attacker không tự chế lệnh hợp lệ |
| Bảo mật nội dung | Toàn bộ payload mã hóa bằng K_c,v |
| Chống replay | nonce3 + request_id + idempotency_key; Bank kiểm nonce/replay |
| Canonical khớp tuyệt đối | Client gửi đúng bytes canonical đã ký; Bank tái canonical để verify |

**Trạng thái:** ⚠️ **Điểm sáng + tồn đọng nhỏ.** Chiều client→Bank rất chắc (ký số +
mã hóa). Chiều **AP_REP (Bank→client) là đối xứng, client không verify chữ ký Bank** —
nếu chuỗi tin cậy đứt (chỉ xảy ra nếu AS bị giả, nay đã chặn) attacker có thể "nuốt"
lệnh và giả phản hồi "thành công". Sau khi §2 đóng lỗ AS, rủi ro này chỉ còn khi
gateway bị chiếm ở tầng TLS-terminate (xem §7).

**Khuyến nghị (tùy chọn):** cấp signing cert cho Bank (theo đúng khuôn KDC ở
[security-upgrade-report.md](security-upgrade-report.md)) và cho client verify chữ ký
AP_REP → mutual auth đầy đủ cho phản hồi giao dịch.

---

## 5. Luồng Admin (CA Admin & Bank Admin)

**Kích hoạt (activate):** admin nhận activation_token (từ provisioning) → sinh
keypair + CSR → `POST /v1/admin/{bank|ca}/activate` → CA cấp cert role
`bank_admin`/`ca_admin` (qua cùng `registerUser`) → lưu cert.

**Đăng nhập (challenge-signature):** client tạo challenge `admin-ca-login:serial:reqid:ts`
→ ký bằng private key → gateway verify chữ ký bằng public key trong cert, kiểm cert
**ACTIVE** + đúng **role** → cấp JWT phiên (8h).

**Control bảo mật:** xác thực bằng sở hữu private key (challenge-signature, chống
replay bằng ts ±5'), phân quyền theo role trong cert (CA-authoritative), cert phải
active. Role do CA đặt, client không chọn được.

**Trạng thái:** ✅ Cơ chế đăng nhập bằng chữ ký cert là vững. ⚠️ **Chưa áp dụng
client-side verifyIssuedCertificate cho 2 luồng admin** (đợt này giới hạn phạm vi ở
luồng khách hàng theo yêu cầu). Hạ tầng đã sẵn (`verifyIssuedCertificate` + chain
delivery giống hệt) — chỉ cần surface `chain_pem` ở 2 response admin và gọi verify.
Rủi ro tương đương Pha 1 khách hàng trước nâng cấp, nhưng bề mặt hẹp (admin nội bộ).

**Khuyến nghị:** áp dụng verify cert cho 2 luồng admin để đồng nhất (bước nhỏ, đã có mẫu).

---

## 6. Luồng Audit / SOC

**Cơ chế:** 3 bảng audit (`bank_audit_log`, `kdc_audit_log`, `certificate_audit_log`)
mỗi bản ghi có `seq/prev_hash/hash` = **hash chain SHA-256** chống sửa/xóa/đảo. Endpoint
`/v1/admin/audit/verify` replay chuỗi và báo mắt xích gãy đầu tiên. Ghi audit
best-effort (không chặn nghiệp vụ).

**Control:** tamper-evidence bằng hash chain; đọc audit qua đường admin (guard ở
gateway, chỉ internal TLS). Enrich semantics cho timeline SOC.

**Trạng thái:** ✅ Hash chain là tamper-evident tốt. Lưu ý vận hành: migration hash
chain phải được áp dụng (đã bổ sung migrator tự động trong compose — xem lịch sử).

**Rủi ro còn lại:** hash chain phát hiện sửa đổi nhưng không chống được xóa toàn bộ
+ ghi lại nếu kẻ tấn công có toàn quyền DB (cần WORM/replication ngoài phạm vi demo).

---

## 7. Lớp vận chuyển (Transport)

| Kênh | Trạng thái | Ghi chú |
| --- | --- | --- |
| Browser ↔ Gateway | ⚠️ **HTTP ở demo** (`localhost:3000`) | **Bắt buộc HTTPS khi deploy** (HSTS, redirect). Chặn network MITM. |
| Gateway ↔ CA/KDC/Bank | ✅ TLS 1.3, verify server bằng gRPC Transport CA | **Server-auth, KHÔNG mTLS**: không service nào đặt `ClientAuth: RequireAndVerifyClientCert`. Danh tính caller dựa vào cô lập mạng + token tầng ứng dụng. |

**Khuyến nghị:**
1. **HTTPS gateway (P0)** — điều kiện cần để mọi verify tầng app có ý nghĩa trước
   network MITM.
2. **Cân nhắc mTLS nội bộ (P2)** — thêm `RequireAndVerifyClientCert` để xác thực
   caller giữa các service (defense-in-depth ngoài cô lập mạng).

---

## 8. Lưu trữ bí mật phía client

- Private key: wrap bằng PIN (PBKDF2-HMAC-SHA256 **210.000 vòng** + AES-256-GCM),
  lưu IndexedDB; import non-extractable khi dùng. Plaintext không rời browser.
- Session key/TGT/Ticket: **chỉ trong RAM** (session.ts), không persist.
- Trust anchor (Root CA): nạp runtime từ file same-origin `/trust/root-ca.pem`
  (không hard-code), công khai — an toàn; fail-closed nếu không nạp được.

**Rủi ro còn lại:** PIN yếu → brute-force offline nếu kẻ tấn công lấy được blob
IndexedDB (giảm thiểu bằng 210k vòng PBKDF2 + nên có chính sách độ mạnh PIN); XSS có
thể lạm dụng phiên đang mở (giảm thiểu bằng CSP — §9).

---

## 9. Tổng hợp rủi ro còn lại & khuyến nghị (ưu tiên)

| Ưu tiên | Hạng mục | Trạng thái |
| --- | --- | --- |
| **P0** | HTTPS cho gateway (HSTS) | Chưa (demo HTTP) |
| **P1** | Áp verify cert cho 2 luồng **admin** (CA/Bank) | Hạ tầng sẵn, chưa bật (phạm vi đợt này = khách hàng) |
| **P1** | Verify chữ ký **AP_REP** (cấp signing cert cho Bank) | Tùy chọn, mutual-auth đầy đủ cho giao dịch |
| **P2** | **Thu hồi** (CRL/OCSP) khi verify chain runtime | Chưa (chain-verify chưa kiểm revocation) |
| **P2** | **mTLS** nội bộ giữa các service | Hiện server-auth TLS 1.3 |
| **P2** | Security headers (CSP, X-Content-Type-Options...) | Chưa |
| **P3** | Chính sách độ mạnh PIN | Nên thêm |

## 10. Tóm tắt trạng thái theo luồng

| Luồng | Xác thực bất đối xứng end-to-end | Trạng thái |
| --- | --- | --- |
| Đăng ký khách hàng | ✅ Verify cert (chain+key+subject) | **Đã đóng** |
| AS Exchange (login) | ✅ Verify kdc_signature | **Đã đóng (lỗ hổng gốc)** |
| TGS Exchange | Kế thừa AS (đối xứng, đúng thiết kế) | An toàn |
| Transfer / AP | Client→Bank ký số ✅; AP_REP đối xứng ⚠️ | Chắc chiều gửi; xét ký AP_REP |
| Admin CA/Bank | Đăng nhập challenge-sig ✅; verify cert lúc nhận ⚠️ | Nên đồng bộ verify cert |
| Audit/SOC | Hash chain tamper-evident ✅ | An toàn (trong phạm vi) |
| Transport | TLS 1.3 nội bộ ✅; HTTPS gateway ⚠️ | Cần HTTPS khi deploy |

---

*Các nhận định dựa trên trạng thái repo tại thời điểm rà soát. Phần "Đã đóng" đã được
kiểm chứng bằng interop test WebCrypto (xem [security-upgrade-report.md](security-upgrade-report.md)).*