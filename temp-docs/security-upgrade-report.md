# Báo cáo triển khai nâng cấp bảo mật — Xác minh AS_REP end-to-end phía client

> Ngày: 2026-07-11
> Liên quan: [report.md](report.md) (phân tích lỗ hổng), [plan.md](plan.md) (kế hoạch).
> Trạng thái: **P0 (GĐ 1 + GĐ 2) đã triển khai và kiểm chứng end-to-end.** GĐ 0
> (HTTPS), GĐ 3 (verify cert lúc đăng ký), GĐ 4 (dài hạn) — còn lại, xem mục 7.

## 1. Tóm tắt

Lỗ hổng gốc: frontend **không xác minh chữ ký** `kdc_signature` trên AS_REP (dù KDC
đã ký), nên toàn bộ chuỗi Kerberos chỉ an toàn bằng độ an toàn của kênh HTTP tới
gateway. Một gateway bị chiếm quyền — hoặc network MITM khi chưa có HTTPS — có thể
giả AS_REP, gán session key do kẻ tấn công chọn, và client không phát hiện được.

Bản nâng cấp này thiết lập **một trust anchor duy nhất: Root CA nhúng vào frontend**,
cấp **certificate cho khóa ký của KDC** (chain về Root qua Client CA), cho KDC gửi
kèm chuỗi cert trong AS_REP, và cho frontend **xác minh chuỗi + chữ ký** trước khi
chấp nhận session key (fail-closed). Kể từ đây, dù gateway bị chiếm, nó không thể giả
AS_REP mà qua mặt được client.

## 2. Trước / Sau

| | Trước | Sau |
| --- | --- | --- |
| Trust anchor phía client | Không có (chỉ tin TLS tới gateway) | **Root CA nhúng lúc build** |
| Khóa ký AS_REP của KDC | Raw keypair, không thuộc chain nào | **Cert leaf `CN=kdc-service`, chain về Root qua Client CA** |
| `kdc_signature` | Được gửi nhưng **client bỏ qua** | **Client verify** (RSA-PSS/SHA-256) sau khi verify chain |
| Salt RSA-PSS khi KDC ký | `PSSSaltLengthAuto` (WebCrypto không verify được) | `PSSSaltLengthEqualsHash` (=32, WebCrypto verify được) |
| Kịch bản gateway giả AS_REP | Không phát hiện | **Bị từ chối** (chain/chữ ký không hợp lệ) |

## 3. Kiến trúc sau nâng cấp (luồng AS Exchange)

```
Đăng ký (một lần, qua gen-certs):
  kdc-private.pem ──CSR──► Client CA ký ──► kdc-signing.crt
  kdc-signing.crt + client-ca.crt ──► kdc-signing-chain.pem   (leaf + intermediate)
  Root CA cert ──► nhúng vào frontend/src/config/trust-anchors.ts

Runtime:
  KDC: ký payload AS_REP (RSA-PSS, salt=hash) bằng kdc-private
       đính kèm kdc_cert_chain = kdc-signing-chain.pem vào ASResponse
  Gateway: chuyển ASResponse nguyên vẹn (không cần đổi code — chain nằm trong bytes opaque)
  Browser (performAsExchange):
     1. giải mã payload  → plaintext bytes (CHÍNH LÀ bytes KDC đã ký)
     2. verifyChainToRoot(kdc_cert_chain) → chain leaf→Client CA→Root nhúng ✔, lấy pubkey leaf
     3. kiểm CN == "kdc-service"
     4. verifyRsaPss(pubkey, kdc_signature, plaintext) ✔
     5. chỉ khi TẤT CẢ ✔ mới lưu session key; ngược lại throw (fail-closed)
```

Điểm mấu chốt về tính đúng đắn: KDC ký `json.Marshal(ASRepPayload)` và cũng mã hóa
**chính struct đó** (`encryptJSON` marshal lại y hệt). Vì `json.Marshal` của Go xác
định (deterministic), **plaintext client giải mã ra chính là bytes đã ký** — nên
client verify trực tiếp trên plaintext, không phải tự tái dựng JSON. Điều này loại bỏ
hoàn toàn rủi ro "khớp byte-for-byte" mà [plan.md](plan.md) lo ngại.

## 4. Thay đổi chi tiết theo thành phần

### 4.1 Backend — KDC (Go)

