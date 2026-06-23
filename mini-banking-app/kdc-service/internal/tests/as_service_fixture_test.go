package kdc_test

// as_service_fixture_test.go
//
// Shared fixture for the AS test suite. The AS service now consumes the same
// injected abstractions as the TGS service, so the fixture wires the in-memory
// certificate repository, replay store and fixed clock (defined in
// tgs_service_test.go) plus ephemeral RSA/K_tgs keys — no .env, key files, or
// real CA/Redis.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

// testFixture bundles everything an AS test needs.
type testFixture struct {
	svc        *ASService
	clientPriv *rsa.PrivateKey
	kdcPriv    *rsa.PrivateKey
	ktgsKey    []byte
	certRepo   memoryCertRepo
	replay     *memoryReplayStore
	clock      fixedClock
	certSN     string
	ownerID    string
}

// newFixture generates a complete fixture — fail-fast on error.
func newFixture(t *testing.T) *testFixture {
	t.Helper()

	clientPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client RSA key: %v", err)
	}
	kdcPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate KDC RSA key: %v", err)
	}
	ktgsKey := randBytes(t, 32)

	const certSN = "sn-001"
	const ownerID = "user-alice"

	now := time.Now().UTC()
	repo := memoryCertRepo{certs: map[string]Certificate{
		certSN: newTestCertificate(t, certSN, ownerID, &clientPriv.PublicKey, now),
	}}
	replay := newMemoryReplayStore()
	clock := fixedClock{now: now}

	svc := NewServiceForTest(ASConfig{
		CertRepo:        repo,
		ReplayStore:     replay,
		Clock:           clock,
		Keys:            &KDCKeys{KTGSKey: ktgsKey, PrivateKey: kdcPriv},
		TGTTTL:          30 * time.Minute,
		TimestampWindow: 5 * time.Minute,
	})

	return &testFixture{
		svc:        svc,
		clientPriv: clientPriv,
		kdcPriv:    kdcPriv,
		ktgsKey:    ktgsKey,
		certRepo:   repo,
		replay:     replay,
		clock:      clock,
		certSN:     certSN,
		ownerID:    ownerID,
	}
}

// newTestCertificate builds an active certificate carrying pub's PKIX PEM and a
// CA-authoritative owner_id, so the AS owner-binding check can be exercised.
// SubjectCN is deliberately a human full name distinct from owner_id, mirroring
// real CA data (the CA sets SubjectCN = full_name): the binding must key on
// owner_id only and must not reject just because SubjectCN differs.
func newTestCertificate(t *testing.T, serial, ownerID string, pub *rsa.PublicKey, now time.Time) Certificate {
	t.Helper()
	return Certificate{
		Serial:       serial,
		OwnerID:      ownerID,
		SubjectCN:    "Truong Thanh Thuan",
		PublicKeyPEM: mustPublicKeyPEM(t, pub),
		Status:       CertificateActive,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
	}
}

func mustPublicKeyPEM(t *testing.T, pub *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func randBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read(%d): %v", n, err)
	}
	return b
}

func assertBytesEqual(t *testing.T, field string, want, got []byte) {
	t.Helper()
	if string(want) != string(got) {
		t.Errorf("%s không khớp\n  want: %x\n  got:  %x", field, want, got)
	}
}
