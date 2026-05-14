package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// RootCA đại diện cho Root Certificate Authority.
// Giữ private key và certificate của CA để ký cert cho user.
type RootCA struct {
	PrivateKey  *rsa.PrivateKey
	Certificate *x509.Certificate
	CertPEM     []byte
}

// LoadOrCreate tải Root CA từ disk nếu đã tồn tại,
// hoặc tự tạo mới (self-signed) nếu chưa có.
//
// Đây là bước khởi động đầu tiên của CA Service.
// Trong production, key nên được bảo vệ bằng HSM hoặc
// được inject qua Kubernetes Secret — không lưu trên disk thường.
func LoadOrCreate(keyPath, certPath string) (*RootCA, error) {
	// Đảm bảo thư mục tồn tại
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	// Kiểm tra cả hai file đã tồn tại chưa
	keyExists := fileExists(keyPath)
	certExists := fileExists(certPath)

	if keyExists && certExists {
		return loadFromDisk(keyPath, certPath)
	}

	// Một trong hai thiếu → tạo lại cả bộ để đảm bảo nhất quán
	fmt.Println("[CA] Root CA not found, generating new self-signed Root CA...")
	return generateAndSave(keyPath, certPath)
}

// loadFromDisk đọc key và cert đã có trên disk.
func loadFromDisk(keyPath, certPath string) (*RootCA, error) {
	// Load private key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("decode CA key PEM: invalid format")
	}
	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Fallback: thử PKCS1 (key cũ có thể dùng format này)
		rsaKey, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse CA key (PKCS8: %v, PKCS1: %v)", err, err2)
		}
		privKey = rsaKey
	}
	rsaKey, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("CA key is not RSA")
	}

	// Load certificate
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	fmt.Printf("[CA] Loaded Root CA from disk (Subject: %s)\n", cert.Subject.CommonName)
	return &RootCA{
		PrivateKey:  rsaKey,
		Certificate: cert,
		CertPEM:     certPEM,
	}, nil
}

// generateAndSave tạo Root CA mới và lưu xuống disk.
func generateAndSave(keyPath, certPath string) (*RootCA, error) {
	// Sinh RSA-4096 private key
	// 4096-bit vì đây là Root CA — ưu tiên bảo mật hơn tốc độ
	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	// Tạo serial number ngẫu nhiên cho Root CA cert, đảm bảo serial number > 0
	var serial *big.Int
	for {
		var err error
		serial, err = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return nil, fmt.Errorf("generate serial: %w", err)
		}
		if serial.Sign() > 0 {
			break
		}
		// Xác suất 1/2^128 — log để biết nếu lạ lùng xảy ra
		fmt.Println("[CA] Warning: serial=0 generated, retrying...")
	}

	// Template cho Root CA certificate
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   "Mini_App_Banking Root CA",
		},
		NotBefore: time.Now().UTC(),
		NotAfter:  time.Now().UTC().AddDate(10, 0, 0), // 10 năm

		// CA constraints — BẮT BUỘC cho Root CA
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0, // Không cho phép intermediate CA
		MaxPathLenZero:        true,

		// Key usage của CA: chỉ ký cert và CRL
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	// Self-sign: CA tự ký cert của chính mình
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("create root cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse generated cert: %w", err)
	}

	// Encode PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPKCS8, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyPKCS8})

	// Lưu key với permission 0600 (chỉ owner đọc được)
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write CA key: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write CA cert: %w", err)
	}

	fmt.Printf("[CA] Generated new Root CA → %s\n", cert.Subject.CommonName)
	fmt.Printf("[CA] Key saved to: %s (permission 0600)\n", keyPath)
	fmt.Printf("[CA] Cert saved to: %s\n", certPath)

	return &RootCA{
		PrivateKey:  privKey,
		Certificate: cert,
		CertPEM:     certPEM,
	}, nil
}

// parseCertPEM parse PEM-encoded certificate.
func parseCertPEM(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

// fileExists kiểm tra file có tồn tại không.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
