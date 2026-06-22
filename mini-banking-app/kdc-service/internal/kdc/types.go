package kdc

import (
	"context"
	"errors"
	"io"
	"time"
)

/**
 * @description Sentinel error returned when a certificate serial number is not found.
 */
var ErrCertificateMissing = errors.New("certificate not found")

/**
 * @description Provides the current time so production code and tests can share the same time boundary.
 */
type Clock interface {
	Now() time.Time
}

/**
 * @description Clock implementation backed by the system UTC time.
 */
type SystemClock struct{}

/**
 * @description Returns the current system time in UTC.
 * @returns {time.Time} Current UTC timestamp.
 */
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

/**
 * @description Stores replay markers with Redis-like SET NX semantics.
 */
type ReplayStore interface {
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
}

/**
 * @description Loads certificate metadata used by the KDC validation flow.
 */
type CertificateRepository interface {
	GetCertificate(ctx context.Context, certSN string) (Certificate, error)
}

/**
 * @description Checks whether a client may request a specific service scope.
 */
type ScopeAuthorizer interface {
	Allowed(ctx context.Context, clientID string, serviceID string, scope string) (bool, error)
}

/**
 * @description Represents the certificate lifecycle state returned by the certificate repository.
 */
type CertificateStatus string

/**
 * @description Supported certificate status values accepted or rejected by the KDC.
 */
const (
	CertificateValid   CertificateStatus = "VALID"
	CertificateActive  CertificateStatus = "ACTIVE"
	CertificateRevoked CertificateStatus = "REVOKED"
	CertificateExpired CertificateStatus = "EXPIRED"
)

/**
 * @description Certificate data required by the TGS exchange.
 */
type Certificate struct {
	Serial       string
	OwnerID      string
	SubjectCN    string
	PublicKeyPEM string
	Status       CertificateStatus
	NotBefore    time.Time
	NotAfter     time.Time
}

/**
 * @description Service contains the core logic for the TGS Exchanges
 */

type TGSService struct {
	// --- TGS-Exchange Fields ---
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

/**
 * @description Service contains the core logic for the AS Exchanges.
 * @note AS shares certRepo/replayStore/clock/rand with the TGS service; kdcKeys
 * (the KDC RSA private key + K_tgs) is the only AS-exclusive dependency.
 */
type ASService struct {
	// --- Shared collaborators (same abstractions used by the TGS service) ---
	certRepo        CertificateRepository
	replayStore     ReplayStore
	clock           Clock
	rand            io.Reader
	timestampWindow time.Duration
	// --- AS-Exchange-only fields ---
	kdcKeys *KDCKeys
	tgtTTL  time.Duration
}

/**
 * @description Input configuration used to build an AS service instance.
 */
type ASConfig struct {
	CertRepo        CertificateRepository
	ReplayStore     ReplayStore
	Clock           Clock
	Random          io.Reader
	Keys            *KDCKeys
	TGTTTL          time.Duration
	TimestampWindow time.Duration
}

/**
 * @description Input configuration used to build a KDC service instance.
 */
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

/**
 * @description Client request for a service ticket during the TGS exchange.
 */
type TGSRequest struct {
	ServiceID      string
	TGTCiphertext  []byte
	Authenticator  []byte
	CertSN         string
	Nonce2         []byte
	RequestedScope string
}

/**
 * @description KDC response containing the encrypted TGS reply and ticket expiry metadata.
 */
type TGSResponse struct {
	EncryptedPayload []byte
	TicketExpiryUnix int64
}

/**
 * @description TGT issued by AS. JSON tags are aligned with TGTPlaintext so TGS can decrypt it directly.
 */
type TGT struct {
	ClientId   string `json:"client_id"`
	CertSN     string `json:"cert_sn,omitempty"`
	SessionKey []byte `json:"k_c_tgs"`
	IssuedAt   int64  `json:"issued_at,omitempty"`
	Expiry     int64  `json:"tgt_expiry,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	ClientIP   string `json:"client_ip,omitempty"`
}

/**
 * @description Plaintext content inside a TGT after decrypting it with the TGS master key.
 */
type TGTPlaintext struct {
	ClientID  string `json:"client_id"`
	CertSN    string `json:"cert_sn,omitempty"`
	KCTGS     []byte `json:"k_c_tgs"`
	IssuedAt  int64  `json:"issued_at,omitempty"`
	Expiry    int64  `json:"tgt_expiry,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	ClientIP  string `json:"client_ip,omitempty"`
}

/**
 * @description AS_REP payload sent from AS to the client.
 */
type ASRepPayload struct {
	SessionKey []byte `json:"k_c_tgs"`
	TGT        []byte `json:"tgt"`
	Nonce1     []byte `json:"nonce_1"`
}

/**
 * @description AS_REP response to client
 */
type ASResponse struct {
	KDCSignature     []byte `json:"kdc_signature"`
	EncryptedKey     []byte `json:"encrypted_key"`
	EncryptedPayload []byte `json:"encrypted_payload"`
}

/**
 * @description Plaintext client authenticator encrypted with K_c_tgs.
 */
type AuthenticatorPlaintext struct {
	ClientID         string `json:"client_id"`
	IdC              string `json:"id_c,omitempty"`
	Timestamp        int64  `json:"ts_3"`
	TimestampUnix    int64  `json:"timestamp,omitempty"`
	NonceReq         string `json:"nonce_req"`
	Nonce            string `json:"nonce,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	RequestedService string `json:"requested_service"`
	ServiceID        string `json:"service_id,omitempty"`
	Scope            string `json:"scope"`
}

/**
 * @description Plaintext service ticket encrypted for the target service.
 */
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

/**
 * @description Plaintext TGS reply encrypted back to the client with K_c_tgs.
 */
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

/**
 * @description In-memory scope allowlist keyed by service ID and scope name.
 */
type StaticScopeAuthorizer map[string]map[string]bool

/**
 * @description Returns whether the requested scope is enabled for the target service.
 * @param {context.Context} _ - Request context, unused by this static implementation.
 * @param {string} _ - Client ID, unused by this static implementation.
 * @param {string} serviceID - Target service ID.
 * @param {string} scope - Requested permission scope.
 * @returns {bool} True when the scope is allowed.
 * @returns {error} Always nil for this implementation.
 */
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
