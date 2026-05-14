package kdc

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrCertificateMissing = errors.New("certificate not found")

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type ReplayStore interface {
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
}

type CertificateRepository interface {
	GetCertificate(ctx context.Context, certSN string) (Certificate, error)
}

type ScopeAuthorizer interface {
	Allowed(ctx context.Context, clientID string, serviceID string, scope string) (bool, error)
}

type CertificateStatus string

const (
	CertificateValid   CertificateStatus = "VALID"
	CertificateActive  CertificateStatus = "ACTIVE"
	CertificateRevoked CertificateStatus = "REVOKED"
	CertificateExpired CertificateStatus = "EXPIRED"
)

type Certificate struct {
	Serial       string
	SubjectCN    string
	PublicKeyPEM string
	Status       CertificateStatus
	NotAfter     time.Time
}

type Service struct {
	tgsKey          []byte
	serviceKeys     map[string][]byte
	replayStore     ReplayStore
	certRepo        CertificateRepository
	scopeAuthorizer ScopeAuthorizer
	clock           Clock
	rand            io.Reader
	ticketTTL       time.Duration
	timestampWindow time.Duration
	replayTTL       time.Duration
}

type Config struct {
	TGSKey          []byte
	ServiceKeys     map[string][]byte
	ReplayStore     ReplayStore
	CertRepo        CertificateRepository
	ScopeAuthorizer ScopeAuthorizer
	Clock           Clock
	Random          io.Reader
	TicketTTL       time.Duration
	TimestampWindow time.Duration
	ReplayTTL       time.Duration
}

type TGSRequest struct {
	ServiceID      string
	TGTCiphertext  []byte
	Authenticator  []byte
	CertSN         string
	Nonce2         []byte
	RequestedScope string
}

type TGSResponse struct {
	EncryptedPayload []byte
	TicketExpiryUnix int64
}

type TGTPlaintext struct {
	ClientID  string `json:"client_id"`
	KCTGS     []byte `json:"k_c_tgs"`
	IssuedAt  int64  `json:"issued_at,omitempty"`
	Expiry    int64  `json:"tgt_expiry,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	ClientIP  string `json:"client_ip,omitempty"`
}

type AuthenticatorPlaintext struct {
	ClientID         string `json:"client_id"`
	Timestamp        int64  `json:"ts_3"`
	NonceReq         string `json:"nonce_req"`
	RequestedService string `json:"requested_service"`
	Scope            string `json:"scope"`
}

type ServiceTicketPlaintext struct {
	ClientID  string `json:"client_id"`
	ServiceID string `json:"service_id"`
	SName     string `json:"sname"`
	KCV       []byte `json:"k_c_v"`
	PublicKey string `json:"pub_c"`
	PubCPEM   string `json:"pub_c_pem"`
	CertSN    string `json:"cert_sn"`
	Scope     string `json:"scope"`
	NonceReq  string `json:"nonce_req"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type TGSReplyPlaintext struct {
	KCV       []byte `json:"k_c_v"`
	ServiceID string `json:"id_v"`
	TicketV   []byte `json:"ticket_v"`
	Nonce2    []byte `json:"nonce2"`
	NonceReq  string `json:"nonce_req"`
	IssuedAt  int64  `json:"ts_4"`
	ExpiresAt int64  `json:"expires_at"`
	Scope     string `json:"scope"`
}

type StaticScopeAuthorizer map[string]map[string]bool

func (a StaticScopeAuthorizer) Allowed(_ context.Context, _ string, serviceID string, scope string) (bool, error) {
	if scope == "" {
		return false, nil
	}
	scopes, ok := a[serviceID]
	if !ok {
		return false, nil
	}
	return scopes[scope], nil
}
