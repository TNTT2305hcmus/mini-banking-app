package grpc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/banking-service/internal/bank"
	capb "mini-banking/pkg/pb/ca"
)

// This file holds the AP-exchange (Kerberos-style) security gate that fronts the
// banking domain: ticket/authenticator decryption, freshness, certificate check,
// replay protection, payload signature verification, and AP_REP encryption.

func (h *Handler) authorize(ctx context.Context, ticketCipher []byte, authenticatorCipher []byte, requiredScope string) (authInfo, error) {
	var out authInfo
	if h.db == nil || h.ca == nil || len(h.bankKV) != aes256KeySize {
		return out, status.Error(codes.FailedPrecondition, "bank handler dependencies are not configured")
	}
	ticket, err := h.decryptTicket(ticketCipher)
	if err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", Reason: "invalid_ticket"})
		return out, status.Error(codes.Unauthenticated, "INVALID_TICKET")
	}
	out.ticket = ticket
	out.sessionKey = ticket.session
	if h.clock.Now().After(ticket.expires) || h.clock.Now().Equal(ticket.expires) {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "ticket_expired"})
		return out, status.Error(codes.Unauthenticated, "INVALID_TICKET")
	}
	if ticket.scope != requiredScope {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "wrong_scope"})
		return out, status.Error(codes.PermissionDenied, "WRONG_SCOPE")
	}

	plain, err := decryptAESGCMEmbedded(ticket.session, authenticatorCipher)
	if err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "invalid_authenticator"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	var authn bank.Authenticator
	if err := json.Unmarshal(plain, &authn); err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "invalid_authenticator"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	if authn.ClientID != ticket.clientID {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "authenticator_client_mismatch"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	out.nonce = authn.Nonce
	out.requestID = authn.RequestID
	if authn.Timestamp == 0 || out.nonce == "" || out.requestID == "" {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "authenticator_missing_fields"})
		return out, status.Error(codes.InvalidArgument, "BAD_REQUEST")
	}
	out.timestamp = time.Unix(authn.Timestamp, 0).UTC()
	if absDuration(h.clock.Now().Sub(out.timestamp)) > freshnessWindow {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "stale_request"})
		return out, status.Error(codes.Unauthenticated, "STALE_REQUEST")
	}

	cert, err := h.ca.VerifyCertificate(ctx, &capb.VerifyCertificateRequest{
		SerialNumber:          ticket.certSN,
		Caller:                "bank-service",
		IncludePublicKeyPem:   true,
		IncludeCertificatePem: false,
	}, grpc.WaitForReady(false))
	if err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "ca_unavailable"})
		return out, status.Error(codes.Unavailable, "SERVICE_UNAVAILABLE")
	}
	nowUnix := h.clock.Now().Unix()
	if cert.GetStatus() != capb.CertStatus_CERT_STATUS_ACTIVE ||
		cert.GetRevokedAtUnix() > 0 ||
		cert.GetNotBeforeUnix() > nowUnix ||
		(cert.GetNotAfterUnix() > 0 && nowUnix >= cert.GetNotAfterUnix()) {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "certificate_not_active"})
		return out, status.Error(codes.Unauthenticated, "CERT_REJECTED")
	}
	if cert.GetOwnerId() != "" && cert.GetOwnerId() != ticket.clientID {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "certificate_owner_mismatch"})
		return out, status.Error(codes.Unauthenticated, "CERT_REJECTED")
	}
	if cert.GetRole() == capb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN {
		out.identityRole = bank.IdentityRoleBankAdmin
	} else {
		// UNKNOWN preserves certificates issued before the CA role migration.
		out.identityRole = bank.IdentityRoleCustomer
	}
	out.publicKeyPEM = cert.GetPublicKeyPem()
	if out.publicKeyPEM == "" {
		out.publicKeyPEM = ticket.publicKeyPEM
	}
	return out, nil
}

// decryptTicket decrypts Ticket_V with the bank's service key and parses it into
// the shared bank.ServiceTicketPlaintext contract (see internal/bank/types.go),
// which mirrors the KDC producer type field-for-field.
func (h *Handler) decryptTicket(ciphertext []byte) (ticketInfo, error) {
	plain, err := decryptAESGCMEmbedded(h.bankKV, ciphertext)
	if err != nil {
		return ticketInfo{}, err
	}
	var t bank.ServiceTicketPlaintext
	if err := json.Unmarshal(plain, &t); err != nil {
		return ticketInfo{}, err
	}
	if t.ClientID == "" || t.CertSN == "" || t.Scope == "" || len(t.KCV) != aes256KeySize || t.ExpiresAt == 0 {
		return ticketInfo{}, errors.New("ticket missing required fields")
	}
	publicKeyPEM := t.PubCPEM
	if publicKeyPEM == "" {
		publicKeyPEM = t.PublicKey
	}
	return ticketInfo{
		clientID:     t.ClientID,
		certSN:       t.CertSN,
		scope:        t.Scope,
		session:      t.KCV,
		expires:      time.Unix(t.ExpiresAt, 0).UTC(),
		publicKeyPEM: publicKeyPEM,
	}, nil
}

