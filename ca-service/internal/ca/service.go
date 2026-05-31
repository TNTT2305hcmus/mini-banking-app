package ca

/**
 * @title CA Service - Core Business Logic
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Processing Certificate Signing Requests (CSRs), verifying signatures, issuing X.509 certificates, and handling revocation rules.
 */

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
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
	"time"
)

var ErrCSRIdentityMismatch = errors.New("CSR identity does not match requested user")

/**
 * @description Service contains the entire business logic of the CA.
 * @note It is injected into the gRPC handler — unaware of the transport layer.
 *
 * @typedef {Object} Service
 * @property {*RootCA} rootCA - The loaded Root CA.
 * @property {*Store} store - The certificate store.
 * @property {string} issuedCertsPath - The directory path to backup issued certificates.
 * @property {int} certValidityDays - The number of days a newly issued certificate remains valid.
 */
type Service struct {
	rootCA           *RootCA
	store            *Store
	issuedCertsPath  string
	certValidityDays int
	extensions       CertificateExtensionConfig
}

/**
 * @description CertificateExtensionConfig contains endpoint URLs embedded into issued certificates.
 * @note The project currently checks revocation through CA gRPC; CRL/OCSP URLs are certificate metadata
 * @note for interoperability and future external publication.
 *
 * @typedef {Object} CertificateExtensionConfig
 * @property {[]string} CRLDistributionPoints - CRL URLs to embed in issued certificates.
 * @property {[]string} OCSPServers - OCSP responder URLs to embed in issued certificates.
 */
type CertificateExtensionConfig struct {
	CRLDistributionPoints []string
	OCSPServers           []string
}

/**
 * @descriptio NewService initializes the CA Service with the loaded Root CA.
 *
 * @function NewService
 * @param {*RootCA} rootCA - The loaded Root CA.
 * @param {*Store} store - The certificate store.
 * @param {string} issuedCertsPath - Directory path for saving issued certificates.
 * @param {int} certValidityDays - Certificate validity period in days.
 * @returns {*Service} A new instance of the CA Service.
 */
func NewService(rootCA *RootCA, store *Store, issuedCertsPath string, certValidityDays int) *Service {
	return NewServiceWithExtensionConfig(rootCA, store, issuedCertsPath, certValidityDays, CertificateExtensionConfig{})
}

/**
 * @descriptio NewServiceWithExtensionConfig initializes the CA Service with certificate endpoint extensions.
 *
 * @function NewServiceWithExtensionConfig
 * @param {*RootCA} rootCA - The loaded Root CA.
 * @param {*Store} store - The certificate store.
 * @param {string} issuedCertsPath - Directory path for saving issued certificates.
 * @param {int} certValidityDays - Certificate validity period in days.
 * @param {CertificateExtensionConfig} extensions - Optional CRL/OCSP endpoint extensions.
 * @returns {*Service} A new instance of the CA Service.
 */
func NewServiceWithExtensionConfig(rootCA *RootCA, store *Store, issuedCertsPath string, certValidityDays int, extensions CertificateExtensionConfig) *Service {
	return &Service{
		rootCA:           rootCA,
		store:            store,
		issuedCertsPath:  issuedCertsPath,
		certValidityDays: certValidityDays,
		extensions: CertificateExtensionConfig{
			CRLDistributionPoints: cloneStrings(extensions.CRLDistributionPoints),
			OCSPServers:           cloneStrings(extensions.OCSPServers),
		},
	}
}

/**
 * @description RegisterUser receives a CSR from a client, verifies it, signs it, and returns an X.509 cert.
 *
 * @function RegisterUser
 * @memberof Service
 * @param {string} csrPEM - The PEM-encoded Certificate Signing Request.
 * @param {string} userID - The ID of the user requesting the certificate (email).
 * @returns {(string, string, int64, error)} The PEM-encoded certificate, serial number (hex), expiration timestamp, and an error if any.
 */
