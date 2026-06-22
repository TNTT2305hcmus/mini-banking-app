package kdc

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

/**
 * @description Loads a certificate and enforces that it is usable right now.
 * @note Shared by the AS and TGS exchanges. Maps a missing certificate to
 * CERT_NOT_FOUND and any other repository failure to INTERNAL_ERROR.
 * @param {context.Context} ctx - Request context for the certificate repository.
 * @param {CertificateRepository} repo - Certificate metadata source.
 * @param {string} certSN - Certificate serial number to look up.
 * @param {time.Time} now - Current time for validity-window checks.
 * @returns {(Certificate, error)} Usable certificate metadata or a domain error.
 */
func loadUsableCert(ctx context.Context, repo CertificateRepository, certSN string, now time.Time) (Certificate, error) {
	cert, err := repo.GetCertificate(ctx, certSN)
	if errors.Is(err, ErrCertificateMissing) {
		return Certificate{}, kdcError(ErrCertNotFound, err)
	}
	if err != nil {
		return Certificate{}, kdcError(ErrInternal, err)
	}
	if err := validateCertUsable(cert, now); err != nil {
		return Certificate{}, err
	}
	return cert, nil
}

/**
 * @description Enforces the shared certificate-usability rules for both the AS and
 * TGS exchanges: lifecycle status, validity window, and public-key presence.
 * @param {Certificate} cert - Certificate metadata loaded from the certificate repository.
 * @param {time.Time} now - Current time used for the validity-window checks.
 * @returns {error} Domain error (CERT_REVOKED / CERT_EXPIRED / CERT_NOT_FOUND / AUTH_INVALID) or nil when usable.
 */
func validateCertUsable(cert Certificate, now time.Time) error {
	switch cert.Status {
	case CertificateRevoked:
		return kdcError(ErrCertRevoked, nil)
	case CertificateExpired:
		return kdcError(ErrCertExpired, nil)
	case CertificateValid, CertificateActive:
	default:
		return kdcError(ErrCertNotFound, nil)
	}
	if !cert.NotBefore.IsZero() && now.Before(cert.NotBefore) {
		return kdcError(ErrAuthInvalid, errors.New("certificate not yet valid"))
	}
	if cert.NotAfter.IsZero() || !cert.NotAfter.After(now) {
		return kdcError(ErrCertExpired, nil)
	}
	if cert.PublicKeyPEM == "" {
		return kdcError(ErrAuthInvalid, errors.New("certificate missing public key"))
	}
	return nil
}

/**
 * @description Parses an RSA public key from a PKIX PEM block.
 * @param {string} publicKeyPEM - PEM-encoded PKIX public key.
 * @returns {(*rsa.PublicKey, error)} The RSA public key or a parsing error.
 */
func parseRSAPublicKeyPEM(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("decode public key pem failed")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key failed: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return rsaPub, nil
}
