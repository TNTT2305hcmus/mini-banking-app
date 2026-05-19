package kdc

// as_rep_test.go
// Unit test cho toàn bộ luồng AS_REP:
//
//   KDC  : BuildAS_REP → ký RSA-PSS trên payload → mã hoá RSA-OAEP bằng pub_c
//   Client: giải mã OAEP → unmarshal SignedData → verify chữ ký PSS của KDC
//
// Test cases:
//   TestBuildASREP_InputValidation   — guard clause mỗi trường bắt buộc
//   TestDecryptASREP_HappyPath       — luồng chính, kiểm tra field match
//   TestDecryptASREP_WrongPrivKey    — sai priv_c → OAEP fail
//   TestDecryptASREP_TamperedCipher  — bit-flip ciphertext → OAEP integrity fail
//   TestDecryptASREP_TamperedSig     — chữ ký KDC bị giả mạo → PSS verify fail
//   TestDecryptASREP_WrongKDCPubKey  — verify bằng sai pub_KDC → PSS verify fail

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// clientDecryptASREP — mô phỏng thao tác Client sau khi nhận AS_REP
//
//  1. RSA-OAEP + SHA-256 + label "AS_REP"  → plaintext
//  2. json.Unmarshal                        → SignedData
//  3. RSA-PSS + SHA-256 verify chữ ký KDC
//  4. Trả về *ASRepPayload đã xác thực
// ─────────────────────────────────────────────────────────────────

func clientDecryptASREP(
	encBytes []byte,
	clientPriv *rsa.PrivateKey,
	kdcPub *rsa.PublicKey,
) (*ASRepPayload, error) {

	// Bước 1 — Giải mã RSA-OAEP
	plain, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		clientPriv,
		encBytes,
		[]byte("AS_REP"),
	)
	if err != nil {
		return nil, err
	}

	// Bước 2 — Unmarshal SignedData
	var signed SignedData
	if err := json.Unmarshal(plain, &signed); err != nil {
		return nil, err
	}

	// Bước 3 — Verify chữ ký RSA-PSS của KDC
	payloadBytes, err := json.Marshal(signed.Payload)
	if err != nil {
		return nil, err
	}
	hashed := sha256.Sum256(payloadBytes)
	if err := rsa.VerifyPSS(
		kdcPub,
		crypto.SHA256,
		hashed[:],
		signed.Signature,
		nil,
	); err != nil {
		return nil, err
	}

	return &signed.Payload, nil
}

// ─────────────────────────────────────────────────────────────────
// TestBuildASREP_InputValidation
// ─────────────────────────────────────────────────────────────────

func TestBuildASREP_InputValidation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	validKey := randBytes(t, 32)
	validTGT := randBytes(t, 64)
	validNonce := randBytes(t, 16)
	const validSN = "cert-sn-001"

	cases := []struct {
		name   string
		kCtgs  []byte
		tgt    []byte
		nonce1 []byte
		certSn string
	}{
		{"empty k_ctgs", nil, validTGT, validNonce, validSN},
		{"empty tgt", validKey, nil, validNonce, validSN},
		{"empty nonce1", validKey, validTGT, nil, validSN},
		{"empty certSn", validKey, validTGT, validNonce, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := f.svc.BuildAS_REP(ctx, tc.kCtgs, tc.tgt, tc.nonce1, tc.certSn)
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

	enc, err := f.svc.BuildAS_REP(context.Background(), kCtgs, tgt, nonce1, "sn-001")
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

	enc, err := f.svc.BuildAS_REP(
		context.Background(),
		randBytes(t, 32), randBytes(t, 64), randBytes(t, 16), "sn-001",
	)
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	wrongPriv, _ := rsa.GenerateKey(rand.Reader, 4096)
	wrongKDC, _ := rsa.GenerateKey(rand.Reader, 1024)

	_, err = clientDecryptASREP(enc, wrongPriv, &wrongKDC.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi OAEP khi dùng sai private key, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_TamperedCiphertext
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_TamperedCiphertext(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(
		context.Background(),
		randBytes(t, 32), randBytes(t, 64), randBytes(t, 16), "sn-001",
	)
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	enc[len(enc)/2] ^= 0xFF // flip bit giữa blob

	_, err = clientDecryptASREP(enc, f.clientPriv, &f.kdcPriv.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi integrity khi ciphertext bị tamper, nhưng không có lỗi")
	}
}

// ─────────────────────────────────────────────────────────────────
// TestDecryptASREP_TamperedSignature
// ─────────────────────────────────────────────────────────────────

func TestDecryptASREP_TamperedSignature(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	enc, err := f.svc.BuildAS_REP(
		context.Background(),
		randBytes(t, 32), randBytes(t, 64), randBytes(t, 16), "sn-001",
	)
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	// Giải mã OAEP để lấy SignedData gốc, sau đó flip bit chữ ký
	plain, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, f.clientPriv, enc, []byte("AS_REP"))
	if err != nil {
		t.Fatalf("decrypt OAEP để tamper sig: %v", err)
	}

	var signed SignedData
	if err := json.Unmarshal(plain, &signed); err != nil {
		t.Fatalf("unmarshal SignedData: %v", err)
	}
	signed.Signature[0] ^= 0xFF // tamper chữ ký

	tamperedBytes, _ := json.Marshal(signed)
	reEnc, err := rsa.EncryptOAEP(
		sha256.New(), rand.Reader,
		&f.clientPriv.PublicKey,
		tamperedBytes,
		[]byte("AS_REP"),
	)
	if err != nil {
		t.Fatalf("re-encrypt OAEP: %v", err)
	}

	_, err = clientDecryptASREP(reEnc, f.clientPriv, &f.kdcPriv.PublicKey)
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

	enc, err := f.svc.BuildAS_REP(
		context.Background(),
		randBytes(t, 32), randBytes(t, 64), randBytes(t, 16), "sn-001",
	)
	if err != nil {
		t.Fatalf("BuildAS_REP: %v", err)
	}

	wrongKDC, _ := rsa.GenerateKey(rand.Reader, 1024)

	_, err = clientDecryptASREP(enc, f.clientPriv, &wrongKDC.PublicKey)
	if err == nil {
		t.Fatal("muốn lỗi PSS verify khi dùng sai pub_KDC, nhưng không có lỗi")
	}
}
