package kdc

/**
 * @title KDC Service - Core Logic
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Handling AS Exchange and TGS Exchange, integrating with CA Service for identity verification.
 */

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	capb "mini_banking/pkg/pb/ca"
)

/**
 * @description Service contains the core logic for the KDC.
 *
 * @typedef {Object} Service
 * @property {capb.CAServiceClient} caClient - gRPC client to communicate with CA Service.
 * @property {*redis.Client} redisClient - Redis client for replay cache.
 */
type Service struct {
	caClient    capb.CAServiceClient
	redisClient *redis.Client
}

/**
 * @description NewService initializes the KDC Service with a CA gRPC client.
 *
 * @function NewService
 * @param {capb.CAServiceClient} caClient - The CA Service gRPC client.
 * @param {*redis.Client} redisClient - The Redis client.
 * @returns {*Service} A new instance of the KDC Service.
 */
func NewService(caClient capb.CAServiceClient, redisClient *redis.Client) *Service {
	return &Service{
		caClient:    caClient,
		redisClient: redisClient,
	}
}

/**
 * @description fetchPublicKeyFromCA retrieves the client's public key by calling the CA Service.
 *
 * @function fetchPublicKeyFromCA
 * @memberof Service
 * @param {context.Context} ctx - The request context.
 * @param {string} certSn - The serial number of the client's certificate.
 * @returns {(*rsa.PublicKey, error)} The RSA public key or an error.
 */
func (s *Service) fetchPublicKeyFromCA(ctx context.Context, certSn string) (*rsa.PublicKey, error) {
	// @note 1. Call gRPC to CA Service to get certificate info
	resp, err := s.caClient.GetCertificate(ctx, &capb.GetCertificateRequest{
		SerialNumber: certSn,
	})
	if err != nil {
		return nil, fmt.Errorf("ca_service: get certificate failed: %w", err)
	}

	// @note 2. Decode PEM block from the response
	block, _ := pem.Decode([]byte(resp.CertificatePem))
	if block == nil {
		return nil, fmt.Errorf("decode certificate pem failed")
	}

	// @note 3. Parse the X.509 certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse x509 certificate failed: %w", err)
	}

	// @note 4. Extract and assert RSA Public Key
	pubKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate does not contain an RSA public key")
	}

	return pubKey, nil
}

/**
 * @description VerifyPreAuthSignature verifies the client's signature (PreAuth_Sig) using their public key.
 * @note This is part of Phase 2 (AS Exchange) to ensure the client owns the private key.
 *
 * @function VerifyPreAuthSignature
 * @memberof Service
 * @param {context.Context} ctx - The request context.
 * @param {string} certSn - Certificate serial number.
 * @param {[]byte} signature - The signature to verify.
 * @param {[]byte} dataToVerify - The original data that was signed.
 * @returns {error} Nil if valid, error otherwise.
 */
func (s *Service) VerifyPreAuthSignature(ctx context.Context, certSn string, signature []byte, dataToVerify []byte) error {
	// @note 1. Get Public Key from CA
	pubKey, err := s.fetchPublicKeyFromCA(ctx, certSn)
	if err != nil {
		return fmt.Errorf("fetch_pubkey_failed: %w", err)
	}

	// @note 2. Compute SHA-256 hash of the data
	hashed := sha256.Sum256(dataToVerify)

	// @note 3. Verify signature using RSA-PKCS1v15 (standard for this project)
	err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature)
	if err != nil {
		return fmt.Errorf("signature_verification_failed: %w", err)
	}

	return nil
}

/**
 * @description GenerateSessionKey generates a secure random 256-bit (32-byte) key.
 * @note This is used for both K_{c,tgs} (AS Exchange) and K_{c,v} (TGS Exchange).
 *
 * @function GenerateSessionKey
 * @memberof Service
 * @returns {([]byte, error)} The generated session key or an error.
 */
func (s *Service) GenerateSessionKey() ([]byte, error) {
	key := make([]byte, 32) // AES-256 key
	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("generate_session_key_failed: %w", err)
	}
	return key, nil
}

/**
 * @description CheckAndStoreNonce checks if a nonce exists in Redis to prevent replay attacks.
 * @note If it doesn't exist, it stores the nonce with a TTL.
 *
 * @function CheckAndStoreNonce
 * @memberof Service
 * @param {context.Context} ctx
 * @param {[]byte} nonce
 * @returns {error} Error if it's a replay attack or Redis fails.
 */
func (s *Service) CheckAndStoreNonce(ctx context.Context, nonce []byte) error {
	nonceHex := fmt.Sprintf("%x", nonce)
	key := fmt.Sprintf("kdc:nonce:%s", nonceHex)

	// Try to set the nonce in Redis with NX (Not eXists)
	// TTL is set to 5 minutes (300 seconds)
	now := time.Now().UTC()
	
	// Value is a JSON string or just simple string. We use simple string for used_at.
	value := fmt.Sprintf("used_at:%d", now.Unix())
	
	success, err := s.redisClient.SetNX(ctx, key, value, 5*time.Minute).Result()
	if err != nil {
		return fmt.Errorf("redis error checking nonce: %w", err)
	}

	if !success {
		return fmt.Errorf("replay attack detected: nonce %s was already used", nonceHex)
	}

	return nil
}
