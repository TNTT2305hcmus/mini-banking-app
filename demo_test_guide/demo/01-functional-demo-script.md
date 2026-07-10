# Functional Demo Script

Mục tiêu: chứng minh hệ thống dùng được từ góc nhìn customer, Admin CA, Admin Bank và Admin SOC.

Thời lượng gợi ý: 8-12 phút.

## 0. Mở đầu

Nói ngắn:

```text
Đây là Mini Banking App có PKI registration, Kerberos-like AS/TGS/AP, banking transfer, Admin CA, Admin Bank và SOC audit dashboard.
Phần này demo chức năng chính trước; phần bảo mật/cryptography sẽ tách riêng ở non-functional demo.
```

Hiển thị nhanh:

- `docker compose -f docker-compose.local.yml ps`
- Frontend `http://localhost:5173`
- API Gateway `http://localhost:3000`

Expected evidence:

- Containers running/healthy.
- Frontend mở được.

## 1. Customer registration

Route:

```text
http://localhost:5173/register
```

Thao tác:

1. Nhập email customer demo.
2. Request OTP.
3. Nhập OTP.
4. Đặt PIN.
5. Browser sinh keypair/CSR.
6. Submit register.

Nói khi quay:

```text
Ở bước đăng ký, private key được sinh trong browser. Server chỉ nhận CSR và registration token sau OTP. Khi đăng ký thành công, CA cấp certificate role customer và Banking Service tạo user/account tương ứng.
```

Expected:

- UI báo đăng ký thành công.
- Customer cert được lưu local.
- Không có private key plaintext rời browser.

Evidence cần lưu:

- Email/owner.
- Cert serial nếu UI có hiển thị.
- `operation_id` nếu có log/UI.

## 2. Customer login

Route:

```text
http://localhost:5173/login
```

Thao tác:

1. Chọn/nhập PIN.
2. Login bằng cert.
3. Vào home.

Nói khi quay:

```text
Login dùng certificate và private key local. Sau khi chứng minh quyền sở hữu cert, flow lấy TGT từ AS và service ticket từ TGS để truy cập banking service.
```

Expected:

- Login thành công.
- Home hiển thị profile/balance.

## 3. Balance and history

Route:

```text
http://localhost:5173/home
```

Thao tác:

1. Mở balance.
2. Mở transaction history.
3. Chỉ ra daily used/remaining nếu UI có.

Nói khi quay:

```text
Balance và history là các service request riêng, dùng service ticket phù hợp. UI cũng hiển thị giới hạn còn lại để tránh người dùng submit transfer chắc chắn fail.
```

Expected:

- Balance đúng.
- History có dữ liệu.
- Không có dữ liệu protocol nhạy cảm như signature trong UI.

## 4. Transfer success

Thao tác:

1. Chọn receiver hợp lệ.
2. Nhập amount hợp lệ.
3. Submit transfer.
4. Xem kết quả.
5. Refresh/quan sát balance và history.

Nói khi quay:

```text
Transfer thành công tạo transaction, cập nhật balance atomic và sinh Bank audit event transfer_completed.
```

Expected:

- Transfer success.
- Balance sender giảm.
- History có transaction mới.

Evidence:

- Transaction id nếu UI hiển thị.
- Amount.
- Thời điểm.

## 5. Transfer fail

Chọn một case dễ quay:

- Amount vượt balance.
- Amount vượt daily remaining.
- Receiver/account invalid nếu UI hỗ trợ.

Thao tác:

1. Nhập amount không hợp lệ.
2. Submit hoặc quan sát UI chặn/cảnh báo.
3. Xác nhận balance/history không hiện success giả.

Nói khi quay:

```text
Case fail không được hiển thị như thành công. UI refresh lại balance/history để tránh trạng thái cũ gây hiểu nhầm.
```

Expected:

- Có error rõ.
- Balance không đổi.
- History không thêm transaction success.

## 6. Admin Bank dashboard

Route:

```text
http://localhost:5173/admin-bank
```

Thao tác:

1. Login bằng Bank Admin cert/PIN.
2. Mở overview.
3. Mở users/accounts.
4. Mở transactions.
5. Mở audit/security tab.

Nói khi quay:

```text
Admin Bank dùng cert-based session riêng. Dashboard cho thấy dữ liệu vận hành: user, account, transaction và audit của domain Bank.
```

Expected:

- Có `bank_admin_session`.
- Overview có số liệu.
- Transactions có transfer vừa thực hiện hoặc seed data.
- Audit có Bank events.

## 7. Admin CA certificate management

Route:

```text
http://localhost:5173/admin-ca
```

Thao tác:

1. Login bằng CA Admin cert/PIN.
2. Mở certificate list.
3. Mở detail một cert.
4. Revoke cert phụ với reason.
5. Mở audit/log nếu UI có tab audit.

Nói khi quay:

```text
Admin CA là cert-based với role ca_admin. Admin có thể xem certificate, xem detail và revoke cert. Không dùng password/static-token cũ làm đường chính.
```

Expected:

- List cert OK.
- Detail có metadata.
- Revoke cert phụ thành công.
- Audit có `looked_up` và `revoked`.

Lưu ý:

- Không revoke cert chính đang dùng cho các bước sau.

## 8. Admin SOC dashboard

Route:

```text
http://localhost:5173/admin-soc
```

Thao tác:

1. Login SOC/security-admin.
2. Mở KDC audit list.
3. Mở timeline theo `operation_id`.
4. Bấm verify.
5. Xem summary.
6. Export CSV/JSON.

Nói khi quay:

```text
SOC là màn hình cross-service security monitoring. Nó đọc KDC audit, nối timeline theo operation_id, verify hash-chain và export evidence cho báo cáo.
```

Expected:

- KDC audit có AS/TGS event nếu KDC `DATABASE_URL` đã bật.
- Timeline có CA/KDC và Bank nếu có cookie Bank Admin.
- Verify không báo tamper trên DB sạch.
- Export tải được file.

## 9. Kết thúc functional demo

Nói ngắn:

```text
Phần functional đã chứng minh user có thể đăng ký, đăng nhập, xem tài khoản, chuyển tiền; Admin Bank/CA/SOC đều có bề mặt quản trị riêng. Phần tiếp theo sẽ tập trung vào cơ chế bảo mật: PKI, AS/TGS/AP, replay protection và audit hash-chain.
```

Checklist sau khi quay:

- Ghi lại testcase đã pass vào `runtime-results.md`.
- Lưu screenshot hoặc video timestamp.
- Lưu SOC export nếu đã tải.
