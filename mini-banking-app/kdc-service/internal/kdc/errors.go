package kdc

import "fmt"

/**
 * @description Stable business error code returned by the KDC domain layer.
 */
type ErrorCode string

/**
 * @description Stable KDC error codes mapped to authentication and authorization failure cases.
 */
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

/**
 * @description Domain error wrapper that preserves a stable code and the original cause.
 */
type KDCError struct {
	Code ErrorCode
	Err  error
}

/**
 * @description Formats the KDC error code and optional wrapped error.
 * @returns {string} Human-readable error string.
 */
func (e *KDCError) Error() string {
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

/**
 * @description Returns the wrapped low-level error for errors.Is/errors.As support.
 * @returns {error} Wrapped error cause.
 */
func (e *KDCError) Unwrap() error {
	return e.Err
}

/**
 * @description Creates a KDC domain error with a stable code and optional cause.
 * @param {ErrorCode} code - Domain error code.
 * @param {error} err - Optional wrapped cause.
 * @returns {error} KDC domain error.
 */
func kdcError(code ErrorCode, err error) error {
	return &KDCError{Code: code, Err: err}
}

/**
 * @description Extracts a stable KDC error code from an error value.
 * @param {error} err - Error returned by the KDC service.
 * @returns {ErrorCode} Matching domain code, empty string for nil, or INTERNAL_ERROR for unknown errors.
 */
func ErrorCodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	if e, ok := err.(*KDCError); ok {
		return e.Code
	}
	return ErrInternal
}
