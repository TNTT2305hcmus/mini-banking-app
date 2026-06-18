package main

// Generates a TGS-REQ body for Postman testing.
//
// Prerequisites:
//  1. Run gen_test_bodies.go to get the AS-REQ body.
//  2. Send AS-REQ in Postman (POST /v1/auth/as-req).
//  3. Save the full Postman response body as asrep_response.json in your CWD.
//  4. Run this script: go run ca-service/scripts/gen_tgs_body.go
//
// Output: tgsreq_body.json (TGS-REQ body for POST /v1/auth/tgs-req)
//
// Optional env vars (override defaults):
//   CLIENT_ID   — client email    (default: jlanguyen1@gmail.com)
//   CERT_SN     — cert serial     (default: from gen_test_bodies.go constant)
//   SERVICE_ID  — target service  (default: bank-service)
//   SCOPE       — requested scope (default: transfer:internal)
//   ASREP_FILE  — input file path (default: asrep_response.json)

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"time"
)

// ── Types mirroring kdc-service internal/kdc/types.go ─────────────────────
// JSON tags must match exactly.

type kdcASResponse struct {
	KDCSignature     []byte `json:"kdc_signature"`
	EncryptedKey     []byte `json:"encrypted_key"`
	EncryptedPayload []byte `json:"encrypted_payload"`
}

type kdcASRepPayload struct {
	SessionKey []byte `json:"k_c_tgs"`
	TGT        []byte `json:"tgt"`
	Nonce1     []byte `json:"nonce_1"`
}

// authenticatorPlaintext matches AuthenticatorPlaintext in kdc-service.
// Fields ts_3 and nonce_req are the primary tags checked by decryptAuthenticator.
type authenticatorPlaintext struct {
	ClientID         string `json:"client_id"`
	Timestamp        int64  `json:"ts_3"`
	NonceReq         string `json:"nonce_req"`
	RequestID        string `json:"request_id"`
	RequestedService string `json:"requested_service"`
	Scope            string `json:"scope"`
}

func main() {
	// ── 0. Configuration ──────────────────────────────────────────────────
	clientID  := envOr("CLIENT_ID", "jlanguyen1@gmail.com")
	certSN    := envOr("CERT_SN", "ddea1adf89997505e337c9602083c27c")
	serviceID := envOr("SERVICE_ID", "bank-service")
	scope     := envOr("SCOPE", "transfer:internal")
	inputFile := envOr("ASREP_FILE", "asrep_response.json")

	// ── 1. Load RSA private key ───────────────────────────────────────────
	privKey := mustLoadPrivKey("test_client.key")

	// ── 2. Read & decode the AS-REP from the Postman response file ────────
	// The file should contain the full API response body saved from Postman.
	// Handles both Node.js Buffer format {"type":"Buffer","data":[...]}
	// and plain base64 string for the encrypted_payload field.
	asRepRaw := mustReadASRepBytes(inputFile)

	// ── 3. Parse ASResponse JSON ──────────────────────────────────────────
	// Go json.Marshal encodes []byte as base64 strings, so the fields are base64.
	var asRep kdcASResponse
	if err := json.Unmarshal(asRepRaw, &asRep); err != nil {
		diefmt("parse KDC ASResponse from %s: %v\n  (make sure the file contains the raw gRPC ASResponse JSON, not the full API wrapper — see the note above)", inputFile, err)
	}
	if len(asRep.EncryptedKey) == 0 {
		diefmt("encrypted_key is missing in the decoded ASResponse — wrong input format?")
	}
	if len(asRep.EncryptedPayload) == 0 {
		diefmt("encrypted_payload is missing in the decoded ASResponse")
	}

	// ── 4. Decrypt ephemeral AES key with RSA-OAEP (label = "AS_REP") ─────
	ephKey, err := rsa.DecryptOAEP(sha256.New(), nil, privKey, asRep.EncryptedKey, []byte("AS_REP"))
	if err != nil {
		diefmt("RSA-OAEP decrypt encrypted_key: %v\n  → Are you using the same test_client.key that was used for the AS-REQ?", err)
	}
	fmt.Fprintf(os.Stderr, "► RSA-OAEP OK — ephemeral AES key: %d bytes\n", len(ephKey))

	// ── 5. Decrypt ASRepPayload with ephemeral AES-256-GCM ────────────────
	payloadJSON, err := aesGCMDecrypt(ephKey, asRep.EncryptedPayload)
	if err != nil {
		diefmt("AES-GCM decrypt encrypted_payload: %v", err)
	}
	var payload kdcASRepPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		diefmt("parse ASRepPayload JSON: %v", err)
	}
	if len(payload.SessionKey) != 32 {
		diefmt("k_c_tgs must be 32 bytes, got %d", len(payload.SessionKey))
	}
	if len(payload.TGT) == 0 {
		diefmt("TGT is empty in AS-REP payload")
	}
	fmt.Fprintf(os.Stderr, "► AS-REP decrypted — k_c_tgs: %x... | TGT: %d bytes\n", payload.SessionKey[:4], len(payload.TGT))

	// ── 6. Build authenticator plaintext ─────────────────────────────────
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		diefmt("generate nonce: %v", err)
	}
	nonceB64 := base64.StdEncoding.EncodeToString(nonceBytes)

	auth := authenticatorPlaintext{
		ClientID:         clientID,
		Timestamp:        time.Now().Unix(),
		NonceReq:         nonceB64,
		RequestID:        newUUID(),
		RequestedService: serviceID,
		Scope:            scope,
	}
	authJSON, _ := json.Marshal(auth)
	fmt.Fprintf(os.Stderr, "► authenticator JSON: %s\n", string(authJSON))

	// ── 7. Encrypt authenticator with K_c_tgs (AES-256-GCM) ──────────────
	encAuth, err := aesGCMEncrypt(payload.SessionKey, authJSON)
	if err != nil {
		diefmt("encrypt authenticator: %v", err)
	}
	fmt.Fprintf(os.Stderr, "► encrypted authenticator: %d bytes\n", len(encAuth))

	// ── 8. Build TGS-REQ body ──────────────────────────────────────────────
	// Field names match api-gateway/src/controller/kdc.controller.ts handleRequestTGS.
	// bytes fields (tgt, authenticator, nonce) are base64-encoded strings because
	// TGSRequest.fromJSON decodes them via bytesFromBase64.
	tgsBody := map[string]any{
		"serviceId":      serviceID,
		"tgtCiphertext":  base64.StdEncoding.EncodeToString(payload.TGT),
		"authenticator":  base64.StdEncoding.EncodeToString(encAuth),
		"certSn":         certSN,
		"nonce":          nonceB64,
		"requestedScope": scope,
	}
	tgsBodyJSON, _ := json.MarshalIndent(tgsBody, "", "  ")

	outFile := "tgsreq_body.json"
	if err := os.WriteFile(outFile, tgsBodyJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: cannot write %s: %v\n", outFile, err)
	} else {
		fmt.Fprintf(os.Stderr, "► TGS-REQ body written to %s\n", outFile)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println(" STEP 5 ─ POST /v1/auth/tgs-req")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("Headers:")
	fmt.Println("  X-Request-ID: req-tgs-001")
	fmt.Println("  Content-Type: application/json")
	fmt.Println()
	fmt.Println("Body (also saved to tgsreq_body.json):")
	fmt.Println(string(tgsBodyJSON))
	fmt.Println()
	fmt.Println("→ TGS_REP success: data.encrypted_payload contains the service ticket")
	fmt.Printf("  encrypted with K_c_tgs (session key), which is AES-GCM with key = %x...\n", payload.SessionKey[:8])
}

