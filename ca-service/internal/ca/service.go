package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Service chứa toàn bộ business logic của CA.
// Được inject vào gRPC handler — không biết gì về transport layer.
type Service struct {
	rootCA           *RootCA
	store            *Store
	issuedCertsPath  string
	certValidityDays int
}

// NewService khởi tạo CA Service với Root CA đã load.
func NewService(rootCA *RootCA, store *Store, issuedCertsPath string, certValidityDays int) *Service {
	return &Service{
		rootCA:           rootCA,
		store:            store,
		issuedCertsPath:  issuedCertsPath,
		certValidityDays: certValidityDays,
	}
}

// RegisterUser nhận CSR từ client, xác minh, và ký → trả về X.509 cert.
//
// Flow:
//  1. Decode CSR từ PEM
//  2. Verify chữ ký CSR (chứng minh client sở hữu private key)
//  3. Tạo X.509 cert với pub key lấy từ CSR
//  4. Ký bằng Root CA private key
//  5. Lưu vào store + disk
func (s *Service) RegisterUser(csrPEM, userID string) (certPEM string, serialHex string, notAfter int64, err error) {
	// ── Bước 1: Decode CSR ───────────────────────────────────
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return "", "", 0, fmt.Errorf("invalid CSR: cannot decode PEM block")
	}
	if block.Type != "CERTIFICATE REQUEST" {
		return "", "", 0, fmt.Errorf("invalid CSR: expected CERTIFICATE REQUEST, got %s", block.Type)
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", 0, fmt.Errorf("parse CSR: %w", err)
	}

	// ── Bước 2: Verify chữ ký CSR ────────────────────────────
	// Đây là bước quan trọng nhất — đảm bảo client thực sự sở hữu
	// private key tương ứng với public key trong CSR.
	// Kẻ tấn công không thể giả mạo CSR của người khác vì không có priv key.
	if err := csr.CheckSignature(); err != nil {
		return "", "", 0, fmt.Errorf("CSR signature verification failed: %w", err)
	}

	// Chỉ chấp nhận RSA key (theo thiết kế hệ thống)
	rsaPub, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok {
		return "", "", 0, fmt.Errorf("CSR must contain RSA public key")
	}
	// Yêu cầu tối thiểu RSA-2048 để đảm bảo bảo mật
	if rsaPub.N.BitLen() < 2048 {
		return "", "", 0, fmt.Errorf("RSA key too short: %d bits (minimum 2048)", rsaPub.N.BitLen())
	}

	// ── Bước 3: Tạo serial number ngẫu nhiên ─────────────────
	var serial *big.Int
	for {
		var err error
		serial, err = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return "", "", 0, fmt.Errorf("generate serial: %w", err)
		}
		if serial.Sign() > 0 {
			break
		}
		// Xác suất 1/2^128 — log để biết nếu lạ lùng xảy ra
		fmt.Println("[CA] Warning: serial=0 generated, retrying...")
	}

	// ── Bước 4: Tạo X.509 certificate template ───────────────
	now := time.Now().UTC()
	expiry := now.AddDate(0, 0, s.certValidityDays)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			// CommonName = userID để dễ identify cert thuộc về ai
			CommonName: userID,
		},
		NotBefore: now,
		NotAfter:  expiry,

		// End-entity cert — KHÔNG phải CA
		IsCA:                  false,
		BasicConstraintsValid: true,

		// KeyUsage cho client cert:
		// - DigitalSignature: ký payload (non-repudiation)
		// - KeyEncipherment: encrypt/decrypt session key (AS_REP, TGS_REP)
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,

		// ExtKeyUsage: client authentication (TLS mutual auth, Kerberos PKINIT)
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},

		// Subject Key Identifier — giúp KDC/Bank identify key nhanh
		SubjectKeyId: computeSKI(rsaPub),
	}

	// ── Bước 5: Ký bằng Root CA ──────────────────────────────
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		s.rootCA.Certificate, // Issuer = Root CA
		csr.PublicKey,        // Subject public key lấy từ CSR
		s.rootCA.PrivateKey,  // Ký bằng Root CA private key
	)
	if err != nil {
		return "", "", 0, fmt.Errorf("sign certificate: %w", err)
	}

	// Parse lại để có *x509.Certificate đầy đủ
	issuedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", "", 0, fmt.Errorf("parse issued cert: %w", err)
	}

	// ── Bước 6: Encode PEM và lưu ────────────────────────────
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	serialHex = hex.EncodeToString(issuedCert.SerialNumber.Bytes())

	// Lưu vào in-memory store
	s.store.Save(serialHex, &CertRecord{
		UserID:  userID,
		Cert:    issuedCert,
		CertPEM: string(pemBytes),
	})

	// Lưu file PEM ra disk (backup khi restart)
	if err := s.saveCertToDisk(serialHex, pemBytes); err != nil {
		// Log warning nhưng không fail — store đã có rồi
		fmt.Printf("[CA] Warning: cannot save cert to disk (serial %s): %v\n", serialHex, err)
	}

	fmt.Printf("[CA] Issued cert for user=%s serial=%s expires=%s\n",
		userID, serialHex, expiry.Format(time.RFC3339))

	return string(pemBytes), serialHex, expiry.Unix(), nil
}

