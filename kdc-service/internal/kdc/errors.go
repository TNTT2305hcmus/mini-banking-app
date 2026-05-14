package kdc

import "fmt"

type ErrorCode string

const (
	ErrAuthInvalid      ErrorCode = "AUTH_INVALID"
	ErrTGTExpired       ErrorCode = "TGT_EXPIRED"
	ErrRequestExpired   ErrorCode = "REQUEST_EXPIRED"
	ErrIdentityMismatch ErrorCode = "IDENTITY_MISMATCH"
	ErrCertNotFound     ErrorCode = "CERT_NOT_FOUND"
	ErrCertRevoked      ErrorCode = "CERT_REVOKED"
	ErrCertExpired      ErrorCode = "CERT_EXPIRED"
	ErrReplayDetected   ErrorCode = "REPLAY_DETECTED"
	ErrScopeDenied      ErrorCode = "SCOPE_DENIED"
	ErrInvalidScope     ErrorCode = "INVALID_SCOPE"
	ErrServiceUnknown   ErrorCode = "SERVICE_UNKNOWN"
	ErrInternal         ErrorCode = "INTERNAL_ERROR"
)

type KDCError struct {
	Code ErrorCode
	Err  error
}

func (e *KDCError) Error() string {
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *KDCError) Unwrap() error {
	return e.Err
}

func kdcError(code ErrorCode, err error) error {
	return &KDCError{Code: code, Err: err}
}

func ErrorCodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	if e, ok := err.(*KDCError); ok {
		return e.Code
	}
	return ErrInternal
}
