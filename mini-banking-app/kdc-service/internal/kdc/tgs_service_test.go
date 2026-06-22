package kdc_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

/**
 * @description Test clock that always returns a fixed timestamp.
 */
type fixedClock struct {
	now time.Time
}

/**
 * @description Returns the fixed test timestamp.
 * @returns {time.Time} Fixed UTC timestamp.
 */
func (c fixedClock) Now() time.Time {
	return c.now
}

/**
 * @description In-memory replay store used to verify SET NX behavior in tests.
 */
type memoryReplayStore struct {
	keys    map[string]time.Time
	calls   int
	lastTTL time.Duration
}

/**
 * @description Creates an empty in-memory replay store for tests.
 * @returns {*memoryReplayStore} Initialized replay store.
 */
func newMemoryReplayStore() *memoryReplayStore {
	return &memoryReplayStore{keys: map[string]time.Time{}}
}

/**
 * @description Stores a replay marker only when the key does not already exist.
 * @param {context.Context} _ - Request context, unused by this test store.
 * @param {string} key - Replay cache key.
 * @param {string} _ - Stored value, unused by this test store.
 * @param {time.Duration} ttl - Replay marker TTL.
 * @returns {bool} True when the key was newly inserted.
 * @returns {error} Always nil for this test store.
 */
func (s *memoryReplayStore) SetNX(_ context.Context, key string, _ string, ttl time.Duration) (bool, error) {
	s.calls++
	s.lastTTL = ttl
	if _, ok := s.keys[key]; ok {
		return false, nil
	}
	s.keys[key] = time.Now().Add(ttl)
	return true, nil
}

/**
 * @description In-memory certificate repository used by KDC service tests.
 */
type memoryCertRepo struct {
	certs map[string]Certificate
}

/**
 * @description Looks up a certificate by serial number in the test map.
 * @param {context.Context} _ - Request context, unused by this test repository.
 * @param {string} certSN - Certificate serial number.
 * @returns {Certificate} Matching test certificate.
 * @returns {error} ErrCertificateMissing when the serial number is absent.
 */
func (r memoryCertRepo) GetCertificate(_ context.Context, certSN string) (Certificate, error) {
	cert, ok := r.certs[certSN]
	if !ok {
		return Certificate{}, ErrCertificateMissing
	}
	return cert, nil
}

/**
 * @description Verifies that a valid TGT decrypts and exposes the expected K_c_tgs.
 * @param {*testing.T} t - Test handle.
 */
func TestDecryptTGTSuccessExtractsKCTGS(t *testing.T) {
	h := newHarness(t)

	tgtCipher := h.mustTGT(t, "alice", h.kctgs, h.now.Add(30*time.Minute))
	tgt, err := decryptJSON[TGTPlaintext](h.tgsKey, tgtCipher)
	if err != nil {
		t.Fatalf("decryptTGT() error = %v", err)
	}
	if tgt.ClientID != "alice" {
		t.Fatalf("client id = %q, want alice", tgt.ClientID)
	}
	if !bytes.Equal(tgt.KCTGS, h.kctgs) {
		t.Fatalf("K_c_tgs mismatch")
	}
}

/**
 * @description Verifies that a valid TGS request returns a usable encrypted reply.
 * @param {*testing.T} t - Test handle.
 */
func TestValidAuthenticatorIssuesTicket(t *testing.T) {
	h := newHarness(t)
	resp, err := h.request(t, "alice", "bank-service", "transfer:internal", h.now)
	if err != nil {
		t.Fatalf("RequestServiceTicket() error = %v", err)
	}
	reply := h.decryptReply(t, resp.EncryptedPayload)
	if !bytes.Equal(reply.Nonce2, h.nonce2) {
		t.Fatalf("nonce2 was not echoed")
	}
	if reply.Scope != "transfer:internal" {
		t.Fatalf("scope = %q", reply.Scope)
	}
	if reply.ServiceID != "bank-service" {
		t.Fatalf("id_v = %q", reply.ServiceID)
	}
	if reply.IssuedAt != h.now.Unix() {
		t.Fatalf("ts_4 = %d, want %d", reply.IssuedAt, h.now.Unix())
	}
	if reply.NonceReq != h.nonceReq {
		t.Fatalf("nonce_req = %q", reply.NonceReq)
	}
	if len(reply.KCV) != aes256KeySize {
		t.Fatalf("K_c_v length = %d", len(reply.KCV))
	}
}