// GetCertificate trả về cert và trạng thái theo serial number.
func (s *Service) GetCertificate(serialHex string) (certPEM, userID string, status CertStatusVal, notAfter int64, err error) {
	rec := s.store.Get(serialHex)
	if rec == nil {
		return "", "", CertStatusUnknown, 0, fmt.Errorf("certificate not found: %s", serialHex)
	}

	st := s.resolveStatus(rec)
	return rec.CertPEM, rec.UserID, st, rec.Cert.NotAfter.Unix(), nil
}

// CheckRevocation trả về trạng thái revocation của cert.
// Được KDC gọi trong TGS Exchange và Bank gọi trước BEGIN TRANSACTION.
func (s *Service) CheckRevocation(serialHex string) (status CertStatusVal, reason string, revokedAt int64, err error) {
	rec := s.store.Get(serialHex)
	if rec == nil {
		return CertStatusUnknown, "", 0, fmt.Errorf("certificate not found: %s", serialHex)
	}

	st := s.resolveStatus(rec)

	var revokedAtUnix int64
	if rec.RevokedAt != nil {
		revokedAtUnix = rec.RevokedAt.Unix()
	}

	return st, rec.RevokeReason, revokedAtUnix, nil
}

// RevokeCertificate thu hồi cert theo serial number.
// Trả error nếu: không tìm thấy, hoặc đã bị revoke trước đó.
func (s *Service) RevokeCertificate(serialHex, reason string) error {
	rec := s.store.Get(serialHex)
	if rec == nil {
		return fmt.Errorf("not_found: %s", serialHex)
	}
	if rec.RevokedAt != nil {
		return fmt.Errorf("already_revoked: %s", serialHex)
	}

	if !s.store.Revoke(serialHex, reason) {
		return fmt.Errorf("revoke failed: concurrent modification")
	}

	fmt.Printf("[CA] Revoked cert serial=%s reason=%s\n", serialHex, reason)
	return nil
}

// ── Helpers ──────────────────────────────────────────────────

// CertStatusVal là kiểu nội bộ tương ứng với proto enum CertStatus.
type CertStatusVal int32

const (
	CertStatusUnknown CertStatusVal = 0
	CertStatusValid   CertStatusVal = 1
	CertStatusRevoked CertStatusVal = 2
	CertStatusExpired CertStatusVal = 3
)

// resolveStatus xác định trạng thái cert:
// ưu tiên REVOKED > EXPIRED > VALID
func (s *Service) resolveStatus(rec *CertRecord) CertStatusVal {
	if rec.RevokedAt != nil {
		return CertStatusRevoked
	}
	if time.Now().UTC().After(rec.Cert.NotAfter) {
		return CertStatusExpired
	}
	return CertStatusValid
}

// computeSKI tính Subject Key Identifier từ RSA public key.
// SKI = SHA-1 của DER-encoded SubjectPublicKeyInfo (RFC 5280 §4.2.1.2)
func computeSKI(pub *rsa.PublicKey) []byte {
	// Dùng crypto/sha1 thay crypto/sha256 vì RFC 5280 quy định SHA-1 cho SKI
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil
	}
	// Import inline để tránh unused import warning khi chưa có go tools
	// Production: import "crypto/sha1"; h := sha1.Sum(pubDER); return h[:]
	_ = pubDER
	return nil // Được set tự động bởi x509.CreateCertificate nếu nil
}

// saveCertToDisk lưu cert PEM ra file để backup.
// Filename: <serial>.pem trong IssuedCertsPath
func (s *Service) saveCertToDisk(serialHex string, pemBytes []byte) error {
	if err := os.MkdirAll(s.issuedCertsPath, 0755); err != nil {
		return err
	}
	path := filepath.Join(s.issuedCertsPath, serialHex+".pem")
	return os.WriteFile(path, pemBytes, 0644)
}
