package ca

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	// "net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrActiveCertificateExists = errors.New("active certificate already exists for owner")
	ErrAlreadyRevoked          = errors.New("certificate already revoked")
	ErrCertificateNotFound     = errors.New("certificate not found")
	ErrCSRIdentityMismatch     = errors.New("CSR identity does not match request")
	ErrInvalidCSR              = errors.New("invalid CSR")
	ErrInvalidInput            = errors.New("invalid input")
)

type CertStatus string

const (
	CertStatusUnknown CertStatus = "unknown"
	CertStatusActive  CertStatus = "active"
	CertStatusRevoked CertStatus = "revoked"
	CertStatusExpired CertStatus = "expired"
)

type AuditAction string

const (
	AuditIssued            AuditAction = "issued"
	AuditRevoked           AuditAction = "revoked"
	AuditLookedUp          AuditAction = "looked_up"
	AuditRevocationChecked AuditAction = "revocation_checked"
	AuditVerifyCertificate AuditAction = "verify_certificate"
)

type CertificateRecord struct {
	SerialNumber      string     `json:"serial_number"`
	OwnerID           string     `json:"owner_id"`
	SubjectCN         string     `json:"subject_cn"`
	SubjectEmail      string     `json:"subject_email"`
	PublicKeyPEM      string     `json:"public_key_pem"`
	CertificatePEM    string     `json:"certificate_pem"`
	FingerprintSHA256 string     `json:"fingerprint_sha256"`
	NotBefore         time.Time  `json:"not_before"`
	NotAfter          time.Time  `json:"not_after"`
	Status            CertStatus `json:"status"`
	IssuedAt          time.Time  `json:"issued_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	RevocationReason  string     `json:"revocation_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type AuditEvent struct {
	SerialNumber string            `json:"serial_number"`
	Action       AuditAction       `json:"action"`
	PerformedBy  string            `json:"performed_by"`
	Reason       string            `json:"reason,omitempty"`
	PerformedAt  time.Time         `json:"performed_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type RegisterInput struct {
	CSRPem       string
	OwnerID      string
	SubjectCN    string
	SubjectEmail string
	RequestID    string
	PerformedBy  string
}

type VerifyInput struct {
	SerialNumber          string
	RequestID             string
	Caller                string
	IncludeCertificatePEM bool
	IncludePublicKeyPEM   bool
}

type ListFilter struct {
	Status       string
	Email        string
	SerialNumber string
	Limit        int
	Offset       int
	RequestID    string
	PerformedBy  string
}

type Repository interface {
	CreateCertificate(context.Context, CertificateRecord) error
	GetCertificate(context.Context, string) (*CertificateRecord, error)
	ListCertificates(context.Context, ListFilter) ([]CertificateRecord, int, error)
	RevokeCertificate(context.Context, string, string, time.Time) (*CertificateRecord, error)
	AppendAudit(context.Context, AuditEvent) error
}

type CertificateExtensionConfig struct {
	CRLDistributionPoints []string
	OCSPServers           []string
}

type Service struct {
	rootCA           *RootCA
	repository       Repository
	issuedCertsPath  string
	certValidityDays int
	extensions       CertificateExtensionConfig
}

func NewService(rootCA *RootCA, repository Repository, issuedCertsPath string, certValidityDays int) *Service {
	return NewServiceWithExtensionConfig(rootCA, repository, issuedCertsPath, certValidityDays, CertificateExtensionConfig{})
}

func NewServiceWithExtensionConfig(rootCA *RootCA, repository Repository, issuedCertsPath string, certValidityDays int, extensions CertificateExtensionConfig) *Service {
	return &Service{
		rootCA:           rootCA,
		repository:       repository,
		issuedCertsPath:  issuedCertsPath,
		certValidityDays: certValidityDays,
		extensions: CertificateExtensionConfig{
			CRLDistributionPoints: cloneStrings(extensions.CRLDistributionPoints),
			OCSPServers:           cloneStrings(extensions.OCSPServers),
		},
	}
}

func (s *Service) RegisterUser(ctx context.Context, in RegisterInput) (*CertificateRecord, error) {
	in.CSRPem = strings.TrimSpace(in.CSRPem)
	in.OwnerID = strings.TrimSpace(in.OwnerID)
	in.SubjectCN = strings.TrimSpace(in.SubjectCN)
	in.SubjectEmail = strings.TrimSpace(strings.ToLower(in.SubjectEmail))
	in.PerformedBy = defaultActor(in.PerformedBy, "system:ca-service")

	// if in.CSRPem == "" {
	// 	return nil, fmt.Errorf("%w: csr_pem is required", ErrInvalidInput)
	// }
	// if in.OwnerID == "" {
	// 	return nil, fmt.Errorf("%w: owner_id is required", ErrInvalidInput)
	// }
	// if in.SubjectCN == "" {
	// 	return nil, fmt.Errorf("%w: subject_cn is required", ErrInvalidInput)
	// }
	// if _, err := mail.ParseAddress(in.SubjectEmail); err != nil || !strings.Contains(in.SubjectEmail, "@") {
	// 	return nil, fmt.Errorf("%w: subject_email must be a valid plain email", ErrInvalidInput)
	// }

	csr, err := parseAndValidateCSR(in.CSRPem)
	if err != nil {
		return nil, err
	}
	// if err := validateCSRIdentity(csr, in.SubjectCN, in.SubjectEmail); err != nil {
	// 	return nil, err
	// }

	publicKeyPEM, err := marshalPublicKeyPEM(csr.PublicKey)
	if err != nil {
		return nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	notBefore := now.Add(-1 * time.Minute)
	notAfter := now.AddDate(0, 0, s.certValidityDays)
	ownerURI, err := url.Parse("urn:mini-banking:owner:" + url.PathEscape(in.OwnerID))
	if err != nil {
		return nil, fmt.Errorf("build owner URI SAN: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   in.SubjectCN,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  false,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		EmailAddresses:        []string{in.SubjectEmail},
		URIs:                  []*url.URL{ownerURI},
		SubjectKeyId:          computeSKI(csr.PublicKey),
		AuthorityKeyId:        issuerKeyID(s.rootCA),
		CRLDistributionPoints: cloneStrings(s.extensions.CRLDistributionPoints),
		OCSPServer:            cloneStrings(s.extensions.OCSPServers),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, s.rootCA.Certificate, csr.PublicKey, s.rootCA.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse issued certificate: %w", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	serialHex := hex.EncodeToString(cert.SerialNumber.Bytes())
	record := CertificateRecord{
		SerialNumber:      serialHex,
		OwnerID:           in.OwnerID,
		SubjectCN:         in.SubjectCN,
		SubjectEmail:      in.SubjectEmail,
		PublicKeyPEM:      publicKeyPEM,
		CertificatePEM:    certPEM,
		FingerprintSHA256: certificateFingerprint(certDER),
		NotBefore:         cert.NotBefore.UTC(),
		NotAfter:          cert.NotAfter.UTC(),
		Status:            CertStatusActive,
		IssuedAt:          now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repository.CreateCertificate(ctx, record); err != nil {
		return nil, err
	}
	_ = s.repository.AppendAudit(ctx, AuditEvent{
		SerialNumber: serialHex,
		Action:       AuditIssued,
		PerformedBy:  in.PerformedBy,
		PerformedAt:  now,
		Metadata: auditMetadata(map[string]string{
			"request_id": in.RequestID,
			"owner_id":   in.OwnerID,
		}),
	})
	if err := s.saveCertToDisk(serialHex, []byte(certPEM)); err != nil {
		fmt.Printf("[CA] warning: cannot save issued certificate %s: %v\n", serialHex, err)
	}

	return &record, nil
}

func (s *Service) VerifyCertificate(ctx context.Context, in VerifyInput) (*CertificateRecord, error) {
	serial := strings.TrimSpace(in.SerialNumber)
	if serial == "" {
		return nil, fmt.Errorf("%w: serial_number is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCertificate(ctx, serial)
	if err != nil {
		return nil, err
	}
	resolved := s.resolveStatus(*record)
	record.Status = resolved

	action := AuditVerifyCertificate
	if in.Caller == "system:kdc-service" || in.Caller == "system:bank-service" {
		action = AuditRevocationChecked
	}
	_ = s.repository.AppendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		Action:       action,
		PerformedBy:  defaultActor(in.Caller, "system:unknown"),
		PerformedAt:  time.Now().UTC(),
		Metadata: auditMetadata(map[string]string{
			"request_id":              in.RequestID,
			"include_certificate_pem": fmt.Sprintf("%t", in.IncludeCertificatePEM),
			"include_public_key_pem":  fmt.Sprintf("%t", in.IncludePublicKeyPEM),
		}),
	})
	return record, nil
}

func (s *Service) GetCertificate(ctx context.Context, serial string) (*CertificateRecord, error) {
	record, err := s.VerifyCertificate(ctx, VerifyInput{
		SerialNumber:          serial,
		Caller:                "legacy:get-certificate",
		IncludeCertificatePEM: true,
		IncludePublicKeyPEM:   true,
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) CheckRevocation(ctx context.Context, serial string) (*CertificateRecord, error) {
	record, err := s.VerifyCertificate(ctx, VerifyInput{
		SerialNumber: serial,
		Caller:       "legacy:check-revocation",
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) ListCertificates(ctx context.Context, filter ListFilter) ([]CertificateRecord, int, error) {
	filter.PerformedBy = defaultActor(filter.PerformedBy, "admin:unknown")
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	records, total, err := s.repository.ListCertificates(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for i := range records {
		records[i].Status = s.resolveStatus(records[i])
	}
	return records, total, nil
}

func (s *Service) GetCertificateDetail(ctx context.Context, serial, requestID, performedBy string) (*CertificateRecord, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return nil, fmt.Errorf("%w: serial_number is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCertificate(ctx, serial)
	if err != nil {
		return nil, err
	}
	record.Status = s.resolveStatus(*record)
	_ = s.repository.AppendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		Action:       AuditLookedUp,
		PerformedBy:  defaultActor(performedBy, "admin:unknown"),
		PerformedAt:  time.Now().UTC(),
		Metadata: auditMetadata(map[string]string{
			"request_id": requestID,
			"owner_id":   record.OwnerID,
		}),
	})
	return record, nil
}

func (s *Service) RevokeCertificate(ctx context.Context, serial, reason, requestID, performedBy string) (*CertificateRecord, error) {
	serial = strings.TrimSpace(serial)
	reason = strings.TrimSpace(reason)
	if serial == "" {
		return nil, fmt.Errorf("%w: serial_number is required", ErrInvalidInput)
	}
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidInput)
	}
	now := time.Now().UTC()
	record, err := s.repository.RevokeCertificate(ctx, serial, reason, now)
	if err != nil {
		return nil, err
	}
	_ = s.repository.AppendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		Action:       AuditRevoked,
		PerformedBy:  defaultActor(performedBy, "admin:unknown"),
		Reason:       reason,
		PerformedAt:  now,
		Metadata: auditMetadata(map[string]string{
			"request_id": requestID,
			"owner_id":   record.OwnerID,
		}),
	})
	return record, nil
}

func (s *Service) resolveStatus(record CertificateRecord) CertStatus {
	if record.RevokedAt != nil || record.Status == CertStatusRevoked {
		return CertStatusRevoked
	}
	if time.Now().UTC().After(record.NotAfter) {
		return CertStatusExpired
	}
	if record.Status == "" || record.Status == CertStatusUnknown {
		return CertStatusUnknown
	}
	return CertStatusActive
}

func parseAndValidateCSR(csrPEM string) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return nil, fmt.Errorf("%w: cannot decode PEM block", ErrInvalidCSR)
	}
	if block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("%w: expected CERTIFICATE REQUEST, got %s", ErrInvalidCSR, block.Type)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse CSR: %v", ErrInvalidCSR, err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("%w: CSR signature verification failed: %v", ErrInvalidCSR, err)
	}
	rsaPub, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: CSR public key must be RSA", ErrInvalidCSR)
	}
	if rsaPub.N.BitLen() < 2048 {
		return nil, fmt.Errorf("%w: RSA key too short: %d bits", ErrInvalidCSR, rsaPub.N.BitLen())
	}
	return csr, nil
}

func validateCSRIdentity(csr *x509.CertificateRequest, subjectCN, subjectEmail string) error {
	if csr.Subject.CommonName != subjectCN {
		return fmt.Errorf("%w: CSR CommonName %q does not match subject_cn %q", ErrCSRIdentityMismatch, csr.Subject.CommonName, subjectCN)
	}
	for _, email := range csr.EmailAddresses {
		if strings.EqualFold(email, subjectEmail) {
			return nil
		}
	}
	return fmt.Errorf("%w: CSR email SAN must contain %q", ErrCSRIdentityMismatch, subjectEmail)
}

func randomSerial() (*big.Int, error) {
	for {
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return nil, fmt.Errorf("generate serial: %w", err)
		}
		if serial.Sign() > 0 {
			return serial, nil
		}
	}
}

func marshalPublicKeyPEM(publicKey any) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

func certificateFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func computeSKI(publicKey any) []byte {
	pubDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil
	}
	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(pubDER, &spki); err != nil {
		return nil
	}
	sum := sha1.Sum(spki.SubjectPublicKey.Bytes)
	return sum[:]
}

func issuerKeyID(rootCA *RootCA) []byte {
	if rootCA == nil || rootCA.Certificate == nil {
		return nil
	}
	if len(rootCA.Certificate.SubjectKeyId) > 0 {
		return append([]byte(nil), rootCA.Certificate.SubjectKeyId...)
	}
	return computeSKI(rootCA.Certificate.PublicKey)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func defaultActor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func auditMetadata(values map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Service) saveCertToDisk(serialHex string, pemBytes []byte) error {
	if s.issuedCertsPath == "" {
		return nil
	}
	if err := os.MkdirAll(s.issuedCertsPath, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.issuedCertsPath, serialHex+".pem"), pemBytes, 0644)
}
