package kdc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

/**
 * @title KDC Service - TGS EXCHANGE LOGIC
 * @author Nguyen Trinh Quang
 * @summary Handling TGS Exchange, integrating with CA Service for identity verification.
 */

/////////////////////////////////////////////////////////
//					Constructor
////////////////////////////////////////////////////////

/**
 * @description Builds a KDC service and validates all security-sensitive dependencies.
 * @param {Config} cfg - Service dependencies, keys, clocks, random source, and TTL settings.
 * @returns {*Service} Ready-to-use KDC service.
 * @returns {error} Validation failure when required keys or dependencies are missing.
 */
func NewTGSService(cfg Config) (*TGSService, error) {
	if len(cfg.TGSKey) != aes256KeySize {
		return nil, errors.New("TGSKey must be 32 bytes")
	}
	if cfg.ReplayStore == nil {
		return nil, errors.New("ReplayStore is required")
	}
	if cfg.CertRepo == nil {
		return nil, errors.New("CertRepo is required")
	}
	if cfg.ScopeAuthorizer == nil {
		return nil, errors.New("ScopeAuthorizer is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = SystemClock{}
	}
	if cfg.Random == nil {
		cfg.Random = rand.Reader
	}
	if cfg.TicketTTL == 0 {
		cfg.TicketTTL = 5 * time.Minute
	}
	if cfg.TimestampWindow == 0 {
		cfg.TimestampWindow = 5 * time.Minute
	}
	if cfg.ReplayTTL == 0 {
		cfg.ReplayTTL = 5 * time.Minute
	}

	serviceKeys := make(map[string][]byte, len(cfg.ServiceKeys))
	for serviceID, key := range cfg.ServiceKeys {
		if len(key) != aes256KeySize {
			return nil, fmt.Errorf("service key for %q must be 32 bytes", serviceID)
		}
		copied := append([]byte(nil), key...)
		serviceKeys[serviceID] = copied
	}

	return &TGSService{
		tgsKey:          append([]byte(nil), cfg.TGSKey...),
		serviceKeys:     serviceKeys,
		replayStore:     cfg.ReplayStore,
		certRepo:        cfg.CertRepo,
		scopeAuthorizer: cfg.ScopeAuthorizer,
		clock:           cfg.Clock,
		rand:            cfg.Random,
		ticketTTL:       cfg.TicketTTL,
		timestampWindow: cfg.TimestampWindow,
		replayTTL:       cfg.ReplayTTL,
	}, nil
}

///////////////////////////////////////////////////////////////////
//						TGS Exchange
//////////////////////////////////////////////////////////////////

/**
 * @description Processes a TGS exchange request and issues a scoped service ticket.
 * @param {context.Context} ctx - Request context used by replay and certificate dependencies.
 * @param {TGSRequest} req - TGT, authenticator, certificate serial, nonce, service ID, and requested scope.
 * @returns {TGSResponse} Encrypted reply for the client plus ticket expiry metadata.
 * @returns {error} KDC domain error when validation or ticket creation fails.
 */
func (s *TGSService) RequestServiceTicket(ctx context.Context, req TGSRequest) (TGSResponse, error) {
	tgt, err := s.decryptTGT(req.TGTCiphertext)
	if err != nil {
		return TGSResponse{}, err
	}

	auth, err := s.decryptAuthenticator(tgt.KCTGS, req.Authenticator)
	if err != nil {
		return TGSResponse{}, err
	}
	if auth.ClientID != tgt.ClientID {
		return TGSResponse{}, kdcError(ErrIdentityMismatch, nil)
	}
	if auth.RequestedService != req.ServiceID {
		return TGSResponse{}, kdcError(ErrAuthInvalid, errors.New("requested service mismatch"))
	}
	if auth.Scope != req.RequestedScope {
		return TGSResponse{}, kdcError(ErrScopeDenied, errors.New("scope mismatch"))
	}
	if err := s.validateTimestampWindow(auth.Timestamp); err != nil {
		return TGSResponse{}, err
	}
	nonce2 := req.Nonce2
	if len(nonce2) == 0 {
		var err error
		nonce2, err = base64.StdEncoding.DecodeString(auth.NonceReq)
		if err != nil {
			return TGSResponse{}, kdcError(ErrAuthInvalid, errors.New("malformed authenticator nonce"))
		}
	} else if auth.NonceReq != base64.StdEncoding.EncodeToString(nonce2) {
		return TGSResponse{}, kdcError(ErrAuthInvalid, errors.New("nonce mismatch"))
	}

	if err := s.checkReplay(ctx, tgt.ClientID, auth.NonceReq, auth.Timestamp, req.ServiceID, auth.RequestID); err != nil {
		return TGSResponse{}, err
	}

	certSN := req.CertSN
	if certSN == "" {
		certSN = tgt.CertSN
	}
	cert, err := s.checkRevocation(ctx, certSN)
	if err != nil {
		return TGSResponse{}, err
	}
	// Identity is keyed on the CA-authoritative owner_id, which the AS already
	// bound into the TGT at issue time. SubjectCN is a cosmetic full_name and is
	// intentionally not used as an identity check.
	if cert.OwnerID != "" && cert.OwnerID != tgt.ClientID {
		return TGSResponse{}, kdcError(ErrIdentityMismatch, nil)
	}

	allowed, err := s.scopeAuthorizer.Allowed(ctx, tgt.ClientID, req.ServiceID, req.RequestedScope)
	if err != nil {
		return TGSResponse{}, err
	}
	if !allowed {
		return TGSResponse{}, kdcError(ErrScopeDenied, nil)
	}

	kcv, err := newSessionKey(s.rand)
	if err != nil {
		return TGSResponse{}, kdcError(ErrInternal, err)
	}
	ticketV, expiresAt, err := s.buildServiceTicket(req.ServiceID, tgt.ClientID, certSN, cert.PublicKeyPEM, req.RequestedScope, auth.NonceReq, kcv)
	if err != nil {
		return TGSResponse{}, err
	}
	encryptedReply, err := s.encryptTGSReply(req.ServiceID, tgt.KCTGS, kcv, ticketV, nonce2, auth.NonceReq, expiresAt, req.RequestedScope)
	if err != nil {
		return TGSResponse{}, err
	}

	return TGSResponse{
		EncryptedPayload: encryptedReply,
		TicketExpiryUnix: expiresAt.Unix(),
	}, nil
}

/**
 * @description Decrypts and validates a TGT using the TGS master key.
 * @param {[]byte} tgtCiphertext - Encrypted TGT supplied by the client.
 * @returns {TGTPlaintext} Valid plaintext TGT.
 * @returns {error} Domain error for malformed, invalid, or expired TGTs.
 */
func (s *TGSService) decryptTGT(tgtCiphertext []byte) (TGTPlaintext, error) {
	tgt, err := decryptJSON[TGTPlaintext](s.tgsKey, tgtCiphertext)
	if err != nil {
		return TGTPlaintext{}, kdcError(ErrAuthInvalid, err)
	}
	if tgt.Expiry == 0 {
		tgt.Expiry = tgt.ExpiresAt
	}
	if tgt.ClientID == "" || len(tgt.KCTGS) != aes256KeySize || tgt.Expiry == 0 {
		return TGTPlaintext{}, kdcError(ErrAuthInvalid, errors.New("malformed TGT"))
	}
	if !time.Unix(tgt.Expiry, 0).After(s.clock.Now()) {
		return TGTPlaintext{}, kdcError(ErrTGTExpired, nil)
	}
	return tgt, nil
}

/**
 * @description Decrypts and validates the client authenticator with K_c_tgs.
 * @param {[]byte} kctgs - Client-to-TGS session key extracted from the TGT.
 * @param {[]byte} authenticator - Encrypted authenticator supplied by the client.
 * @returns {AuthenticatorPlaintext} Valid authenticator payload.
 * @returns {error} Domain error for malformed or undecryptable authenticators.
 */
func (s *TGSService) decryptAuthenticator(kctgs []byte, authenticator []byte) (AuthenticatorPlaintext, error) {
	auth, err := decryptJSON[AuthenticatorPlaintext](kctgs, authenticator)
	if err != nil {
		return AuthenticatorPlaintext{}, kdcError(ErrAuthInvalid, err)
	}
	if auth.ClientID == "" {
		auth.ClientID = auth.IdC
	}
	if auth.Timestamp == 0 {
		auth.Timestamp = auth.TimestampUnix
	}
	if auth.NonceReq == "" {
		auth.NonceReq = auth.Nonce
	}
	if auth.RequestedService == "" {
		auth.RequestedService = auth.ServiceID
	}
	if auth.ClientID == "" || auth.Timestamp == 0 || auth.NonceReq == "" || auth.RequestID == "" || auth.RequestedService == "" || auth.Scope == "" {
		return AuthenticatorPlaintext{}, kdcError(ErrAuthInvalid, errors.New("malformed authenticator"))
	}
	return auth, nil
}

/**
 * @description Ensures an authenticator timestamp is within the configured clock-skew window.
 * @param {int64} ts - Authenticator Unix timestamp.
 * @returns {error} REQUEST_EXPIRED when the timestamp is outside the allowed window.
 */
func (s *TGSService) validateTimestampWindow(ts int64) error {
	if !timestampWithinWindow(s.clock.Now(), ts, s.timestampWindow) {
		return kdcError(ErrRequestExpired, nil)
	}
	return nil
}

/**
 * @description Reports whether a Unix timestamp is within +/- window of now.
 * @note Shared by the AS freshness check (validateFreshness) and the TGS
 * authenticator timestamp check (validateTimestampWindow).
 * @param {time.Time} now - Reference time.
 * @param {int64} unixTs - Timestamp to test, in Unix seconds.
 * @param {time.Duration} window - Maximum allowed absolute skew.
 * @returns {bool} True when within the window.
 */
func timestampWithinWindow(now time.Time, unixTs int64, window time.Duration) bool {
	delta := now.Sub(time.Unix(unixTs, 0))
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

/**
 * @description Records a replay key and rejects duplicate client nonce/timestamp combinations.
 * @param {context.Context} ctx - Request context for the replay store.
 * @param {string} clientID - Client identity from the TGT.
 * @param {string} nonceReq - Client request nonce from the authenticator.
 * @param {int64} ts - Authenticator Unix timestamp.
 * @returns {error} REPLAY_DETECTED for duplicates or INTERNAL_ERROR for store failures.
 */
func (s *TGSService) checkReplay(ctx context.Context, clientID string, nonceReq string, ts int64, serviceID string, requestID string) error {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s:%s", clientID, nonceReq, ts, serviceID, requestID)))
	key := "replay:tgs:" + hex.EncodeToString(hash[:])
	ok, err := s.replayStore.SetNX(ctx, key, "1", s.replayTTL)
	if err != nil {
		return kdcError(ErrInternal, err)
	}
	if !ok {
		return kdcError(ErrReplayDetected, nil)
	}
	return nil
}

/**
 * @description Loads certificate metadata and enforces revocation, expiry, identity, and public key requirements.
 * @param {context.Context} ctx - Request context for the certificate repository.
 * @param {string} certSN - Client certificate serial number.
 * @returns {Certificate} Valid certificate metadata.
 * @returns {error} Domain error for missing, revoked, expired, or malformed certificates.
 */
func (s *TGSService) checkRevocation(ctx context.Context, certSN string) (Certificate, error) {
	return loadUsableCert(ctx, s.certRepo, certSN, s.clock.Now())
}

/**
 * @description Builds the encrypted service ticket that only the target service can decrypt.
 * @param {string} serviceID - Target service identifier.
 * @param {string} clientID - Authenticated client identifier.
 * @param {string} certSN - Client certificate serial number.
 * @param {string} publicKeyPEM - Client public key embedded for downstream signature checks.
 * @param {string} scope - Authorized service scope.
 * @param {string} nonceReq - Client request nonce for binding and audit.
 * @param {[]byte} kcv - Client-to-service session key.
 * @returns {[]byte} Encrypted service ticket.
 * @returns {time.Time} Ticket expiry timestamp.
 * @returns {error} Domain error for unknown services or encryption failures.
 */
func (s *TGSService) buildServiceTicket(serviceID string, clientID string, certSN string, publicKeyPEM string, scope string, nonceReq string, kcv []byte) ([]byte, time.Time, error) {
	serviceKey, ok := s.serviceKeys[serviceID]
	if !ok {
		return nil, time.Time{}, kdcError(ErrServiceUnknown, nil)
	}
	issuedAt := s.clock.Now()
	expiresAt := issuedAt.Add(s.ticketTTL)
	ticket := ServiceTicketPlaintext{
		ClientID:  clientID,
		ServiceID: serviceID,
		SName:     serviceID,
		KCV:       append([]byte(nil), kcv...),
		PublicKey: publicKeyPEM,
		PubCPEM:   publicKeyPEM,
		CertSN:    certSN,
		Scope:     scope,
		NonceReq:  nonceReq,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	ciphertext, err := encryptJSON(serviceKey, ticket, s.rand)
	if err != nil {
		return nil, time.Time{}, kdcError(ErrInternal, err)
	}
	return ciphertext, expiresAt, nil
}

/**
 * @description Encrypts the TGS reply for the client with K_c_tgs.
 * @param {string} serviceID - Target service identifier.
 * @param {[]byte} kctgs - Client-to-TGS session key.
 * @param {[]byte} kcv - Client-to-service session key.
 * @param {[]byte} ticketV - Encrypted service ticket.
 * @param {[]byte} nonce2 - Client nonce echoed for freshness.
 * @param {string} nonceReq - Authenticator nonce echoed for request binding.
 * @param {time.Time} expiresAt - Service ticket expiry timestamp.
 * @param {string} scope - Authorized service scope.
 * @returns {[]byte} Encrypted TGS reply.
 * @returns {error} Domain error for encryption failures.
 */
func (s *TGSService) encryptTGSReply(serviceID string, kctgs []byte, kcv []byte, ticketV []byte, nonce2 []byte, nonceReq string, expiresAt time.Time, scope string) ([]byte, error) {
	issuedAt := s.clock.Now()
	reply := TGSReplyPlaintext{
		KCV:       append([]byte(nil), kcv...),
		ServiceID: serviceID,
		TicketV:   append([]byte(nil), ticketV...),
		Nonce2:    append([]byte(nil), nonce2...),
		NonceReq:  nonceReq,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: expiresAt.Unix(),
		Scope:     scope,
	}
	ciphertext, err := encryptJSON(kctgs, reply, s.rand)
	if err != nil {
		return nil, kdcError(ErrInternal, err)
	}
	return ciphertext, nil
}
