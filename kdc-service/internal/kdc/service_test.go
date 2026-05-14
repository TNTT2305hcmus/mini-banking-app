package kdc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type memoryReplayStore struct {
	keys    map[string]time.Time
	calls   int
	lastTTL time.Duration
}

func newMemoryReplayStore() *memoryReplayStore {
	return &memoryReplayStore{keys: map[string]time.Time{}}
}

func (s *memoryReplayStore) SetNX(_ context.Context, key string, _ string, ttl time.Duration) (bool, error) {
	s.calls++
	s.lastTTL = ttl
	if _, ok := s.keys[key]; ok {
		return false, nil
	}
	s.keys[key] = time.Now().Add(ttl)
	return true, nil
}

type memoryCertRepo struct {
	certs map[string]Certificate
}

func (r memoryCertRepo) GetCertificate(_ context.Context, certSN string) (Certificate, error) {
	cert, ok := r.certs[certSN]
	if !ok {
		return Certificate{}, ErrCertificateMissing
	}
	return cert, nil
}

func TestDecryptTGTSuccessExtractsKCTGS(t *testing.T) {
	h := newHarness(t)

	tgtCipher := h.mustTGT(t, "alice", h.kctgs, h.now.Add(30*time.Minute))
	tgt, err := h.svc.decryptTGT(tgtCipher)
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

func TestAuthenticatorTimestampTooOldRejectsRequestExpired(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "alice", "bank-service", "transfer:internal", h.now.Add(-301*time.Second))
	assertCode(t, err, ErrRequestExpired)
}

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

func TestInvalidRequestedScopeRejects(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "alice", "bank-service", "admin:everything", h.now)
	assertCode(t, err, ErrScopeDenied)
}

func TestAuthenticatorClientMismatchRejects(t *testing.T) {
	h := newHarness(t)
	_, err := h.request(t, "mallory", "bank-service", "transfer:internal", h.now)
	assertCode(t, err, ErrIdentityMismatch)
}

func TestAuthenticatorServiceMismatchRejects(t *testing.T) {
	h := newHarness(t)
	req := h.validRequest(t, "alice", "bank-service", "transfer:internal", h.now)
	auth := AuthenticatorPlaintext{
		ClientID:         "alice",
		Timestamp:        h.now.Unix(),
		NonceReq:         h.nonceReq,
		RequestedService: "other-service",
		Scope:            "transfer:internal",
	}
	req.Authenticator = h.mustEncrypt(t, h.kctgs, auth)

	_, err := h.svc.RequestServiceTicket(context.Background(), req)
	assertCode(t, err, ErrAuthInvalid)
}

func TestAuthenticatorScopeMismatchRejects(t *testing.T) {
	h := newHarness(t)
	req := h.validRequest(t, "alice", "bank-service", "transfer:internal", h.now)
	auth := AuthenticatorPlaintext{
		ClientID:         "alice",
		Timestamp:        h.now.Unix(),
		NonceReq:         h.nonceReq,
		RequestedService: "bank-service",
		Scope:            "account:read",
	}
	req.Authenticator = h.mustEncrypt(t, h.kctgs, auth)

	_, err := h.svc.RequestServiceTicket(context.Background(), req)
	assertCode(t, err, ErrScopeDenied)
}

type harness struct {
	svc       *Service
	repo      memoryCertRepo
	now       time.Time
	tgsKey    []byte
	serviceKV []byte
	kctgs     []byte
	nonce2    []byte
	nonceReq  string
	certSN    string
	pubKey    string
	replay    *memoryReplayStore
}

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
	svc, err := NewService(Config{
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
		certSN:    certSN,
		pubKey:    pubKey,
		replay:    replay,
	}
}

func (h *harness) request(t *testing.T, authClientID string, serviceID string, scope string, ts time.Time) (TGSResponse, error) {
	t.Helper()
	return h.svc.RequestServiceTicket(context.Background(), h.validRequest(t, authClientID, serviceID, scope, ts))
}

func (h *harness) validRequest(t *testing.T, authClientID string, serviceID string, scope string, ts time.Time) TGSRequest {
	t.Helper()
	tgt := h.mustTGT(t, "alice", h.kctgs, h.now.Add(30*time.Minute))
	auth := AuthenticatorPlaintext{
		ClientID:         authClientID,
		Timestamp:        ts.Unix(),
		NonceReq:         h.nonceReq,
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

func (h *harness) mustTGT(t *testing.T, clientID string, kctgs []byte, expiry time.Time) []byte {
	t.Helper()
	return h.mustEncrypt(t, h.tgsKey, TGTPlaintext{
		ClientID: clientID,
		KCTGS:    kctgs,
		IssuedAt: h.now.Unix(),
		Expiry:   expiry.Unix(),
	})
}

func (h *harness) mustEncrypt(t *testing.T, key []byte, payload any) []byte {
	t.Helper()
	out, err := encryptJSON(key, payload, bytes.NewReader(bytes.Repeat([]byte{0x55}, 1024)))
	if err != nil {
		t.Fatalf("encryptJSON() error = %v", err)
	}
	return out
}

func (h *harness) decryptReply(t *testing.T, ciphertext []byte) TGSReplyPlaintext {
	t.Helper()
	reply, err := decryptJSON[TGSReplyPlaintext](h.kctgs, ciphertext)
	if err != nil {
		t.Fatalf("decrypt reply error = %v", err)
	}
	return reply
}

func (h *harness) decryptTicket(t *testing.T, ciphertext []byte) ServiceTicketPlaintext {
	t.Helper()
	ticket, err := decryptJSON[ServiceTicketPlaintext](h.serviceKV, ciphertext)
	if err != nil {
		t.Fatalf("decrypt ticket error = %v", err)
	}
	return ticket
}

func assertCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	if got := ErrorCodeOf(err); got != want {
		t.Fatalf("error code = %s, want %s; err=%v", got, want, err)
	}
}

func fixtureKey(label string) []byte {
	sum := sha256.Sum256([]byte("test-fixture:" + label))
	return sum[:]
}