func TestStandardAuthenticatorFieldsIssueTicket(t *testing.T) {
	h := newHarness(t)
	auth := AuthenticatorPlaintext{
		IdC:           "alice",
		TimestampUnix: h.now.Unix(),
		Nonce:         h.nonceReq,
		RequestID:     h.requestID,
		ServiceID:     "bank-service",
		Scope:         "transfer:internal",
	}
	req := TGSRequest{
		ServiceID:      "bank-service",
		TGTCiphertext:  h.mustTGT(t, "alice", h.kctgs, h.now.Add(30*time.Minute)),
		Authenticator:  h.mustEncrypt(t, h.kctgs, auth),
		RequestedScope: "transfer:internal",
	}

	resp, err := h.svc.RequestServiceTicket(context.Background(), req)
	if err != nil {
		t.Fatalf("RequestServiceTicket() error = %v", err)
	}
	reply := h.decryptReply(t, resp.EncryptedPayload)
	if reply.Scope != "transfer:internal" || reply.ServiceID != "bank-service" {
		t.Fatalf("unexpected reply scope/service = %q/%q", reply.Scope, reply.ServiceID)
	}
}

/**
 * @description Verifies that stale authenticators are rejected with REQUEST_EXPIRED.
 * @param {*testing.T} t - Test handle.
 */
func TestAuthenticatorTimestampTooOldRejectsRequestExpired(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "alice", "bank-service", "transfer:internal", h.now.Add(-301*time.Second))
	assertCode(t, err, ErrRequestExpired)
}

/**
 * @description Verifies that the same nonce and timestamp cannot be replayed twice.
 * @param {*testing.T} t - Test handle.
 */
func TestReplaySameNonceAndTimestampRejectsSecondRequest(t *testing.T) {
	h := newHarness(t)
	req := h.validRequest(t, "alice", "bank-service", "transfer:internal", h.now)
	if _, err := h.svc.RequestServiceTicket(context.Background(), req); err != nil {
		t.Fatalf("first RequestServiceTicket() error = %v", err)
	}
	_, err := h.svc.RequestServiceTicket(context.Background(), req)
	assertCode(t, err, ErrReplayDetected)
	if h.replay.lastTTL != 5*time.Minute {
		t.Fatalf("replay ttl = %s, want 5m", h.replay.lastTTL)
	}
	if h.replay.calls != 2 {
		t.Fatalf("replay SetNX calls = %d, want 2", h.replay.calls)
	}
}

/**
 * @description Verifies that revoked certificates are rejected before ticket issuance.
 * @param {*testing.T} t - Test handle.
 */
func TestRevokedCertificateRejects(t *testing.T) {
	h := newHarness(t)
	h.repo.certs[h.certSN] = Certificate{
		Serial:       h.certSN,
		SubjectCN:    "alice",
		PublicKeyPEM: h.pubKey,
		Status:       CertificateRevoked,
		NotAfter:     h.now.Add(time.Hour),
	}
	_, err := h.request(t, "alice", "bank-service", "transfer:internal", h.now)
	assertCode(t, err, ErrCertRevoked)
}

/**
 * @description Verifies that Ticket_v carries the client public key, scope, identity, and session key.
 * @param {*testing.T} t - Test handle.
 */
