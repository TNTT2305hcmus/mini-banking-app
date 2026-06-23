package kdc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

/**
 * @title KDC Service - AS EXCHANGE LOGIC
 * @author Le Nguyen Quoc Thai (lnqt), Truong Thanh Thuan
 * @summary Handles the AS Exchange: binds the requested identity to the client
 * certificate, verifies pre-authentication, issues the TGT and packages AS_REP.
 */

/////////////////////////////////////////////////////////
//					Constructor
////////////////////////////////////////////////////////

/**
 * @description Builds an AS service from injected, shared collaborators.
 * @param {ASConfig} cfg - CA certificate repository, replay store, clock, random source, KDC keys and TTLs.
 * @returns {(*ASService, error)} A ready AS service or a validation error when required dependencies are missing.
 */
func NewASService(cfg ASConfig) (*ASService, error) {
	if cfg.Keys == nil {
		return nil, errors.New("KDC keys are required")
	}
	if len(cfg.Keys.KTGSKey) != aes256KeySize {
		return nil, errors.New("K_tgs must be 32 bytes")
	}
	if cfg.Keys.PrivateKey == nil {
		return nil, errors.New("KDC private key is required")
	}
	if cfg.CertRepo == nil {
		return nil, errors.New("CertRepo is required")
	}
	if cfg.ReplayStore == nil {
		return nil, errors.New("ReplayStore is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = SystemClock{}
	}
	if cfg.Random == nil {
		cfg.Random = rand.Reader
	}
	if cfg.TGTTTL == 0 {
		cfg.TGTTTL = 5 * time.Minute
	}
	if cfg.TimestampWindow == 0 {
		cfg.TimestampWindow = 5 * time.Minute
	}

	return &ASService{
		certRepo:        cfg.CertRepo,
		replayStore:     cfg.ReplayStore,
		clock:           cfg.Clock,
		rand:            cfg.Random,
		timestampWindow: cfg.TimestampWindow,
		kdcKeys:         cfg.Keys,
		tgtTTL:          cfg.TGTTTL,
	}, nil
}

/////////////////////////////////////////////////////
//					AS Exchange
////////////////////////////////////////////////////

/**
 * @description Runs the full AS exchange and issues an encrypted AS_REP.
 * @note Order: freshness window, replay protection, identity-bound pre-auth,
 * session key, encrypted TGT, AS_REP. The client public key fetched during
 * authentication is reused to package AS_REP (single CA round-trip).
 * @param {context.Context} ctx - Request context for CA and replay dependencies.
 * @param {string} ownerID - Claimed client identity (must match the certificate owner).
 * @param {string} certSn - Client certificate serial number.
 * @param {[]byte} nonce - Client nonce_1, echoed back inside AS_REP.
 * @param {int64} timestamp - Client request timestamp (Unix seconds).
 * @param {[]byte} signature - Client signature over the canonical pre-auth payload.
 * @returns {([]byte, int64, error)} Marshaled AS_REP, TGT expiry (Unix seconds) and a KDC domain error.
 */
func (s *ASService) IssueTGT(ctx context.Context, ownerID, certSn string, nonce []byte, timestamp int64, signature []byte) ([]byte, int64, error) {
	if err := s.validateFreshness(timestamp); err != nil {
		return nil, 0, err
	}
	if err := s.checkASReplay(ctx, ownerID, nonce, timestamp); err != nil {
		return nil, 0, err
	}

	dataToVerify, err := buildASCanonicalPayload(certSn, ownerID, nonce, timestamp)
	if err != nil {
		return nil, 0, kdcError(ErrInternal, err)
	}

	clientPubKey, err := s.authenticateClient(ctx, certSn, ownerID, signature, dataToVerify)
	if err != nil {
		return nil, 0, err
	}

	sessionKey, err := newSessionKey(s.rand)
	if err != nil {
		return nil, 0, kdcError(ErrInternal, err)
	}

	issuedAt := s.clock.Now()
	tgt, err := s.encryptTGT(ownerID, sessionKey, certSn, issuedAt)
	if err != nil {
		return nil, 0, kdcError(ErrInternal, err)
	}

	asRep, err := s.BuildAS_REP(clientPubKey, sessionKey, tgt, nonce)
	if err != nil {
		return nil, 0, kdcError(ErrInternal, err)
	}

	return asRep, issuedAt.Add(s.tgtTTL).Unix(), nil
}

/**
 * @description Authenticates the client and binds the requested identity to its certificate.
 * @note FAIL-CLOSED binding: a valid signature only proves possession of the
 * certificate's private key — never ownership of owner_id. The CA-authoritative
 * owner_id MUST be present and equal to the claimed owner_id, otherwise the
 * request is rejected. This is the control that prevents owner_id forgery.
 * @param {context.Context} ctx - Request context for the certificate repository.
 * @param {string} certSn - Client certificate serial number.
 * @param {string} ownerID - Claimed client identity from the request.
 * @param {[]byte} signature - Client signature to verify.
 * @param {[]byte} dataToVerify - Canonical pre-auth payload that was signed.
 * @returns {(*rsa.PublicKey, error)} The validated client public key (reused for AS_REP) or a domain error.
 */
func (s *ASService) authenticateClient(ctx context.Context, certSn, ownerID string, signature, dataToVerify []byte) (*rsa.PublicKey, error) {
	cert, err := loadUsableCert(ctx, s.certRepo, certSn, s.clock.Now())
	if err != nil {
		return nil, err
	}

	// Bind against the CA-authoritative owner_id only. owner_id is set by the
	// gateway from the OTP-verified registration token (server-side, not client
	// controlled) and is the real identity key. SubjectCN is a cosmetic display
	// name (the CA sets it to the client-chosen full_name), so it must NOT be
	// used as an identity check.
	if cert.OwnerID == "" || cert.OwnerID != ownerID {
		return nil, kdcError(ErrIdentityMismatch, errors.New("owner_id does not match certificate owner"))
	}

	pubKey, err := parseRSAPublicKeyPEM(cert.PublicKeyPEM)
	if err != nil {
		return nil, kdcError(ErrAuthInvalid, err)
	}
	if err := verifySignature(pubKey, dataToVerify, signature); err != nil {
		return nil, err
	}
	return pubKey, nil
}

/**
 * @description Verifies a client pre-auth signature using the certificate public key.
 * @note Signature-only primitive retained for unit tests; the production flow uses
 * authenticateClient, which additionally enforces the owner_id binding.
 * @param {context.Context} ctx - Request context for the certificate repository.
 * @param {string} certSn - Certificate serial number.
 * @param {[]byte} signature - Signature to verify.
 * @param {[]byte} dataToVerify - Original signed data.
 * @returns {error} Nil if valid, KDC domain error otherwise.
 */
func (s *ASService) VerifyPreAuthSignature(ctx context.Context, certSn string, signature []byte, dataToVerify []byte) error {
	cert, err := loadUsableCert(ctx, s.certRepo, certSn, s.clock.Now())
	if err != nil {
		return err
	}
	pubKey, err := parseRSAPublicKeyPEM(cert.PublicKeyPEM)
	if err != nil {
		return kdcError(ErrAuthInvalid, err)
	}
	return verifySignature(pubKey, dataToVerify, signature)
}

/**
 * @description Verifies an RSA signature, accepting legacy PKCS#1 v1.5 and blueprint-preferred RSA-PSS.
 * @param {*rsa.PublicKey} pubKey - Signer public key.
 * @param {[]byte} dataToVerify - Signed data.
 * @param {[]byte} signature - Signature bytes.
 * @returns {error} Nil if valid, AUTH_INVALID otherwise.
 */
func verifySignature(pubKey *rsa.PublicKey, dataToVerify []byte, signature []byte) error {
	hashed := sha256.Sum256(dataToVerify)
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature); err == nil {
		return nil
	}
	if err := rsa.VerifyPSS(pubKey, crypto.SHA256, hashed[:], signature, nil); err != nil {
		return kdcError(ErrAuthInvalid, fmt.Errorf("signature_verification_failed: %w", err))
	}
	return nil
}

