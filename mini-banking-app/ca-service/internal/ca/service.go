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
	"net/mail"
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
	ErrCertificateNotRevokable = errors.New("certificate type cannot be revoked through client revoke flow")
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

const (
	CertTypeRootCA         = "root_ca"
	CertTypeIntermediateCA = "intermediate_ca"
	CertTypeServiceTLS     = "service_tls"
	CertTypeClient         = "client"
	RootCAID               = "root-ca"
	ClientCAID             = "client-ca"
)

const (
	IssuerRoleRootCA   = "root_ca"
	IssuerRoleClientCA = "client_ca"
	IssuerStatusActive = "active"
)

type AuditAction string

const (
	AuditIssued            AuditAction = "issued"
	AuditRevoked           AuditAction = "revoked"
	AuditLookedUp          AuditAction = "looked_up"
	AuditVerifyCertificate AuditAction = "verify_certificate"
	// Written by the issuer provisioning flow / chain checks (may also be
	// inserted directly by provisioning scripts); listed here so the audit
	// read API accepts every action the DB CHECK constraint allows.
	AuditIssuerProvisioned AuditAction = "issuer_provisioned"
	AuditChainVerified     AuditAction = "chain_verified"
)

type CertificateRecord struct {
	SerialNumber      string       `json:"serial_number"`
	CertType          string       `json:"cert_type"`
	IssuerID          string       `json:"issuer_id"`
	IssuerCommonName  string       `json:"issuer_common_name"`
	IssuerSerial      string       `json:"issuer_serial_number"`
	OwnerID           string       `json:"owner_id"`
	Role              IdentityRole `json:"role"`
	SubjectCN         string       `json:"subject_cn"`
	SubjectEmail      string       `json:"subject_email"`
	PublicKeyPEM      string       `json:"public_key_pem"`
	CertificatePEM    string       `json:"certificate_pem"`
	ChainPEM          string       `json:"chain_pem,omitempty"`
	ChainFingerprints []string     `json:"chain_fingerprints,omitempty"`
	FingerprintSHA256 string       `json:"fingerprint_sha256"`
	IsCA              bool         `json:"is_ca"`
	KeyUsage          []string     `json:"key_usage,omitempty"`
	ExtendedKeyUsage  []string     `json:"extended_key_usage,omitempty"`
	NotBefore         time.Time    `json:"not_before"`
	NotAfter          time.Time    `json:"not_after"`
	Status            CertStatus   `json:"status"`
	IssuedAt          time.Time    `json:"issued_at"`
	RevokedAt         *time.Time   `json:"revoked_at,omitempty"`
	RevocationReason  string       `json:"revocation_reason,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type IssuerRecord struct {
	IssuerID          string    `json:"issuer_id"`
	ParentIssuerID    string    `json:"parent_issuer_id,omitempty"`
	CommonName        string    `json:"common_name"`
	CertRole          string    `json:"cert_role"`
	SerialNumber      string    `json:"serial_number"`
	CertificatePEM    string    `json:"certificate_pem"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	SubjectKeyID      string    `json:"subject_key_id,omitempty"`
	AuthorityKeyID    string    `json:"authority_key_id,omitempty"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AuditEvent struct {
	SerialNumber string            `json:"serial_number"`
	CertType     string            `json:"cert_type,omitempty"`
	IssuerID     string            `json:"issuer_id,omitempty"`
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
	Role         IdentityRole
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
	CertType     string
	IssuerID     string
	OwnerID      string
	SubjectEmail string
	SerialNumber string
	Limit        int
	Offset       int
	RequestID    string
	PerformedBy  string
}

// AuditFilter selects audit events for the admin read API. Zero values mean
// "no filter"; From/To bound performed_at as a half-open range [From, To).
type AuditFilter struct {
	SerialNumber string
	Action       string
	PerformedBy  string
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
}

type Repository interface {
	UpsertIssuer(context.Context, IssuerRecord) error
	CreateCertificate(context.Context, CertificateRecord) error
	GetCertificate(context.Context, string) (*CertificateRecord, error)
	ListCertificates(context.Context, ListFilter) ([]CertificateRecord, int, error)
	RevokeCertificate(context.Context, string, string, time.Time) (*CertificateRecord, error)
	AppendAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, AuditFilter) ([]AuditEvent, int, error)
}

type CertificateExtensionConfig struct {
	CRLDistributionPoints []string
	OCSPServers           []string
}

type Service struct {
	rootCA           *RootCA
	signerCA         *RootCA
	repository       Repository
	issuedCertsPath  string
	certValidityDays int
	extensions       CertificateExtensionConfig
}

func NewService(signerCA *RootCA, repository Repository, issuedCertsPath string, certValidityDays int) *Service {
	return NewServiceWithExtensionConfig(signerCA, repository, issuedCertsPath, certValidityDays, CertificateExtensionConfig{})
}

func NewServiceWithExtensionConfig(signerCA *RootCA, repository Repository, issuedCertsPath string, certValidityDays int, extensions CertificateExtensionConfig) *Service {
	return NewServiceWithRootCAAndExtensionConfig(nil, signerCA, repository, issuedCertsPath, certValidityDays, extensions)
}

func NewServiceWithRootCAAndExtensionConfig(rootCA, signerCA *RootCA, repository Repository, issuedCertsPath string, certValidityDays int, extensions CertificateExtensionConfig) *Service {
	return &Service{
		rootCA:           rootCA,
		signerCA:         signerCA,
		repository:       repository,
		issuedCertsPath:  issuedCertsPath,
		certValidityDays: certValidityDays,
		extensions: CertificateExtensionConfig{
			CRLDistributionPoints: cloneStrings(extensions.CRLDistributionPoints),
			OCSPServers:           cloneStrings(extensions.OCSPServers),
		},
	}
}

func (s *Service) InitializeIssuerChain(ctx context.Context) error {
	return s.ensureIssuerChain(ctx, time.Now().UTC())
}

func (s *Service) RegisterUser(ctx context.Context, in RegisterInput) (*CertificateRecord, error) {
	in.CSRPem = strings.TrimSpace(in.CSRPem)
	in.OwnerID = strings.TrimSpace(in.OwnerID)
	in.SubjectCN = strings.TrimSpace(in.SubjectCN)
	in.SubjectEmail = strings.TrimSpace(strings.ToLower(in.SubjectEmail))
	in.PerformedBy = defaultActor(in.PerformedBy, "system:ca-service")
	role, validRole := NormalizeIdentityRole(in.Role)
	if !validRole {
		return nil, fmt.Errorf("%w: unsupported identity role %q", ErrInvalidInput, in.Role)
	}
	in.Role = role

	if in.CSRPem == "" {
		return nil, fmt.Errorf("%w: csr_pem is required", ErrInvalidInput)
	}
	if in.OwnerID == "" {
		return nil, fmt.Errorf("%w: owner_id is required", ErrInvalidInput)
	}
	if in.SubjectCN == "" {
		return nil, fmt.Errorf("%w: subject_cn is required", ErrInvalidInput)
	}
	if _, err := mail.ParseAddress(in.SubjectEmail); err != nil || !strings.Contains(in.SubjectEmail, "@") {
		return nil, fmt.Errorf("%w: subject_email must be a valid plain email", ErrInvalidInput)
	}

	csr, err := parseAndValidateCSR(in.CSRPem)
	if err != nil {
		return nil, err
	}
	if err := validateCSRIdentity(csr, in.SubjectCN, in.SubjectEmail); err != nil {
		return nil, err
	}

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
		AuthorityKeyId:        issuerKeyID(s.signerCA),
		CRLDistributionPoints: cloneStrings(s.extensions.CRLDistributionPoints),
		OCSPServer:            cloneStrings(s.extensions.OCSPServers),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, s.signerCA.Certificate, csr.PublicKey, s.signerCA.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse issued certificate: %w", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	serialHex := hex.EncodeToString(cert.SerialNumber.Bytes())
	chainPEM, chainFingerprints := s.issuerChain()
	record := CertificateRecord{
		SerialNumber:      serialHex,
		CertType:          CertTypeClient,
		IssuerID:          ClientCAID,
		IssuerCommonName:  s.signerCA.Certificate.Subject.CommonName,
		IssuerSerial:      hex.EncodeToString(s.signerCA.Certificate.SerialNumber.Bytes()),
		OwnerID:           in.OwnerID,
		Role:              in.Role,
		SubjectCN:         in.SubjectCN,
		SubjectEmail:      in.SubjectEmail,
		PublicKeyPEM:      publicKeyPEM,
		CertificatePEM:    certPEM,
		ChainPEM:          chainPEM,
		ChainFingerprints: chainFingerprints,
		FingerprintSHA256: certificateFingerprint(certDER),
		IsCA:              false,
		KeyUsage:          []string{"digitalSignature", "keyEncipherment"},
		ExtendedKeyUsage:  []string{"clientAuth"},
		NotBefore:         cert.NotBefore.UTC(),
		NotAfter:          cert.NotAfter.UTC(),
		Status:            CertStatusActive,
		IssuedAt:          now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.ensureIssuerChain(ctx, now); err != nil {
		return nil, err
	}
	if err := s.repository.CreateCertificate(ctx, record); err != nil {
		return nil, err
	}
	s.appendAudit(ctx, AuditEvent{
		SerialNumber: serialHex,
		CertType:     CertTypeClient,
		IssuerID:     ClientCAID,
		Action:       AuditIssued,
		PerformedBy:  in.PerformedBy,
		PerformedAt:  now,
		Metadata: auditMetadata(map[string]string{
			"request_id": in.RequestID,
			"owner_id":   in.OwnerID,
			"role":       string(in.Role),
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
	role, validRole := NormalizeIdentityRole(record.Role)
	if !validRole {
		return nil, fmt.Errorf("invalid persisted identity role %q", record.Role)
	}
	record.Role = role
	resolved := s.resolveStatus(*record)
	record.Status = resolved

	// Revocation checks from KDC/Bank and generic verifies share one action
	// ("verify_certificate", per the DB enum); callers are told apart by
	// performed_by.
	action := AuditVerifyCertificate
	s.appendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		CertType:     record.CertType,
		IssuerID:     record.IssuerID,
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
	filter.PerformedBy = defaultActor(filter.PerformedBy, "admin-ca:unknown")
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
	s.appendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		CertType:     record.CertType,
		IssuerID:     record.IssuerID,
		Action:       AuditLookedUp,
		PerformedBy:  defaultActor(performedBy, "admin-ca:unknown"),
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
	existing, err := s.repository.GetCertificate(ctx, serial)
	if err != nil {
		return nil, err
	}
	if existing.CertType != CertTypeClient {
		return nil, fmt.Errorf("%w: cert_type=%s serial_number=%s", ErrCertificateNotRevokable, existing.CertType, serial)
	}
	now := time.Now().UTC()
	record, err := s.repository.RevokeCertificate(ctx, serial, reason, now)
	if err != nil {
		return nil, err
	}
	s.appendAudit(ctx, AuditEvent{
		SerialNumber: serial,
		CertType:     record.CertType,
		IssuerID:     record.IssuerID,
		Action:       AuditRevoked,
		PerformedBy:  defaultActor(performedBy, "admin-ca:unknown"),
		Reason:       reason,
		PerformedAt:  now,
		Metadata: auditMetadata(map[string]string{
			"request_id": requestID,
			"owner_id":   record.OwnerID,
		}),
	})
	return record, nil
}

// ListAuditEvents is the read side of the audit log for the admin dashboard.
// It never writes an audit event itself to avoid feedback loops.
func (s *Service) ListAuditEvents(ctx context.Context, filter AuditFilter) ([]AuditEvent, int, error) {
	filter.SerialNumber = strings.TrimSpace(filter.SerialNumber)
	filter.PerformedBy = strings.TrimSpace(filter.PerformedBy)
	filter.Action = strings.TrimSpace(strings.ToLower(filter.Action))
	switch filter.Action {
	case "", string(AuditIssued), string(AuditRevoked), string(AuditLookedUp),
		string(AuditVerifyCertificate), string(AuditIssuerProvisioned), string(AuditChainVerified):
	default:
		return nil, 0, fmt.Errorf("%w: unsupported audit action %q", ErrInvalidInput, filter.Action)
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		return nil, 0, fmt.Errorf("%w: audit filter to must not be before from", ErrInvalidInput)
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repository.ListAudit(ctx, filter)
}

// appendAudit records an event on a best-effort basis: storage failures are
// logged but never propagated, so auditing cannot break the main request path.
func (s *Service) appendAudit(ctx context.Context, event AuditEvent) {
	if err := s.repository.AppendAudit(ctx, event); err != nil {
		fmt.Printf("[CA] warning: cannot append audit event action=%s serial=%s: %v\n", event.Action, event.SerialNumber, err)
	}
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

func issuerKeyID(issuerCA *RootCA) []byte {
	if issuerCA == nil || issuerCA.Certificate == nil {
		return nil
	}
	if len(issuerCA.Certificate.SubjectKeyId) > 0 {
		return append([]byte(nil), issuerCA.Certificate.SubjectKeyId...)
	}
	return computeSKI(issuerCA.Certificate.PublicKey)
}

func (s *Service) ensureIssuerChain(ctx context.Context, now time.Time) error {
	if s.repository == nil || s.signerCA == nil || s.signerCA.Certificate == nil {
		return nil
	}
	now = now.UTC()
	if s.rootCA != nil && s.rootCA.Certificate != nil {
		if err := s.repository.UpsertIssuer(ctx, issuerRecordFromCA(RootCAID, "", IssuerRoleRootCA, s.rootCA, now)); err != nil {
			return fmt.Errorf("store Root CA issuer metadata: %w", err)
		}
	}

	if err := s.repository.UpsertIssuer(ctx, issuerRecordFromCA(ClientCAID, RootCAID, IssuerRoleClientCA, s.signerCA, now)); err != nil {
		return fmt.Errorf("store Client CA issuer metadata: %w", err)
	}
	return nil
}

func issuerRecordFromCA(issuerID, parentIssuerID, role string, ca *RootCA, now time.Time) IssuerRecord {
	cert := ca.Certificate
	return IssuerRecord{
		IssuerID:          issuerID,
		ParentIssuerID:    parentIssuerID,
		CommonName:        cert.Subject.CommonName,
		CertRole:          role,
		SerialNumber:      hex.EncodeToString(cert.SerialNumber.Bytes()),
		CertificatePEM:    string(ca.CertPEM),
		FingerprintSHA256: certificateFingerprint(cert.Raw),
		SubjectKeyID:      hex.EncodeToString(cert.SubjectKeyId),
		AuthorityKeyID:    hex.EncodeToString(cert.AuthorityKeyId),
		NotBefore:         cert.NotBefore.UTC(),
		NotAfter:          cert.NotAfter.UTC(),
		Status:            IssuerStatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func (s *Service) issuerChain() (string, []string) {
	if s.signerCA == nil || s.signerCA.Certificate == nil {
		return "", nil
	}
	var chain strings.Builder
	var fingerprints []string

	chain.WriteString(strings.TrimSpace(string(s.signerCA.CertPEM)))
	chain.WriteString("\n")
	fingerprints = append(fingerprints, certificateFingerprint(s.signerCA.Certificate.Raw))

	if s.rootCA != nil && s.rootCA.Certificate != nil {
		chain.WriteString(strings.TrimSpace(string(s.rootCA.CertPEM)))
		chain.WriteString("\n")
		fingerprints = append(fingerprints, certificateFingerprint(s.rootCA.Certificate.Raw))
	}

	return chain.String(), fingerprints
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
