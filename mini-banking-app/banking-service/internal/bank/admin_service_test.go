package bank

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type adminTestClock struct{ now time.Time }

func (c adminTestClock) Now() time.Time { return c.now }

func TestListAdminUsersReturnsEmptyListAndNormalizesPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users u")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT u.id::text, u.email, u.full_name, u.status,")).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "full_name", "status", "account_count", "total_balance", "created_at",
		}))

	service := NewService(db, adminTestClock{now: time.Now().UTC()})
	result, err := service.ListAdminUsers(context.Background(), AdminUsersFilter{Limit: 0, Offset: -5})
	if err != nil {
		t.Fatalf("ListAdminUsers() error = %v", err)
	}
	if result.Total != 0 || len(result.Users) != 0 || result.Limit != 20 || result.Offset != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCheckUserEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	service := NewService(db, adminTestClock{now: time.Now().UTC()})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, status FROM users WHERE lower(email) = lower($1) LIMIT 1`)).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).
			AddRow("11111111-1111-4111-8111-111111111111", "active"))

	found, err := service.CheckUserEmail(context.Background(), " alice@example.com ")
	if err != nil {
		t.Fatalf("CheckUserEmail() error = %v", err)
	}
	if !found.Exists || found.UserID != "11111111-1111-4111-8111-111111111111" || found.Status != "active" {
		t.Fatalf("unexpected found result: %+v", found)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, status FROM users WHERE lower(email) = lower($1) LIMIT 1`)).
		WithArgs("new@example.com").
		WillReturnError(sql.ErrNoRows)

	missing, err := service.CheckUserEmail(context.Background(), "new@example.com")
	if err != nil {
		t.Fatalf("CheckUserEmail() missing error = %v", err)
	}
	if missing.Exists {
		t.Fatalf("missing result Exists = true, want false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestAdminFiltersRejectInvalidValuesBeforeQuery(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	service := NewService(db, adminTestClock{now: time.Now().UTC()})

	_, err = service.ListAdminTransactions(context.Background(), AdminTransactionsFilter{FromUnix: 20, ToUnix: 10})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("invalid date range error = %v, want ErrBadRequest", err)
	}
	_, err = service.ListAdminAuditEvents(context.Background(), AdminAuditFilter{Action: "not_allowed"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("invalid audit action error = %v, want ErrBadRequest", err)
	}
	_, err = service.ListAdminUserAccounts(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("invalid user id error = %v, want ErrBadRequest", err)
	}
}