func (h *Handler) decryptTransferPayload(cipherPayload []byte, iv []byte, sessionKey []byte) (transferPayload, []byte, []byte, error) {
	var raw []byte
	var err error
	if len(iv) > 0 {
		raw, err = decryptAESGCMDetached(sessionKey, iv, cipherPayload)
	} else {
		raw, err = decryptAESGCMEmbedded(sessionKey, cipherPayload)
	}
	if err != nil {
		return transferPayload{}, nil, nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transferPayload{}, nil, nil, err
	}

	var payloadRaw json.RawMessage
	var sigText string
	if v, ok := envelope["payload"]; ok {
		payloadRaw = v
		if s, ok := envelope["client_signature"]; ok {
			_ = json.Unmarshal(s, &sigText)
		}
	} else {
		payloadMap := map[string]json.RawMessage{}
		for k, v := range envelope {
			if k == "client_signature" || k == "signature" {
				_ = json.Unmarshal(v, &sigText)
				continue
			}
			payloadMap[k] = v
		}
		payloadRaw, err = json.Marshal(payloadMap)
		if err != nil {
			return transferPayload{}, nil, nil, err
		}
	}
	if sigText == "" {
		if s, ok := envelope["signature"]; ok {
			_ = json.Unmarshal(s, &sigText)
		}
	}
	signature, err := base64.StdEncoding.DecodeString(sigText)
	if err != nil {
		signature, err = hex.DecodeString(sigText)
	}
	if err != nil || len(signature) == 0 {
		return transferPayload{}, nil, nil, errors.New("missing signature")
	}

	canonical, err := canonicalJSON(payloadRaw)
	if err != nil {
		return transferPayload{}, nil, nil, err
	}
	var payload transferPayload
	if err := json.Unmarshal(canonical, &payload); err != nil {
		return transferPayload{}, nil, nil, err
	}
	if payload.Currency == "" {
		payload.Currency = "VND"
	}
	if payload.Amount <= 0 || payload.FromAccountNumber == "" || payload.ToAccountNumber == "" || payload.IdempotencyKey == "" {
		return transferPayload{}, nil, nil, errors.New("invalid transfer fields")
	}
	return payload, canonical, signature, nil
}

func (h *Handler) markReplay(ctx context.Context, auth authInfo) error {
	hash := sha256.Sum256([]byte(auth.ticket.clientID + "|" + auth.nonce + "|" + strconv.FormatInt(auth.timestamp.Unix(), 10) + "|" + auth.requestID))
	key := "replay:bank:" + hex.EncodeToString(hash[:])
	if h.replay != nil {
		ok, err := h.replay.SetNX(ctx, key, "1", replayTTL)
		if err == nil {
			if !ok {
				h.bank.Audit(ctx, bank.AuditEvent{Action: "replay_detected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "redis_replay"})
				return status.Error(codes.Unauthenticated, "REPLAY_DETECTED")
			}
			return nil
		}
	}
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO used_nonces(nonce, used_at, expires_at) VALUES ($1, $2, $3)`,
		key, h.clock.Now(), h.clock.Now().Add(replayTTL))
	if err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "replay_detected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "db_replay"})
		return status.Error(codes.Unauthenticated, "REPLAY_DETECTED")
	}
	return nil
}

func (h *Handler) encryptAPRep(key []byte, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encryptAESGCMEmbedded(key, body, h.random)
}

func encryptAESGCMEmbedded(key []byte, plaintext []byte, random io.Reader) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...), nil
}

func decryptAESGCMEmbedded(key []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
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
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func decryptAESGCMDetached(key []byte, nonce []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce length")
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func verifyRSAPSS(publicKeyPEM string, payload []byte, signature []byte) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return errors.New("invalid public key pem")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if cert, certErr := x509.ParseCertificate(block.Bytes); certErr == nil {
			pubAny = cert.PublicKey
		} else {
			return err
		}
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return errors.New("unsupported public key type")
	}
	sum := sha256.Sum256(payload)
	return rsa.VerifyPSS(pub, crypto.SHA256, sum[:], signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
