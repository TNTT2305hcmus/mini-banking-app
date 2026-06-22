# Lộ trình refactor `banking-service`

Mục tiêu: tách `internal/grpc/handler.go` (đang ~880 dòng, trộn 3 mối quan tâm)
thành các tầng rõ ràng, đồng nhất với `ca-service` và `kdc-service`.

## Nguyên tắc phân tầng

| Tầng | Vị trí | Trách nhiệm |
|------|--------|-------------|
| Transport + Security (AP exchange) | `internal/grpc/` | gRPC, giải mã ticket/authenticator, verify chữ ký, chống replay, mã hoá AP_REP |
| Business (nghiệp vụ) | `internal/bank/service.go` | CreateUser, Transfer, GetBalance, GetHistory — validate, transaction boundary, quy tắc nghiệp vụ |
| Data access (repository) | `internal/bank/repository.go` | Toàn bộ câu SQL (gồm `InsertAudit`) |
| Errors | `internal/bank/errors.go` | Sentinel errors |
| Audit | `internal/bank/audit.go` | `AuditEvent` + `Service.Audit` (tiện ích cắt ngang, dùng chung handler + service) |

> Ghi chú quyết định: 4 nghiệp vụ **để chung `service.go`** (cùng một miền, sát
> convention `ca-service`). Chỉ tách `repository.go` (tầng data-access khác hẳn),
> `errors.go`, và `audit.go` (cross-cutting). Không tách `transfer.go`/`query.go`
> để tránh chia vụn. Nếu sau này `service.go` > ~500 dòng mới cân nhắc tách thêm.

**Ranh giới mấu chốt:** crypto/auth (Kerberos AP exchange) **ở lại handler**;
nghiệp vụ + SQL + audit **chuyển sang package `bank`**. Handler chỉ: xác thực →
gọi `bank.Service` → map kết quả/lỗi sang protobuf/status.

## Ràng buộc bắt buộc giữ nguyên

1. **gRPC status code không đổi.** `api-gateway/src/middleware/errorHandler.ts`
   map lỗi *chỉ theo status code*; message được relay nguyên văn. Vì vậy:
   - Sentinel error trong `bank` đặt message = đúng code công khai cũ
     (`NOT_FOUND`, `FORBIDDEN`, `INSUFFICIENT_FUNDS`, `ACCOUNT_NOT_ACTIVE`,
     `BAD_REQUEST`, `DAILY_LIMIT_EXCEEDED`, `IDEMPOTENCY_FAILED`).
   - `toStatusError` map mỗi sentinel về đúng code cũ.
2. **Transaction nguyên tử.** `Transfer` phải giữ 1 transaction `Serializable`
   với `FOR UPDATE`. Repository method nhận `Querier` (thoả cả `*sql.DB` lẫn
   `*sql.Tx`) để service kiểm soát ranh giới tx, không vỡ khoá.
3. **Hành vi audit không đổi.** Mọi sự kiện audit hiện có vẫn được ghi.

## Trạng thái

- [x] **Giai đoạn 0 — CreateUser** (đã xong)
  - `service.go`: `Service`, `Clock`, `CreateUser`, helper `newUUID` / `accountNumberFromUser`.
  - `repository.go`: `Querier`, `Repository`, `BeginTx`, `InsertUser`, `InsertDefaultAccount`.
  - `errors.go`: `ErrNotConfigured`, `ErrInvalidInput`, `ErrUserExists`.
  - Handler `CreateUser` mỏng + `toStatusError`.

- [x] **Giai đoạn 1 — Errors + Audit (nền tảng dùng chung)**
  - `errors.go`: thêm sentinel cho transfer/balance/history (xem mục Ràng buộc #1).
  - `audit.go`: chuyển `auditEvent` → `bank.AuditEvent`; thêm `Service.Audit(ctx, AuditEvent)`.
  - `repository.go`: thêm `InsertAudit(ctx, Querier, AuditEvent)`.
  - Handler: thay mọi `h.audit(...)` (trong `authorize`, `markReplay`) bằng `h.bank.Audit(...)`;
    xoá method `audit` và type `auditEvent` khỏi handler.

- [x] **Giai đoạn 2 — GetBalance + GetHistory** (trong `service.go`)
  - `repository.go`: `GetAccountBalance`, `GetAccountOwner`, `CountHistory`, `ListHistory`.
  - `query.go`: `Service.GetBalance(caller, accountID)`, `Service.GetHistory(caller, accountID, limit, offset)`;
    kiểm tra ownership + audit `forbidden_ownership`; chuẩn hoá phân trang.
  - Kiểu trả về thuần Go: `BalanceResult`, `HistoryResult`, `TransactionRecord` (status dạng string).
  - Handler: gọi service, map `string` → pb enum (`accountStatus`, `transactionStatus`) — giữ ở handler.

- [x] **Giai đoạn 3 — TransferMoney** (trong `service.go`)
  - `repository.go`: `FindTransactionByIdempotencyKey`, `LoadAccountForUpdate`,
    `SumCompletedTransfersSince`, `LockLedgerLastHash`, `UpdateAccountBalance`,
    `InsertTransaction`, `UpdateLedger`.
  - `transfer.go`: `Service.Transfer(caller, TransferInput)` gồm idempotency +
    `executeTransfer` (tx Serializable, kiểm tra số dư/hạn mức/active/currency,
    tính hash-chain ledger, audit). Trả `txID`.
  - Handler `TransferMoney`: giữ giải mã payload + verify chữ ký + replay; sau đó
    gọi `h.bank.Transfer(...)`, mã hoá AP_REP.

- [x] **Giai đoạn 4 — Dọn dẹp handler**
  - Xoá khỏi handler: `executeTransfer`, `findIdempotentTransaction`, `newUUID`,
    `rollbackUnlessCommitted` (không còn dùng).
  - Giữ ở handler: `authorize`, `decryptTicket`, `decryptTransferPayload`,
    `markReplay`, `verifyRSAPSS`, `encryptAPRep`, helper crypto/AES,
    `firstString/firstUnix/firstBytes`, `absDuration`,
    `accountStatus/transactionStatus`.

## Hợp đồng dữ liệu giữa handler và service

```go
// Thông tin caller đã xác thực, handler dựng sau AP exchange rồi truyền vào service.
type Caller struct { ClientID, CertSN, Scope, Nonce, RequestID string }

type TransferInput struct {
    FromAccountID, ToAccountID string
    Amount                     int64
    Currency, Description      string
    IdempotencyKey             string
    Canonical, Signature       []byte   // để verify đã xong + ghi hash-chain
}
```

## Kiểm thử mỗi giai đoạn

Sau mỗi giai đoạn chạy:

```
go build ./...
go vet ./internal/...
go test ./internal/grpc/ ./internal/bank/
```

Test gRPC hiện có (`internal/grpc/handler_test.go`) phải tiếp tục pass —
đây là lưới an toàn xác nhận hành vi không đổi.

## Kết quả kỳ vọng

| File | Ước lượng |
|------|-----------|
| `internal/grpc/handler.go` | ~450 dòng (chỉ transport + crypto/auth) |
| `internal/bank/service.go` | ~360 dòng (4 nghiệp vụ) |
| `internal/bank/repository.go` | ~230 dòng |
| `internal/bank/audit.go` | ~30 dòng |
| `internal/bank/errors.go` | ~25 dòng |

Không file nào phình to; line count phân bổ đều theo tầng.
