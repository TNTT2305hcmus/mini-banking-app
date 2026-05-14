package kdc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

func NewService(cfg Config) (*Service, error) {
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

	return &Service{
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

func (s *Service) RequestServiceTicket(ctx context.Context, req TGSRequest) (TGSResponse, error) {
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
	nonceReq := base64.StdEncoding.EncodeToString(req.Nonce2)
	if auth.NonceReq != nonceReq {
		return TGSResponse{}, kdcError(ErrAuthInvalid, errors.New("nonce mismatch"))
	}

	if err := s.checkReplay(ctx, tgt.ClientID, auth.NonceReq, auth.Timestamp); err != nil {
		return TGSResponse{}, err
	}

	cert, err := s.checkRevocation(ctx, req.CertSN)
	if err != nil {
		return TGSResponse{}, err
	}
	if cert.SubjectCN != "" && cert.SubjectCN != tgt.ClientID {
		return TGSResponse{}, kdcError(ErrIdentityMismatch, nil)
	}

	allowed, err := s.scopeAuthorizer.Allowed(ctx, tgt.ClientID, req.ServiceID, req.RequestedScope)
	if err != nil {
		return TGSResponse{}, err
	}
	if !allowed {
		return TGSResponse{}, kdcError(ErrScopeDenied, nil)
	}

	kcv := make([]byte, aes256KeySize)
	if _, err := io.ReadFull(s.rand, kcv); err != nil {
		return TGSResponse{}, kdcError(ErrInternal, err)
	}
	ticketV, expiresAt, err := s.buildServiceTicket(req.ServiceID, tgt.ClientID, req.CertSN, cert.PublicKeyPEM, req.RequestedScope, auth.NonceReq, kcv)
	if err != nil {
		return TGSResponse{}, err
	}
	encryptedReply, err := s.encryptTGSReply(req.ServiceID, tgt.KCTGS, kcv, ticketV, req.Nonce2, auth.NonceReq, expiresAt, req.RequestedScope)
	if err != nil {
		return TGSResponse{}, err
	}

	return TGSResponse{
		EncryptedPayload: encryptedReply,
		TicketExpiryUnix: expiresAt.Unix(),
	}, nil
}

func (s *Service) decryptTGT(tgtCiphertext []byte) (TGTPlaintext, error) {
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

func (s *Service) decryptAuthenticator(kctgs []byte, authenticator []byte) (AuthenticatorPlaintext, error) {
	auth, err := decryptJSON[AuthenticatorPlaintext](kctgs, authenticator)
	if err != nil {
		return AuthenticatorPlaintext{}, kdcError(ErrAuthInvalid, err)
	}
	if auth.ClientID == "" || auth.Timestamp == 0 || auth.NonceReq == "" || auth.RequestedService == "" || auth.Scope == "" {
		return AuthenticatorPlaintext{}, kdcError(ErrAuthInvalid, errors.New("malformed authenticator"))
	}
	return auth, nil
}

func (s *Service) validateTimestampWindow(ts int64) error {
	now := s.clock.Now()
	delta := now.Sub(time.Unix(ts, 0))
	if delta < 0 {
		delta = -delta
	}
	if delta > s.timestampWindow {
		return kdcError(ErrRequestExpired, nil)
	}
	return nil
}

func (s *Service) checkReplay(ctx context.Context, clientID string, nonceReq string, ts int64) error {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", clientID, nonceReq, ts)))
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

func (s *Service) checkRevocation(ctx context.Context, certSN string) (Certificate, error) {
	cert, err := s.certRepo.GetCertificate(ctx, certSN)
	if errors.Is(err, ErrCertificateMissing) {
		return Certificate{}, kdcError(ErrCertNotFound, err)
	}
	if err != nil {
		return Certificate{}, kdcError(ErrInternal, err)
	}
	switch cert.Status {
	case CertificateRevoked:
		return Certificate{}, kdcError(ErrCertRevoked, nil)
	case CertificateExpired:
		return Certificate{}, kdcError(ErrCertExpired, nil)
	case CertificateValid, CertificateActive:
	default:
		return Certificate{}, kdcError(ErrCertNotFound, nil)
	}
	if cert.NotAfter.IsZero() || !cert.NotAfter.After(s.clock.Now()) {
		return Certificate{}, kdcError(ErrCertExpired, nil)
	}
	if cert.PublicKeyPEM == "" {
		return Certificate{}, kdcError(ErrAuthInvalid, errors.New("certificate missing public key"))
	}
	return cert, nil
}

func (s *Service) buildServiceTicket(serviceID string, clientID string, certSN string, publicKeyPEM string, scope string, nonceReq string, kcv []byte) ([]byte, time.Time, error) {
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

func (s *Service) encryptTGSReply(serviceID string, kctgs []byte, kcv []byte, ticketV []byte, nonce2 []byte, nonceReq string, expiresAt time.Time, scope string) ([]byte, error) {
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
