package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionCADevBuildsLayeredCAHierarchy(t *testing.T) {
	certsDir := t.TempDir()
	t.Setenv("CA_CERTS_DIR", certsDir)
	t.Setenv("ROOT_CA_KEY_PASSWORD", "test-root-ca-password")

	staleCAServiceCA := filepath.Join(certsDir, "grpc", "ca-server-ca.crt")
	if err := os.MkdirAll(filepath.Dir(staleCAServiceCA), 0755); err != nil {
		t.Fatalf("mkdir stale cert dir: %v", err)
	}
	if err := os.WriteFile(staleCAServiceCA, []byte("stale"), 0644); err != nil {
		t.Fatalf("write stale cert: %v", err)
	}

	main()

	rootCA := mustReadProvisionedCert(t, filepath.Join(certsDir, "root-ca", "ca.crt"))
	grpcCA := mustReadProvisionedCert(t, filepath.Join(certsDir, "intermediate", "grpc-ca.crt"))
	clientCA := mustReadProvisionedCert(t, filepath.Join(certsDir, "intermediate", "client-ca.crt"))
	caServer := mustReadProvisionedCert(t, filepath.Join(certsDir, "grpc", "ca-server.crt"))

	if err := rootCA.CheckSignatureFrom(rootCA); err != nil {
		t.Fatalf("Root CA must be self-signed and self-verifiable: %v", err)
	}
	if !rootCA.IsCA || rootCA.Subject.CommonName != "Mini_App_Banking Root CA" {
		t.Fatalf("unexpected Root CA certificate: is_ca=%t cn=%q", rootCA.IsCA, rootCA.Subject.CommonName)
	}

	if err := grpcCA.CheckSignatureFrom(rootCA); err != nil {
		t.Fatalf("gRPC Transport CA must be signed by Root CA: %v", err)
	}
	if !grpcCA.IsCA || grpcCA.Subject.CommonName != "Mini_App_Banking gRPC Transport CA" {
		t.Fatalf("unexpected gRPC Transport CA certificate: is_ca=%t cn=%q", grpcCA.IsCA, grpcCA.Subject.CommonName)
	}

	if err := clientCA.CheckSignatureFrom(rootCA); err != nil {
		t.Fatalf("Client CA must be signed by Root CA: %v", err)
	}
	if !clientCA.IsCA || clientCA.Subject.CommonName != "Mini_App_Banking Client CA" {
		t.Fatalf("unexpected Client CA certificate: is_ca=%t cn=%q", clientCA.IsCA, clientCA.Subject.CommonName)
	}

	if err := caServer.CheckSignatureFrom(grpcCA); err != nil {
		t.Fatalf("CA Service TLS certificate must be signed by gRPC Transport CA: %v", err)
	}
	if err := caServer.CheckSignatureFrom(rootCA); err == nil {
		t.Fatal("CA Service TLS certificate must not be signed directly by Root CA")
	}
	if got := caServer.Subject.CommonName; got != "ca-service" {
		t.Fatalf("unexpected CA Service TLS CN: %q", got)
	}
	if !hasExtKeyUsage(caServer, x509.ExtKeyUsageServerAuth) {
		t.Fatalf("CA Service TLS certificate must include serverAuth EKU, got %+v", caServer.ExtKeyUsage)
	}
	if _, err := os.Stat(staleCAServiceCA); !os.IsNotExist(err) {
		t.Fatalf("stale ca-server-ca.crt must be removed, stat err=%v", err)
	}
}

func mustReadProvisionedCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read certificate %s: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("decode certificate PEM %s: no PEM block", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate %s: %v", path, err)
	}
	return cert
}

func hasExtKeyUsage(cert *x509.Certificate, want x509.ExtKeyUsage) bool {
	for _, got := range cert.ExtKeyUsage {
		if got == want {
			return true
		}
	}
	return false
}