func (s *Service) RegisterUser(csrPEM, userID string) (certPEM string, serialHex string, notAfter int64, err error) {
	// @note Decode CSR
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

	// @note Verify CSR signature by public key
	if err := csr.CheckSignature(); err != nil {
		return "", "", 0, fmt.Errorf("CSR signature verification failed: %w", err)
	}

	// @note Only accept RSA keys (based on system design)
	rsaPub, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok {
		return "", "", 0, fmt.Errorf("CSR must contain RSA public key")
	}
	// @note Require a minimum of RSA-2048 for security guarantees
	if rsaPub.N.BitLen() < 2048 {
		return "", "", 0, fmt.Errorf("RSA key too short: %d bits (minimum 2048)", rsaPub.N.BitLen())
	}

	email, uri, err := buildSubjectAltNames(userID)
	if err != nil {
		return "", "", 0, err
	}
	if err := validateCSRIdentity(csr, userID, email, uri); err != nil {
		return "", "", 0, err
	}

	// @note Generate a random serial number
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
		fmt.Println("[CA] Warning: serial=0 generated, retrying...")
	}

	// @note Create X.509 certificate template
	now := time.Now().UTC()
	expiry := now.AddDate(0, 0, s.certValidityDays)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			// CommonName = userID to easily identify who the cert belongs to
			CommonName: userID,
		},
		NotBefore: now,
		NotAfter:  expiry,

		// @note End-entity cert — NOT a CA
		IsCA:                  false,
		BasicConstraintsValid: true,

		// @note KeyUsage for client cert:
		// @note DigitalSignature: sign payloads (non-repudiation)
		// @note KeyEncipherment: encrypt/decrypt session keys (AS_REP, TGS_REP)
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,

		// @note ExtKeyUsage: client authentication (TLS mutual auth, Kerberos PKINIT)
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},

		// @note Standards-oriented identity and key identifiers for TLS/KDC/Bank checks.
		EmailAddresses: []string{email},
		URIs:           []*url.URL{uri},
		SubjectKeyId:   computeSKI(rsaPub),
		AuthorityKeyId: issuerKeyID(s.rootCA),

		// @note Optional publication endpoints. Project revocation checks still use CA gRPC.
		CRLDistributionPoints: cloneStrings(s.extensions.CRLDistributionPoints),
		OCSPServer:            cloneStrings(s.extensions.OCSPServers),
	}

	// @note Sign with Root CA
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		s.rootCA.Certificate,
		csr.PublicKey,
		s.rootCA.PrivateKey,
	)
	if err != nil {
		return "", "", 0, fmt.Errorf("sign certificate: %w", err)
	}

	// @note Parse again to get the full *x509.Certificate object
	issuedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", "", 0, fmt.Errorf("parse issued cert: %w", err)
	}

	// @note Encode to PEM and save
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	serialHex = hex.EncodeToString(issuedCert.SerialNumber.Bytes())

	// @note Save to the store before returning the certificate to the caller.
	if err := s.store.SaveIssued(serialHex, &CertRecord{
		UserID:  userID,
		Cert:    issuedCert,
		CertPEM: string(pemBytes),
	}, now); err != nil {
		return "", "", 0, fmt.Errorf("persist issued certificate: %w", err)
	}

	// @note Backup the PEM file to disk
	if err := s.saveCertToDisk(serialHex, pemBytes); err != nil {
		// @note Log warning but do not fail — the cert is already in the store
		fmt.Printf("[CA] Warning: cannot save cert to disk (serial %s): %v\n", serialHex, err)
	}

	fmt.Printf("[CA] Issued cert for user=%s serial=%s expires=%s\n",
		userID, serialHex, expiry.Format(time.RFC3339))

	return string(pemBytes), serialHex, expiry.Unix(), nil
}

/**
 * @description GetCertificate returns the certificate and its status by serial number.
 *
 * @function GetCertificate
 * @memberof Service
 * @param {string} serialHex - The hex string representation of the certificate's serial number.
 * @returns {(string, string, CertStatusVal, int64, error)} The certificate PEM, user ID, status, expiration timestamp, and an error if any.
 */
func (s *Service) GetCertificate(serialHex string) (certPEM, userID string, status CertStatusVal, notAfter int64, err error) {
	rec := s.store.Get(serialHex)
	if rec == nil {
		return "", "", CertStatusUnknown, 0, fmt.Errorf("certificate not found: %s", serialHex)
	}

	st := s.resolveStatus(rec)
	return rec.CertPEM, rec.UserID, st, rec.Cert.NotAfter.Unix(), nil
}

/**
 * @description CheckRevocation returns the revocation status of a certificate.
 * @note It is called by KDC during TGS Exchange and by the Bank before BEGIN TRANSACTION.
 *
 * @function CheckRevocation
 * @memberof Service
 * @param {string} serialHex - The hex string representation of the certificate's serial number.
 * @returns {(CertStatusVal, string, int64, error)} The status, revocation reason, revocation timestamp, and an error if any.
 */
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

/**
 * @description RevokeCertificate revokes a certificate by its serial number.
 * @note Returns an error if the certificate is not found or has already been revoked.
 *
 * @function RevokeCertificate
 * @memberof Service
 * @param {string} serialHex - The hex string representation of the certificate's serial number.
 * @param {string} reason - The reason for revoking the certificate.
 * @returns {error} An error if the revocation fails, otherwise nil.
 */