- [kdc-service/internal/kdc/as_service.go](kdc-service/internal/kdc/as_service.go)
  - Ký AS_REP bằng `rsa.SignPSS(..., &rsa.PSSOptions{SaltLength: PSSSaltLengthEqualsHash, Hash: SHA256})`
    thay cho `nil` opts → salt cố định 32 byte để WebCrypto verify được.
  - Đính kèm `KDCCertChain: s.signingChainPEM` vào `ASResponse`.
- [kdc-service/internal/kdc/types.go](kdc-service/internal/kdc/types.go)
  - `ASResponse` thêm field `KDCCertChain string json:"kdc_cert_chain,omitempty"`.
  - `ASService`/`ASConfig` thêm `signingChainPEM` / `SigningChainPEM`.
- [kdc-service/internal/kdc/service.go](kdc-service/internal/kdc/service.go)
  - Thêm `loadSigningChain()` đọc file chain (best-effort: thiếu thì log cảnh báo,
    không chặn boot); truyền vào `ASConfig`.
- [kdc-service/internal/config/env.go](kdc-service/internal/config/env.go)
  - Thêm biến môi trường tùy chọn `KDC_SIGNING_CHAIN_PATH`.
- [kdc-service/.env.example](kdc-service/.env.example) — tài liệu biến mới.

### 4.2 Sinh chứng chỉ — gen-certs

- [scripts/gen-certs/gen-certs.sh](scripts/gen-certs/gen-certs.sh) và
  [scripts/gen-certs/gen-certs.ps1](scripts/gen-certs/gen-certs.ps1)
  - Thêm bước cấp **KDC signing cert**: tái dùng `kdc-private.pem` hiện có (đúng khóa
    KDC ký AS_REP), tạo CSR, ký bằng **Client CA** thành leaf `CN=kdc-service`
    (`basicConstraints=CA:FALSE`, `keyUsage=digitalSignature`).
  - Ghi `kdc-signing.crt` và `kdc-signing-chain.pem` (leaf + Client CA).

### 4.3 Compose / môi trường

- [docker-compose.local.yml](docker-compose.local.yml) và
  [docker-compose.demo.yml](docker-compose.demo.yml)
  - Set `KDC_SIGNING_CHAIN_PATH: /certs/kdc/kdc-signing-chain.pem` cho `kdc-service`
    (thư mục `kdc-service/certs` vốn đã được mount nên không cần thêm volume).

### 4.4 Frontend

- [frontend/src/config/trust-anchors.ts](frontend/src/config/trust-anchors.ts) **(mới)**
  - Trust anchor Root CA **nạp runtime, KHÔNG hard-code**: fetch từ file tĩnh
    `frontend/public/trust/root-ca.pem` (phục vụ **same-origin** bởi SPA tại
    `/trust/root-ca.pem`, đổi được qua env `VITE_ROOT_CA_URL`). Có `setRootCaPem()`
    cho test/SSR. Fail-closed nếu không nạp được. File do `gen-certs` sinh ra
    (`.pem` đã gitignore — artifact như các cert khác).
  - ⚠️ Ràng buộc: anchor phải do **chính origin frontend** phục vụ, KHÔNG lấy qua
    gateway (nếu không, bên kiểm soát gateway vừa cấp anchor giả vừa cấp AS_REP giả).
