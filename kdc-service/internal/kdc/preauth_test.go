package kdc

// preauth_test.go
// Unit test cho:
//   VerifyPreAuthSignature — RSA-PKCS1v15 + SHA-256 verify chữ ký Client
//   GenerateSessionKey     — kiểm tra độ dài và entropy

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// TestVerifyPreAuthSignature_Valid
// ─────────────────────────────────────────────────────────────────

func TestVerifyPreAuthSignature_Valid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	data := []byte("user-alice|tgs-service|deadbeef|1716900000")
	hashed := sha256.Sum256(data)

	sig, err := rsa.SignPKCS1v15(rand.Reader, f.clientPriv, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}

	err = f.svc.VerifyPreAuthSignature(context.Background(), "sn-001", sig, data)
	if err != nil {
		t.Errorf("VerifyPreAuthSignature hợp lệ: muốn nil, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// TestVerifyPreAuthSignature_WrongSignature
// Chữ ký là random bytes hoàn toàn → phải fail.
// ─────────────────────────────────────────────────────────────────

func TestVerifyPreAuthSignature_WrongSignature(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	data := []byte("user-alice|tgs-service|deadbeef|1716900000")
	badSig := randBytes(t, 256)

	err := f.svc.VerifyPreAuthSignature(context.Background(), "sn-001", badSig, data)
	if err == nil {
		t.Fatal("muốn lỗi verify với chữ ký random, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestVerifyPreAuthSignature_TamperedData
// Ký đúng data gốc, nhưng server nhận data khác → phải fail.
// ─────────────────────────────────────────────────────────────────

func TestVerifyPreAuthSignature_TamperedData(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	original := []byte("user-alice|tgs-service|deadbeef|1716900000")
	hashed := sha256.Sum256(original)
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.clientPriv, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}

	// Attacker gửi data khác với cùng chữ ký
	tampered := []byte("user-mallory|tgs-service|deadbeef|1716900000")
	err = f.svc.VerifyPreAuthSignature(context.Background(), "sn-001", sig, tampered)
	if err == nil {
		t.Fatal("muốn lỗi verify khi data bị tamper, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestGenerateSessionKey_Length
// Session key phải đúng 32 byte (AES-256).
// ─────────────────────────────────────────────────────────────────

func TestGenerateSessionKey_Length(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	key, err := f.svc.GenerateSessionKey()
	if err != nil {
		t.Fatalf("GenerateSessionKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("độ dài session key: want 32, got %d", len(key))
	}
}

// ─────────────────────────────────────────────────────────────────
// TestGenerateSessionKey_Entropy
// Hai lần gọi liên tiếp phải cho kết quả khác nhau.
// ─────────────────────────────────────────────────────────────────

func TestGenerateSessionKey_Entropy(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	k1, err := f.svc.GenerateSessionKey()
	if err != nil {
		t.Fatalf("lần 1: %v", err)
	}
	k2, err := f.svc.GenerateSessionKey()
	if err != nil {
		t.Fatalf("lần 2: %v", err)
	}

	if bytes.Equal(k1, k2) {
		t.Error("hai session key liên tiếp giống nhau — RNG không hoạt động đúng")
	}
}
