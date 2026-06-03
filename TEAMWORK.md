# Mini-Banking-App - Teamwork WBS

Tài liệu này tổng hợp trạng thái sau khi regenerate protobuf stubs và chia WBS tiếp theo thành 6 sprint, mỗi sprint 4 ngày.

## 1. Nguyên Tắc Phân Công

- Kế hoạch kéo dài 24 ngày, chia thành 6 Sprint, mỗi Sprint 4 ngày.
- Mỗi Sprint được chia theo output rõ ràng; mặc định capacity khả thi mỗi người là 10-12 giờ làm việc tập trung, phần còn lại dành cho daily sync, review, fix bug và demo nội bộ.
- Chia ownership theo module để tránh giẫm chân:
  - Thanh: Lead, CA Service, Web Admin, integration Admin PKI, review, release/demo.
  - Quang: KDC Service, Kerberos-like AS/TGS flow, hỗ trợ Web Customer phần auth/ticket/session.
  - Thuận: API Gateway, Bank Service, Customer registration, Web Customer, Gateway orchestration.
  - Thái: API Gateway, Bank Service, transfer/balance/history flow, Web Customer, validation/rate limit/idempotency.
- Web Admin do Thanh phụ trách chính.
- Web Customer do Quang, Thuận, Thái phối hợp; Quang tập trung auth/ticket, Thuận tập trung registration/balance/history, Thái tập trung transfer và trạng thái giao dịch.
- Mỗi task phải có output rõ ràng. Không merge code khi chưa đạt Definition of Done và chưa được review.
- Nếu bị block trên 2 giờ, báo trong daily sync và đổi sang task độc lập trong cùng Sprint.

## 4. WBS Tổng Quát

| WBS | Hạng mục | Owner chính | Kết quả mong đợi |
| --- | --- | --- | --- |
| 1.0 | Foundation & Contract baseline | Thanh | Chốt service boundary, module path, proto mapping, coding convention và backlog kỹ thuật. |
| 2.0 | CA Service | Thanh | RegisterUser, VerifyCertificate, admin list/detail/revoke, certificate metadata, audit log. |
| 3.0 | KDC Service | Quang | AS Exchange, TGS Exchange, replay protection, scoped ticket, CA VerifyCertificate integration. |
| 4.0 | API Gateway skeleton & shared middleware | Thuận | REST route groups, request-id, error envelope, validation, gRPC clients, config/env. |
| 5.0 | API Gateway orchestration | Thuận | OTP/PKI/Auth/Bank/Admin REST flows forward đúng sang CA/KDC/Bank. |
| 6.0 | Bank Service foundation | Thái | CreateUser, users/accounts store, seed/demo accounts, service config. |
| 7.0 | Bank transaction pipeline | Thái | AP Exchange, TransferMoney, idempotency, replay, authorization, audit log, ledger hash-chain. |
| 8.0 | Bank read APIs | Thuận | GetBalance, GetHistory, ownership/scope check, pagination/history contract. |
| 9.0 | Web Admin | Thanh | Admin login MVP, certificate list/search/detail/revoke, status/audit display. |
| 10.0 | Web Customer registration | Thuận | OTP verify, key generation, CSR enrollment, certificate receive/store flow. |
| 11.0 | Web Customer auth/ticket | Quang | AS_REQ, TGS_REQ, ticket/session state in memory, scope selection. |
| 12.0 | Web Customer banking | Thái | Balance, history, signed transfer, idempotency key, transfer result UX. |
| 13.0 | Integration, QA, Deploy | Thanh | End-to-end test, demo script, README/WORKFLOW update, Vercel deploy, release notes. |

## Sprint 1 - Foundation Và Contract Baseline

Sprint Goal: Tạo baseline code chạy được sau gen-proto, chốt contract module để các Sprint sau không bị nghẽn.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Chốt module boundaries, service ownership, route mount convention và sprint board. | Cấu trúc làm việc thống nhất với `blueprint/structure.md`. |
| Thanh | Sửa CA handler compile với proto mới: `owner_id`, `subject_cn`, `subject_email`. | CA gRPC handler không còn dùng field cũ `UserId`. |
| Thanh | Scaffold/stub các RPC CA còn thiếu: `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail`, `RevokeCertificate`. | CA Service có đủ method signatures theo `ca.proto`. |
| Quang | Sửa KDC enum status từ `CERT_STATUS_VALID` sang `CERT_STATUS_ACTIVE` và rà import path `mini-banking`/`mini_banking`. | KDC qua được lỗi build đầu tiên sau gen-proto. |
| Quang | Sửa KDC handler field names theo proto mới: `IdC`, `Signature`, `Tgt`, `Scope`, `ServiceId`. | KDC gRPC handler khớp `kdc.proto`. |
| Thuận | Scaffold API Gateway: route groups, gRPC clients, config/env, request-id middleware. | Gateway boot được với route skeleton và `/health`. |
| Thuận | Scaffold Customer REST routes: OTP, PKI, AS_REQ, TGS_REQ, balance/history query. | Customer route skeleton đã mount. |
| Thái | Scaffold Bank Service route/client surface trong Gateway và Bank Service contract theo `bank.proto`. | Bank route skeleton và Bank gRPC client interface rõ ràng. |
| Thái | Scaffold shared validation/error envelope/rate-limit hook cho Bank/Admin-sensitive APIs. | Gateway có middleware nền dùng lại được. |

