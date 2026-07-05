package ca

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegisterVerifyRevokeLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	signer := newTestClientCA(t)
	svc := NewService(signer, store, t.TempDir(), 365)
	csrPEM := newCSR(t, "Alice Nguyen", "alice@example.com")

	issued, err := svc.RegisterUser(ctx, RegisterInput{
		CSRPem:       csrPEM,
		OwnerID:      "user-001",
		SubjectCN:    "Alice Nguyen",
		SubjectEmail: "alice@example.com",
		RequestID:    "req-register",
		PerformedBy:  "gateway:pki-register",
	})
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if issued.SerialNumber == "" || issued.CertificatePEM == "" || issued.PublicKeyPEM == "" {
		t.Fatal("expected issued certificate, serial and public key")
	}
	if issued.OwnerID != "user-001" || issued.SubjectEmail != "alice@example.com" {
		t.Fatalf("unexpected metadata: %+v", issued)
	}
	if issued.FingerprintSHA256 == "" || len(issued.FingerprintSHA256) != 64 {
		t.Fatalf("expected SHA-256 fingerprint, got %q", issued.FingerprintSHA256)
	}
	if issued.CertType != CertTypeClient || issued.IssuerID != ClientCAID {
		t.Fatalf("expected client cert issued by Client CA, got type=%q issuer=%q", issued.CertType, issued.IssuerID)
	}
	issuedCert := mustParseTestCertPEM(t, issued.CertificatePEM)
	if err := issuedCert.CheckSignatureFrom(signer.Certificate); err != nil {
		t.Fatalf("expected issued cert to be signed by Client CA: %v", err)
	}

	verified, err := svc.VerifyCertificate(ctx, VerifyInput{
		SerialNumber:          issued.SerialNumber,
		Caller:                "system:kdc-service",
		RequestID:             "req-verify",
		IncludePublicKeyPEM:   true,
		IncludeCertificatePEM: false,
	})
	if err != nil {
		t.Fatalf("VerifyCertificate: %v", err)
	}
	if verified.Status != CertStatusActive {
		t.Fatalf("expected active cert, got %s", verified.Status)
	}
	if verified.PublicKeyPEM == "" {
		t.Fatal("expected public key to be stored for KDC/Bank verification")
	}

	detail, err := svc.GetCertificateDetail(ctx, issued.SerialNumber, "req-detail", "admin:thanh")
	if err != nil {
		t.Fatalf("GetCertificateDetail: %v", err)
	}
	if detail.SubjectCN != "Alice Nguyen" {
		t.Fatalf("unexpected detail CN: %s", detail.SubjectCN)
	}

	revoked, err := svc.RevokeCertificate(ctx, issued.SerialNumber, "key_compromised", "req-revoke", "admin:thanh")
	if err != nil {
		t.Fatalf("RevokeCertificate: %v", err)
	}
	if revoked.Status != CertStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("expected revoked metadata, got %+v", revoked)
	}

	afterRevoke, err := svc.VerifyCertificate(ctx, VerifyInput{SerialNumber: issued.SerialNumber, Caller: "system:bank-service"})
	if err != nil {
		t.Fatalf("VerifyCertificate after revoke: %v", err)
	}
	if afterRevoke.Status != CertStatusRevoked {
		t.Fatalf("expected revoked status, got %s", afterRevoke.Status)
	}

	events := store.AuditEvents()
	if len(events) < 4 {
		t.Fatalf("expected audit events for issue/verify/detail/revoke, got %d", len(events))
	}
}

func TestRegisterRejectsCSRIdentityMismatch(t *testing.T) {
	svc := NewService(newTestClientCA(t), NewStore(), t.TempDir(), 365)
	_, err := svc.RegisterUser(context.Background(), RegisterInput{
		CSRPem:       newCSR(t, "Alice Nguyen", "alice@example.com"),
		OwnerID:      "user-001",
		SubjectCN:    "Mallory Nguyen",
		SubjectEmail: "alice@example.com",
		PerformedBy:  "gateway:pki-register",
	})
	if !errors.Is(err, ErrCSRIdentityMismatch) {
		t.Fatalf("expected ErrCSRIdentityMismatch, got %v", err)
	}
}

