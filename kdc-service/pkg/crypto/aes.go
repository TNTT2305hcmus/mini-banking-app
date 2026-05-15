package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"kdc-service/config"
)

// Load K_TGS aes key and TGT_EXP
var ENV = config.LoadEnv()

// This function aims to encrypt TGT with the AES-256-GCM algorithm
// TGT payload includes: {client_id, K_{c,tgs}, ticket_exp}
// Return: Encrypted_TGT
func EncryptTGT(payload []byte) ([]byte, error) {
	// Create block cipher
	block, err := aes.NewCipher(ENV.K_TGS)

	if err != nil {
		return nil, err
	}

	// Use GCM mode
	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return nil, err
	}

	// Random nonce with gcm.NonceSize()
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt
	E_tgt := gcm.Seal(nonce, nonce, payload, nil)

	return E_tgt, nil
}
