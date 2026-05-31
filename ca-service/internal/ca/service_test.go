package ca_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"mini-banking/ca-service/internal/ca"
)

// newTestRootCA tạo Root CA tạm thời cho test (RSA-2048 để test nhanh hơn 4096)
func newTestRootCA(t *testing.T) *ca.RootCA {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   "Mini_App_Banking Test Root CA",
		},
		NotBefore:             time.Now().UTC().Add(-time.Minute),
		NotAfter:              time.Now().UTC().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("create test root CA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse test root CA cert: %v", err)
	}

	return &ca.RootCA{
		PrivateKey:  privKey,
		Certificate: cert,
		CertPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
	}
}

// newTestService tạo CA Service với Root CA và store tạm cho test
func newTestService(t *testing.T) *ca.Service {
	t.Helper()
	rootCA := newTestRootCA(t)
	store := ca.NewStore()
	svc := ca.NewService(rootCA, store, t.TempDir(), 365)
	return svc
}

func newTestServiceWithExtensions(t *testing.T, extensions ca.CertificateExtensionConfig) *ca.Service {
	t.Helper()
	rootCA := newTestRootCA(t)
	store := ca.NewStore()
	return ca.NewServiceWithExtensionConfig(rootCA, store, t.TempDir(), 365, extensions)
}

// generateValidCSR tạo CSR hợp lệ với RSA-2048 key
func generateValidCSR(t *testing.T) (csrPEM string, privKey *rsa.PrivateKey) {
	return generateValidCSRForUser(t, "test-user@example.com")
}

func generateValidCSRForUser(t *testing.T, userID string) (csrPEM string, privKey *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	uri, err := url.Parse("urn:mini-banking:user:" + url.PathEscape(userID))
	if err != nil {
		t.Fatalf("parse test URI SAN: %v", err)
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID,
			Organization: []string{"Mini_App_Banking"},
		},
		EmailAddresses: []string{userID},
		URIs:           []*url.URL{uri},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return string(pemBytes), key
}

func parseIssuedCert(t *testing.T, certPEM string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("cannot decode returned cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse returned cert: %v", err)
	}
	return cert
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsURI(values []*url.URL, want string) bool {
	for _, value := range values {
		if value != nil && value.String() == want {
			return true
		}
	}
	return false
}

// generateTamperedCSR tạo CSR với chữ ký bị giả mạo:
// public key của user A nhưng ký bằng private key của user B
func generateTamperedCSR(t *testing.T) string {
	t.Helper()

	// Tạo 2 key khác nhau
	keyA, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyB, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Tạo CSR với pubKey của A nhưng ký bằng privKey của B
	// → CheckSignature() sẽ fail vì pubKey và signature không khớp
	template := &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "attacker@example.com"},
	}

	// Trick: encode pubKey của A vào template rồi sign bằng keyB
	// x509.CreateCertificateRequest sẽ dùng keyB để sign
	// Nhưng pubKey trong CSR sẽ là pubKey của keyB (không phải A)
	// → Đây là CSR hợp lệ về mặt kỹ thuật nhưng attacker không có privKey của A
	//
	// Để tạo CSR thực sự giả mạo (pubKey A, sign B), ta cần thao tác raw ASN.1
	// Cách đơn giản hơn cho test: tạo CSR hợp lệ rồi flip 1 byte trong signature
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, template, keyA)

	// Flip byte cuối của signature để làm signature sai
	csrDER[len(csrDER)-1] ^= 0xFF

	_ = keyB // suppress unused warning

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	}))
}

// ── Tests: RegisterUser ───────────────────────────────────────

// TestRegisterUser_ValidCSR kiểm tra happy path:
// CSR hợp lệ → CA cấp X.509 cert thành công
func TestRegisterUser_ValidCSR(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "alice@example.com")

	certPEM, serial, notAfter, err := svc.RegisterUser(csrPEM, "alice@example.com")

	// Không có lỗi
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	// Cert PEM không rỗng
	if certPEM == "" {
		t.Error("expected non-empty certPEM")
	}

	// Serial không rỗng
	if serial == "" {
		t.Error("expected non-empty serial")
	}

	// NotAfter phải trong tương lai
	if notAfter <= time.Now().Unix() {
		t.Errorf("expected notAfter in future, got %d", notAfter)
	}

	// Parse cert và kiểm tra các field
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("cannot decode returned cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse returned cert: %v", err)
	}

	// CommonName phải là userID
	if cert.Subject.CommonName != "alice@example.com" {
		t.Errorf("expected CN=alice@example.com, got %s", cert.Subject.CommonName)
	}

	// Cert phải là end-entity (không phải CA)
	if cert.IsCA {
		t.Error("issued cert must not be a CA cert")
	}

	// KeyUsage phải có DigitalSignature
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("cert must have DigitalSignature key usage")
	}

	if !containsString(cert.EmailAddresses, "alice@example.com") {
		t.Errorf("expected email SAN alice@example.com, got %v", cert.EmailAddresses)
	}
	expectedURI := "urn:mini-banking:user:" + url.PathEscape("alice@example.com")
	if !containsURI(cert.URIs, expectedURI) {
		t.Errorf("expected URI SAN %s, got %v", expectedURI, cert.URIs)
	}
	if len(cert.SubjectKeyId) == 0 {
		t.Error("expected non-empty SubjectKeyId")
	}
	if len(cert.AuthorityKeyId) == 0 {
		t.Error("expected non-empty AuthorityKeyId")
	}

	t.Logf("✅ Issued cert: CN=%s serial=%s", cert.Subject.CommonName, serial)
}

