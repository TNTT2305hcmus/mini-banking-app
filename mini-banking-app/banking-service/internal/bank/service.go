package bank

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Clock abstracts time so callers (and tests) can inject a deterministic source.
type Clock interface {
	Now() time.Time
}

// Caller carries the authenticated identity the gRPC handler extracts during the
// AP exchange and passes into business methods for ownership checks and audit.
type Caller struct {
	ClientID  string
	CertSN    string
	Scope     string
	Nonce     string
	RequestID string
}

// Service owns the bank business logic. It orchestrates Repository calls and
// transaction boundaries; the gRPC handler stays thin and delegates here.
type Service struct {
	repo  *Repository
	clock Clock
}

func NewService(db *sql.DB, clk Clock) *Service {
	return &Service{repo: NewRepository(db), clock: clk}
}

type CreateUserInput struct {
	UserID       string // owner_id của cert
	SubjectEmail string
	FullName     string
}

type CreateUserResult struct {
	UserID    string
	Status    string
	CreatedAt time.Time
}

// CreateUser creates a user (keyed by the certificate owner id) together with a
// default account, in a single transaction.
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (CreateUserResult, error) {
	if s.repo == nil || s.repo.db == nil {
		return CreateUserResult{}, ErrNotConfigured
	}

	userID := strings.TrimSpace(in.UserID)
	email := strings.TrimSpace(in.SubjectEmail)
	fullName := strings.TrimSpace(in.FullName)
	if userID == "" || email == "" || fullName == "" {
		return CreateUserResult{}, ErrInvalidInput
	}

	tx, err := s.repo.BeginTx(ctx, nil)
	if err != nil {
		return CreateUserResult{}, fmt.Errorf("begin create user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := s.clock.Now()
	if err := s.repo.InsertUser(ctx, tx, userID, email, fullName, now); err != nil {
		return CreateUserResult{}, ErrUserExists
	}

	accountID := newUUID()
	accountNumber := accountNumberFromUser(userID)
	if err := s.repo.InsertDefaultAccount(ctx, tx, accountID, userID, accountNumber, now); err != nil {
		return CreateUserResult{}, fmt.Errorf("create default account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CreateUserResult{}, fmt.Errorf("commit create user: %w", err)
	}
	return CreateUserResult{UserID: userID, Status: "active", CreatedAt: now}, nil
}

// --- Balance ---------------------------------------------------------------

type BalanceResult struct {
	AccountNumber string
	Balance       int64
	Currency      string
	Status        string
}

func (s *Service) GetBalance(ctx context.Context, caller Caller, accountID string) (BalanceResult, error) {
	acc, err := s.repo.GetAccountBalance(ctx, s.repo.db, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return BalanceResult{}, ErrAccountNotFound
	}
	if err != nil {
		return BalanceResult{}, fmt.Errorf("query balance: %w", err)
	}
	if acc.OwnerID != caller.ClientID {
		s.Audit(ctx, AuditEvent{Action: "forbidden_ownership", UserID: caller.ClientID, AccountID: accountID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: "balance_account_owner_mismatch"})
		return BalanceResult{}, ErrForbidden
	}
	return BalanceResult{AccountNumber: acc.AccountNumber, Balance: acc.Balance, Currency: acc.Currency, Status: acc.Status}, nil
}

// --- History ---------------------------------------------------------------

type TransactionRecord struct {
	TransactionID     string
	FromAccountNumber string
	ToAccountNumber   string
	Amount            int64
	Currency          string
	Status            string
	Description       string
	Scope             string
	CreatedAtUnix     int64
	CompletedAtUnix   int64
}

type HistoryResult struct {
	Records []TransactionRecord
	Total   int32
	Limit   int
	Offset  int
}

func (s *Service) GetHistory(ctx context.Context, caller Caller, accountID string, limit, offset int) (HistoryResult, error) {
	ownerID, err := s.repo.GetAccountOwner(ctx, s.repo.db, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryResult{}, ErrAccountNotFound
	}
	if err != nil {
		return HistoryResult{}, fmt.Errorf("query account owner: %w", err)
	}
	if ownerID != caller.ClientID {
		s.Audit(ctx, AuditEvent{Action: "forbidden_ownership", UserID: caller.ClientID, AccountID: accountID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: "history_account_owner_mismatch"})
		return HistoryResult{}, ErrForbidden
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	total, err := s.repo.CountHistory(ctx, s.repo.db, accountID)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("count history: %w", err)
	}
	records, err := s.repo.ListHistory(ctx, s.repo.db, accountID, limit, offset)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("query history: %w", err)
	}
	return HistoryResult{Records: records, Total: total, Limit: limit, Offset: offset}, nil
}

// --- Transfer --------------------------------------------------------------

// TransferInput is the decrypted, signature-verified transfer the handler hands
// to the service. Canonical/Signature are carried through for the hash-chain
// ledger record.
type TransferInput struct {
	FromAccountID  string
	ToAccountID    string
	Amount         int64
	Currency       string
	Description    string
	IdempotencyKey string
	Canonical      []byte
	Signature      []byte
}

// Transfer applies an idempotent money transfer and returns the transaction id.
// If the idempotency key already maps to a completed transaction, that id is
// returned without re-executing.
func (s *Service) Transfer(ctx context.Context, caller Caller, in TransferInput) (string, error) {
	txID, err := s.findIdempotentTransaction(ctx, in.IdempotencyKey)
	if err != nil {
		s.Audit(ctx, AuditEvent{Action: "transfer_rejected", UserID: caller.ClientID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: err.Error()})
		return "", ErrIdempotency
	}
	if txID != "" {
		return txID, nil
	}
	return s.executeTransfer(ctx, caller, in)
}

func (s *Service) findIdempotentTransaction(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	txID, txStatus, err := s.repo.FindTransactionByIdempotencyKey(ctx, s.repo.db, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if txStatus == "completed" {
		return txID, nil
	}
	return "", fmt.Errorf("previous transaction status=%s", txStatus)
}

func (s *Service) executeTransfer(ctx context.Context, caller Caller, in TransferInput) (string, error) {
	tx, err := s.repo.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", fmt.Errorf("begin transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	reject := func(reason string) {
		s.Audit(ctx, AuditEvent{Action: "transfer_rejected", UserID: caller.ClientID, AccountID: in.FromAccountID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: reason})
	}

	from, err := s.repo.LoadAccountForUpdate(ctx, tx, in.FromAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		reject("from_account_not_found")
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query from account: %w", err)
	}
	to, err := s.repo.LoadAccountForUpdate(ctx, tx, in.ToAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		reject("to_account_not_found")
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query to account: %w", err)
	}
	if from.UserID != caller.ClientID {
		s.Audit(ctx, AuditEvent{Action: "forbidden_ownership", UserID: caller.ClientID, AccountID: in.FromAccountID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: "from_account_owner_mismatch"})
		return "", ErrForbidden
	}
	if from.UserStatus != "active" || from.Status != "active" || to.Status != "active" {
		reject("account_not_active")
		return "", ErrAccountNotActive
	}
	if from.Currency != in.Currency || to.Currency != in.Currency {
		reject("currency_mismatch")
		return "", ErrBadRequest
	}
	if from.Balance < in.Amount {
		s.Audit(ctx, AuditEvent{Action: "insufficient_funds", UserID: caller.ClientID, AccountID: in.FromAccountID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: "insufficient_funds"})
		return "", ErrInsufficientFunds
	}

	startOfDay := s.clock.Now().UTC().Truncate(24 * time.Hour)
	spentToday, err := s.repo.SumCompletedTransfersSince(ctx, tx, in.FromAccountID, startOfDay, s.clock.Now())
	if err != nil {
		return "", fmt.Errorf("query daily transfer total: %w", err)
	}
	if spentToday+in.Amount > from.Limit {
		reject("daily_limit_exceeded")
		return "", ErrDailyLimitExceeded
	}

	previousHash, err := s.repo.LockLedgerLastHash(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("lock ledger: %w", err)
	}
	if previousHash == "" {
		previousHash = "genesis"
	}
	txID := newUUID()
	createdAt := s.clock.Now()
	payloadHashBytes := sha256.Sum256(in.Canonical)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])
	signatureText := base64.StdEncoding.EncodeToString(in.Signature)
	currentHashBytes := sha256.Sum256([]byte(previousHash + payloadHash + signatureText + txID + createdAt.Format(time.RFC3339Nano)))
	currentHash := hex.EncodeToString(currentHashBytes[:])

	if err := s.repo.DebitAccount(ctx, tx, in.Amount, in.FromAccountID); err != nil {
		return "", fmt.Errorf("debit account: %w", err)
	}
	if err := s.repo.CreditAccount(ctx, tx, in.Amount, in.ToAccountID); err != nil {
		return "", fmt.Errorf("credit account: %w", err)
	}
	if err := s.repo.InsertTransaction(ctx, tx, TransactionRow{
		ID:             txID,
		FromAccountID:  in.FromAccountID,
		ToAccountID:    in.ToAccountID,
		FromNumber:     from.Number,
		ToNumber:       to.Number,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Description:    in.Description,
		PayloadHash:    payloadHash,
		Signature:      signatureText,
		CertSerial:     caller.CertSN,
		Scope:          caller.Scope,
		Nonce:          caller.Nonce,
		IdempotencyKey: in.IdempotencyKey,
		PreviousHash:   previousHash,
		CurrentHash:    currentHash,
		CreatedAt:      createdAt,
	}); err != nil {
		reject("transaction_insert_failed")
		return "", fmt.Errorf("insert transaction: %w", err)
	}
	if err := s.repo.UpdateLedger(ctx, tx, currentHash, txID, createdAt); err != nil {
		return "", fmt.Errorf("update ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transfer: %w", err)
	}
	s.Audit(ctx, AuditEvent{Action: "transfer_completed", UserID: caller.ClientID, AccountID: in.FromAccountID, TransactionID: txID, CertSerial: caller.CertSN, RequestID: caller.RequestID, Reason: "ok"})
	return txID, nil
}

func newUUID() string {
	var b [16]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func accountNumberFromUser(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	n := int64(sum[0])<<40 | int64(sum[1])<<32 | int64(sum[2])<<24 | int64(sum[3])<<16 | int64(sum[4])<<8 | int64(sum[5])
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("10%010d", n%10000000000)
}
