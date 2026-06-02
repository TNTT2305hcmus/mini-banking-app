package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	record, serial := newStoreTestCert(t, "alice@example.com")

	store, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	if err := store.Save(serial, record); err != nil {
		t.Fatalf("Save: %v", err)
	}
	revoked, err := store.Revoke(serial, "key_compromised")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if !revoked {
		t.Fatal("expected Revoke to succeed")
	}

	reloaded, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("reload NewPersistentStore: %v", err)
	}
	got := reloaded.Get(serial)
	if got == nil {
		t.Fatal("expected certificate after reload")
	}
	if got.UserID != "alice@example.com" {
		t.Fatalf("expected user alice@example.com, got %s", got.UserID)
	}
	if got.RevokedAt == nil {
		t.Fatal("expected revocation timestamp after reload")
	}
	if got.RevokeReason != "key_compromised" {
		t.Fatalf("expected revoke reason key_compromised, got %s", got.RevokeReason)
	}
}

func TestStoreGetReturnsDefensiveCopy(t *testing.T) {
	store := NewStore()
	record, serial := newStoreTestCert(t, "bob@example.com")
	if err := store.Save(serial, record); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := store.Get(serial)
	if got == nil {
		t.Fatal("expected certificate")
	}
	mutatedAt := time.Unix(123, 0).UTC()
	got.UserID = "mallory@example.com"
	got.Cert.NotAfter = time.Unix(456, 0).UTC()
	got.RevokedAt = &mutatedAt
	got.RevokeReason = "mutated"

	again := store.Get(serial)
	if again.UserID != "bob@example.com" {
		t.Fatalf("store user mutated through Get: %s", again.UserID)
	}
	if again.RevokedAt != nil {
		t.Fatal("store revocation timestamp mutated through Get")
	}
	if again.Cert.NotAfter.Unix() == 456 {
		t.Fatal("store certificate pointer escaped through Get")
	}
	if again.RevokeReason != "" {
		t.Fatalf("store revoke reason mutated through Get: %s", again.RevokeReason)
	}

	if revoked, err := store.Revoke(serial, "operator_request"); err != nil {
		t.Fatalf("Revoke: %v", err)
	} else if !revoked {
		t.Fatal("expected Revoke to succeed")
	}
	revokedCopy := store.Get(serial)
	*revokedCopy.RevokedAt = time.Unix(0, 0).UTC()

	final := store.Get(serial)
	if final.RevokedAt.Unix() == 0 {
		t.Fatal("store revokedAt pointer escaped through Get")
	}
}

func TestStoreSaveIssuedRejectsDuplicateActiveCertificateForUser(t *testing.T) {
	store := NewStore()
	now := time.Now().UTC()

	first, firstSerial := newStoreTestCert(t, "alice@example.com")
	if err := store.SaveIssued(firstSerial, first, now); err != nil {
		t.Fatalf("SaveIssued first cert: %v", err)
	}

	second, secondSerial := newStoreTestCert(t, "alice@example.com")
	err := store.SaveIssued(secondSerial, second, now)
	if !errors.Is(err, ErrActiveCertificateExists) {
		t.Fatalf("expected ErrActiveCertificateExists, got %v", err)
	}

	if revoked, err := store.Revoke(firstSerial, "replacement_requested"); err != nil {
		t.Fatalf("Revoke first cert: %v", err)
	} else if !revoked {
		t.Fatal("expected first cert revoke to succeed")
	}
	if err := store.SaveIssued(secondSerial, second, now); err != nil {
		t.Fatalf("SaveIssued after revoke: %v", err)
	}
}

func newStoreTestCert(t *testing.T, commonName string) (*CertRecord, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          x509Serial(t),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().UTC().Add(-time.Minute),
		NotAfter:              time.Now().UTC().Add(time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	serial := hex.EncodeToString(cert.SerialNumber.Bytes())
	return &CertRecord{
		UserID:  commonName,
		Cert:    cert,
		CertPEM: certPEM,
	}, serial
}

func x509Serial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	if serial.Sign() == 0 {
		return big.NewInt(1)
	}
	return serial
}
