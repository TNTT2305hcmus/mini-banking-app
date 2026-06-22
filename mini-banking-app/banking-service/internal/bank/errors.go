package bank

import "errors"

// Sentinel errors returned by the business layer. The gRPC handler maps each to
// a status code (see internal/grpc/handler.go:toStatusError). Their messages are
// the terse public codes the API gateway relays verbatim to clients, so do not
// reword them without checking the gateway contract.
var (
	ErrNotConfigured = errors.New("database is not configured")
	ErrInvalidInput  = errors.New("user_id, subject_email and full_name are required")
	ErrUserExists    = errors.New("user already exists")

	ErrAccountNotFound    = errors.New("NOT_FOUND")
	ErrForbidden          = errors.New("FORBIDDEN")
	ErrAccountNotActive   = errors.New("ACCOUNT_NOT_ACTIVE")
	ErrBadRequest         = errors.New("BAD_REQUEST")
	ErrInsufficientFunds  = errors.New("INSUFFICIENT_FUNDS")
	ErrDailyLimitExceeded = errors.New("DAILY_LIMIT_EXCEEDED")
	ErrIdempotency        = errors.New("IDEMPOTENCY_FAILED")
)