- [frontend/src/services/chain-verify.ts](frontend/src/services/chain-verify.ts) **(mới)**
  - Xác minh chuỗi X.509 trong browser (WebCrypto), không thư viện ngoài:
    parse DER, verify chữ ký từng mắt xích (RSA PKCS#1v1.5/SHA-256), chain name
    matching (so DN đã chuẩn hóa), kiểm thời hạn, intermediate phải là CA, kết thúc
    đúng ở Root nhúng. Trả về public key leaf (import RSA-PSS) + subject CN.
- [frontend/src/services/key.service.ts](frontend/src/services/key.service.ts)
  - Thêm `verifyRsaPss(pubKey, sig, data)` (RSA-PSS/SHA-256, saltLength 32).
- [frontend/src/services/as-exchange/as-exchange.service.ts](frontend/src/services/as-exchange/as-exchange.service.ts)
  - Sau khi giải mã AS_REP, **trước khi lưu session**: verify chain về Root, kiểm
    `CN=kdc-service`, verify `kdc_signature` trên đúng plaintext. Bất kỳ lỗi nào ⇒
    throw. Cập nhật lại comment "mutual auth" cho đúng ngữ nghĩa.

## 5. Kiểm chứng (bằng chứng)

| Hạng mục | Kết quả |
| --- | --- |
| `go build ./...` (kdc-service) | ✅ exit 0 |
| `go test ./internal/kdc/...` | ✅ `ok` (salt đổi không phá test cũ) |
| `openssl verify` chain leaf→Client CA→Root | ✅ `kdc-signing.crt: OK` |
| Khóa cert khớp `kdc-private.pem` | ✅ MATCH |
| KDC nạp chain lúc chạy | ✅ log `loaded KDC signing chain ... (3660 bytes)` |
| `vite build` toàn app (2280 modules) | ✅ built (chỉ warning chunk-size, không liên quan) |
| Toàn stack | ✅ 8/8 container healthy, frontend HTTP 200 |
| **Interop crypto test** (WebCrypto, Node 20) | ✅ **ALL PASSED** |

**Interop test** (mô phỏng đúng tham số ký của Go, xác minh bằng chính code
`chain-verify.ts` + `verifyRsaPss` đã bundle) chứng minh:

```
PASS: chain verifies to embedded Root CA, CN=kdc-service
PASS: genuine KDC signature verifies
PASS: tampered signature rejected
PASS: tampered payload rejected
PASS: chain not reaching embedded root is rejected
```

## 6. Lỗi mà test đã phát hiện (minh bạch)

Test interop đã bắt **2 bug thật** trong bản `chain-verify.ts` đầu tiên, đã sửa:

1. **So khớp DN quá chặt.** So raw DER của issuer/subject thất bại vì Root/Client CA
   do Go encode (UTF8String) còn leaf do openssl encode (PrintableString) cho cùng
   một DN. → Sửa: so khớp **DN đã chuẩn hóa** (OID=value, trim/lowercase). Ràng buộc
   mật mã thật vẫn nằm ở việc verify chữ ký, nên đổi này không làm yếu bảo mật.
2. **Bỏ sót intermediate trong bundle.** `verifyChainToRoot` chỉ lấy block cert đầu
   tiên của PEM, bỏ Client CA → chain không tới được Root. → Sửa: nạp **tất cả** block.

Đây chính là lý do bước kiểm thử interop được đặt là "nguồn chân lý" trong plan.

## 7. Còn lại (theo plan)

| GĐ | Việc | Ghi chú |
| --- | --- | --- |
| **0** | Bật HTTPS cho gateway (HSTS, redirect) | Việc hạ tầng/ops; hiện chạy `http://localhost:3000` cho demo. **Cần trước khi deploy.** |
| **3** | Verify cert lúc đăng ký (chain + khớp public key + subject) | Hạ tầng `chain-verify.ts` đã sẵn sàng; chỉ cần gọi trong luồng đăng ký và (nếu thiếu) bổ sung `chain_pem` vào response đăng ký. |
| **4** | CRL/OCSP (thu hồi), security headers, xoay khóa | Dài hạn. `chain-verify` hiện **chưa** kiểm thu hồi. |

## 8. Ghi chú vận hành

- **Sinh lại cert:** chạy `scripts/gen-certs/gen-certs.ps1` (hoặc `.sh`) sẽ tạo
  `kdc-signing-chain.pem`; các file cert này **không commit** (đã gitignore), sinh
  cục bộ. Sau khi đổi code KDC cần `docker compose build kdc-service`.
- **Xoay khóa:** xoay Client CA hay khóa/cert KDC **không** cần đụng frontend
  (client verify chain động qua Root anchor). **Chỉ** khi xoay **Root CA** mới cần
  thay file `frontend/public/trust/root-ca.pem` (gen-certs tự đặt) — không cần sửa
  code, không rebuild bundle (anchor nạp runtime). Root sống lâu, offline nên hiếm.
- **Fail-closed:** nếu KDC không có `KDC_SIGNING_CHAIN_PATH`, AS_REP thiếu
  `kdc_cert_chain` và client **từ chối đăng nhập** (đúng thiết kế). Đảm bảo đã chạy
  gen-certs trước khi dựng stack.

---

*Tham chiếu mã trỏ tới trạng thái repo tại thời điểm triển khai. Chứng chỉ nhúng
`ROOT_CA_PEM` là bản công khai, an toàn để commit; mọi khóa riêng vẫn ở phía server.*
