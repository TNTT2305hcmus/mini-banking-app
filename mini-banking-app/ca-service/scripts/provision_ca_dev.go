package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")

	password := os.Getenv("ROOT_CA_KEY_PASSWORD")
	if password == "" {
		panic("ROOT_CA_KEY_PASSWORD is required")
	}

	certsDir := os.Getenv("CA_CERTS_DIR")
	if certsDir == "" {
		certsDir = "certs"
	}

	rootCADir := filepath.Join(certsDir, "root-ca")
	grpcDir := filepath.Join(certsDir, "grpc")
	issuedDir := filepath.Join(certsDir, "issued")
	storeDir := filepath.Join(certsDir, "ca-store")

	mustMkdir(rootCADir)
	mustMkdir(grpcDir)
	mustMkdir(issuedDir)
	mustMkdir(storeDir)

	rootKey := mustRSAKey(4096)
	rootCertDER := mustSelfSignedCA(rootKey, "Mini_App_Banking Root CA", 10*365*24*time.Hour)
	rootCert := mustParseCert(rootCertDER)
	rootCAPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCertDER})
	mustWrite(filepath.Join(rootCADir, "ca.key"), encryptedPrivateKeyPEM(rootKey, password), 0600)
	mustWrite(filepath.Join(rootCADir, "ca.crt"), rootCAPEM, 0644)

	// CA Service presents a gRPC server cert signed by its own Root CA.
	// Other services trust this public Root CA through ca-server-ca.crt.
	mustWrite(filepath.Join(grpcDir, "ca-server-ca.crt"), rootCAPEM, 0644)

	serverKey := mustRSAKey(2048)
	serverDER := mustSignedCert(
		serverKey,
		rootCert,
		rootKey,
		"ca-service",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		[]string{"ca-service", "localhost"},
		[]net.IP{net.ParseIP("127.0.0.1")},
	)
	mustWrite(filepath.Join(grpcDir, "ca-server.key"), privateKeyPEM(serverKey), 0600)
	mustWrite(filepath.Join(grpcDir, "ca-server.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), 0644)

	fmt.Printf("Provisioned local CA dev certificates and one-way gRPC TLS server cert under %s\n", certsDir)
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

func mustSelfSignedCA(key *rsa.PrivateKey, cn string, validity time.Duration) []byte {
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: mustSerial(),
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   cn,
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(validity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return der
}

func mustSignedCert(key *rsa.PrivateKey, issuer *x509.Certificate, issuerKey *rsa.PrivateKey, cn string, eku []x509.ExtKeyUsage, dns []string, ips []net.IP) []byte {
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: mustSerial(),
		Subject: pkix.Name{
			Organization: []string{"Mini_App_Banking"},
			CommonName:   cn,
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.AddDate(2, 0, 0),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           eku,
		DNSNames:              dns,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		panic(err)
	}
	return der
}

func mustSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	if serial.Sign() == 0 {
		return big.NewInt(1)
	}
	return serial
}

func mustParseCert(der []byte) *x509.Certificate {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return cert
}

func privateKeyPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func encryptedPrivateKeyPEM(key *rsa.PrivateKey, password string) []byte {
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(err)
	}
	salt := randomBytes(16)
	nonce := randomBytes(12)
	derived := pbkdf2SHA256([]byte(password), salt, 100000, 32)
	block, err := aes.NewCipher(derived)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type: "ENCRYPTED PRIVATE KEY",
		Headers: map[string]string{
			"Cipher":     "AES-256-GCM",
			"KDF":        "PBKDF2-HMAC-SHA256",
			"Iterations": "100000",
			"Salt":       base64.StdEncoding.EncodeToString(salt),
			"Nonce":      base64.StdEncoding.EncodeToString(nonce),
		},
		Bytes: gcm.Seal(nil, nonce, keyDER, nil),
	})
}

func randomBytes(size int) []byte {
	out := make([]byte, size)
	if _, err := rand.Read(out); err != nil {
		panic(err)
	}
	return out
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2Key(sha256.New, password, salt, iterations, keyLen)
}

func pbkdf2Key(h func() hash.Hash, password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var derived []byte
	block := make([]byte, len(salt)+4)
	copy(block, salt)

	for i := 1; i <= numBlocks; i++ {
		block[len(salt)] = byte(i >> 24)
		block[len(salt)+1] = byte(i >> 16)
		block[len(salt)+2] = byte(i >> 8)
		block[len(salt)+3] = byte(i)
		prf.Reset()
		prf.Write(block)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)

		for j := 1; j < iterations; j++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLen]
}

func mustWrite(path string, data []byte, perm os.FileMode) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		panic(err)
	}
}
