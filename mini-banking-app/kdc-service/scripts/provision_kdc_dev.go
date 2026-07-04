package main

// Provision khóa Kerberos cho KDC (dev/local).
// Sinh:
//   - K_tgs: khóa AES-256 (32 byte raw) chia sẻ giữa AS và TGS -> certs/k_tgs.key
//   - RSA private key của KDC (PKCS#1 PEM)                      -> certs/kdc-private.pem
//   - RSA public key tương ứng (PKIX PEM, để client mã hóa)    -> certs/kdc-public.pem
//
// Chạy ĐỨNG TẠI thư mục repo (mini-banking-app/mini-banking-app):
//   go run ./kdc-service/scripts/provision_kdc_dev.go
//
// Khóa đã tồn tại thì script BỎ QUA (không ghi đè) để tránh desync với
// BANK_KEY_K_V của banking-service. Đặt FORCE=1 để buộc sinh lại.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

const (
	kTGSPath    = "kdc-service/certs/k_tgs.key"
	privKeyPath = "kdc-service/certs/kdc-private.pem"
	pubKeyPath  = "kdc-service/certs/kdc-public.pem"
)

func main() {
	force := os.Getenv("FORCE") == "1"

	if exists(kTGSPath) && exists(privKeyPath) && !force {
		fmt.Println("[KDC] Khóa đã tồn tại, bỏ qua. Đặt FORCE=1 để sinh lại.")
		printBankKeyHint()
		return
	}

	mustMkdir("kdc-service/certs")

	// 1. K_tgs — 32 byte ngẫu nhiên (AES-256), ghi dạng raw đúng như LoadKeys mong đợi.
	ktgs := randomBytes(32)
	mustWrite(kTGSPath, ktgs, 0600)

	// 2. RSA private key của KDC (2048-bit), xuất PKCS#1 PEM.
	priv := mustRSAKey(2048)
	mustWrite(privKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}), 0600)

	// 3. Public key (PKIX) để gateway/frontend mã hóa pre-auth gửi KDC.
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		panic(err)
	}
	mustWrite(pubKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}), 0644)

	fmt.Println("[KDC] Đã sinh khóa:")
	fmt.Printf("  - %s (K_tgs, 32 byte)\n", kTGSPath)
	fmt.Printf("  - %s (RSA private key)\n", privKeyPath)
	fmt.Printf("  - %s (RSA public key)\n", pubKeyPath)
	printBankKeyHintFor(ktgs)
}

// In gợi ý BANK_KEY_K_V từ K_tgs hiện có trên đĩa.
func printBankKeyHint() {
	ktgs, err := os.ReadFile(kTGSPath)
	if err != nil || len(ktgs) != 32 {
		return
	}
	printBankKeyHintFor(ktgs)
}

func printBankKeyHintFor(ktgs []byte) {
	fmt.Println()
	fmt.Println("[Bank] banking-service dùng K_tgs làm service key (demo).")
	fmt.Println("       Đặt vào banking-service/.env:")
	fmt.Printf("  BANK_KEY_K_V=%s\n", hex.EncodeToString(ktgs))
	fmt.Printf("  (base64 tương đương: %s)\n", base64.StdEncoding.EncodeToString(ktgs))
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