/**
 * @description Generates a secure random 256-bit session key (K_c_tgs).
 * @returns {([]byte, error)} The 32-byte key or an error.
 */
func (s *ASService) GenerateSessionKey() ([]byte, error) {
	return newSessionKey(s.rand)
}

/**
 * @description Ensures the request timestamp is within the configured freshness window.
 * @param {int64} timestamp - Client request timestamp (Unix seconds).
 * @returns {error} REQUEST_EXPIRED when outside the window.
 */
func (s *ASService) validateFreshness(timestamp int64) error {
	if !timestampWithinWindow(s.clock.Now(), timestamp, s.timestampWindow) {
		return kdcError(ErrRequestExpired, nil)
	}
	return nil
}

/**
 * @description Records an AS replay marker and rejects duplicate identity/nonce/timestamp tuples.
 * @param {context.Context} ctx - Request context for the replay store.
 * @param {string} clientID - Claimed client identity.
 * @param {[]byte} nonce - Client nonce.
 * @param {int64} timestamp - Client request timestamp.
 * @returns {error} REPLAY_DETECTED for duplicates, INTERNAL_ERROR on store failure.
 */
func (s *ASService) checkASReplay(ctx context.Context, clientID string, nonce []byte, timestamp int64) error {
	nonceText := base64.StdEncoding.EncodeToString(nonce)
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", clientID, nonceText, timestamp)))
	key := "replay:as:" + hex.EncodeToString(hash[:])

	ok, err := s.replayStore.SetNX(ctx, key, "1", s.timestampWindow)
	if err != nil {
		return kdcError(ErrInternal, err)
	}
	if !ok {
		return kdcError(ErrReplayDetected, nil)
	}
	return nil
}

///////////////////////////////////////////////////////////
//					TGT and AS_REP
//////////////////////////////////////////////////////////