func TestTicketVContainsPublicKeyAndScope(t *testing.T) {
	h := newHarness(t)
	resp, err := h.request(t, "alice", "bank-service", "transfer:internal", h.now)
	if err != nil {
		t.Fatalf("RequestServiceTicket() error = %v", err)
	}
	reply := h.decryptReply(t, resp.EncryptedPayload)
	ticket := h.decryptTicket(t, reply.TicketV)
	if ticket.PublicKey != h.pubKey {
		t.Fatalf("pub_c = %q", ticket.PublicKey)
	}
	if ticket.PubCPEM != h.pubKey {
		t.Fatalf("pub_c_pem = %q", ticket.PubCPEM)
	}
	if ticket.Scope != "transfer:internal" {
		t.Fatalf("scope = %q", ticket.Scope)
	}
	if ticket.ClientID != "alice" {
		t.Fatalf("client_id = %q", ticket.ClientID)
	}
	if ticket.ServiceID != "bank-service" || ticket.SName != "bank-service" {
		t.Fatalf("service_id/sname = %q/%q", ticket.ServiceID, ticket.SName)
	}
	if ticket.CertSN != h.certSN {
		t.Fatalf("cert_sn = %q", ticket.CertSN)
	}
	if ticket.NonceReq != h.nonceReq {
		t.Fatalf("nonce_req = %q", ticket.NonceReq)
	}
	if ticket.ExpiresAt != h.now.Add(5*time.Minute).Unix() {
		t.Fatalf("expires_at = %d", ticket.ExpiresAt)
	}
	if !bytes.Equal(ticket.KCV, reply.KCV) {
		t.Fatalf("ticket K_c_v does not match TGS_REP K_c_v")
	}
	if len(ticket.KCV) != aes256KeySize {
		t.Fatalf("ticket K_c_v length = %d", len(ticket.KCV))
	}
}

/**
 * @description Verifies that unauthorized scopes are denied.
 * @param {*testing.T} t - Test handle.
 */
func TestInvalidRequestedScopeRejects(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "alice", "bank-service", "admin:everything", h.now)
	assertCode(t, err, ErrScopeDenied)
}

/**
 * @description Verifies that the authenticator client must match the TGT client.
 * @param {*testing.T} t - Test handle.
 */
func TestAuthenticatorClientMismatchRejects(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "mallory", "bank-service", "transfer:internal", h.now)
	assertCode(t, err, ErrIdentityMismatch)
}

/**
 * @description Verifies that the authenticator service must match the requested service.
 * @param {*testing.T} t - Test handle.
 */
func TestAuthenticatorServiceMismatchRejects(t *testing.T) {
	h := newHarness(t)
	req := h.validRequest(t, "alice", "bank-service", "transfer:internal", h.now)
	auth := AuthenticatorPlaintext{
		ClientID:         "alice",
		Timestamp:        h.now.Unix(),
		NonceReq:         h.nonceReq,
		RequestID:        h.requestID,
		RequestedService: "other-service",
		Scope:            "transfer:internal",
	}
	req.Authenticator = h.mustEncrypt(t, h.kctgs, auth)

	_, err := h.svc.RequestServiceTicket(context.Background(), req)
	assertCode(t, err, ErrAuthInvalid)
}

/**
 * @description Verifies that the authenticator scope must match the requested scope.
 * @param {*testing.T} t - Test handle.
 */
func TestAuthenticatorScopeMismatchRejects(t *testing.T) {
	h := newHarness(t)
	req := h.validRequest(t, "alice", "bank-service", "transfer:internal", h.now)
	auth := AuthenticatorPlaintext{
		ClientID:         "alice",
		Timestamp:        h.now.Unix(),
		NonceReq:         h.nonceReq,
		RequestID:        h.requestID,
		RequestedService: "bank-service",
		Scope:            "account:read",
	}
	req.Authenticator = h.mustEncrypt(t, h.kctgs, auth)

	_, err := h.svc.RequestServiceTicket(context.Background(), req)
	assertCode(t, err, ErrScopeDenied)
}

/**
 * @description Test fixture that owns all keys, payloads, and fake dependencies for KDC tests.
 */
type harness struct {
	svc       *TGSService
	repo      memoryCertRepo
	now       time.Time
	tgsKey    []byte
	serviceKV []byte
	kctgs     []byte
	nonce2    []byte
	nonceReq  string
	requestID string
	certSN    string
	pubKey    string
	replay    *memoryReplayStore
}

