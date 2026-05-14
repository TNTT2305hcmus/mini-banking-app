package kdc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
)

const (
	aes256KeySize = 32
	gcmNonceSize  = 12
)

func encryptJSON(key []byte, plaintext any, random io.Reader) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires a 32-byte key")
	}
	body, err := json.Marshal(plaintext)
	if err != nil {
		return nil, err
	}
	return encryptBytes(key, body, random)
}

func decryptJSON[T any](key []byte, ciphertext []byte) (T, error) {
	var out T
	body, err := decryptBytes(key, ciphertext)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}

func encryptBytes(key []byte, plaintext []byte, random io.Reader) ([]byte, error) {
	if random == nil {
		random = rand.Reader
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, sealed...), nil
}

func decryptBytes(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	body := ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, body, nil)
}
