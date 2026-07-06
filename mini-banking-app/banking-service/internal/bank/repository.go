package bank

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Querier is satisfied by both *sql.DB and *sql.Tx, so repository methods can
// run standalone or inside a caller-managed transaction (e.g. transfers that
// need a single Serializable tx with FOR UPDATE locks).
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repository owns all SQL for the bank domain. The service layer composes these
// calls (and manages transaction boundaries) to implement business operations.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// BeginTx exposes the underlying transaction so the service can run several
// repository calls atomically by passing the returned *sql.Tx as the Querier.
func (r *Repository) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, opts)
}

func (r *Repository) InsertUser(ctx context.Context, q Querier, id, email, fullName string, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO users(id, email, full_name, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'active', $4, $4)`,
		id, email, fullName, now)
	return err
}

func (r *Repository) InsertDefaultAccount(ctx context.Context, q Querier, accountID, userID, accountNumber string, now time.Time) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO accounts(id, user_id, account_number, balance, daily_transfer_limit, currency, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 50000000, 'VND', 'active', $5, $5)`,
		accountID, userID, accountNumber, int64(50000000), now)
	return err
}

func (r *Repository) InsertAudit(ctx context.Context, q Querier, e AuditEvent) error {
	metadata := []byte("{}")
	if len(e.Metadata) > 0 {
		if b, err := json.Marshal(e.Metadata); err == nil {
			metadata = b
		}
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO bank_audit_log(action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata)
		 VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8)`,
		e.Action, e.UserID, e.AccountID, e.TransactionID, e.CertSerial, e.RequestID, e.Reason, string(metadata))
	return err
}

// AuditRecord is a stored bank_audit_log row returned to the admin read API.
// Nullable columns come back as empty strings; Metadata is the raw JSONB text.
type AuditRecord struct {
	ID            string
	Action        string
	UserID        string
	AccountID     string
	TransactionID string
	CertSerial    string
	RequestID     string
	Reason        string
	MetadataJSON  string
	CreatedAt     time.Time
}

// AuditListFilter selects audit rows for the admin read API. Zero values mean
// "no filter"; From/To bound created_at as a half-open range [From, To).
type AuditListFilter struct {
	Action     string
	UserID     string
	CertSerial string
	RequestID  string
	From       time.Time
	To         time.Time
	Limit      int
	Offset     int
}

func (r *Repository) ListAudit(ctx context.Context, q Querier, f AuditListFilter) ([]AuditRecord, int, error) {
	where := ` WHERE ($1 = '' OR action = $1)
	   AND ($2 = '' OR user_id = NULLIF($2, '')::uuid)
	   AND ($3 = '' OR cert_serial = $3)
	   AND ($4 = '' OR request_id = $4)
	   AND ($5::timestamptz IS NULL OR created_at >= $5)
	   AND ($6::timestamptz IS NULL OR created_at < $6)`
	args := []any{f.Action, f.UserID, f.CertSerial, f.RequestID, nullableTime(f.From), nullableTime(f.To)}

	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM bank_audit_log`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := q.QueryContext(ctx,
		`SELECT id::text, action, COALESCE(user_id::text, ''), COALESCE(account_id::text, ''),
		        COALESCE(transaction_id::text, ''), COALESCE(cert_serial, ''), COALESCE(request_id, ''),
		        COALESCE(reason, ''), COALESCE(metadata::text, '{}'), created_at
		 FROM bank_audit_log`+where+`
		 ORDER BY created_at DESC
		 LIMIT $7 OFFSET $8`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []AuditRecord
	for rows.Next() {
		var rec AuditRecord
		if err := rows.Scan(&rec.ID, &rec.Action, &rec.UserID, &rec.AccountID, &rec.TransactionID,
			&rec.CertSerial, &rec.RequestID, &rec.Reason, &rec.MetadataJSON, &rec.CreatedAt); err != nil {
			return nil, 0, err
		}
		rec.CreatedAt = rec.CreatedAt.UTC()
		records = append(records, rec)
	}
	return records, total, rows.Err()
}

func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

// AccountBalance is the projection backing GetBalance, including the owner id so
// the service can enforce ownership.
type AccountBalance struct {
	OwnerID       string
	AccountNumber string
	Balance       int64
	Currency      string
	Status        string
}

func (r *Repository) GetAccountBalance(ctx context.Context, q Querier, accountID string) (AccountBalance, error) {
	var a AccountBalance
	err := q.QueryRowContext(ctx,
		`SELECT user_id::text, account_number, balance, currency, status
		 FROM accounts WHERE id = $1`, accountID).
		Scan(&a.OwnerID, &a.AccountNumber, &a.Balance, &a.Currency, &a.Status)
	return a, err
}

// AccountProfile gộp thông tin user + tài khoản cho luồng xem thông tin cá nhân (/auth/me).
type AccountProfile struct {
	OwnerID       string
	FullName      string
	Email         string
	UserStatus    string
	AccountID     string
	AccountNumber string
	Balance       int64
	Limit         int64
	Currency      string
	AccountStatus string
}

const profileSelect = `SELECT u.id::text, u.full_name, u.email, u.status,
        a.id::text, a.account_number, a.balance, a.daily_transfer_limit, a.currency, a.status
 FROM accounts a JOIN users u ON u.id = a.user_id`

func scanProfile(row *sql.Row) (AccountProfile, error) {
	var p AccountProfile
	err := row.Scan(&p.OwnerID, &p.FullName, &p.Email, &p.UserStatus,
		&p.AccountID, &p.AccountNumber, &p.Balance, &p.Limit, &p.Currency, &p.AccountStatus)
	return p, err
}

// GetAccountProfileByID lấy profile theo account id (dùng khi caller chỉ định account_id).
func (r *Repository) GetAccountProfileByID(ctx context.Context, q Querier, accountID string) (AccountProfile, error) {
	return scanProfile(q.QueryRowContext(ctx, profileSelect+` WHERE a.id = $1`, accountID))
}

// GetDefaultAccountProfileByUser lấy profile của tài khoản đầu tiên của user (account_id rỗng → /auth/me).
func (r *Repository) GetDefaultAccountProfileByUser(ctx context.Context, q Querier, userID string) (AccountProfile, error) {
	return scanProfile(q.QueryRowContext(ctx, profileSelect+` WHERE u.id = $1 ORDER BY a.created_at ASC LIMIT 1`, userID))
}

func (r *Repository) GetAccountOwner(ctx context.Context, q Querier, accountID string) (string, error) {
	var ownerID string
	err := q.QueryRowContext(ctx, `SELECT user_id::text FROM accounts WHERE id = $1`, accountID).Scan(&ownerID)
	return ownerID, err
}

func (r *Repository) CountHistory(ctx context.Context, q Querier, accountID string) (int32, error) {
	var total int32
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions WHERE from_account_id = $1 OR to_account_id = $1`,
		accountID).Scan(&total)
	return total, err
}

func (r *Repository) ListHistory(ctx context.Context, q Querier, accountID string, limit, offset int) ([]TransactionRecord, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id::text, COALESCE(from_account_number, ''), COALESCE(to_account_number, ''),
		        amount, currency, status, COALESCE(description, ''), scope,
		        EXTRACT(EPOCH FROM created_at)::bigint,
		        COALESCE(EXTRACT(EPOCH FROM completed_at)::bigint, 0)
		 FROM transactions
		 WHERE from_account_id = $1 OR to_account_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TransactionRecord
	for rows.Next() {
		var rec TransactionRecord
		if err := rows.Scan(&rec.TransactionID, &rec.FromAccountNumber, &rec.ToAccountNumber,
			&rec.Amount, &rec.Currency, &rec.Status, &rec.Description, &rec.Scope,
			&rec.CreatedAtUnix, &rec.CompletedAtUnix); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// FindTransactionByIdempotencyKey returns the transaction id and status for a
// given idempotency key, or sql.ErrNoRows if none exists.
func (r *Repository) FindTransactionByIdempotencyKey(ctx context.Context, q Querier, key string) (txID, txStatus string, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT id::text, status FROM transactions WHERE idempotency_key = $1`, key).
		Scan(&txID, &txStatus)
	return txID, txStatus, err
}

