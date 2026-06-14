package kdc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"kdc-service/internal/config"
	"time"

	capb "mini_banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
)

/////////////////////////////////////////////////////////
//					Constructor
////////////////////////////////////////////////////////

var ENV *config.EnvConfig

func getEnvConfig() *config.EnvConfig {
	if ENV == nil {
		ENV = config.LoadEnv()
	}
	return ENV
}

/**
 * @description NewService initializes the KDC Service with a CA gRPC client.
 *
 * @function NewService
 * @param {capb.CAServiceClient} caClient - The CA Service gRPC client.
 * @param {*redis.Client} redisClient - The Redis client.
 * @returns {*Service} A new instance of the KDC Service.
 */
func NewASService(caClient capb.CAServiceClient, redisClient *redis.Client) (*ASService, error) {
	env := getEnvConfig()
	keys, err := LoadKeys(
		env.KTGSPath,
		env.KDCPrivatePath,
	)
	if err != nil {
		return nil, fmt.Errorf("load_kdc_keys_failed: %w", err)
	}

	return &ASService{
		caClient:    caClient,
		redisClient: redisClient,
		kdcKeys:     keys,
	}, nil
}

/////////////////////////////////////////////////////
//					AS Exchange
////////////////////////////////////////////////////

/**
 * @title KDC Service - AS EXCHANGE LOGIC
 * @author Le Nguyen Quoc Thai (lnqt)
 * @summary Handling AS Exchange  integrating with CA Service for identity verification.
 */

/**
 * @description fetchPublicKeyFromCA retrieves the client's public key by calling the CA Service.
 *
 * @function fetchPublicKeyFromCA
 * @memberof Service
 * @param {context.Context} ctx - The request context.
 * @param {string} certSn - The serial number of the client's certificate.
 * @returns {(*rsa.PublicKey, error)} The RSA public key or an error.
 */
func (s *ASService) fetchPublicKeyFromCA(ctx context.Context, certSn string) (*rsa.PublicKey, error) {
	verifyReq := &capb.VerifyCertificateRequest{
		SerialNumber:          certSn,
		Caller:                "kdc-service",
		IncludePublicKeyPem:   true,
		IncludeCertificatePem: true,
	}

	resp, err := s.caClient.VerifyCertificate(ctx, verifyReq)
	if err != nil {
		return nil, fmt.Errorf("ca_service: verify certificate failed: %w", err)
	}
	if resp.Status != capb.CertStatus_CERT_STATUS_ACTIVE {
		return nil, fmt.Errorf("certificate is not active")
	}
	now := time.Now().UTC().Unix()
	if resp.NotBeforeUnix != 0 && resp.NotBeforeUnix > now {
		return nil, fmt.Errorf("certificate is not yet valid")
	}
	if resp.NotAfterUnix != 0 && resp.NotAfterUnix <= now {
		return nil, fmt.Errorf("certificate expired")
	}

	if resp.PublicKeyPem != "" {
		return parseRSAPublicKeyPEM(resp.PublicKeyPem)
	}
	return parseRSAPublicKeyFromCertificatePEM(resp.CertificatePem)
}

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