// ── Helpers ────────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func diefmt(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}

func mustLoadPrivKey(path string) *rsa.PrivateKey {
	raw, err := os.ReadFile(path)
	if err != nil {
		diefmt("read private key %s: %v\n  → Run gen_test_bodies.go first to generate/load the key.", path, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		diefmt("%s is not a valid PEM file", path)
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		diefmt("parse RSA private key from %s: %v", path, err)
	}
	fmt.Fprintln(os.Stderr, "► Loaded private key from", path)
	return key
}

// mustReadASRepBytes reads the AS-REP response file and returns the raw bytes
// of the KDC ASResponse JSON.
//
// Accepted file formats:
//
//  1. Full API response (Postman "Save Response"):
//     {"success":true,"data":{"encrypted_payload":<Buffer or base64>,...},...}
//
//  2. Raw KDC ASResponse JSON:
//     {"kdc_signature":"...","encrypted_key":"...","encrypted_payload":"..."}
func mustReadASRepBytes(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		diefmt("read %s: %v\n  → In Postman: send AS-REQ → click \"Save Response\" → save as %s in your CWD.", path, err, path)
	}

	// Try full API wrapper first.
	var apiResp struct {
		Data struct {
			EncryptedPayload json.RawMessage `json:"encrypted_payload"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &apiResp) == nil && len(apiResp.Data.EncryptedPayload) > 0 {
		fmt.Fprintln(os.Stderr, "► Detected full API response format")
		return decodeBufferField(apiResp.Data.EncryptedPayload)
	}

	// Otherwise assume the file is already the raw KDC ASResponse JSON.
	fmt.Fprintln(os.Stderr, "► Treating file as raw ASResponse JSON")
	return raw
}

// decodeBufferField handles two formats for a bytes field in a JSON response:
//
//  1. Node.js Buffer (Express default serialization of Buffer):
//     {"type":"Buffer","data":[123,34,...]}
//
//  2. Base64-encoded string (explicit .toString("base64")):
//     "eyJrZGNfc2lnbmF0dXJlIjoiLi4uIn0="
func decodeBufferField(raw json.RawMessage) []byte {
	// Case 1: Node.js Buffer object
	var bufObj struct {
		Type string `json:"type"`
		Data []int  `json:"data"`
	}
	if json.Unmarshal(raw, &bufObj) == nil && bufObj.Type == "Buffer" && len(bufObj.Data) > 0 {
		out := make([]byte, len(bufObj.Data))
		for i, v := range bufObj.Data {
			out[i] = byte(v)
		}
		fmt.Fprintf(os.Stderr, "► Decoded Node.js Buffer: %d bytes\n", len(out))
		return out
	}

	// Case 2: base64 string
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(s)
			if err != nil {
				diefmt("encrypted_payload is a string but not valid base64: %v", err)
			}
		}
		fmt.Fprintf(os.Stderr, "► Decoded base64 string: %d bytes\n", len(decoded))
		return decoded
	}

	preview := string(raw)
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	diefmt("encrypted_payload has unrecognized format:\n  %s", preview)
	return nil
}

// aesGCMDecrypt decrypts a nonce-prefixed AES-256-GCM ciphertext.
// Format: [12-byte nonce][ciphertext+tag]
func aesGCMDecrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short (%d bytes, need ≥%d)", len(ciphertext), gcm.NonceSize())
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

// aesGCMEncrypt encrypts plaintext with AES-256-GCM and prepends the nonce.
// Format: [12-byte nonce][ciphertext+tag]
func aesGCMEncrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...), nil
}

// newUUID returns a random UUID v4 string.
func newUUID() string {
	b := make([]byte, 16)
	io.ReadFull(rand.Reader, b) //nolint:errcheck
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
