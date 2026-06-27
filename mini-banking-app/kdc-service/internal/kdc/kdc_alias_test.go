package kdc_test

import "kdc-service/internal/kdc"

const aes256KeySize = 32

type ASConfig = kdc.ASConfig
type ASRepPayload = kdc.ASRepPayload
type ASResponse = kdc.ASResponse
type ASService = kdc.ASService
type AuthenticatorPlaintext = kdc.AuthenticatorPlaintext
type Certificate = kdc.Certificate
type CertificateStatus = kdc.CertificateStatus
type Config = kdc.Config
type ErrorCode = kdc.ErrorCode
type KDCKeys = kdc.KDCKeys
type ServiceTicketPlaintext = kdc.ServiceTicketPlaintext
type StaticScopeAuthorizer = kdc.StaticScopeAuthorizer
type TGSReplyPlaintext = kdc.TGSReplyPlaintext
type TGSRequest = kdc.TGSRequest
type TGSResponse = kdc.TGSResponse
type TGSService = kdc.TGSService
type TGT = kdc.TGT
type TGTPlaintext = kdc.TGTPlaintext

var ErrCertificateMissing = kdc.ErrCertificateMissing

const (
	CertificateValid   = kdc.CertificateValid
	CertificateActive  = kdc.CertificateActive
	CertificateRevoked = kdc.CertificateRevoked
	CertificateExpired = kdc.CertificateExpired

	ErrAuthInvalid      = kdc.ErrAuthInvalid
	ErrCertExpired      = kdc.ErrCertExpired
	ErrCertNotFound     = kdc.ErrCertNotFound
	ErrCertRevoked      = kdc.ErrCertRevoked
	ErrIdentityMismatch = kdc.ErrIdentityMismatch
	ErrInternal         = kdc.ErrInternal
	ErrReplayDetected   = kdc.ErrReplayDetected
	ErrRequestExpired   = kdc.ErrRequestExpired
	ErrScopeDenied      = kdc.ErrScopeDenied
	ErrServiceUnknown   = kdc.ErrServiceUnknown
	ErrTGTExpired       = kdc.ErrTGTExpired
)

var ErrorCodeOf = kdc.ErrorCodeOf
var NewTGSService = kdc.NewTGSService