Definition of Done:

- CA Service và KDC Service không còn lỗi compile do field/enum proto cũ.
- API Gateway có skeleton route groups và gRPC clients cho CA/KDC/Bank.
- Bank Service có contract baseline theo generated stubs.
- Các gap còn lại được ghi rõ để xử lý trong Sprint 2.

## Sprint 2 - CA Service Và KDC Service Core

Sprint Goal: Hoàn thiện CA/KDC theo blueprint để các flow PKI và Kerberos-like có nền tảng tích hợp.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Thiết kế CA repository/store theo schema `certificates` và `certificate_audit_log`. | Repository interface rõ, có thể thay JSON store bằng DB adapter. |
| Thanh | Implement lưu certificate metadata: serial, owner, subject, public key, fingerprint, validity. | RegisterUser ghi đủ metadata phục vụ Admin/KDC/Bank. |
| Thanh | Implement `VerifyCertificate` trả status, validity, public key/certificate theo flags. | KDC/Bank có một RPC chính để trust certificate. |
| Thanh | Implement admin list/detail/revoke tối thiểu và audit event. | Admin APIs dùng được qua Gateway. |
| Quang | Đổi AS pre-auth sang gọi `CA.VerifyCertificate`. | KDC không còn phụ thuộc legacy `GetCertificate` cho AS. |
| Quang | Chuẩn hóa AS_REQ: nonce, timestamp, request_id, signature verification. | AS Exchange có replay/freshness/signature check. |
| Quang | Chuẩn hóa TGS_REQ: TGT, authenticator, scope, service_id, replay check. | TGS Exchange cấp scoped service ticket. |
| Thuận | Implement Gateway `/v1/pki/register`, `/v1/auth/as-req`, `/v1/auth/tgs-req`. | Gateway forward đúng PKI/Auth flows sang CA/KDC. |
| Thái | Hỗ trợ Gateway validation cho auth binary/base64 fields, scope và error envelope. | Auth routes reject input sai schema trước khi forward. |

Definition of Done:

- `go test ./...` CA/KDC pass hoặc còn lỗi được ghi rõ và không thuộc phần core vừa làm.
- `VerifyCertificate` là RPC chính cho KDC.
- Gateway gọi được CA/KDC qua PKI/Auth routes.
- Demo nội bộ có thể chạy PKI register -> AS_REQ -> TGS_REQ ở mức smoke.

## Sprint 3 - Bank Service Và Gateway Orchestration

Sprint Goal: Hoàn thiện Bank Service MVP và nối các REST banking routes qua API Gateway.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Hỗ trợ CA revocation/verification fixtures cho Bank tests. | Bank tests có dữ liệu cert active/revoked/expired. |
| Thanh | Implement revoke compensation khi PKI enrollment thành công nhưng Bank CreateUser fail. | Enrollment flow không để cert active mồ côi. |
| Quang | Chuẩn hóa ticket payload cho Bank: `ticket_id`, `key_version`, `scope`, `service_id`, TTL. | Bank giải mã được Ticket_v theo contract ổn định. |
| Quang | Test KDC ticket với Bank service key. | Ticket_v tương thích với Bank AP Exchange. |
| Thuận | Implement Gateway orchestration: OTP/registration token -> CA RegisterUser -> Bank CreateUser. | Registration end-to-end đi qua Gateway. |
| Thuận | Implement Gateway balance/history routes và Bank read client. | `/balance/query` và `/transactions/query` forward đúng. |
| Thái | Implement Bank `CreateUser`, user/account seed/demo và account status. | Bank có user/account baseline. |
| Thái | Implement Bank AP pipeline: decrypt ticket, verify authenticator, replay cache, scope/ownership check. | Bank có auth pipeline trước transfer/read. |
| Thái | Implement `TransferMoney`, idempotency key, ACID update, audit reject và ledger hash-chain. | Transfer flow MVP có bảo vệ replay/double processing. |