func parseRSAPublicKeyFromCertificatePEM(certificatePEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(certificatePEM))
	if block == nil {
		return nil, fmt.Errorf("decode certificate pem failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse x509 certificate failed: %w", err)
	}
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
func (s *ASService) VerifyPreAuthSignature(ctx context.Context, certSn string, signature []byte, dataToVerify []byte) error {
	// @note 1. Get Public Key from CA
	pubKey, err := s.fetchPublicKeyFromCA(ctx, certSn)
	if err != nil {
		return fmt.Errorf("fetch_pubkey_failed: %w", err)
	}

	// @note 2. Compute SHA-256 hash of the data
	hashed := sha256.Sum256(dataToVerify)

	// @note 3. Accept both legacy PKCS#1 v1.5 tests and the blueprint-preferred RSA-PSS.
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature); err == nil {
		return nil
	}
	if err := rsa.VerifyPSS(pubKey, crypto.SHA256, hashed[:], signature, nil); err != nil {
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
func (s *ASService) GenerateSessionKey() ([]byte, error) {
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
func (s *ASService) CheckAndStoreNonce(ctx context.Context, nonce []byte) error {
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

func (s *ASService) CheckAndStoreASReplay(ctx context.Context, clientID string, nonce []byte, timestamp int64) error {
	nonceText := base64.StdEncoding.EncodeToString(nonce)
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", clientID, nonceText, timestamp)))
	key := "replay:as:" + hex.EncodeToString(hash[:])
	value := fmt.Sprintf("used_at:%d", time.Now().UTC().Unix())

	success, err := s.redisClient.SetNX(ctx, key, value, 5*time.Minute).Result()
	if err != nil {
		return fmt.Errorf("redis error checking AS replay: %w", err)
	}
	if !success {
		return fmt.Errorf("replay attack detected: duplicate nonce from %s", clientID)
	}
	return nil
}

///////////////////////////////////////////////////////////
//					TGT and AS_REP
//////////////////////////////////////////////////////////

/**
 * @title Packaging TGT and AS_REP for AS-Exchange
 * @author Truong Thanh Thuan
 * @summary Handling create TGT and packaging in to AS_REP granting permission to use service.
 */

/**
 * @description GenerateEncryptedTGT creates and encrypts a TGT using AES-GCM.
 * @note The TGT is encrypted using K_tgs so only the TGS can decrypt it.
 *
 * @function GenerateEncryptedTGT
 * @memberof Service
 * @param {string} clientId - Client identifier.
 * @param {[]byte} k_ctgs - Session key K_{c,tgs}.
 * @returns {([]byte, error)} Encrypted TGT bytes or error.
 */
func (s *ASService) GenerateEncryptedTGT(clientId string, k_ctgs []byte, certSn ...string) ([]byte, error) {

	// @note 1. Validate input
	if clientId == "" {
		return nil, fmt.Errorf("client_id is required")
	}

	if len(k_ctgs) == 0 {
		return nil, fmt.Errorf("session key k_c_tgs is empty")
	}

	// @note 2. Calculate TGT expiration time
	now := time.Now().UTC()
	exp := now.Add(getEnvConfig().TGTExp).Unix()

	// @note 3. Build TGT payload
	payload := TGT{
		ClientId:   clientId,
		SessionKey: k_ctgs,
		IssuedAt:   now.Unix(),
		Expiry:     exp,
		ExpiresAt:  exp,
	}
	if len(certSn) > 0 {
		payload.CertSN = certSn[0]
	}

	encryptedTGT, err := encryptJSON(s.kdcKeys.KTGSKey, payload, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("encrypt_tgt_failed: %w", err)
	}

	return encryptedTGT, nil
}

/**
 * @description BuildAS_REP creates the AS_REP message for the client.
 * @note The payload is signed using the KDC private key,
 * then encrypted using the client's public key.
 *
 * @function BuildAS_REP
 * @memberof Service
 * @param {context.Context} ctx - Request context.
 * @param {[]byte} k_ctgs - Session key K_{c,tgs}.
 * @param {[]byte} tgt - Encrypted Ticket Granting Ticket.
 * @param {[]byte} nonce1 - Client nonce.
 * @param {string} certSn - Client certificate serial number.
 * @returns {([]byte, error)} Encrypted AS_REP or error.
 */
func (s *ASService) BuildAS_REP(
	ctx context.Context,
	k_ctgs []byte,
	tgt []byte,
	nonce1 []byte,
	certSn string,
) ([]byte, error) {

	// @note 1. Validate input
	if len(k_ctgs) == 0 {
		return nil, fmt.Errorf("session key k_c_tgs is empty")
	}

	if len(tgt) == 0 {
		return nil, fmt.Errorf("tgt is empty")
	}

	if len(nonce1) == 0 {
		return nil, fmt.Errorf("nonce1 is empty")
	}

	if certSn == "" {
		return nil, fmt.Errorf("certificate serial number is required")
	}

	// @note 2. Construct AS_REP payload
	payload := ASRepPayload{
		SessionKey: k_ctgs,
		TGT:        tgt,
		Nonce1:     nonce1,
	}

	// @note 3. Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal_as_rep_payload_failed: %w", err)
	}

	// @note 4. Hash payload using SHA-256
	hashed := sha256.Sum256(payloadBytes)

	// @note 5. Sign payload using RSA-PSS
	signature, err := rsa.SignPSS(
		rand.Reader,
		s.kdcKeys.PrivateKey,
		crypto.SHA256,
		hashed[:],
		nil,
	)

	if err != nil {
		return nil, fmt.Errorf("sign_as_rep_failed: %w", err)
	}

	// @note 6. Wrap payload + signature
	finalData := SignedData{
		Payload:   payload,
		Signature: signature,
	}

	// @note 7. Serialize signed response
	finalDataBytes, err := json.Marshal(finalData)
	if err != nil {
		return nil, fmt.Errorf("marshal_signed_as_rep_failed: %w", err)
	}

	// @note 8. Retrieve client public key from CA Service
	clientPubKey, err := s.fetchPublicKeyFromCA(ctx, certSn)
	if err != nil {
		return nil, fmt.Errorf(
			"fetch_client_public_key_failed: %w",
			err,
		)
	}

	// @note 9. Encrypt AS_REP using client's RSA public key
	encryptedASRep, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		clientPubKey,
		finalDataBytes,
		[]byte("AS_REP"),
	)

	if err != nil {
		return nil, fmt.Errorf(
			"encrypt_as_rep_failed: %w",
			err,
		)
	}

	return encryptedASRep, nil
}