// AccountRecord is the row locked during a transfer (account + owner status).
type AccountRecord struct {
	ID         string
	UserID     string
	UserStatus string
	Number     string
	Balance    int64
	Limit      int64
	Currency   string
	Status     string
}

func (r *Repository) LoadAccountForUpdate(ctx context.Context, q Querier, accountID string) (AccountRecord, error) {
	var a AccountRecord
	err := q.QueryRowContext(ctx,
		`SELECT a.id::text, a.user_id::text, u.status, a.account_number, a.balance,
		        a.daily_transfer_limit, a.currency, a.status
		 FROM accounts a JOIN users u ON u.id = a.user_id
		 WHERE a.id = $1
		 FOR UPDATE`, accountID).
		Scan(&a.ID, &a.UserID, &a.UserStatus, &a.Number, &a.Balance, &a.Limit, &a.Currency, &a.Status)
	return a, err
}

// LoadAccountForUpdateByNumber khóa hàng tài khoản theo số tài khoản (account_number).
// Dùng prefix SELECT giống LoadAccountForUpdate để mọi logic downstream (và test) không đổi.
func (r *Repository) LoadAccountForUpdateByNumber(ctx context.Context, q Querier, accountNumber string) (AccountRecord, error) {
	var a AccountRecord
	err := q.QueryRowContext(ctx,
		`SELECT a.id::text, a.user_id::text, u.status, a.account_number, a.balance,
		        a.daily_transfer_limit, a.currency, a.status
		 FROM accounts a JOIN users u ON u.id = a.user_id
		 WHERE a.account_number = $1
		 FOR UPDATE`, accountNumber).
		Scan(&a.ID, &a.UserID, &a.UserStatus, &a.Number, &a.Balance, &a.Limit, &a.Currency, &a.Status)
	return a, err
}

