# Đánh giá ảnh hưởng khi nâng cấp kiến trúc CA

## 0. Nâng cấp CA

Phần CA đã được phát triển theo kiến trúc phân tầng:

- Root CA là trust anchor cao nhất, self-signed, chỉ ký Intermediate CA.
- gRPC Transport CA là Intermediate CA dùng để ký service TLS certificate cho CA/KDC/Bank.
- Client CA là Intermediate CA dùng để ký user/client certificate khi đăng ký PKI.
- `ca-server.crt` là server TLS certificate của CA Service, không phải CA certificate.
- Internal gRPC verify TLS bằng file `grpc-ca.crt` được phân phối dạng trust bundle gồm gRPC Transport CA và Root CA.
- User/client certificate hợp lệ phải có metadata thể hiện chain `Client CA -> Root CA`.

Các thay đổi chính đã ảnh hưởng đến code:

- CA DB có bảng `ca_issuers` và mở rộng bảng `certificates` với `cert_type`, `issuer_id`, `issuer_common_name`, `issuer_serial_number`, `chain_pem`, `chain_fingerprints`, `is_ca`, `key_usage`, `extended_key_usage`.
- CA Service runtime signer đã chuyển sang dùng Client CA để ký user certificate; Root CA không ký trực tiếp user cert.
- `RegisterUser`, `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail`, `RevokeCertificate` đã mang thêm issuer/chain metadata.
- Admin CA API/UI đã hiển thị và filter theo `cert_type`, `issuer_id`, issuer metadata, chain fingerprints.
- Revoke qua Admin CA chỉ áp dụng cho `cert_type=client`; Root CA, Intermediate CA và service TLS cert phải bị từ chối.
- KDC và Bank verify path đã kiểm tra client cert theo `cert_type=client`, `issuer_id=client-ca` và chain metadata.
- Chốt prefix đang dùng trong code là `/v1/admin-ca/*`, role đang dùng là `admin-ca`

## 1. Đồng bộ hóa

Các điểm chốt để tránh lệch pha:

- Prefix route Admin CA: `/v1/admin-ca/*`.
- Role admin:
  - Admin CA dùng role `admin-ca`.
  - Admin Bank dùng role `admin-bank` 
- Audit endpoint:
  - Admin CA UI hiện giữ tab "Audit endpoint pending".
  - Thuận cần chốt route và response để Thanh nối UI.
- Request id:
  - Gateway dùng HTTP `X-Request-ID`.
  - CA admin gRPC đang nhận request id qua metadata.
  - Bank user flow dùng `request_id` trong body/authenticator.
  - Demo/testcase cần ghi rõ nguồn request id theo từng flow.
- Cert path/env:
  - `grpc-ca.crt` là trust bundle cho internal gRPC.
  - `client-ca` là signer cho user/client cert.
  - Root CA không ký trực tiếp user cert hoặc service TLS cert.
- Test data:
  - User cert dùng cho KDC/Bank nên sinh qua flow đăng ký thật.
  - Nếu seed trực tiếp DB, phải ghi rõ đó là dữ liệu demo và đảm bảo đủ issuer/chain metadata.

Kết luận: nâng cấp CA đã làm thay đổi contract tin cậy của toàn hệ thống. Các phần còn lại không cần viết lại từ đầu, nhưng phải đồng bộ theo metadata mới: `cert_type`, `issuer_id`, chain fingerprints, revoke guard và trust bundle `grpc-ca.crt`.

---

## 2. Ảnh hưởng đến nhiệm vụ trong PROCESS.md

### Ảnh hưởng đến Thái

Phần của Thái là Admin Bank API + Frontend Admin Bank. Layered CA ảnh hưởng chủ yếu đến các dữ liệu liên quan certificate trong audit, ledger và security view.

Các điểm cần chú ý:

- Bank không nên coi `cert_serial` là đủ để chứng minh trust. Serial chỉ là khóa tra cứu; kết quả verify từ CA mới là nguồn tin cậy.
- Khi hiển thị audit hoặc transaction liên quan certificate, nên giữ hoặc hiển thị thêm metadata nếu có:
  - `cert_type`
  - `issuer_id`
  - `issuer_common_name`
  - `chain_fingerprints`
- Event `certificate_rejected` trong `bank_audit_log` cần đủ reason/metadata để phân biệt:
  - cert revoked hoặc expired
  - cert không phải `client`
  - issuer không phải `client-ca`
  - thiếu hoặc lệch chain fingerprints
- Nếu Thái chọn hướng thêm Bank admin gRPC methods, cần bảo đảm proto/server/client không làm mất metadata trong `Ticket_V` và verify response từ CA.
- Nếu Thái chọn hướng Gateway query trực tiếp Bank Postgres cho demo, phần hiển thị vẫn phải ghi rõ đây là dữ liệu vận hành Bank, không phải dữ liệu CA trust đầy đủ.

Checklist nên thêm vào phần của Thái:

- Admin Bank audit table hiển thị được `cert_serial` và reason reject.
- Không seed hoặc giả lập transaction với cert thiếu issuer/chain metadata nếu testcase cần đi qua KDC/Bank thật.
- Negative test Bank nên có case cert bị revoke hoặc cert metadata không hợp lệ.
- Response Admin Bank nên giữ format dễ nối với Audit tab: action, user_id, cert_serial, request_id, reason, metadata, created_at.

### Ảnh hưởng đến Quang

Phần của Quang là demo end-to-end, Docker Compose, seed/test data và testcase list. Đây là phần chịu ảnh hưởng mạnh nhất về thứ tự chạy, cert path và dữ liệu seed.

Thứ tự setup đúng sau layered CA:

1. Provision CA để tạo Root CA, gRPC Transport CA và Client CA.
2. Chạy script gen-certs để tạo service TLS cert cho CA/KDC/Bank bằng gRPC Transport CA.
3. Copy hoặc mount `grpc-ca.crt` đúng chỗ cho Gateway/KDC/Bank.
4. Start CA, KDC, Bank, Gateway, Frontend.
5. Tạo user certificate qua registration flow thật nếu cần demo KDC/Bank.

Các điểm cần kiểm tra trong compose/env:

- CA Service có đường dẫn Root CA và Client CA signer đúng.
- Gateway/KDC/Bank dùng `grpc-ca.crt` dạng trust bundle để verify internal gRPC TLS.
- KDC/Bank service TLS cert được ký bởi gRPC Transport CA.
- Không còn service nào trỏ về `ca-server-ca.crt`.
- Không dùng lại output `grpc-ca.*` self-signed cũ trong `scripts/gen-certs/out`.

Smoke test nên bổ sung:

- Admin CA list trả được các loại cert nếu đã có dữ liệu: `root_ca`, `intermediate_ca`, `service_tls`, `client`.
- Detail client cert có `issuer_id=client-ca` và có `chain_fingerprints`.
- Revoke client cert active thành công.
- Revoke non-client cert trả `422 CERT_TYPE_NOT_REVOKABLE`.
- Cert đã revoke không dùng được cho AS/TGS/Bank flow.
- KDC/Bank vẫn gọi CA `VerifyCertificate` và reject cert thiếu metadata chain.

Rủi ro demo nếu không cập nhật:

- Compose chạy port thành công nhưng gRPC TLS fail vì mount sai trust bundle.
- Seed trực tiếp CA DB làm cert thiếu issuer/chain metadata, khiến KDC/Bank reject.
- Smoke script gọi nhầm route cũ `/v1/admin/ca/*` trong khi code hiện dùng `/v1/admin-ca/*`.
- Demo revoke nhầm Root/Intermediate/service TLS cert và gây hiểu sai về CA lifecycle.

### Ảnh hưởng đến Thuận

Phần của Thuận là audit log còn thiếu. Layered CA làm audit cần giàu metadata hơn để giải thích được vì sao một certificate được chấp nhận hoặc bị từ chối.

Các điểm cần cập nhật:

- CA audit read API nên expose/filter được:
  - `action`
  - `serial_number`
  - `cert_type`
  - `issuer_id`
  - `performed_by`
  - time range
  - `limit`
  - `offset`
- CA audit nên ghi rõ context khi detail/revoke:
  - `looked_up`
  - `revoked`
  - revoke bị chặn do non-client cert nếu nhóm muốn audit cả attempt thất bại
- Bank audit nên giữ metadata đủ để debug cert rejection:
  - status revoked/expired
  - cert type mismatch
  - issuer mismatch
  - chain fingerprints missing/mismatch
- `performed_by` hiện đã truyền được qua proto cho Admin CA detail/revoke.
- `X-Request-ID` đang được Gateway truyền xuống CA gRPC metadata; nếu muốn lưu request id vào CA audit, CA handler/service cần đọc metadata hoặc bổ sung proto field rõ ràng.

Testcase audit nên bổ sung:

- Đăng ký user mới -> CA audit có issue event với `cert_type=client`, `issuer_id=client-ca`.
- Mở detail client cert -> CA audit có `looked_up`.
- Revoke client cert -> CA audit có `revoked`.
- Revoke Root CA, Intermediate CA hoặc service TLS -> trả 422 và không đổi trạng thái cert.
- Login/Bank flow với cert revoked -> có reject event phù hợp.
- Bank reject do cert metadata sai hoặc thiếu chain -> audit có reason đủ rõ.

Điểm phối hợp với Thanh:

- Khi Thuận có endpoint đọc CA audit, Thanh nối vào Audit tab trong `AdminCA.tsx`.
- Response nên thống nhất shape để frontend dùng ổn định: `{ items, total, limit, offset }`.
- Field thời gian nên dùng một dạng nhất quán, ưu tiên Unix seconds hoặc ISO string và ghi rõ trong contract.
