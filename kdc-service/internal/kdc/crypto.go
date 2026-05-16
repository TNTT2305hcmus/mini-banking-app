package kdc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
)

/**
 * @description AES-GCM sizing constants used by all encrypted payload helpers.
 */
const (
	aes256KeySize = 32
	gcmNonceSize  = 12
)

/**
 * @description Marshals a payload to JSON and encrypts it with AES-256-GCM.
 * @param {[]byte} key - AES-256 key used for encryption.
 * @param {any} plaintext - JSON-serializable payload.
 * @param {io.Reader} random - Random source used to generate the GCM nonce.
 * @returns {[]byte} Nonce-prefixed ciphertext.
 * @returns {error} Encryption or marshaling failure.
 */
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

/**
 * @description Decrypts AES-256-GCM ciphertext and unmarshals the JSON payload into the requested type.
 * @param {[]byte} key - AES-256 key used for decryption.
 * @param {[]byte} ciphertext - Nonce-prefixed ciphertext.
 * @returns {T} Decoded plaintext payload.
 * @returns {error} Decryption or unmarshaling failure.
 */
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

/**
 * @description Encrypts raw bytes with AES-256-GCM and prefixes the nonce to the ciphertext.
 * @param {[]byte} key - AES-256 key used for encryption.
 * @param {[]byte} plaintext - Raw plaintext bytes.
 * @param {io.Reader} random - Random source used to generate the nonce.
 * @returns {[]byte} Nonce-prefixed ciphertext.
 * @returns {error} Encryption failure.
 */
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

/**
 * @description Decrypts nonce-prefixed AES-256-GCM ciphertext.
 * @param {[]byte} key - AES-256 key used for decryption.
 * @param {[]byte} ciphertext - Nonce-prefixed ciphertext.
 * @returns {[]byte} Raw plaintext bytes.
 * @returns {error} Decryption failure.
 */
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
