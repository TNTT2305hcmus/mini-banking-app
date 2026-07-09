package bank

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (r *Repository) GetAdminOverview(ctx context.Context) (AdminOverview, error) {
	var result AdminOverview
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE status = 'active'),
			(SELECT COUNT(*) FROM accounts),
			(SELECT COALESCE(SUM(balance), 0)::bigint FROM accounts),
			(SELECT COUNT(*) FROM transactions),
			(SELECT COUNT(*) FROM transactions WHERE status = 'completed'),
			(SELECT COUNT(*) FROM transactions WHERE status = 'failed'),
			(SELECT COUNT(*) FROM bank_audit_log WHERE created_at >= NOW() - INTERVAL '24 hours')
	`).Scan(
		&result.TotalUsers,
		&result.ActiveUsers,
		&result.TotalAccounts,
		&result.TotalBalance,
		&result.TotalTransactions,
		&result.CompletedTransactions,
		&result.FailedTransactions,
		&result.AuditEvents24h,
	)
	return result, err
}

func adminUsersWhere(filter AdminUsersFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.Email != "" {
		args = append(args, "%"+filter.Email+"%")
		clauses = append(clauses, fmt.Sprintf("u.email ILIKE $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("u.status = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) CountAdminUsers(ctx context.Context, filter AdminUsersFilter) (int64, error) {
	where, args := adminUsersWhere(filter)
	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users u"+where, args...).Scan(&total)
	return total, err
}

func (r *Repository) ListAdminUsers(ctx context.Context, filter AdminUsersFilter) ([]AdminUser, error) {
	where, args := adminUsersWhere(filter)
	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT u.id::text, u.email, u.full_name, u.status,
		       COUNT(a.id)::bigint, COALESCE(SUM(a.balance), 0)::bigint,
		       u.created_at
		FROM users u
		LEFT JOIN accounts a ON a.user_id = u.id` + where + `
		GROUP BY u.id, u.email, u.full_name, u.status, u.created_at
		ORDER BY u.created_at DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]AdminUser, 0)
	for rows.Next() {
		var user AdminUser
		if err := rows.Scan(&user.UserID, &user.Email, &user.FullName, &user.Status,
			&user.AccountCount, &user.TotalBalance, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *Repository) ListAdminUserAccounts(ctx context.Context, userID string) ([]AdminAccount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, account_number, balance, currency, status, created_at, daily_transfer_limit
		FROM accounts
		WHERE user_id = $1::uuid
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]AdminAccount, 0)
	for rows.Next() {
		var account AdminAccount
		if err := rows.Scan(&account.AccountID, &account.AccountNumber, &account.Balance,
			&account.Currency, &account.Status, &account.CreatedAt, &account.DailyTransferLimit); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func adminTransactionsWhere(filter AdminTransactionsFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.AccountID != "" {
		args = append(args, filter.AccountID)
		position := len(args)
		clauses = append(clauses, fmt.Sprintf("(t.from_account_id = $%d::uuid OR t.to_account_id = $%d::uuid)", position, position))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("t.status = $%d", len(args)))
	}
	if filter.FromUnix > 0 {
		args = append(args, time.Unix(filter.FromUnix, 0).UTC())
		clauses = append(clauses, fmt.Sprintf("t.created_at >= $%d", len(args)))
	}
	if filter.ToUnix > 0 {
		args = append(args, time.Unix(filter.ToUnix, 0).UTC())
		clauses = append(clauses, fmt.Sprintf("t.created_at <= $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) CountAdminTransactions(ctx context.Context, filter AdminTransactionsFilter) (int64, error) {
	where, args := adminTransactionsWhere(filter)
	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM transactions t"+where, args...).Scan(&total)
	return total, err
}

func (r *Repository) ListAdminTransactions(ctx context.Context, filter AdminTransactionsFilter) ([]AdminTransaction, error) {
	where, args := adminTransactionsWhere(filter)
	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT t.id::text, COALESCE(t.from_account_number, ''),
		       COALESCE(t.to_account_number, ''), t.amount, t.currency, t.status,
		       COALESCE(t.description, ''), t.cert_serial,
		       COALESCE(t.current_hash, ''), t.created_at
		FROM transactions t` + where + `
		ORDER BY t.created_at DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := make([]AdminTransaction, 0)
	for rows.Next() {
		var transaction AdminTransaction
		if err := rows.Scan(&transaction.TransactionID, &transaction.FromAccountNumber,
			&transaction.ToAccountNumber, &transaction.Amount, &transaction.Currency,
			&transaction.Status, &transaction.Description, &transaction.CertSerial,
			&transaction.CurrentHash, &transaction.CreatedAt); err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	return transactions, rows.Err()
}

func adminAuditWhere(filter AdminAuditFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.Action != "" {
		args = append(args, filter.Action)
		clauses = append(clauses, fmt.Sprintf("l.action = $%d", len(args)))
	}
	if filter.UserID != "" {
		args = append(args, filter.UserID)
		clauses = append(clauses, fmt.Sprintf("l.user_id = $%d::uuid", len(args)))
	}
	if filter.CertSerial != "" {
		args = append(args, filter.CertSerial)
		clauses = append(clauses, fmt.Sprintf("l.cert_serial = $%d", len(args)))
	}
	if filter.RequestID != "" {
		args = append(args, filter.RequestID)
		clauses = append(clauses, fmt.Sprintf("l.request_id = $%d", len(args)))
	}
	if filter.FromUnix > 0 {
		args = append(args, time.Unix(filter.FromUnix, 0).UTC())
		clauses = append(clauses, fmt.Sprintf("l.created_at >= $%d", len(args)))
	}
	if filter.ToUnix > 0 {
		args = append(args, time.Unix(filter.ToUnix, 0).UTC())
		clauses = append(clauses, fmt.Sprintf("l.created_at <= $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) CountAdminAuditEvents(ctx context.Context, filter AdminAuditFilter) (int64, error) {
	where, args := adminAuditWhere(filter)
	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bank_audit_log l"+where, args...).Scan(&total)
	return total, err
}

func (r *Repository) ListAdminAuditEvents(ctx context.Context, filter AdminAuditFilter) ([]AdminAuditEvent, error) {
	where, args := adminAuditWhere(filter)
	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT l.id::text, l.action, COALESCE(l.user_id::text, ''),
		       COALESCE(l.account_id::text, ''), COALESCE(l.transaction_id::text, ''),
		       COALESCE(l.cert_serial, ''), COALESCE(l.request_id, ''),
		       COALESCE(l.reason, ''), COALESCE(l.metadata, '{}'::jsonb)::text,
		       l.created_at
		FROM bank_audit_log l` + where + `
		ORDER BY l.created_at DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]AdminAuditEvent, 0)
	for rows.Next() {
		var event AdminAuditEvent
		if err := rows.Scan(&event.EventID, &event.Action, &event.UserID, &event.AccountID,
			&event.TransactionID, &event.CertSerial, &event.RequestID, &event.Reason,
			&event.MetadataJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
