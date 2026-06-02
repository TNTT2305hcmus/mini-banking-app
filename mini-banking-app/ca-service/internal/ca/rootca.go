package ca

/**
 * @title CA Service - Root CA Manager
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Loading an existing, pre-provisioned Root Certificate Authority from disk.
 */

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash"
	"os"
	"strconv"
	"time"
)

const rootCAKeyPasswordEnv = "ROOT_CA_KEY_PASSWORD"

const (
	encryptedPrivateKeyPEMType = "ENCRYPTED PRIVATE KEY"
	rootCAKeyCipher            = "AES-256-GCM"
	rootCAKeyKDF               = "PBKDF2-HMAC-SHA256"
	rootCAKeyNonceSize         = 12
	rootCAKeySize              = 32
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
 * @description LoadKeyAndCert loads the pre-provisioned Root CA key and certificate from disk.
 * @note This function intentionally fails closed when the key or certificate is missing.
 * @note In production, the key should be protected by an HSM or injected via Kubernetes Secret, not stored on a regular disk.
 *
 * @function LoadKeyAndCert
 * @param {string} keyPath - The path to the private key file.
 * @param {string} certPath - The path to the certificate file.
 * @returns {(*RootCA, error)} The loaded Root CA, and an error if any.
 */
func LoadKeyAndCert(keyPath, certPath string) (*RootCA, error) {
	keyExists := fileExists(keyPath)
	certExists := fileExists(certPath)

	if keyExists && certExists {
		return loadFromDisk(keyPath, certPath)
	}

	if !keyExists && !certExists {
		return nil, fmt.Errorf("root CA key and certificate are missing; refusing to generate a new root automatically")
	}
	if !keyExists {
		return nil, fmt.Errorf("root CA key is missing at %s; refusing to continue with only a certificate", keyPath)
	}
	return nil, fmt.Errorf("root CA certificate is missing at %s; refusing to continue with only a private key", certPath)
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
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("decode CA key PEM: invalid format")
	}
	keyDER, err := decryptPrivateKeyPEM(block)
	if err != nil {
		return nil, err
	}
	privKey, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		rsaKey, err2 := x509.ParsePKCS1PrivateKey(keyDER)
		if err2 != nil {
			return nil, fmt.Errorf("parse CA key (PKCS8: %v, PKCS1: %v)", err, err2)
		}
		privKey = rsaKey
	}
	rsaKey, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("CA key is not RSA")
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}
	if err := validateLoadedRootCA(rsaKey, cert); err != nil {
		return nil, fmt.Errorf("validate CA key/cert: %w", err)
	}

	fmt.Printf("[CA] Loaded Root CA from disk (Subject: %s)\n", cert.Subject.CommonName)
	return &RootCA{
		PrivateKey:  rsaKey,
		Certificate: cert,
		CertPEM:     certPEM,
	}, nil
}

func validateLoadedRootCA(privKey *rsa.PrivateKey, cert *x509.Certificate) error {
	if err := privKey.Validate(); err != nil {
		return fmt.Errorf("invalid RSA private key: %w", err)
	}

	certPubKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("root CA certificate public key is not RSA")
	}
	if certPubKey.N.Cmp(privKey.N) != 0 || certPubKey.E != privKey.E {
		return fmt.Errorf("root CA private key does not match certificate public key")
	}
	if !cert.BasicConstraintsValid {
		return fmt.Errorf("root CA certificate basic constraints are not valid")
	}
	if !cert.IsCA {
		return fmt.Errorf("root CA certificate is not marked as a CA")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return fmt.Errorf("root CA certificate is missing KeyUsageCertSign")
	}

	now := time.Now().UTC()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("root CA certificate is not valid before %s", cert.NotBefore.Format(time.RFC3339))
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("root CA certificate expired at %s", cert.NotAfter.Format(time.RFC3339))
	}
	if err := cert.CheckSignatureFrom(cert); err != nil {
		return fmt.Errorf("root CA certificate self-signature is invalid: %w", err)
	}
	return nil
}

func decryptPrivateKeyPEM(block *pem.Block) ([]byte, error) {
	if block.Type != encryptedPrivateKeyPEMType {
		return nil, fmt.Errorf("root CA private key PEM is not encrypted; refusing to load plaintext private key")
	}
	passphrase := os.Getenv(rootCAKeyPasswordEnv)
	if passphrase == "" {
		return nil, fmt.Errorf("%s is required to decrypt Root CA private key", rootCAKeyPasswordEnv)
	}
	keyDER, err := decryptPrivateKeyEnvelope(block, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt CA key PEM: %w", err)
	}
	return keyDER, nil
}

func decryptPrivateKeyEnvelope(block *pem.Block, passphrase string) ([]byte, error) {
	if block.Headers["Cipher"] != rootCAKeyCipher {
		return nil, fmt.Errorf("unsupported key cipher: %s", block.Headers["Cipher"])
	}
	if block.Headers["KDF"] != rootCAKeyKDF {
		return nil, fmt.Errorf("unsupported key KDF: %s", block.Headers["KDF"])
	}
	iterations, err := strconv.Atoi(block.Headers["Iterations"])
	if err != nil || iterations <= 0 {
		return nil, fmt.Errorf("invalid key KDF iterations")
	}
	salt, err := base64.StdEncoding.DecodeString(block.Headers["Salt"])
	if err != nil || len(salt) == 0 {
		return nil, fmt.Errorf("invalid key salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(block.Headers["Nonce"])
	if err != nil || len(nonce) != rootCAKeyNonceSize {
		return nil, fmt.Errorf("invalid key nonce")
	}

	key := pbkdf2SHA256([]byte(passphrase), salt, iterations, rootCAKeySize)
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, block.Bytes, nil)
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2Key(sha256.New, password, salt, iterations, keyLen)
}

func pbkdf2Key(h func() hash.Hash, password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var derived []byte
	block := make([]byte, len(salt)+4)
	copy(block, salt)

	for i := 1; i <= numBlocks; i++ {
		block[len(salt)] = byte(i >> 24)
		block[len(salt)+1] = byte(i >> 16)
		block[len(salt)+2] = byte(i >> 8)
		block[len(salt)+3] = byte(i)

		prf.Reset()
		prf.Write(block)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)

		for j := 1; j < iterations; j++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLen]
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
