// package main

// import (
// 	"crypto"
// 	"crypto/rand"
// 	"crypto/rsa"
// 	"crypto/sha256"
// 	"crypto/x509"
// 	"crypto/x509/pkix"
// 	"encoding/base64"
// 	"encoding/json"
// 	"encoding/pem"
// 	"fmt"
// 	"os"
// 	"time"
// )

// const testEmail = "jlanguyen1@gmail.com"
// const testFullName = "Thuan Truong"
// const certSn = "ddea1adf89997505e337c9602083c27c"

// // canonical payload phải khớp đúng với asCanonicalPayload trong handler.go
// type asCanonicalPayload struct {
// 	CertSn    string `json:"cert_sn"`
// 	IdC       string `json:"id_c"`
// 	Nonce     string `json:"nonce"`
// 	Timestamp int64  `json:"timestamp"`
// }

// func main() {
// 	// ── 1. Load or generate RSA-2048 key pair ────────────────────────
// 	// Reuse existing key so CSR (step 3) and AS-REQ signature (step 4)
// 	// always use the same private key — avoids verify mismatch in KDC.
// 	var privKey *rsa.PrivateKey
// 	if raw, err := os.ReadFile("test_client.key"); err == nil {
// 		if block, _ := pem.Decode(raw); block != nil {
// 			if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
// 				privKey = k
// 				fmt.Fprintln(os.Stderr, "► Loaded existing key from test_client.key")
// 			}
// 		}
// 	}
// 	if privKey == nil {
// 		var err error
// 		privKey, err = rsa.GenerateKey(rand.Reader, 2048)
// 		if err != nil {
// 			panic(err)
// 		}
// 		privPEM := pem.EncodeToMemory(&pem.Block{
// 			Type:  "RSA PRIVATE KEY",
// 			Bytes: x509.MarshalPKCS1PrivateKey(privKey),
// 		})
// 		if err := os.WriteFile("test_client.key", privPEM, 0600); err != nil {
// 			panic(err)
// 		}
// 		fmt.Fprintln(os.Stderr, "► Generated new key → test_client.key")
// 	}

// 	// ── 2. Generate PKCS#10 CSR ──────────────────────────────────────
// 	csrTemplate := &x509.CertificateRequest{
// 		Subject: pkix.Name{
// 			CommonName:   testEmail,
// 			Organization: []string{"Mini Banking"},
// 			Country:      []string{"VN"},
// 		},
// 		EmailAddresses: []string{testEmail},
// 	}
// 	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privKey)
// 	if err != nil {
// 		panic(err)
// 	}
// 	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

// 	// ── 3. AS-REQ body ───────────────────────────────────────────────
// 	// nonce: 16 random bytes, base64
// 	nonce := make([]byte, 16)
// 	rand.Read(nonce)
// 	nonceB64 := base64.StdEncoding.EncodeToString(nonce)

// 	timestamp := time.Now().Unix()

// 	canonical := asCanonicalPayload{
// 		CertSn:    certSn,
// 		IdC:       testEmail,
// 		Nonce:     nonceB64,
// 		Timestamp: timestamp,
// 	}
// 	canonicalBytes, _ := json.Marshal(canonical)

// 	hashed := sha256.Sum256(canonicalBytes)
// 	sig, err := rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, hashed[:], nil)
// 	if err != nil {
// 		panic(err)
// 	}
// 	sigB64 := base64.StdEncoding.EncodeToString(sig)

// 	// self-verify để chắc chắn signature đúng trước khi in
// 	if verErr := rsa.VerifyPSS(&privKey.PublicKey, crypto.SHA256, hashed[:], sig, nil); verErr != nil {
// 		fmt.Fprintf(os.Stderr, "BUG: self-verify thất bại: %v\n", verErr)
// 		os.Exit(1)
// 	}
// 	fmt.Fprintf(os.Stderr, "► canonical : %s\n", string(canonicalBytes))
// 	fmt.Fprintf(os.Stderr, "► sha256    : %x\n", hashed)
// 	fmt.Fprintf(os.Stderr, "► sig prefix: %x\n", sig[:8])
// 	fmt.Fprintf(os.Stderr, "► sig len   : %d\n", len(sig))