func TestRegisterRejectsDuplicateActiveOwner(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newTestClientCA(t), NewStore(), t.TempDir(), 365)

	_, err := svc.RegisterUser(ctx, RegisterInput{
		CSRPem:       newCSR(t, "Alice Nguyen", "alice@example.com"),
		OwnerID:      "user-001",
		SubjectCN:    "Alice Nguyen",
		SubjectEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("first RegisterUser: %v", err)
	}

	_, err = svc.RegisterUser(ctx, RegisterInput{
		CSRPem:       newCSR(t, "Alice Nguyen", "alice2@example.com"),
		OwnerID:      "user-001",
		SubjectCN:    "Alice Nguyen",
		SubjectEmail: "alice2@example.com",
	})
	if !errors.Is(err, ErrActiveCertificateExists) {
		t.Fatalf("expected duplicate active owner error, got %v", err)
	}
}

func TestListCertificatesFiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newTestClientCA(t), NewStore(), t.TempDir(), 365)

	registerForTest(t, svc, "user-001", "Alice Nguyen", "alice@example.com")
	registerForTest(t, svc, "user-002", "Bob Tran", "bob@example.com")

	records, total, err := svc.ListCertificates(ctx, ListFilter{SubjectEmail: "bob", Limit: 1, Offset: 0})
	if err != nil {
		t.Fatalf("ListCertificates: %v", err)
	}
	if total != 1 || len(records) != 1 {
		t.Fatalf("expected one Bob record, total=%d len=%d", total, len(records))
	}
	if records[0].OwnerID != "user-002" {
		t.Fatalf("unexpected owner: %s", records[0].OwnerID)
	}

	records, total, err = svc.ListCertificates(ctx, ListFilter{CertType: CertTypeClient, IssuerID: ClientCAID, Limit: 10})
	if err != nil {
		t.Fatalf("ListCertificates by issuer/type: %v", err)
	}
	if total != 2 || len(records) != 2 {
		t.Fatalf("expected two client certs issued by client-ca, total=%d len=%d", total, len(records))
	}
}

func TestRegisterStoresIssuerChainMetadata(t *testing.T) {
	ctx := context.Background()
	root, client := newTestClientCAWithRoot(t)
	store := NewStore()
	svc := NewServiceWithRootCAAndExtensionConfig(root, client, store, t.TempDir(), 365, CertificateExtensionConfig{})

	issued := registerForTest(t, svc, "user-001", "Alice Nguyen", "alice@example.com")
	if issued.ChainPEM == "" {
		t.Fatal("expected issued certificate to carry issuer chain PEM")
	}
	if got := strings.Count(issued.ChainPEM, "BEGIN CERTIFICATE"); got != 2 {
		t.Fatalf("expected client-ca + root-ca chain PEM, got %d cert(s)", got)
	}
	if len(issued.ChainFingerprints) != 2 {
		t.Fatalf("expected client-ca + root-ca fingerprints, got %d", len(issued.ChainFingerprints))
	}
	if _, ok := store.issuers[RootCAID]; !ok {
		t.Fatalf("expected %s issuer metadata to be persisted", RootCAID)
	}
	clientIssuer, ok := store.issuers[ClientCAID]
	if !ok {
		t.Fatalf("expected %s issuer metadata to be persisted", ClientCAID)
	}
	if clientIssuer.ParentIssuerID != RootCAID || clientIssuer.CertRole != IssuerRoleClientCA {
		t.Fatalf("unexpected client issuer metadata: %+v", clientIssuer)
	}

	verified, err := svc.VerifyCertificate(ctx, VerifyInput{SerialNumber: issued.SerialNumber})
	if err != nil {
		t.Fatalf("VerifyCertificate: %v", err)
	}
	if verified.IssuerID != ClientCAID || len(verified.ChainFingerprints) != 2 {
		t.Fatalf("expected issuer chain metadata on verify, got %+v", verified)
	}
}

