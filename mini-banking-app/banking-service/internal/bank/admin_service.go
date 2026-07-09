package bank

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

var bankAuditActions = map[string]bool{
	"transfer_completed":   true,
	"transfer_rejected":    true,
	"replay_detected":      true,
	"invalid_signature":    true,
	"certificate_rejected": true,
	"forbidden_ownership":  true,
	"insufficient_funds":   true,
}

func normalizeAdminPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func validateAdminDateRange(fromUnix, toUnix int64) error {
	if fromUnix < 0 || toUnix < 0 || (fromUnix > 0 && toUnix > 0 && fromUnix > toUnix) {
		return ErrBadRequest
	}
	return nil
}

func (s *Service) GetAdminOverview(ctx context.Context) (AdminOverview, error) {
	if s.repo == nil || s.repo.db == nil {
		return AdminOverview{}, ErrNotConfigured
	}
	result, err := s.repo.GetAdminOverview(ctx)
	if err != nil {
		return AdminOverview{}, fmt.Errorf("query admin overview: %w", err)
	}
	return result, nil
}

func (s *Service) ListAdminUsers(ctx context.Context, filter AdminUsersFilter) (AdminUsersResult, error) {
	if s.repo == nil || s.repo.db == nil {
		return AdminUsersResult{}, ErrNotConfigured
	}
	filter.Email = strings.TrimSpace(strings.ToLower(filter.Email))
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	if filter.Status != "" && filter.Status != "active" && filter.Status != "locked" {
		return AdminUsersResult{}, ErrBadRequest
	}
	filter.Limit, filter.Offset = normalizeAdminPagination(filter.Limit, filter.Offset)
	total, err := s.repo.CountAdminUsers(ctx, filter)
	if err != nil {
		return AdminUsersResult{}, fmt.Errorf("count admin users: %w", err)
	}
	users, err := s.repo.ListAdminUsers(ctx, filter)
	if err != nil {
		return AdminUsersResult{}, fmt.Errorf("list admin users: %w", err)
	}
	return AdminUsersResult{Users: users, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) ListAdminUserAccounts(ctx context.Context, userID string) ([]AdminAccount, error) {
	if s.repo == nil || s.repo.db == nil {
		return nil, ErrNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if !uuidPattern.MatchString(userID) {
		return nil, ErrBadRequest
	}
	accounts, err := s.repo.ListAdminUserAccounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list admin user accounts: %w", err)
	}

	startOfDay := s.clock.Now().UTC().Truncate(24 * time.Hour)
	for i := range accounts {
		spentToday, err := s.repo.SumCompletedTransfersSince(ctx, s.repo.db, accounts[i].AccountID, startOfDay, s.clock.Now())
		if err != nil {
			spentToday = 0
		}
		// Ghép thông tin hạn mức và đã dùng vào Currency: "VND|limit|used"
		accounts[i].Currency = fmt.Sprintf("%s|%d|%d", accounts[i].Currency, accounts[i].DailyTransferLimit, spentToday)
	}

	return accounts, nil
}

func (s *Service) ListAdminTransactions(ctx context.Context, filter AdminTransactionsFilter) (AdminTransactionsResult, error) {
	if s.repo == nil || s.repo.db == nil {
		return AdminTransactionsResult{}, ErrNotConfigured
	}
	filter.AccountID = strings.TrimSpace(filter.AccountID)
	filter.Status = strings.TrimSpace(strings.ToLower(filter.Status))
	if filter.AccountID != "" && !uuidPattern.MatchString(filter.AccountID) {
		return AdminTransactionsResult{}, ErrBadRequest
	}
	if filter.Status != "" && filter.Status != "pending" && filter.Status != "completed" && filter.Status != "failed" {
		return AdminTransactionsResult{}, ErrBadRequest
	}
	if err := validateAdminDateRange(filter.FromUnix, filter.ToUnix); err != nil {
		return AdminTransactionsResult{}, err
	}
	filter.Limit, filter.Offset = normalizeAdminPagination(filter.Limit, filter.Offset)
	total, err := s.repo.CountAdminTransactions(ctx, filter)
	if err != nil {
		return AdminTransactionsResult{}, fmt.Errorf("count admin transactions: %w", err)
	}
	transactions, err := s.repo.ListAdminTransactions(ctx, filter)
	if err != nil {
		return AdminTransactionsResult{}, fmt.Errorf("list admin transactions: %w", err)
	}
	return AdminTransactionsResult{Transactions: transactions, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) ListAdminAuditEvents(ctx context.Context, filter AdminAuditFilter) (AdminAuditResult, error) {
	if s.repo == nil || s.repo.db == nil {
		return AdminAuditResult{}, ErrNotConfigured
	}
	filter.Action = strings.TrimSpace(strings.ToLower(filter.Action))
	filter.UserID = strings.TrimSpace(filter.UserID)
	filter.CertSerial = strings.TrimSpace(filter.CertSerial)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	if filter.Action != "" && !bankAuditActions[filter.Action] {
		return AdminAuditResult{}, ErrBadRequest
	}
	if filter.UserID != "" && !uuidPattern.MatchString(filter.UserID) {
		return AdminAuditResult{}, ErrBadRequest
	}
	if err := validateAdminDateRange(filter.FromUnix, filter.ToUnix); err != nil {
		return AdminAuditResult{}, err
	}
	filter.Limit, filter.Offset = normalizeAdminPagination(filter.Limit, filter.Offset)
	total, err := s.repo.CountAdminAuditEvents(ctx, filter)
	if err != nil {
		return AdminAuditResult{}, fmt.Errorf("count admin audit events: %w", err)
	}
	events, err := s.repo.ListAdminAuditEvents(ctx, filter)
	if err != nil {
		return AdminAuditResult{}, fmt.Errorf("list admin audit events: %w", err)
	}
	return AdminAuditResult{Events: events, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}