/**
 * @description Builds and AES-256-GCM encrypts a TGT with K_tgs (so only the TGS can read it).
 * @param {string} clientId - Authenticated client identifier.
 * @param {[]byte} k_ctgs - Session key K_c_tgs.
 * @param {string} certSn - Client certificate serial number embedded for TGS-side identity checks.
 * @returns {([]byte, error)} Encrypted TGT bytes or an error.
 */
func (s *ASService) GenerateEncryptedTGT(clientId string, k_ctgs []byte, certSn string) ([]byte, error) {
	return s.encryptTGT(clientId, k_ctgs, certSn, s.clock.Now())
}

func (s *ASService) encryptTGT(clientId string, k_ctgs []byte, certSn string, issuedAt time.Time) ([]byte, error) {
	if clientId == "" {
		return nil, fmt.Errorf("client_id is required")
	}
	if len(k_ctgs) == 0 {
		return nil, fmt.Errorf("session key k_c_tgs is empty")
	}

	exp := issuedAt.Add(s.tgtTTL).Unix()
	payload := TGT{
		ClientId:   clientId,
		CertSN:     certSn,
		SessionKey: k_ctgs,
		IssuedAt:   issuedAt.Unix(),
		Expiry:     exp,
		ExpiresAt:  exp,
	}

	encryptedTGT, err := encryptJSON(s.kdcKeys.KTGSKey, payload, s.rand)
	if err != nil {
		return nil, fmt.Errorf("encrypt_tgt_failed: %w", err)
	}
	return encryptedTGT, nil
}

/**
 * @description Builds the AS_REP for the client using hybrid encryption.
 * @note The payload is signed with the KDC RSA-PSS private key, encrypted with a
 * fresh AES-256 key, and that key is wrapped with the client's RSA public key
 * (RSA-OAEP, label "AS_REP"). Hybrid encryption is required because the payload
 * is larger than a single RSA block.
 * @param {*rsa.PublicKey} clientPubKey - Client public key (already fetched during authentication).
 * @param {[]byte} k_ctgs - Session key K_c_tgs.
 * @param {[]byte} tgt - Encrypted TGT.
 * @param {[]byte} nonce1 - Client nonce echoed back to the client.
 * @returns {([]byte, error)} Marshaled AS_REP (ASResponse) or an error.
 */
func (s *ASService) BuildAS_REP(clientPubKey *rsa.PublicKey, k_ctgs []byte, tgt []byte, nonce1 []byte) ([]byte, error) {
	if clientPubKey == nil {
		return nil, fmt.Errorf("client public key is required")
	}
	if len(k_ctgs) == 0 {
		return nil, fmt.Errorf("session key k_c_tgs is empty")
	}
	if len(tgt) == 0 {
		return nil, fmt.Errorf("tgt is empty")
	}
	if len(nonce1) == 0 {
		return nil, fmt.Errorf("nonce1 is empty")
	}

	payload := ASRepPayload{
		SessionKey: k_ctgs,
		TGT:        tgt,
		Nonce1:     nonce1,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal_as_rep_payload_failed: %w", err)
	}

	hashed := sha256.Sum256(payloadBytes)
	signature, err := rsa.SignPSS(s.rand, s.kdcKeys.PrivateKey, crypto.SHA256, hashed[:], nil)
	if err != nil {
		return nil, fmt.Errorf("sign_as_rep_payload_failed: %w", err)
	}

	// Hybrid encryption: encrypt the payload with a fresh AES key, then wrap that
	// AES key with the client's RSA public key.
	aesKey, err := newSessionKey(s.rand)
	if err != nil {
		return nil, fmt.Errorf("generate_ephemeral_aes_key_failed: %w", err)
	}

	encryptedPayload, err := encryptJSON(aesKey, payload, s.rand)
	if err != nil {
		return nil, fmt.Errorf("encrypt_as_rep_payload_failed: %w", err)
	}

	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), s.rand, clientPubKey, aesKey, []byte("AS_REP"))
	if err != nil {
		return nil, fmt.Errorf("encrypt_ephemeral_aes_key_failed: %w", err)
	}

	asRep, err := json.Marshal(ASResponse{
		KDCSignature:     signature,
		EncryptedKey:     encryptedKey,
		EncryptedPayload: encryptedPayload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal_as_rep_failed: %w", err)
	}
	return asRep, nil
}

/**
 * @description Canonical pre-auth payload signed by the client and verified by the AS.
 */
type asCanonicalPayload struct {
	CertSn    string `json:"cert_sn"`
	OwnerId   string `json:"owner_id"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

/**
 * @description Serializes the canonical pre-auth payload for signature verification.
 * @returns {([]byte, error)} JSON bytes that must match what the client signed.
 */
func buildASCanonicalPayload(certSn, ownerID string, nonce []byte, timestamp int64) ([]byte, error) {
	return json.Marshal(asCanonicalPayload{
		CertSn:    certSn,
		OwnerId:   ownerID,
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Timestamp: timestamp,
	})
}
