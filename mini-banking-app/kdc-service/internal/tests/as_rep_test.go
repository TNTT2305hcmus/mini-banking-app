package kdc_test

// as_rep_test.go
// Unit tests for the full AS_REP flow (hybrid encryption):
//
//   KDC   : BuildAS_REP → RSA-PSS sign payload → AES-256-GCM encrypt payload →
//           RSA-OAEP wrap the AES key with pub_c → marshal ASResponse
//   Client: unmarshal ASResponse → RSA-OAEP unwrap AES key → AES-GCM decrypt
//           payload → RSA-PSS verify the KDC signature
//
// Test cases:
//   TestBuildASREP_InputValidation   — guard clause for each required input
//   TestDecryptASREP_HappyPath       — main flow, field match
//   TestDecryptASREP_WrongPrivKey    — wrong priv_c → OAEP unwrap fail
//   TestDecryptASREP_TamperedPayload — tampered AES ciphertext → GCM integrity fail
//   TestDecryptASREP_TamperedSig     — tampered KDC signature → PSS verify fail
//   TestDecryptASREP_WrongKDCPubKey  — verify with wrong pub_KDC → PSS verify fail

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// clientDecryptASREP — simulates the client handling a hybrid AS_REP.
//
//  1. json.Unmarshal                            → ASResponse envelope
//  2. RSA-OAEP (SHA-256, label "AS_REP")        → AES key
//  3. AES-256-GCM open                          → ASRepPayload bytes
//  4. RSA-PSS + SHA-256 verify the KDC signature over the payload
// ─────────────────────────────────────────────────────────────────

func clientDecryptASREP(
	encBytes []byte,
	clientPriv *rsa.PrivateKey,
	kdcPub *rsa.PublicKey,
) (*ASRepPayload, error) {

	var env ASResponse
	if err := json.Unmarshal(encBytes, &env); err != nil {
		return nil, err
	}

	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, clientPriv, env.EncryptedKey, []byte("AS_REP"))
	if err != nil {
		return nil, err
	}

	payloadBytes, err := aesGCMOpen(aesKey, env.EncryptedPayload)
	if err != nil {
		return nil, err
	}

	var payload ASRepPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, err
	}

	canonical, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	hashed := sha256.Sum256(canonical)
	if err := rsa.VerifyPSS(kdcPub, crypto.SHA256, hashed[:], env.KDCSignature, nil); err != nil {
		return nil, err
	}

	return &payload, nil
}

// aesGCMOpen decrypts nonce-prefixed AES-256-GCM ciphertext (nonce || ct || tag).
func aesGCMOpen(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// ─────────────────────────────────────────────────────────────────
// TestBuildASREP_InputValidation
// ─────────────────────────────────────────────────────────────────

func TestBuildASREP_InputValidation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	validKey := randBytes(t, 32)
	validTGT := randBytes(t, 64)
	validNonce := randBytes(t, 16)
	clientPub := &f.clientPriv.PublicKey

	cases := []struct {
		name   string
		pub    *rsa.PublicKey
		kCtgs  []byte
		tgt    []byte
		nonce1 []byte
	}{
		{"nil pubkey", nil, validKey, validTGT, validNonce},
		{"empty k_ctgs", clientPub, nil, validTGT, validNonce},
		{"empty tgt", clientPub, validKey, nil, validNonce},
		{"empty nonce1", clientPub, validKey, validTGT, nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := f.svc.BuildAS_REP(tc.pub, tc.kCtgs, tc.tgt, tc.nonce1)
			if err == nil {
				t.Errorf("BuildAS_REP(%q): muốn lỗi nhưng không có", tc.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_HappyPath
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	kCtgs := randBytes(t, 32)
	tgt := randBytes(t, 64)
	nonce1 := randBytes(t, 16)

	enc, err := f.svc.BuildAS_REP(&f.clientPriv.PublicKey, kCtgs, tgt, nonce1)
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	payload, err := clientDecryptASREP(enc, f.clientPriv, &f.kdcPriv.PublicKey)
	if err != nil {
		t.Fatalf("clientDecryptASREP: %v", err)
	}

	assertBytesEqual(t, "SessionKey", kCtgs, payload.SessionKey)
	assertBytesEqual(t, "TGT", tgt, payload.TGT)
	assertBytesEqual(t, "Nonce1", nonce1, payload.Nonce1)
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_WrongPrivKey
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_WrongPrivKey(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(&f.clientPriv.PublicKey, randBytes(t, 32), randBytes(t, 64), randBytes(t, 16))
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	wrongPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	wrongKDC, _ := rsa.GenerateKey(rand.Reader, 2048)

	_, err = clientDecryptASREP(enc, wrongPriv, &wrongKDC.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi OAEP khi dùng sai private key, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_TamperedPayload
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_TamperedPayload(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(&f.clientPriv.PublicKey, randBytes(t, 32), randBytes(t, 64), randBytes(t, 16))
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	var env ASResponse
	if err := json.Unmarshal(enc, &env); err != nil {
		t.Fatalf("unmarshal ASResponse: %v", err)
	}
	env.EncryptedPayload[len(env.EncryptedPayload)/2] ^= 0xFF // flip a byte in the AES ciphertext
	tampered, _ := json.Marshal(env)

	_, err = clientDecryptASREP(tampered, f.clientPriv, &f.kdcPriv.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi integrity khi payload bị tamper, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_TamperedSignature
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_TamperedSignature(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(&f.clientPriv.PublicKey, randBytes(t, 32), randBytes(t, 64), randBytes(t, 16))
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	var env ASResponse
	if err := json.Unmarshal(enc, &env); err != nil {
		t.Fatalf("unmarshal ASResponse: %v", err)
	}
	env.KDCSignature[0] ^= 0xFF // tamper the KDC signature
	tampered, _ := json.Marshal(env)

	_, err = clientDecryptASREP(tampered, f.clientPriv, &f.kdcPriv.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi PSS verify khi chữ ký bị tamper, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_WrongKDCPubKey
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_WrongKDCPubKey(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(&f.clientPriv.PublicKey, randBytes(t, 32), randBytes(t, 64), randBytes(t, 16))
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	wrongKDC, _ := rsa.GenerateKey(rand.Reader, 2048)

	_, err = clientDecryptASREP(enc, f.clientPriv, &wrongKDC.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi PSS verify khi dùng sai pub_KDC, nhưng không có lỗi")
	}
}
