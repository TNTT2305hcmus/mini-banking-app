# Testcase Index

Đây là file index cho bộ testcase final. Testcase chi tiết được tách theo nhóm để dễ chạy rehearsal và cập nhật kết quả.

## 1. Quy ước trạng thái

| Trạng thái | Ý nghĩa |
|---|---|
| `PASS` | Đã chạy trên stack runtime thật và kết quả đúng expected. |
| `FAIL` | Đã chạy trên stack runtime thật và kết quả sai expected. |
| `SKIP` | Chủ động bỏ qua, có lý do rõ. |
| `RUNTIME PENDING` | Chưa chạy runtime thật, chưa được claim pass. |
| `CODE PASS / RUNTIME PENDING` | Code path/route đã có bằng chứng tĩnh, nhưng chưa chạy stack thật. |
| `PENDING_MANUAL_DB` | Cần thao tác DB có kiểm soát, chỉ chạy trên môi trường disposable. |

Nguyên tắc: không ghi runtime `PASS` nếu chưa chạy Docker/terminal stack thật.

## 2. File testcase

| File | Mục đích |
|---|---|
| `functional-testcases.md` | Test chức năng: register, login, balance/history, transfer, Admin CA, Admin Bank, SOC. |
| `security-testcases.md` | Test bảo mật/non-functional: PKI, cert auth, AS/TGS/AP, replay, revoked cert, rollback, hash-chain. |
| `smoke-testcases.md` | Bộ test ngắn trước khi quay, bám theo `scripts/demo/smoke-test.ps1` và `.sh`. |
| `audit-testcases.md` | Test audit chi tiết CA/KDC/Bank/SOC, timeline, verify, summary, export. |
| `runtime-results.md` | Bảng điền kết quả rehearsal cuối: môi trường, commit, pass/fail, bằng chứng. |

## 3. Thứ tự chạy đề xuất

1. Chạy smoke:
   - `scripts/demo/smoke-test.ps1` trên Windows;
   - hoặc `scripts/demo/smoke-test.sh` trên Linux/macOS/Git Bash.
2. Chạy functional testcase theo UI.
3. Chạy security testcase theo UI/curl.
4. Chạy audit testcase và export bằng chứng.
5. Cập nhật `runtime-results.md`.

## 4. Checklist tổng

| Nhóm | File | Trạng thái hiện tại | Ghi chú |
|---|---|---|---|
| Smoke | `smoke-testcases.md` | RUNTIME PENDING | Script đã cập nhật; chờ stack thật. |
| Functional | `functional-testcases.md` | RUNTIME PENDING | Cần browser/OTP/cert/PIN. |
| Security | `security-testcases.md` | RUNTIME PENDING | Cần negative cases có kiểm soát. |
| Audit/SOC | `audit-testcases.md` | CODE PASS / RUNTIME PENDING | Có nhiều case code evidence; chờ rehearsal. |
| Runtime result | `runtime-results.md` | PENDING | Điền sau khi chạy cuối. |

## 5. Bằng chứng cần lưu

- Smoke output terminal.
- Screenshot UI cho register/login/transfer/Admin CA/Admin Bank/SOC.
- Export audit CSV/JSON từ SOC.
- `operation_id` dùng cho timeline.
- Ghi chú commit/branch, ngày chạy, compose file, env mode.
