package main

// Provision Kerberos keys for local development.
//
// Generated files:
//   - K_tgs: AS/TGS key                                      -> certs/k_tgs.key
//   - K_v:   identical copies for KDC and Banking Service    -> both services' certs/k_v.key
//   - KDC RSA private/public key pair                        -> certs/kdc-*.pem
//
// Run from the runtime root (mini-banking-app/mini-banking-app):
//
//   go run ./kdc-service/scripts/provision_kdc_dev.go
//
// Existing files are preserved. Set FORCE=1 to regenerate every key together.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

const (
	kTGSPath    = "kdc-service/certs/k_tgs.key"
	kdcKVPath   = "kdc-service/certs/k_v.key"
	bankKVPath  = "banking-service/certs/k_v.key"
	privKeyPath = "kdc-service/certs/kdc-private.pem"
	pubKeyPath  = "kdc-service/certs/kdc-public.pem"
)

func main() {
	force := os.Getenv("FORCE") == "1"
	mustMkdir("kdc-service/certs")

	if force {
		writeSymmetricKey(kTGSPath, "K_tgs")
		writeServiceKeyCopies(randomBytes(32))
		writeRSAKeyPair()
	} else {
		ensureSymmetricKey(kTGSPath, "K_tgs")
		ensureServiceKeyCopies()
		ensureRSAKeyPair()
	}

	printBankKeyHint()
}

func ensureServiceKeyCopies() {
	kdcKey, kdcExists := readOptionalKey(kdcKVPath)
	bankKey, bankExists := readOptionalKey(bankKVPath)

	switch {
	case kdcExists && bankExists:
		if !bytes.Equal(kdcKey, bankKey) {
			panic("K_v mismatch between KDC and Banking Service; restore matching copies or run with FORCE=1")
		}
		mustChmod(kdcKVPath, 0644)
		mustChmod(bankKVPath, 0644)
		fmt.Println("[KDC/Bank] Matching K_v copies already exist, preserving both.")
	case kdcExists:
		mustChmod(kdcKVPath, 0644)
		mustWrite(bankKVPath, kdcKey, 0644)
		fmt.Printf("[Bank] Created %s from the existing KDC K_v.\n", bankKVPath)
	case bankExists:
		mustChmod(bankKVPath, 0644)
		mustWrite(kdcKVPath, bankKey, 0644)
		fmt.Printf("[KDC] Created %s from the existing Bank K_v.\n", kdcKVPath)
	default:
		writeServiceKeyCopies(randomBytes(32))
	}
}

func writeServiceKeyCopies(key []byte) {
	// Local/demo containers run as different non-root users. The host copies
	// therefore need read permission; Compose still mounts both read-only.
	mustWrite(kdcKVPath, key, 0644)
	mustWrite(bankKVPath, key, 0644)
	fmt.Printf("[KDC] Generated %s (K_v, 32 bytes).\n", kdcKVPath)
	fmt.Printf("[Bank] Generated matching %s (K_v, 32 bytes).\n", bankKVPath)
}

func readOptionalKey(path string) ([]byte, bool) {
	key, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		panic(err)
	}
	if len(key) != 32 {
		panic(fmt.Sprintf("%s must contain exactly 32 raw bytes", path))
	}
	return key, true
}

func ensureSymmetricKey(path, name string) {
	if exists(path) {
		fmt.Printf("[KDC] %s already exists, preserving %s.\n", path, name)
		return
	}
	writeSymmetricKey(path, name)
}

func writeSymmetricKey(path, name string) {
	mustWrite(path, randomBytes(32), 0600)
	fmt.Printf("[KDC] Generated %s (%s, 32 bytes).\n", path, name)
}

func ensureRSAKeyPair() {
	if exists(privKeyPath) && exists(pubKeyPath) {
		fmt.Println("[KDC] RSA key pair already exists, preserving it.")
		return
	}
	if exists(privKeyPath) || exists(pubKeyPath) {
		panic("incomplete KDC RSA key pair; restore the missing file or run with FORCE=1")
	}
	writeRSAKeyPair()
}

func writeRSAKeyPair() {
	priv := mustRSAKey(2048)
	mustWrite(privKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}), 0600)

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		panic(err)
	}
	mustWrite(pubKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}), 0644)
	fmt.Println("[KDC] Generated KDC RSA private/public key pair.")
}

func printBankKeyHint() {
	kv, err := os.ReadFile(kdcKVPath)
	if err != nil || len(kv) != 32 {
		panic("K_v must be a raw 32-byte key")
	}

	fmt.Println()
	fmt.Println("[Bank] K_v is separate from K_tgs.")
	fmt.Println("       KDC and Bank keep matching K_v copies in their own certs directories.")
	fmt.Println("       For a direct Bank run, use BANK_KEY=certs/k_v.key.")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mustMkdir(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}
}

func mustRSAKey(bits int) *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		panic(err)
	}
	return key
}

func randomBytes(size int) []byte {
	out := make([]byte, size)
	if _, err := rand.Read(out); err != nil {
		panic(err)
	}
	return out
}

func mustWrite(path string, data []byte, perm os.FileMode) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		panic(err)
	}
}

func mustChmod(path string, perm os.FileMode) {
	if err := os.Chmod(path, perm); err != nil {
		panic(err)
	}
}