Definition of Done:

- Bank Service có `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory` theo proto.
- Gateway không ghi trực tiếp CA DB/Bank DB.
- Registration, balance/history và transfer có smoke test qua Gateway.
- Transfer reject phải ghi audit với reason đủ dùng cho demo.

## Sprint 4 - Web Admin Và Web Customer

Sprint Goal: Xây UI chức năng chính cho Admin và Customer, tích hợp với Gateway thay vì mock rời rạc.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Tạo Web Admin layout, login MVP, API client và route guard. | Admin app boot được và có session demo. |
| Thanh | Implement certificate list/search/detail/revoke modal có reason. | Admin quản lý certificate qua Gateway. |
| Thanh | Hiển thị status, validity, fingerprint, revocation reason và audit metadata cơ bản. | Admin PKI dashboard đủ cho demo. |
| Quang | Implement Web Customer AS_REQ/TGS_REQ flow và ticket/session state trong RAM. | Customer app lấy được TGT và Ticket_v. |
| Quang | Hỗ trợ crypto contract cho frontend: data ký, nonce/timestamp/request_id, base64 encoding. | Frontend auth payload khớp KDC. |
| Thuận | Implement Web Customer OTP/PKI enrollment: OTP verify, key generation, CSR submit. | Customer đăng ký và nhận certificate. |
| Thuận | Implement balance/history screens và API client handling. | Customer xem số dư/lịch sử qua Gateway. |
| Thái | Implement transfer form, idempotency key, signed payload và transfer result UX. | Customer thực hiện transfer demo được. |
| Thái | Hoàn thiện frontend error states cho bank/auth failures. | UI không vỡ khi replay/revoked/forbidden/insufficient funds. |

Definition of Done:

- Web Admin chạy được flow login -> list -> detail -> revoke.
- Web Customer chạy được flow OTP/PKI -> AS/TGS -> balance/history -> transfer.
- Private key sinh ở browser và không gửi plaintext lên server.
- Frontend chỉ gọi Gateway, không gọi trực tiếp CA/KDC/Bank.

## Sprint 5 - Integration, QA Và Security Hardening

Sprint Goal: Kiểm thử toàn hệ thống, sửa lỗi bảo mật/luồng nghiệp vụ, đảm bảo các invariant trong blueprint.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Regression CA: register/verify/list/detail/revoke/audit. | CA regression checklist pass. |
| Thanh | Cross-test Web Admin với Gateway/CA và sửa lỗi UI/Admin flow. | Admin demo ổn định. |
| Quang | Regression KDC: AS/TGS/replay/expired/revoked cert/scope denied. | KDC regression checklist pass. |
| Quang | Cross-test Web Customer auth/ticket với Gateway/KDC. | Auth/ticket demo ổn định. |
| Thuận | Regression Gateway: OTP/PKI/Auth/balance/history route validation và error envelope. | Gateway regression checklist pass. |
| Thuận | Cross-test Customer registration và bank read paths. | Registration + read flow ổn định. |
| Thái | Regression Bank: CreateUser/transfer/idempotency/replay/ledger/audit. | Bank regression checklist pass. |
| Thái | Cross-test transfer end-to-end, gồm insufficient funds, forbidden ownership, revoked cert. | Transfer demo ổn định với positive/negative cases. |

Definition of Done:

- End-to-end smoke flow pass: OTP/PKI -> Bank CreateUser -> AS_REQ -> TGS_REQ -> balance/history -> transfer -> admin revoke.
- Replay, revoked certificate, wrong scope, wrong ownership và duplicate idempotency key đều có test hoặc checklist.
- Không response lỗi nào lộ private data, key material hoặc raw ticket không cần thiết.
- README/WORKFLOW/TEAMWORK được cập nhật nếu có thay đổi scope/API/security decision.

## Sprint 6 - Deploy, Demo Và Handoff

Sprint Goal: Đóng gói demo, deploy frontend lên Vercel, chuẩn bị tài liệu bàn giao và known issues.