func TestRegisterUser_WithCRLAndOCSPExtensions(t *testing.T) {
	svc := newTestServiceWithExtensions(t, ca.CertificateExtensionConfig{
		CRLDistributionPoints: []string{"https://ca.example.test/crl.pem"},
		OCSPServers:           []string{"https://ca.example.test/ocsp"},
	})
	csrPEM, _ := generateValidCSRForUser(t, "extensions@example.com")

	certPEM, _, _, err := svc.RegisterUser(csrPEM, "extensions@example.com")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	cert := parseIssuedCert(t, certPEM)

	if !containsString(cert.CRLDistributionPoints, "https://ca.example.test/crl.pem") {
		t.Errorf("expected CRL distribution point, got %v", cert.CRLDistributionPoints)
	}
	if !containsString(cert.OCSPServer, "https://ca.example.test/ocsp") {
		t.Errorf("expected OCSP server, got %v", cert.OCSPServer)
	}
}

func TestRegisterUser_RejectsCSRIdentityMismatch(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "mallory@example.com")

	_, _, _, err := svc.RegisterUser(csrPEM, "alice@example.com")
	if err == nil {
		t.Fatal("expected error for CSR identity mismatch")
	}
}

func TestRegisterUser_RejectsDuplicateActiveCertificateForUser(t *testing.T) {
	svc := newTestService(t)
	userID := "duplicate@example.com"
	firstCSR, _ := generateValidCSRForUser(t, userID)
	if _, _, _, err := svc.RegisterUser(firstCSR, userID); err != nil {
		t.Fatalf("first RegisterUser failed: %v", err)
	}

	secondCSR, _ := generateValidCSRForUser(t, userID)
	_, _, _, err := svc.RegisterUser(secondCSR, userID)
	if err == nil {
		t.Fatal("expected duplicate active certificate error")
	}
}

// TestRegisterUser_TamperedCSR kiểm tra bảo vệ cốt lõi:
// CSR bị giả mạo chữ ký → CA từ chối, không cấp cert
func TestRegisterUser_TamperedCSR(t *testing.T) {
	svc := newTestService(t)
	tamperedCSR := generateTamperedCSR(t)

	_, _, _, err := svc.RegisterUser(tamperedCSR, "attacker@example.com")

	// PHẢI có lỗi — đây là test quan trọng nhất
	if err == nil {
		t.Fatal("expected error for tampered CSR, but got nil — SECURITY ISSUE!")
	}

	t.Logf("✅ Tampered CSR correctly rejected: %v", err)
}

// TestRegisterUser_EmptyCSR kiểm tra input validation
func TestRegisterUser_EmptyCSR(t *testing.T) {
	svc := newTestService(t)

	_, _, _, err := svc.RegisterUser("", "user@example.com")

	if err == nil {
		t.Fatal("expected error for empty CSR")
	}
	t.Logf("✅ Empty CSR rejected: %v", err)
}

// TestRegisterUser_InvalidPEM kiểm tra PEM không hợp lệ
func TestRegisterUser_InvalidPEM(t *testing.T) {
	svc := newTestService(t)

	_, _, _, err := svc.RegisterUser("not-a-pem-string", "user@example.com")

	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	t.Logf("✅ Invalid PEM rejected: %v", err)
}

// TestRegisterUser_WrongPEMType kiểm tra PEM đúng format nhưng sai type
func TestRegisterUser_WrongPEMType(t *testing.T) {
	svc := newTestService(t)

	// Gửi CERTIFICATE thay vì CERTIFICATE REQUEST
	wrongPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE", // Sai type
		Bytes: []byte("fake-data"),
	}))

	_, _, _, err := svc.RegisterUser(wrongPEM, "user@example.com")

	if err == nil {
		t.Fatal("expected error for wrong PEM type")
	}
	t.Logf("✅ Wrong PEM type rejected: %v", err)
}

// ── Tests: GetCertificate ─────────────────────────────────────

