package grpc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewTLSTransportCredentialsLoadsServerCertificate(t *testing.T) {
	certPath, keyPath := writeTestServerCertificate(t)

	creds, err := newTLSTransportCredentials(SecurityConfig{
		ServerCertPath: certPath,
		ServerKeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("newTLSTransportCredentials failed: %v", err)
	}
	if creds == nil {
		t.Fatal("expected transport credentials")
	}
}

func TestNewTLSTransportCredentialsRequiresServerCertificateAndKey(t *testing.T) {
	_, err := newTLSTransportCredentials(SecurityConfig{})
	if err == nil || !strings.Contains(err.Error(), "GRPC_SERVER_CERT_PATH") {
		t.Fatalf("expected missing certificate path error, got %v", err)
	}

	_, err = newTLSTransportCredentials(SecurityConfig{ServerCertPath: "server.crt"})
	if err == nil || !strings.Contains(err.Error(), "GRPC_SERVER_KEY_PATH") {
		t.Fatalf("expected missing key path error, got %v", err)
	}
}

func writeTestServerCertificate(t *testing.T) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "ca-service",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"ca-service", "localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create test cert: %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("write test cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write test key: %v", err)
	}
	return certPath, keyPath
}