func TestRevokeRejectsNonClientCertificate(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	svc := NewService(newTestClientCA(t), store, t.TempDir(), 365)
	now := time.Now().UTC()
	record := CertificateRecord{
		SerialNumber:      "service-serial",
		CertType:          CertTypeServiceTLS,
		IssuerID:          ClientCAID,
		IssuerCommonName:  "Mini_App_Banking Test Client CA",
		IssuerSerial:      "02",
		SubjectCN:         "banking-service",
		CertificatePEM:    "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n",
		FingerprintSHA256: strings.Repeat("a", 64),
		NotBefore:         now.Add(-time.Minute),
		NotAfter:          now.Add(time.Hour),
		Status:            CertStatusActive,
		IssuedAt:          now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.CreateCertificate(ctx, record); err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	_, err := svc.RevokeCertificate(ctx, record.SerialNumber, "operator_request", "req-revoke", "admin:thanh")
	if !errors.Is(err, ErrCertificateNotRevokable) {
		t.Fatalf("expected ErrCertificateNotRevokable, got %v", err)
	}
	after, err := store.GetCertificate(ctx, record.SerialNumber)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if after.Status != CertStatusActive || after.RevokedAt != nil {
		t.Fatalf("expected service TLS cert to remain active, got %+v", after)
	}
}

func TestPersistentStoreSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")

	store, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	svc := NewService(newTestClientCA(t), store, t.TempDir(), 365)
	issued := registerForTest(t, svc, "user-001", "Alice Nguyen", "alice@example.com")
	if _, err := svc.RevokeCertificate(ctx, issued.SerialNumber, "operator_request", "req-revoke", "admin:thanh"); err != nil {
		t.Fatalf("RevokeCertificate: %v", err)
	}

	reloaded, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	record, err := reloaded.GetCertificate(ctx, issued.SerialNumber)
	if err != nil {
		t.Fatalf("GetCertificate after reload: %v", err)
	}
	if record.Status != CertStatusRevoked || record.RevocationReason != "operator_request" {
		t.Fatalf("expected revoked cert after reload, got %+v", record)
	}
	if len(reloaded.AuditEvents()) < 2 {
		t.Fatal("expected audit events to survive reload")
	}
}

func registerForTest(t *testing.T, svc *Service, ownerID, subjectCN, subjectEmail string) *CertificateRecord {
	t.Helper()
	record, err := svc.RegisterUser(context.Background(), RegisterInput{
		CSRPem:       newCSR(t, subjectCN, subjectEmail),
		OwnerID:      ownerID,
		SubjectCN:    subjectCN,
		SubjectEmail: subjectEmail,
		PerformedBy:  "test",
	})
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	return record
}

func newTestRootCA(t *testing.T) *RootCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
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
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create root cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse root cert: %v", err)
	}
	return &RootCA{
		PrivateKey:  key,
		Certificate: cert,
		CertPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

func newTestClientCA(t *testing.T) *RootCA {
	t.Helper()
	_, client := newTestClientCAWithRoot(t)
	return client
}

func newTestClientCAWithRoot(t *testing.T) (*RootCA, *RootCA) {
	t.Helper()
	root := newTestRootCA(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client CA key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   "Mini_App_Banking Test Client CA",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, root.Certificate, &key.PublicKey, root.PrivateKey)
	if err != nil {
		t.Fatalf("create client CA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse client CA cert: %v", err)
	}
	client := &RootCA{
		PrivateKey:  key,
		Certificate: cert,
		CertPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
	return root, client
}

func mustParseTestCertPEM(t *testing.T, certPEM string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("expected certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate PEM: %v", err)
	}
	return cert
}

func newCSR(t *testing.T, subjectCN, subjectEmail string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	template := &x509.CertificateRequest{
		Subject:        pkix.Name{CommonName: subjectCN, Organization: []string{"Mini_App_Banking"}},
		EmailAddresses: []string{subjectEmail},
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