func (s *Service) RevokeCertificate(serialHex, reason string) error {
	rec := s.store.Get(serialHex)
	if rec == nil {
		return fmt.Errorf("not_found: %s", serialHex)
	}
	if rec.RevokedAt != nil {
		return fmt.Errorf("already_revoked: %s", serialHex)
	}

	revoked, err := s.store.Revoke(serialHex, reason)
	if err != nil {
		return fmt.Errorf("persist revocation: %w", err)
	}
	if !revoked {
		return fmt.Errorf("revoke failed: concurrent modification")
	}

	fmt.Printf("[CA] Revoked cert serial=%s reason=%s\n", serialHex, reason)
	return nil
}

// ===================================================================
// ============================= Helpers =============================
// ===================================================================

/**
 * CertStatusVal is an internal type corresponding to the proto enum CertStatus.
 *
 * @typedef {int32} CertStatusVal
 */
type CertStatusVal int32

const (
	CertStatusUnknown CertStatusVal = 0
	CertStatusValid   CertStatusVal = 1
	CertStatusRevoked CertStatusVal = 2
	CertStatusExpired CertStatusVal = 3
)

/**
 * @description resolveStatus determines the overall status of a certificate.
 * @note Priority: REVOKED > EXPIRED > VALID.
 *
 * @function resolveStatus
 * @memberof Service
 * @param {*CertRecord} rec - The certificate record to check.
 * @returns {CertStatusVal} The resolved status of the certificate.
 */
func (s *Service) resolveStatus(rec *CertRecord) CertStatusVal {
	if rec.RevokedAt != nil {
		return CertStatusRevoked
	}
	if time.Now().UTC().After(rec.Cert.NotAfter) {
		return CertStatusExpired
	}
	return CertStatusValid
}

/**
 * @description computeSKI computes the Subject Key Identifier from an RSA public key.
 * @note SKI = SHA-1 of the DER-encoded SubjectPublicKeyInfo (RFC 5280 §4.2.1.2).
 *
 * @function computeSKI
 * @param {*rsa.PublicKey} pub - The RSA public key.
 * @returns {[]byte} The computed SKI byte array.
 */
func computeSKI(pub *rsa.PublicKey) []byte {
	// @note Use crypto/sha1 instead of crypto/sha256 because RFC 5280 dictates SHA-1 for SKI
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
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
	if pub, ok := rootCA.Certificate.PublicKey.(*rsa.PublicKey); ok {
		return computeSKI(pub)
	}
	if rootCA.PrivateKey != nil {
		return computeSKI(&rootCA.PrivateKey.PublicKey)
	}
	return nil
}

func buildSubjectAltNames(userID string) (string, *url.URL, error) {
	parsedEmail, err := mail.ParseAddress(userID)
	if err != nil || parsedEmail.Address != userID {
		return "", nil, fmt.Errorf("userID must be a plain email address for certificate SAN: %s", userID)
	}
	uri, err := url.Parse("urn:mini-banking:user:" + url.PathEscape(userID))
	if err != nil {
		return "", nil, fmt.Errorf("build user URI SAN: %w", err)
	}
	return parsedEmail.Address, uri, nil
}

func validateCSRIdentity(csr *x509.CertificateRequest, userID, email string, uri *url.URL) error {
	if csr.Subject.CommonName != userID {
		return fmt.Errorf("%w: CSR CommonName %q does not match userID %q", ErrCSRIdentityMismatch, csr.Subject.CommonName, userID)
	}
	for _, csrEmail := range csr.EmailAddresses {
		if csrEmail != email {
			return fmt.Errorf("%w: CSR email SAN %q does not match userID %q", ErrCSRIdentityMismatch, csrEmail, userID)
		}
	}
	for _, csrURI := range csr.URIs {
		if csrURI == nil || csrURI.String() != uri.String() {
			return fmt.Errorf("%w: CSR URI SAN %q does not match userID URI %q", ErrCSRIdentityMismatch, csrURI, uri)
		}
	}
	return nil
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

/**
 * @description saveCertToDisk saves the certificate PEM out to a file for backup.
 * @note Filename: <serial>.pem inside the IssuedCertsPath directory.
 *
 * @function saveCertToDisk
 * @memberof Service
 * @param {string} serialHex - The hex string representation of the certificate's serial number.
 * @param {[]byte} pemBytes - The PEM-encoded certificate bytes.
 * @returns {error} An error if the file operation fails.
 */
func (s *Service) saveCertToDisk(serialHex string, pemBytes []byte) error {
	if err := os.MkdirAll(s.issuedCertsPath, 0755); err != nil {
		return err
	}
	path := filepath.Join(s.issuedCertsPath, serialHex+".pem")
	return os.WriteFile(path, pemBytes, 0644)
}
