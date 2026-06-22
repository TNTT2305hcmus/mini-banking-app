package kdc_test

// fixture_test.go
//
// Khởi tạo toàn bộ dữ liệu dùng chung cho test suite mà KHÔNG cần
// thêm bất kỳ file nào vào production code.
//
// Chiến lược:
//   - TestMain set env var dummy để package init (var ENV = LoadEnv()) không Fatalf.
//   - newFixture sinh key ephemeral, build KDCKeys trực tiếp,
//     rồi gọi NewServiceForTest để inject — không đọc file thật.
//
// YÊU CẦU: file service_for_test.go phải nằm trong internal/kdc/
//           (xem file đính kèm cùng output).

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	capb "mini_banking/pkg/pb/ca"

	"google.golang.org/grpc"
)

// ─────────────────────────────────────────────────────────────────
// fakeCAClient — stub gRPC, trả certPEM mà không cần CA Service thật
// ─────────────────────────────────────────────────────────────────

type fakeCAClient struct {
	capb.CAServiceClient
	certPEM []byte
}

func (f *fakeCAClient) GetCertificate(
	_ context.Context,
	_ *capb.GetCertificateRequest,
	_ ...grpc.CallOption,
) (*capb.GetCertificateResponse, error) {
	return &capb.GetCertificateResponse{
		CertificatePem: string(f.certPEM),
	}, nil
}

func (f *fakeCAClient) VerifyCertificate(
	_ context.Context,
	_ *capb.VerifyCertificateRequest,
	_ ...grpc.CallOption,
) (*capb.VerifyCertificateResponse, error) {
	return &capb.VerifyCertificateResponse{
		Status:         capb.CertStatus_CERT_STATUS_ACTIVE,
		CertificatePem: string(f.certPEM),
		NotBeforeUnix:  time.Now().Add(-time.Minute).Unix(),
		NotAfterUnix:   time.Now().Add(time.Hour).Unix(),
	}, nil
}

// ─────────────────────────────────────────────────────────────────
// testFixture — bundle tất cả deps cần thiết cho một test
// ─────────────────────────────────────────────────────────────────

type testFixture struct {
	svc        *ASService
	clientPriv *rsa.PrivateKey
	kdcPriv    *rsa.PrivateKey
	ktgsKey    []byte
}

// newFixture sinh fixture hoàn chỉnh — fail-fast nếu có lỗi.
func newFixture(t *testing.T) *testFixture {
	t.Helper()

	clientPriv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("generate client RSA key: %v", err)
	}

	kdcPriv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate KDC RSA key: %v", err)
	}

	ktgsKey := make([]byte, 32)
	if _, err := rand.Read(ktgsKey); err != nil {
		t.Fatalf("generate K_tgs: %v", err)
	}

	certPEM := makeSelfSignedCert(t, clientPriv)

	svc := NewServiceForTest(
		&fakeCAClient{certPEM: certPEM},
		nil,
		&KDCKeys{
			KTGSKey:    ktgsKey,
			PrivateKey: kdcPriv,
			RawPrivKey: nil,
		},
	)

	return &testFixture{
		svc:        svc,
		clientPriv: clientPriv,
		kdcPriv:    kdcPriv,
		ktgsKey:    ktgsKey,
	}
}

// ─────────────────────────────────────────────────────────────────
// Setup — set env var tối thiểu để var ENV = LoadEnv() không Fatalf
// ─────────────────────────────────────────────────────────────────

var testTempDir string

var _ = func() int {
	var err error
	testTempDir, err = os.MkdirTemp("", "kdc-test-*")
	if err != nil {
		panic("cannot create temp dir: " + err.Error())
	}

	// Ghi K_tgs dummy 32 byte
	ktgsPath := filepath.Join(testTempDir, "k_tgs.key")
	dummyKey := make([]byte, 32)
	os.WriteFile(ktgsPath, dummyKey, 0600)

	// Ghi RSA private key dummy (PKCS#1 PEM)
	dummyRSA, _ := rsa.GenerateKey(rand.Reader, 2048)
	privPath := filepath.Join(testTempDir, "kdc_private.pem")
	privDER := x509.MarshalPKCS1PrivateKey(dummyRSA)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})
	os.WriteFile(privPath, privPEM, 0600)

	os.Setenv("GRPC_PORT", "50051")
	os.Setenv("CA_PORT", "50052")
	os.Setenv("TGT_EXP", "30m")
	os.Setenv("K_TGS_PATH", ktgsPath)
	os.Setenv("KDC_PRIVATE_KEY_PATH", privPath)
	os.Setenv("REDIS_URI", "redis://localhost:6379")

	return 0
}()

func TestMain(m *testing.M) {
	code := m.Run()
	os.RemoveAll(testTempDir)
	os.Exit(code)
}

// ─────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────

func makeSelfSignedCert(t *testing.T, priv *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject: pkix.Name{
			CommonName:   "test-client",
			Organization: []string{"TestOrg"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create self-signed cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
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