/**
 * @description Creates a complete deterministic KDC test harness.
 * @param {*testing.T} t - Test handle.
 * @returns {*harness} Ready-to-use test harness.
 */
func newHarness(t *testing.T) *harness {
	t.Helper()
	now := time.Unix(1715500700, 0).UTC()
	tgsKey := fixtureKey("tgs")
	serviceKV := fixtureKey("bank-service")
	kctgs := fixtureKey("client-tgs-session")
	pubKey := "-----BEGIN PUBLIC KEY-----\nfixture-public-key\n-----END PUBLIC KEY-----"
	certSN := "2E4A6C8B9F1D2C3E4F5A6B7C8D9E0F1A"
	repo := memoryCertRepo{certs: map[string]Certificate{
		certSN: {
			Serial:       certSN,
			SubjectCN:    "alice",
			PublicKeyPEM: pubKey,
			Status:       CertificateValid,
			NotAfter:     now.Add(time.Hour),
		},
	}}
	replay := newMemoryReplayStore()
	svc, err := NewTGSService(Config{
		TGSKey:      tgsKey,
		ServiceKeys: map[string][]byte{"bank-service": serviceKV},
		ReplayStore: replay,
		CertRepo:    repo,
		ScopeAuthorizer: StaticScopeAuthorizer{
			"bank-service": {
				"transfer:internal": true,
				"account:read":      true,
			},
		},
		Clock:           fixedClock{now: now},
		Random:          bytes.NewReader(bytes.Repeat([]byte{0x44}, 4096)),
		TicketTTL:       5 * time.Minute,
		TimestampWindow: 5 * time.Minute,
		ReplayTTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return &harness{
		svc:       svc,
		repo:      repo,
		now:       now,
		tgsKey:    tgsKey,
		serviceKV: serviceKV,
		kctgs:     kctgs,
		nonce2:    []byte("nonce-2-16-byte"),
		nonceReq:  "bm9uY2UtMi0xNi1ieXRl",
		requestID: "tgs-request-001",
		certSN:    certSN,
		pubKey:    pubKey,
		replay:    replay,
	}
}

/**
 * @description Sends a TGS request through the service using a generated valid request body.
 * @param {*testing.T} t - Test handle.
 * @param {string} authClientID - Client ID placed in the authenticator.
 * @param {string} serviceID - Service ID placed in the authenticator.
 * @param {string} scope - Requested scope.
 * @param {time.Time} ts - Authenticator timestamp.
 * @returns {TGSResponse} Service response.
 * @returns {error} KDC service error.
 */
func (h *harness) request(t *testing.T, authClientID string, serviceID string, scope string, ts time.Time) (TGSResponse, error) {
	t.Helper()
	return h.svc.RequestServiceTicket(context.Background(), h.validRequest(t, authClientID, serviceID, scope, ts))
}

/**
 * @description Builds a syntactically valid TGS request with configurable authenticator fields.
 * @param {*testing.T} t - Test handle.
 * @param {string} authClientID - Client ID placed in the authenticator.
 * @param {string} serviceID - Service ID placed in the authenticator.
 * @param {string} scope - Requested scope.
 * @param {time.Time} ts - Authenticator timestamp.
 * @returns {TGSRequest} Test TGS request.
 */
func (h *harness) validRequest(t *testing.T, authClientID string, serviceID string, scope string, ts time.Time) TGSRequest {
	t.Helper()
	tgt := h.mustTGT(t, "alice", h.kctgs, h.now.Add(30*time.Minute))
	auth := AuthenticatorPlaintext{
		ClientID:         authClientID,
		Timestamp:        ts.Unix(),
		NonceReq:         h.nonceReq,
		RequestID:        h.requestID,
		RequestedService: serviceID,
		Scope:            scope,
	}
	return TGSRequest{
		ServiceID:      "bank-service",
		TGTCiphertext:  tgt,
		Authenticator:  h.mustEncrypt(t, h.kctgs, auth),
		CertSN:         h.certSN,
		Nonce2:         h.nonce2,
		RequestedScope: scope,
	}
}

/**
 * @description Creates an encrypted TGT fixture with the harness TGS master key.
 * @param {*testing.T} t - Test handle.
 * @param {string} clientID - Client ID to embed in the TGT.
 * @param {[]byte} kctgs - Client-to-TGS session key.
 * @param {time.Time} expiry - TGT expiry timestamp.
 * @returns {[]byte} Encrypted TGT.
 */
func (h *harness) mustTGT(t *testing.T, clientID string, kctgs []byte, expiry time.Time) []byte {
	t.Helper()
	return h.mustEncrypt(t, h.tgsKey, TGTPlaintext{
		ClientID: clientID,
		CertSN:   h.certSN,
		KCTGS:    kctgs,
		IssuedAt: h.now.Unix(),
		Expiry:   expiry.Unix(),
	})
}

/**
 * @description Encrypts a payload for test setup and fails the test on errors.
 * @param {*testing.T} t - Test handle.
 * @param {[]byte} key - AES-256 encryption key.
 * @param {any} payload - JSON-serializable payload.
 * @returns {[]byte} Encrypted payload.
 */
func (h *harness) mustEncrypt(t *testing.T, key []byte, payload any) []byte {
	t.Helper()
	out, err := encryptJSON(key, payload, bytes.NewReader(bytes.Repeat([]byte{0x55}, 1024)))
	if err != nil {
		t.Fatalf("encryptJSON() error = %v", err)
	}
	return out
}

/**
 * @description Decrypts a TGS reply fixture and fails the test on errors.
 * @param {*testing.T} t - Test handle.
 * @param {[]byte} ciphertext - Encrypted TGS reply.
 * @returns {TGSReplyPlaintext} Decrypted TGS reply.
 */
func (h *harness) decryptReply(t *testing.T, ciphertext []byte) TGSReplyPlaintext {
	t.Helper()
	reply, err := decryptJSON[TGSReplyPlaintext](h.kctgs, ciphertext)
	if err != nil {
		t.Fatalf("decrypt reply error = %v", err)
	}
	return reply
}

/**
 * @description Decrypts a service ticket fixture and fails the test on errors.
 * @param {*testing.T} t - Test handle.
 * @param {[]byte} ciphertext - Encrypted service ticket.
 * @returns {ServiceTicketPlaintext} Decrypted service ticket.
 */
func (h *harness) decryptTicket(t *testing.T, ciphertext []byte) ServiceTicketPlaintext {
	t.Helper()
	ticket, err := decryptJSON[ServiceTicketPlaintext](h.serviceKV, ciphertext)
	if err != nil {
		t.Fatalf("decrypt ticket error = %v", err)
	}
	return ticket
}

/**
 * @description Asserts that an error carries the expected KDC error code.
 * @param {*testing.T} t - Test handle.
 * @param {error} err - Error returned by the service.
 * @param {ErrorCode} want - Expected KDC error code.
 */
func assertCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	if got := ErrorCodeOf(err); got != want {
		t.Fatalf("error code = %s, want %s; err=%v", got, want, err)
	}
}

/**
 * @description Derives a deterministic 32-byte test key from a label.
 * @param {string} label - Test key label.
 * @returns {[]byte} Deterministic AES-256 key.
 */
func fixtureKey(label string) []byte {
	sum := sha256.Sum256([]byte("test-fixture:" + label))
	return sum[:]
}

func encryptJSON(key []byte, plaintext any, random io.Reader) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires a 32-byte key")
	}
	body, err := json.Marshal(plaintext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, body, nil)
	return append(nonce, sealed...), nil
}

func decryptJSON[T any](key []byte, ciphertext []byte) (T, error) {
	var out T
	block, err := aes.NewCipher(key)
	if err != nil {
		return out, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return out, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return out, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	body := ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return out, err
	}
	return out, nil
}
