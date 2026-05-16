package kdc

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

/**
 * @description KDCKeys contains all cryptographic materials
 * used by the KDC service.
 *
 * @typedef {Object} KDCKeys
 * @property {[]byte} KTGSKey - Symmetric AES key shared between AS and TGS.
 * @property {*rsa.PrivateKey} PrivateKey - RSA private key of the KDC.
 * @property {[]byte} RawPrivKey - Raw PEM bytes of the private key.
 */
type KDCKeys struct {
	KTGSKey    []byte
	PrivateKey *rsa.PrivateKey
	RawPrivKey []byte
}

/**
 * @description LoadKeys loads and validates the KDC cryptographic keys
 * from filesystem storage.
 *
 * @note Supported RSA private key formats:
 * - PKCS#1
 * - PKCS#8
 *
 * @function LoadKeys
 * @param {string} ktgsPath - Path to AES key file (K_tgs).
 * @param {string} privKeyPath - Path to RSA private key PEM file.
 * @returns {(*KDCKeys, error)} Loaded keys or error.
 */
func LoadKeys(
	ktgsPath string,
	privKeyPath string,
) (*KDCKeys, error) {

	// @note 1. Validate input paths
	if ktgsPath == "" {
		return nil, fmt.Errorf("ktgs key path is required")
	}

	if privKeyPath == "" {
		return nil, fmt.Errorf("private key path is required")
	}

	// @note 2. Read KTGS symmetric key file
	ktgsBytes, err := os.ReadFile(ktgsPath)
	if err != nil {
		return nil, fmt.Errorf(
			"read_ktgs_key_failed: %w",
			err,
		)
	}

	// @note 3. Remove trailing newline characters
	ktgsBytes = cleanNewline(ktgsBytes)

	// @note 4. Validate AES-256 key length
	if len(ktgsBytes) != 32 {
		return nil, fmt.Errorf(
			"invalid_ktgs_key_length: expected 32 bytes, got %d bytes",
			len(ktgsBytes),
		)
	}

	// @note 5. Read RSA private key file
	privBytes, err := os.ReadFile(privKeyPath)
	if err != nil {
		return nil, fmt.Errorf(
			"read_private_key_failed: %w",
			err,
		)
	}

	// @note 6. Decode PEM block
	block, _ := pem.Decode(privBytes)

	if block == nil {
		return nil, fmt.Errorf(
			"decode_pem_failed: invalid private key pem format",
		)
	}

	var rsaPrivKey *rsa.PrivateKey

	// @note 7. Try parsing as PKCS#1
	rsaPrivKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)

	if err != nil {

		// @note 8. Fallback to PKCS#8 parsing
		parsedKey, errPkcs8 := x509.ParsePKCS8PrivateKey(
			block.Bytes,
		)

		if errPkcs8 != nil {
			return nil, fmt.Errorf(
				"parse_private_key_failed: pkcs1_error=%v, pkcs8_error=%v",
				err,
				errPkcs8,
			)
		}

		// @note 9. Assert RSA private key type
		var ok bool

		rsaPrivKey, ok = parsedKey.(*rsa.PrivateKey)

		if !ok {
			return nil, fmt.Errorf(
				"invalid_private_key_type: expected RSA private key",
			)
		}
	}

	// @note 10. Validate RSA private key
	if rsaPrivKey == nil {
		return nil, fmt.Errorf(
			"rsa_private_key_is_nil",
		)
	}

	// @note 11. Return loaded keys
	return &KDCKeys{
		KTGSKey:    ktgsBytes,
		PrivateKey: rsaPrivKey,
		RawPrivKey: privBytes,
	}, nil
}

/**
 * @description cleanNewline removes trailing newline characters
 * from file content.
 *
 * @note This is useful when keys are stored in text files
 * with accidental newline characters at the end.
 *
 * @function cleanNewline
 * @param {[]byte} data - Raw file content.
 * @returns {[]byte} Cleaned byte slice.
 */
func cleanNewline(data []byte) []byte {

	// Remove trailing '\n'
	if len(data) > 0 &&
		data[len(data)-1] == '\n' {

		data = data[:len(data)-1]
	}

	// Remove trailing '\r'
	if len(data) > 0 &&
		data[len(data)-1] == '\r' {

		data = data[:len(data)-1]
	}

	return data
}
