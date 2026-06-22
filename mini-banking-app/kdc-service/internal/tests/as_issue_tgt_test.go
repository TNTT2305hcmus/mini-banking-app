package kdc_test

// as_issue_tgt_test.go
// Tests for the AS exchange orchestrator IssueTGT, including the security
// regression for owner_id forgery:
//
//   TestIssueTGT_HappyPath            — matching owner_id issues a TGT bound to that identity
//   TestIssueTGT_ForgedOwnerRejected  — valid foreign cert + faked owner_id is rejected (fail-closed)
//   TestIssueTGT_ReplayRejected       — the same request replayed is rejected

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// asCanonical mirrors the unexported canonical pre-auth payload in package kdc.
// Field order MUST match so the bytes (and therefore the signature) are identical.
type asCanonical struct {
	CertSn    string `json:"cert_sn"`
	OwnerId   string `json:"owner_id"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

// signCanonical produces a client RSA-PSS signature over the canonical payload.
func signCanonical(t *testing.T, priv *rsa.PrivateKey, certSn, ownerID string, nonce []byte, ts int64) []byte {
	t.Helper()
	canonical, err := json.Marshal(asCanonical{
		CertSn:    certSn,
		OwnerId:   ownerID,
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("marshal canonical: %v", err)
	}
	hashed := sha256.Sum256(canonical)
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, hashed[:], nil)
	if err != nil {
		t.Fatalf("sign canonical: %v", err)
	}
	return sig
}

func TestIssueTGT_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	nonce := randBytes(t, 16)
	ts := f.clock.now.Unix()
	sig := signCanonical(t, f.clientPriv, f.certSN, f.ownerID, nonce, ts)

	asRep, exp, err := f.svc.IssueTGT(context.Background(), f.ownerID, f.certSN, nonce, ts, sig)
	if err != nil {
		t.Fatalf("IssueTGT happy path: %v", err)
	}
	if exp <= 0 {
		t.Errorf("TGT expiry phải > 0, got %d", exp)
	}

	payload, err := clientDecryptASREP(asRep, f.clientPriv, &f.kdcPriv.PublicKey)
	if err != nil {
		t.Fatalf("clientDecryptASREP: %v", err)
	}
	assertBytesEqual(t, "Nonce1", nonce, payload.Nonce1)

	// The TGT must be bound to the authenticated owner identity.
	tgt, err := tgsDecryptTGT(payload.TGT, f.ktgsKey)
	if err != nil {
		t.Fatalf("tgsDecryptTGT: %v", err)
	}
	if tgt.ClientId != f.ownerID {
		t.Errorf("TGT ClientId: want %q, got %q", f.ownerID, tgt.ClientId)
	}
}

// TestIssueTGT_ForgedOwnerRejected is the core security regression: an attacker
// holding their OWN valid certificate + private key signs a request that claims
// a victim's owner_id. The signature is valid (proves key possession) but the
// owner_id does not match the certificate owner, so the AS must reject it and
// must NOT mint a TGT for the victim.
func TestIssueTGT_ForgedOwnerRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	const victim = "user-victim"
	nonce := randBytes(t, 16)
	ts := f.clock.now.Unix()
	// Attacker signs with their own (valid) key over the forged owner_id.
	sig := signCanonical(t, f.clientPriv, f.certSN, victim, nonce, ts)

	_, _, err := f.svc.IssueTGT(context.Background(), victim, f.certSN, nonce, ts, sig)
	if err == nil {
		t.Fatal("forged owner_id phải bị từ chối, nhưng IssueTGT thành công")
	}
	if code := ErrorCodeOf(err); code != ErrIdentityMismatch {
		t.Fatalf("muốn IDENTITY_MISMATCH, got %v", code)
	}
}

func TestIssueTGT_ReplayRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	nonce := randBytes(t, 16)
	ts := f.clock.now.Unix()
	sig := signCanonical(t, f.clientPriv, f.certSN, f.ownerID, nonce, ts)

	if _, _, err := f.svc.IssueTGT(context.Background(), f.ownerID, f.certSN, nonce, ts, sig); err != nil {
		t.Fatalf("first IssueTGT: %v", err)
	}

	_, _, err := f.svc.IssueTGT(context.Background(), f.ownerID, f.certSN, nonce, ts, sig)
	if err == nil {
		t.Fatal("replay phải bị từ chối, nhưng IssueTGT thành công lần hai")
	}
	if code := ErrorCodeOf(err); code != ErrReplayDetected {
		t.Fatalf("muốn REPLAY_DETECTED, got %v", code)
	}
}
