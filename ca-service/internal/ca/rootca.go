package ca

/**
 * @title CA Service - Root CA Manager
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Loading existing Root CA from disk or generating a new self-signed RSA-4096 Root Certificate Authority.
 */

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

/**
 * @description RootCA represents the Root Certificate Authority.
 * @note It holds the private key and certificate of the CA to sign user certificates.
 *
 * @typedef {Object} RootCA
 * @property {*rsa.PrivateKey} PrivateKey - The RSA private key of the CA.
 * @property {*x509.Certificate} Certificate - The x509 certificate of the CA.
 * @property {[]byte} CertPEM - The PEM-encoded certificate.
 */
type RootCA struct {
	PrivateKey  *rsa.PrivateKey
	Certificate *x509.Certificate
	CertPEM     []byte
}

/**
 * @description LoadOrCreate loads the Root CA from disk if it already exists, or generates a new self-signed one if it does not.
 * @note This is the first bootstrapping step of the CA Service.
 * @note In production, the key should be protected by an HSM or injected via Kubernetes Secret — not stored on a regular disk.
 *
 * @function LoadOrCreate
 * @param {string} keyPath - The path to the private key file.
 * @param {string} certPath - The path to the certificate file.
 * @returns {(*RootCA, error)} The loaded or newly created Root CA, and an error if any.
 */
func LoadOrCreate(keyPath, certPath string) (*RootCA, error) {
	// @note Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	// @note Check if both files exist
	keyExists := fileExists(keyPath)
	certExists := fileExists(certPath)

	if keyExists && certExists {
		return loadFromDisk(keyPath, certPath)
	}

	// @note If either is missing -> recreate both to ensure consistency
	fmt.Println("[CA] Root CA not found, generating new self-signed Root CA...")
	return generateAndSave(keyPath, certPath)
}

/**
 * @description loadFromDisk reads the existing key and certificate from the disk.
 *
 * @function loadFromDisk
 * @param {string} keyPath - The path to the private key file.
 * @param {string} certPath - The path to the certificate file.
 * @returns {(*RootCA, error)} The loaded Root CA, and an error if any.
 */
func loadFromDisk(keyPath, certPath string) (*RootCA, error) {
	// @note Load private key
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
		// @note Fallback: try PKCS1 (older keys might use this format)
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

	// @note Load certificate
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

/**
 * @descroption generateAndSave generates a new Root CA and saves it to the disk.
 *
 * @function generateAndSave
 * @param {string} keyPath - The path to save the private key.
 * @param {string} certPath - The path to save the certificate.
 * @returns {(*RootCA, error)} The generated Root CA, and an error if any.
 */
func generateAndSave(keyPath, certPath string) (*RootCA, error) {
	// @note Generate an RSA-4096 private key (prioritizing security over speed)
	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	// @note Generate a random serial number for the Root CA cert, ensuring serial number > 0
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
		// @note Although the probability is 1/2^128 — log to know if something weird happens
		fmt.Println("[CA] Warning: serial=0 generated, retrying...")
	}

	// @note Template for the Root CA certificate
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   "Mini_App_Banking Root CA",
		},
		NotBefore: time.Now().UTC(),
		// @note The expiry is 10 years
		NotAfter: time.Now().UTC().AddDate(10, 0, 0),

		// @note CA constraints - Mandatory for Root CA
		IsCA:                  true,
		BasicConstraintsValid: true,
		// @note Do not allow intermediate CAs
		MaxPathLen:     0,
		MaxPathLenZero: true,

		// @note CA key usage: only sign certs and CRLs
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	// @note Self-sign: the CA signs its own certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("create root cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse generated cert: %w", err)
	}

	// @note Encode PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPKCS8, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyPKCS8})

	// @note Save the key with 0600 permission (readable only by the owner)
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

/**
 * @description parseCertPEM parses a PEM-encoded certificate.
 *
 * @function parseCertPEM
 * @param {[]byte} pemBytes - The PEM-encoded certificate bytes.
 * @returns {(*x509.Certificate, error)} The parsed certificate, and an error if any.
 */
func parseCertPEM(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

/**
 * @description fileExists checks if a file exists.
 *
 * @function fileExists
 * @param {string} path - The file path to check.
 * @returns {bool} True if the file exists, false otherwise.
 */
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