func (r *Repository) SumCompletedTransfersSince(ctx context.Context, q Querier, fromAccountID string, since, until time.Time) (int64, error) {
	var spent int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM transactions
		 WHERE from_account_id = $1 AND status = 'completed' AND created_at >= $2 AND created_at <= $3`,
		fromAccountID, since, until).Scan(&spent)
	return spent, err
}

func (r *Repository) LockLedgerLastHash(ctx context.Context, q Querier) (string, error) {
	var previousHash string
	err := q.QueryRowContext(ctx, `SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE`).Scan(&previousHash)
	return previousHash, err
}

func (r *Repository) DebitAccount(ctx context.Context, q Querier, amount int64, accountID string) error {
	_, err := q.ExecContext(ctx, `UPDATE accounts SET balance = balance - $1 WHERE id = $2`, amount, accountID)
	return err
}

func (r *Repository) CreditAccount(ctx context.Context, q Querier, amount int64, accountID string) error {
	_, err := q.ExecContext(ctx, `UPDATE accounts SET balance = balance + $1 WHERE id = $2`, amount, accountID)
	return err
}

// TransactionRow is the completed transfer record persisted to the ledger.
type TransactionRow struct {
	ID             string
	FromAccountID  string
	ToAccountID    string
	FromNumber     string
	ToNumber       string
	Amount         int64
	Currency       string
	Description    string
	PayloadHash    string
	Signature      string
	CertSerial     string
	Scope          string
	Nonce          string
	IdempotencyKey string
	PreviousHash   string
	CurrentHash    string
	CreatedAt      time.Time
}

func (r *Repository) InsertTransaction(ctx context.Context, q Querier, t TransactionRow) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO transactions(
		    id, from_account_id, to_account_id, from_account_number, to_account_number,
		    amount, currency, status, description, payload_hash, client_signature,
		    cert_serial, scope, nonce, idempotency_key, previous_hash, current_hash,
		    created_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $17)`,
		t.ID, t.FromAccountID, t.ToAccountID, t.FromNumber, t.ToNumber,
		t.Amount, t.Currency, t.Description, t.PayloadHash, t.Signature,
		t.CertSerial, t.Scope, t.Nonce, t.IdempotencyKey,
		t.PreviousHash, t.CurrentHash, t.CreatedAt)
	return err
}

func (r *Repository) UpdateLedger(ctx context.Context, q Querier, currentHash, txID string, at time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE ledger_state SET last_hash = $1, last_transaction_id = $2, updated_at = $3 WHERE id = 'main'`,
		currentHash, txID, at)
	return err
}