// 	// ── 4. Print bodies ──────────────────────────────────────────────

// 	fmt.Println("═══════════════════════════════════════════════════════")
// 	fmt.Println(" Private key: test_client.key  (xoá file để reset key)")
// 	fmt.Println("═══════════════════════════════════════════════════════")

// 	fmt.Println()
// 	fmt.Println("━━━━ STEP 1 ─ POST /v1/otp/request ━━━━")
// 	fmt.Println("Headers:")
// 	fmt.Println(`  X-Request-ID: req-otp-001`)
// 	fmt.Println("Body:")
// 	printJSON(map[string]any{
// 		"email": testEmail,
// 	})

// 	fmt.Println()
// 	fmt.Println("━━━━ STEP 2 ─ POST /v1/otp/verify ━━━━")
// 	fmt.Println("Headers:")
// 	fmt.Println(`  X-Request-ID: req-otp-002`)
// 	fmt.Println("Body:")
// 	printJSON(map[string]any{
// 		"email": testEmail,
// 		"otp":   "123456",
// 	})
// 	fmt.Println("→ Lấy 'reg_token' từ response.data.reg_token")

// 	fmt.Println()
// 	fmt.Println("━━━━ STEP 3 ─ POST /v1/pki/register ━━━━")
// 	fmt.Println("Headers:")
// 	fmt.Println(`  X-Request-ID: req-reg-001`)
// 	fmt.Println(`  Authorization: Bearer <reg_token từ step 2>`)
// 	fmt.Println("Body:")
// 	printJSON(map[string]any{
// 		"idC":      testEmail,
// 		"fullName": testFullName,
// 		"csrPem":   csrPEM,
// 	})
// 	fmt.Println("→ Lấy 'cert_serial' từ response.data.cert_serial")

// 	asReqBody := map[string]any{
// 		"clientId":         testEmail,
// 		"nonce":            nonceB64,
// 		"timestamp":        timestamp,
// 		"certSn":           certSn,
// 		"preAuthSignature": sigB64,
// 	}
// 	asReqBodyBytes, _ := json.MarshalIndent(asReqBody, "", "  ")

// 	// Ghi AS-REQ body ra file nếu đã có CERT_SN thật
// 	if certSn != "REPLACE_WITH_CERT_SERIAL_FROM_REGISTER" {
// 		outFile := "asreq_body.json"
// 		if err := os.WriteFile(outFile, asReqBodyBytes, 0644); err != nil {
// 			fmt.Fprintf(os.Stderr, "WARNING: không ghi được %s: %v\n", outFile, err)
// 		} else {
// 			fmt.Fprintf(os.Stderr, "► AS-REQ body đã ghi vào %s\n", outFile)
// 		}
// 	}

// 	fmt.Println()
// 	fmt.Println("━━━━ STEP 4 ─ POST /v1/auth/as-req ━━━━")
// 	fmt.Printf("(Canonical payload được ký: %s)\n", string(canonicalBytes))
// 	fmt.Println("Headers:")
// 	fmt.Println(`  X-Request-ID: req-as-001`)
// 	if certSn == "REPLACE_WITH_CERT_SERIAL_FROM_REGISTER" {
// 		fmt.Println("Body (chạy lại với CERT_SN=<serial> để có body đúng):")
// 	} else {
// 		fmt.Println("Body (đã lưu vào asreq_body.json — dùng file này để gửi request):")
// 	}
// 	fmt.Println(string(asReqBodyBytes))

// 	if certSn == "REPLACE_WITH_CERT_SERIAL_FROM_REGISTER" {
// 		fmt.Println()
// 		fmt.Println("━━━━ Tái tạo AS-REQ body (sau khi có serial) ━━━━")
// 		fmt.Println("Chạy lại script với CERT_SN=<serial>:")
// 		fmt.Printf("  CERT_SN=<serial> go run ca-service/scripts/gen_test_bodies.go\n")
// 	}
// }

// func printJSON(v any) {
// 	b, _ := json.MarshalIndent(v, "", "  ")
// 	fmt.Println(string(b))
// }
