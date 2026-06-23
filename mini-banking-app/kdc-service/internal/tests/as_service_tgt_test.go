package kdc_test

// tgt_test.go
// Unit test cho GenerateEncryptedTGT (AES-256-GCM):
//
//   TestGenerateEncryptedTGT_InputValidation — guard clause clientId / k_ctgs rỗng
//   TestTGT_HappyPath                        — mã hoá → giải mã → field match
//   TestTGT_NonDeterministic                 — IV ngẫu nhiên → hai output khác nhau
//   TestTGT_TamperedCiphertext               — bit-flip → GCM auth tag fail
//   TestTGT_WrongKTGSKey                     — sai K_tgs → GCM fail

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// tgsDecryptTGT — mô phỏng TGS giải mã TGT bằng K_tgs (AES-256-GCM)
// ─────────────────────────────────────────────────────────────────

func tgsDecryptTGT(encTGT []byte, ktgsKey []byte) (*TGT, error) {
	block, err := aes.NewCipher(ktgsKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := encTGT[:nonceSize], encTGT[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	var t TGT
	if err := json.Unmarshal(plain, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ─────────────────────────────────────────────────────────────────
// TestGenerateEncryptedTGT_InputValidation
// ─────────────────────────────────────────────────────────────────

func TestGenerateEncryptedTGT_InputValidation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	validKey := randBytes(t, 32)

	cases := []struct {
		name     string
		clientId string
		kCtgs    []byte
	}{
		{"empty clientId", "", validKey},
		{"empty k_ctgs", "user-001", nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := f.svc.GenerateEncryptedTGT(tc.clientId, tc.kCtgs, f.certSN)
			if err == nil {
				t.Errorf("GenerateEncryptedTGT(%q): muốn lỗi nhưng không có", tc.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────
// TestTGT_HappyPath
// ─────────────────────────────────────────────────────────────────

func TestTGT_HappyPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	clientId := "user-alice"
	kCtgs := randBytes(t, 32)

	enc, err := f.svc.GenerateEncryptedTGT(clientId, kCtgs, f.certSN)
	if err != nil {
		t.Fatalf("GenerateEncryptedTGT: %v", err)
	}

	tgt, err := tgsDecryptTGT(enc, f.ktgsKey)
	if err != nil {
		t.Fatalf("tgsDecryptTGT: %v", err)
	}

	if tgt.ClientId != clientId {
		t.Errorf("ClientId: want %q, got %q", clientId, tgt.ClientId)
	}
	assertBytesEqual(t, "SessionKey", kCtgs, tgt.SessionKey)
	if tgt.ExpiresAt <= 0 {
		t.Errorf("ExpiresAt phải > 0, got %d", tgt.ExpiresAt)
	}
}

// ─────────────────────────────────────────────────────────────────
// TestTGT_NonDeterministic
// ─────────────────────────────────────────────────────────────────

func TestTGT_NonDeterministic(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	kCtgs := randBytes(t, 32)

	enc1, err := f.svc.GenerateEncryptedTGT("user-alice", kCtgs, f.certSN)
	if err != nil {
		t.Fatalf("lần 1: %v", err)
	}
	enc2, err := f.svc.GenerateEncryptedTGT("user-alice", kCtgs, f.certSN)
	if err != nil {
		t.Fatalf("lần 2: %v", err)
	}

	if bytes.Equal(enc1, enc2) {
		t.Error("hai lần mã hoá cùng input cho ra cùng output — IV có vẻ không ngẫu nhiên")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestTGT_TamperedCiphertext
// ─────────────────────────────────────────────────────────────────

func TestTGT_TamperedCiphertext(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.GenerateEncryptedTGT("user-alice", randBytes(t, 32), f.certSN)
	if err != nil {
		t.Fatalf("GenerateEncryptedTGT: %v", err)
	}

	enc[len(enc)/2] ^= 0xFF

	_, err = tgsDecryptTGT(enc, f.ktgsKey)
	if err == nil {
		t.Fatal("muốn lỗi GCM auth khi ciphertext bị tamper, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestTGT_WrongKTGSKey
// ─────────────────────────────────────────────────────────────────

func TestTGT_WrongKTGSKey(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.GenerateEncryptedTGT("user-alice", randBytes(t, 32), f.certSN)
	if err != nil {
		t.Fatalf("GenerateEncryptedTGT: %v", err)
	}

	wrongKey := randBytes(t, 32)
	_, err = tgsDecryptTGT(enc, wrongKey)
	if err == nil {
		t.Fatal("muốn lỗi GCM khi dùng sai K_tgs, nhưng không có lỗi")
	}
}