| Thành viên | Nhiệm vụ | Output |
| --- | --- | --- |
| Thanh | Chuẩn hóa env/config cho Web Admin và Admin PKI API. | Admin deploy config checklist. |
| Thanh | Deploy Web Admin lên Vercel và kiểm tra certificate management trên preview. | Web Admin Vercel preview chạy được. |
| Thanh | Viết demo script Admin: login, list, detail, revoke. | Admin demo script. |
| Quang | Chuẩn hóa env/config cho Customer auth/KDC flow. | Auth deploy config checklist. |
| Quang | Hỗ trợ smoke test AS/TGS trên môi trường deploy. | Customer auth flow chạy được sau deploy. |
| Quang | Viết demo script Auth: AS_REQ, TGS_REQ, ticket scope. | Auth demo script. |
| Thuận | Deploy Web Customer lên Vercel, cấu hình API base URL/CORS. | Web Customer Vercel preview chạy được. |
| Thuận | Viết demo script Customer: register, balance, history. | Customer demo script. |
| Thái | Kiểm tra backend runtime config cho Gateway/Bank, CORS, env và build logs. | Deploy/runtime issue list đã xử lý. |
| Thái | Viết demo script Transfer: signed transfer, idempotency, audit. | Transfer demo script. |

Definition of Done:

- Web Admin và Web Customer deploy được lên Vercel.
- Demo script đầy đủ cho Admin, Registration, Auth, Balance/History và Transfer.
- Final release notes có known issues, cách chạy local, cách deploy và tài khoản/demo data.
- Các thành viên bàn giao module theo ownership, kèm test/checklist tương ứng.

## 2. Definition Of Done Tổng

| Area | Done khi |
| --- | --- |
| CA Service | `go test ./...` pass; `RegisterUser`, `VerifyCertificate`, `ListCertificates`, `GetCertificateDetail`, `RevokeCertificate` khớp proto; có audit event tối thiểu cho issue/verify/detail/revoke. |
| KDC Service | `go test ./...` pass; AS/TGS handler khớp proto mới; KDC chỉ lấy public key qua CA `VerifyCertificate`; replay/timestamp/scope/TTL được test. |
| Bank Service | `go test ./...` pass; `CreateUser`, `TransferMoney`, `GetBalance`, `GetHistory` khớp proto; có authorization pipeline, idempotency, audit log và ledger hash-chain. |
| API Gateway | TypeScript build pass; route groups theo blueprint có validation, request id, error envelope, rate limit/admin auth và gRPC forwarding; không ghi trực tiếp CA DB/Bank DB. |
| Web Admin | Deploy được lên Vercel; có login MVP, certificate list/search/detail/revoke; thao tác admin đi qua Gateway và có reason/audit metadata. |
| Web Customer | Deploy được lên Vercel; có OTP/PKI enrollment, AS/TGS auth, balance/history và transfer flow; private key sinh ở browser và không gửi plaintext lên server. |
| Integration | Smoke flow chạy được: OTP/PKI enrollment -> Bank CreateUser -> AS_REQ -> TGS_REQ -> balance/history -> transfer -> admin list/detail/revoke. |
| Documentation | `WORKFLOW.md`, blueprint/api docs và `TEAMWORK.md` được cập nhật khi thay đổi scope/API/security decision; có demo script và known issues. |

## 6. Rủi Ro Cần Theo Dõi

| Rủi ro | Tác động | Cách xử lý |
| --- | --- | --- |
| Import path `mini-banking` vs `mini_banking` chưa thống nhất | Build fail giữa CA/KDC/pkg | Chốt một module path trong Sprint 1, regenerate proto nếu cần. |
| CA Store đang là JSON thay vì PostgreSQL | Admin dashboard và audit không đạt blueprint | Sprint 2 chuyển sang repository/DB hoặc adapter rõ ràng. |
| KDC còn dùng legacy CA RPC | KDC/Bank không nhận đủ public key/status/validity theo contract mới | Đổi sang `VerifyCertificate` trong Sprint 2. |
| Bank Service chưa có core implementation đầy đủ | End-to-end transfer không hoàn tất | Thái + Thuận ưu tiên Bank core ngay Sprint 3, không để dồn cuối. |
| Gateway source thiếu route groups | Frontend không có REST surface để tích hợp | Thuận + Thái dựng skeleton ngay Sprint 1 và hoàn thiện trong Sprint 3. |
| WebCrypto/CSR/ticket handling ở Web Customer phức tạp | Registration/auth frontend dễ chậm | Quang hỗ trợ crypto contract, Thuận/Thái chia nhỏ enrollment và auth screens. |
| Deploy Vercel phụ thuộc env/CORS/API URL | Preview deploy chạy nhưng không gọi được backend | Chuẩn hóa env và CORS trong Sprint 6 trước khi deploy. |
| 24 ngày bao phủ cả backend/frontend/test/deploy khá chặt | Dễ thiếu thời gian hardening | Mỗi Sprint phải có demo cuối Sprint và backlog lỗi rõ ràng, tránh để integration tới Sprint 6. |