// TestGetCertificate_AfterRegister kiểm tra GetCertificate sau khi register
func TestGetCertificate_AfterRegister(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "bob@example.com")

	// Register trước
	_, serial, _, err := svc.RegisterUser(csrPEM, "bob@example.com")
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	// Get cert vừa cấp
	certPEM, userID, certStatus, notAfter, err := svc.GetCertificate(serial)

	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if certPEM == "" {
		t.Error("expected non-empty certPEM")
	}
	if userID != "bob@example.com" {
		t.Errorf("expected userID=bob@example.com, got %s", userID)
	}
	if certStatus != ca.CertStatusValid {
		t.Errorf("expected VALID, got %d", certStatus)
	}
	if notAfter <= time.Now().Unix() {
		t.Error("expected notAfter in future")
	}

	t.Logf("✅ GetCertificate: userID=%s status=VALID", userID)
}

// TestGetCertificate_NotFound kiểm tra serial không tồn tại
func TestGetCertificate_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, _, _, _, err := svc.GetCertificate("nonexistent-serial-123")

	if err == nil {
		t.Fatal("expected error for nonexistent serial")
	}
	t.Logf("✅ Nonexistent serial correctly returns error: %v", err)
}

// ── Tests: CheckRevocation ────────────────────────────────────

// TestCheckRevocation_ValidCert kiểm tra cert chưa revoke → VALID
func TestCheckRevocation_ValidCert(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "carol@example.com")

	_, serial, _, _ := svc.RegisterUser(csrPEM, "carol@example.com")

	status, reason, revokedAt, err := svc.CheckRevocation(serial)

	if err != nil {
		t.Fatalf("CheckRevocation: %v", err)
	}
	if status != ca.CertStatusValid {
		t.Errorf("expected VALID, got %d", status)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %s", reason)
	}
	if revokedAt != 0 {
		t.Errorf("expected revokedAt=0, got %d", revokedAt)
	}

	t.Logf("✅ Active cert status=VALID")
}

// TestCheckRevocation_AfterRevoke kiểm tra cert sau khi revoke → REVOKED
func TestCheckRevocation_AfterRevoke(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "dave@example.com")

	_, serial, _, _ := svc.RegisterUser(csrPEM, "dave@example.com")

	// Revoke cert
	err := svc.RevokeCertificate(serial, "key_compromised")
	if err != nil {
		t.Fatalf("RevokeCertificate: %v", err)
	}

	// Check revocation
	status, reason, revokedAt, err := svc.CheckRevocation(serial)

	if err != nil {
		t.Fatalf("CheckRevocation after revoke: %v", err)
	}
	if status != ca.CertStatusRevoked {
		t.Errorf("expected REVOKED, got %d", status)
	}
	if reason != "key_compromised" {
		t.Errorf("expected reason=key_compromised, got %s", reason)
	}
	if revokedAt == 0 {
		t.Error("expected revokedAt to be set")
	}

	t.Logf("✅ Revoked cert status=REVOKED reason=%s", reason)
}

// TestRevokeCertificate_AlreadyRevoked kiểm tra revoke 2 lần → lỗi
func TestRevokeCertificate_AlreadyRevoked(t *testing.T) {
	svc := newTestService(t)
	csrPEM, _ := generateValidCSRForUser(t, "eve@example.com")

	_, serial, _, _ := svc.RegisterUser(csrPEM, "eve@example.com")

	// Revoke lần 1
	_ = svc.RevokeCertificate(serial, "reason1")

	// Revoke lần 2 → phải lỗi
	err := svc.RevokeCertificate(serial, "reason2")

	if err == nil {
		t.Fatal("expected error when revoking already-revoked cert")
	}
	t.Logf("✅ Double revoke correctly rejected: %v", err)
}

// ── Tests: Anti-replay / Isolation ───────────────────────────

// TestMultipleUsers_IsolatedCerts kiểm tra mỗi user có cert độc lập
func TestMultipleUsers_IsolatedCerts(t *testing.T) {
	svc := newTestService(t)

	users := []string{"user1@test.com", "user2@test.com", "user3@test.com"}
	serials := make([]string, len(users))

	for i, userID := range users {
		csrPEM, _ := generateValidCSRForUser(t, userID)
		_, serial, _, err := svc.RegisterUser(csrPEM, userID)
		if err != nil {
			t.Fatalf("RegisterUser %s: %v", userID, err)
		}
		serials[i] = serial
	}

	// Tất cả serial phải khác nhau
	serialSet := make(map[string]bool)
	for _, s := range serials {
		if serialSet[s] {
			t.Fatalf("duplicate serial detected: %s", s)
		}
		serialSet[s] = true
	}

	// Revoke user1 không ảnh hưởng user2
	_ = svc.RevokeCertificate(serials[0], "test")

	status, _, _, _ := svc.CheckRevocation(serials[1])
	if status != ca.CertStatusValid {
		t.Error("revoking user1 must not affect user2")
	}

	t.Logf("✅ %d users, all serials unique, revocation isolated", len(users))
}
