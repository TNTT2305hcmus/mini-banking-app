package ca

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testRootCAPassword = "test-root-ca-password"

func TestLoadFromDiskAcceptsValidRootCA(t *testing.T) {
	key := newRootCATestKey(t)
	certDER := newRootCATestCert(t, key, key, nil)
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	rootCA, err := loadFromDisk(keyPath, certPath)
	if err != nil {
		t.Fatalf("loadFromDisk failed: %v", err)
	}
	if rootCA.PrivateKey == nil || rootCA.Certificate == nil || len(rootCA.CertPEM) == 0 {
		t.Fatal("expected loaded Root CA fields to be populated")
	}
}

func TestLoadFromDiskRejectsMismatchedRootCAKeyAndCert(t *testing.T) {
	key := newRootCATestKey(t)
	certKey := newRootCATestKey(t)
	certDER := newRootCATestCert(t, certKey, certKey, nil)
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	_, err := loadFromDisk(keyPath, certPath)
	if err == nil {
		t.Fatal("expected mismatched root CA key/cert to be rejected")
	}
	assertErrorContains(t, err, "does not match")
}

func TestLoadFromDiskRejectsRootCACertWithoutCAFlag(t *testing.T) {
	key := newRootCATestKey(t)
	certDER := newRootCATestCert(t, key, key, func(cert *x509.Certificate) {
		cert.IsCA = false
	})
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	_, err := loadFromDisk(keyPath, certPath)
	if err == nil {
		t.Fatal("expected non-CA root certificate to be rejected")
	}
	assertErrorContains(t, err, "not marked as a CA")
}

func TestLoadFromDiskRejectsRootCACertWithoutCertSignUsage(t *testing.T) {
	key := newRootCATestKey(t)
	certDER := newRootCATestCert(t, key, key, func(cert *x509.Certificate) {
		cert.KeyUsage = x509.KeyUsageCRLSign
	})
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	_, err := loadFromDisk(keyPath, certPath)
	if err == nil {
		t.Fatal("expected root certificate without cert-sign usage to be rejected")
	}
	assertErrorContains(t, err, "KeyUsageCertSign")
}

func TestLoadFromDiskRejectsExpiredRootCACert(t *testing.T) {
	key := newRootCATestKey(t)
	certDER := newRootCATestCert(t, key, key, func(cert *x509.Certificate) {
		cert.NotBefore = time.Now().UTC().AddDate(-2, 0, 0)
		cert.NotAfter = time.Now().UTC().AddDate(-1, 0, 0)
	})
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	_, err := loadFromDisk(keyPath, certPath)
	if err == nil {
		t.Fatal("expected expired root certificate to be rejected")
	}
	assertErrorContains(t, err, "expired")
}

func TestLoadFromDiskRejectsRootCACertWithInvalidSelfSignature(t *testing.T) {
	key := newRootCATestKey(t)
	signerKey := newRootCATestKey(t)
	certDER := newRootCATestCert(t, key, signerKey, nil)
	keyPath, certPath := writeRootCATestFiles(t, key, certDER)

	_, err := loadFromDisk(keyPath, certPath)
	if err == nil {
		t.Fatal("expected invalid self-signed root certificate to be rejected")
	}
	assertErrorContains(t, err, "self-signature")
}

func newRootCATestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate root CA test key: %v", err)
	}
	return key
}

func newRootCATestCert(t *testing.T, certKey, signerKey *rsa.PrivateKey, mutate func(*x509.Certificate)) []byte {
	t.Helper()
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   "Mini_App_Banking Test Root CA",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	if mutate != nil {
		mutate(template)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &certKey.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("create root CA test cert: %v", err)
	}
	return certDER
}

func writeRootCATestFiles(t *testing.T, key *rsa.PrivateKey, certDER []byte) (string, string) {
	t.Helper()
	t.Setenv(rootCAKeyPasswordEnv, testRootCAPassword)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "ca.key")
	certPath := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(keyPath, encryptedRootCATestKeyPEM(t, key), 0600); err != nil {
		t.Fatalf("write root CA test key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write root CA test cert: %v", err)
	}
	return keyPath, certPath
}

func encryptedRootCATestKeyPEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal root CA test key: %v", err)
	}

	salt := randomBytes(t, 16)
	nonce := randomBytes(t, rootCAKeyNonceSize)
	iterations := 1000
	derivedKey := pbkdf2SHA256([]byte(testRootCAPassword), salt, iterations, rootCAKeySize)

	aesBlock, err := aes.NewCipher(derivedKey)
	if err != nil {
		t.Fatalf("create test AES cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		t.Fatalf("create test GCM: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type: encryptedPrivateKeyPEMType,
		Headers: map[string]string{
			"Cipher":     rootCAKeyCipher,
			"KDF":        rootCAKeyKDF,
			"Iterations": strconv.Itoa(iterations),
			"Salt":       base64.StdEncoding.EncodeToString(salt),
			"Nonce":      base64.StdEncoding.EncodeToString(nonce),
		},
		Bytes: gcm.Seal(nil, nonce, keyDER, nil),
	})
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	out := make([]byte, size)
	if _, err := rand.Read(out); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return out
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %q", want, err.Error())
	}
}
