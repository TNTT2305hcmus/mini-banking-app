package main

/**
 * @title Provision gRPC TLS Certificates (All Services)
 * @author Tran Nguyen Tri Thanh (tntt)
 *
 * @summary
 *   Generates a single gRPC Transport CA and issues TLS server certificates
 *   for every internal service (ca-service, kdc-service, banking-service).
 *   The transport CA certificate is distributed to api-gateway so it can
 *   verify all server certificates with one CA_CERT_PATH setting.
 *
 * @usage
 *   Run from the project root (mini-banking-app/):
 *     go run ./ca-service/scripts/provision_grpc_certs/
 *
 * @output
 *   ca-service/certs/grpc/
 *     grpc-ca.key          Transport CA private key (keep secret; needed only for re-issuance)
 *     grpc-ca.crt          Transport CA certificate (distributed to all clients)
 *     ca-server.key        CA service gRPC TLS private key
 *     ca-server.crt        CA service gRPC TLS certificate
 *
 *   kdc-service/certs/grpc/
 *     grpc-ca.crt          Copy of transport CA certificate
 *     kdc-server.key
 *     kdc-server.crt
 *
 *   banking-service/certs/grpc/
 *     grpc-ca.crt          Copy of transport CA certificate
 *     bank-server.key
 *     bank-server.crt
 *
 *   api-gateway/certs/
 *     grpc-ca.crt          Copy of transport CA certificate → set as CA_CERT_PATH
 */

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// ── certificate validity windows ──────────────────────────────────────────────

const (
	transportCAValidity  = 5 * 365 * 24 * time.Hour // 5 years
	serverCertValidity   = 2 * 365 * 24 * time.Hour // 2 years
	transportCAKeyBits   = 2048
	serverCertKeyBits    = 2048
)

// ── service descriptor ────────────────────────────────────────────────────────

// serviceSpec describes the TLS server certificate to issue for one service.
type serviceSpec struct {
	// cn is the X.509 Common Name (also used in log output).
	cn string
	// dnsNames are the DNS SANs embedded in the certificate.
	// Include both the Docker/k8s service name and "localhost" for local dev.
	dnsNames []string
	// ips are the IP SANs embedded in the certificate.
	ips []net.IP
	// outDir is the directory (relative to project root) where key and cert are written.
	outDir string
	// keyFile is the file name for the private key (e.g. "ca-server.key").
	keyFile string
	// crtFile is the file name for the certificate (e.g. "ca-server.crt").
	crtFile string
}

// ── entry point ───────────────────────────────────────────────────────────────

func main() {
	services := []serviceSpec{
		{
			cn:       "ca-service",
			dnsNames: []string{"ca-service", "localhost"},
			ips:      []net.IP{net.ParseIP("127.0.0.1")},
			outDir:   "ca-service/certs/grpc",
			keyFile:  "ca-server.key",
			crtFile:  "ca-server.crt",
		},
		{
			cn:       "kdc-service",
			dnsNames: []string{"kdc-service", "localhost"},
			ips:      []net.IP{net.ParseIP("127.0.0.1")},
			outDir:   "kdc-service/certs/grpc",
			keyFile:  "kdc-server.key",
			crtFile:  "kdc-server.crt",
		},
		{
			cn:       "banking-service",
			dnsNames: []string{"banking-service", "bank-service", "localhost"},
			ips:      []net.IP{net.ParseIP("127.0.0.1")},
			outDir:   "banking-service/certs/grpc",
			keyFile:  "bank-server.key",
			crtFile:  "bank-server.crt",
		},
	}

	// ── 1. Transport CA ───────────────────────────────────────────────────────
	//
	// One CA signs every service's gRPC TLS certificate.
	// api-gateway loads the CA cert once (CA_CERT_PATH) and can verify all servers.

	fmt.Println("[CA] Generating gRPC Transport CA …")
	caKey := mustGenerateRSAKey(transportCAKeyBits)
	caDER := mustSelfSignedCA(caKey, "Mini_Banking Dev gRPC CA", transportCAValidity)
	caCert := mustParseCert(caDER)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	// Persist the transport CA under ca-service (it is the CA's responsibility).
	mustMkdir("ca-service/certs/grpc")
	mustWrite("ca-service/certs/grpc/grpc-ca.key", privateKeyPEM(caKey), 0o600)
	mustWrite("ca-service/certs/grpc/grpc-ca.crt", caPEM, 0o644)
	fmt.Println("[CA] Transport CA written → ca-service/certs/grpc/grpc-ca.{key,crt}")

	// ── 2. Server certificates ────────────────────────────────────────────────

	for _, svc := range services {
		fmt.Printf("[CA] Issuing server certificate for %s …\n", svc.cn)

		mustMkdir(svc.outDir)

		svcKey := mustGenerateRSAKey(serverCertKeyBits)
		svcCertDER := mustIssuedCert(
			svcKey,
			caCert,
			caKey,
			svc.cn,
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			svc.dnsNames,
			svc.ips,
		)

		mustWrite(filepath.Join(svc.outDir, svc.keyFile), privateKeyPEM(svcKey), 0o600)
		mustWrite(
			filepath.Join(svc.outDir, svc.crtFile),
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: svcCertDER}),
			0o644,
		)

		// Each service also receives the transport CA cert so its own CA-client
		// connections can validate peer certificates without extra configuration.
		mustWrite(filepath.Join(svc.outDir, "grpc-ca.crt"), caPEM, 0o644)

		fmt.Printf("[CA] → %s/%s  (key: %s)\n", svc.outDir, svc.crtFile, svc.keyFile)
	}

	// ── 3. API Gateway ────────────────────────────────────────────────────────
	//
	// The gateway is an HTTP/REST server, not a gRPC server, so it only needs the
	// CA certificate to act as a TLS client when calling ca-service and kdc-service.
	// Set  CA_CERT_PATH=certs/grpc-ca.crt  in api-gateway/.env

	fmt.Println("[CA] Distributing transport CA cert → api-gateway …")
	mustMkdir("api-gateway/certs")
	mustWrite("api-gateway/certs/grpc-ca.crt", caPEM, 0o644)
	fmt.Println("[CA] → api-gateway/certs/grpc-ca.crt")

	printSummary()
}

// ── certificate helpers ───────────────────────────────────────────────────────

func mustGenerateRSAKey(bits int) *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		panic(fmt.Sprintf("generate RSA-%d key: %v", bits, err))
	}
	return key
}

// mustSelfSignedCA creates a self-signed CA certificate.
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
		panic(fmt.Sprintf("create self-signed CA (%s): %v", cn, err))
	}
	return der
}

// mustIssuedCert creates a leaf certificate signed by the given CA.
func mustIssuedCert(
	key *rsa.PrivateKey,
	issuer *x509.Certificate,
	issuerKey *rsa.PrivateKey,
	cn string,
	eku []x509.ExtKeyUsage,
	dnsNames []string,
	ips []net.IP,
) []byte {
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: mustSerial(),
		Subject: pkix.Name{
			Country:      []string{"VN"},
			Organization: []string{"Mini_App_Banking"},
			CommonName:   cn,
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(serverCertValidity),
		IsCA:                  false,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           eku,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		panic(fmt.Sprintf("issue cert for %s: %v", cn, err))
	}
	return der
}

func mustParseCert(der []byte) *x509.Certificate {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(fmt.Sprintf("parse certificate: %v", err))
	}
	return cert
}

func mustSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(fmt.Sprintf("generate serial number: %v", err))
	}
	if serial.Sign() == 0 {
		return big.NewInt(1)
	}
	return serial
}

func privateKeyPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// ── filesystem helpers ────────────────────────────────────────────────────────

func mustMkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		panic(fmt.Sprintf("mkdir %s: %v", path, err))
	}
}

func mustWrite(path string, data []byte, perm os.FileMode) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(fmt.Sprintf("mkdir for %s: %v", path, err))
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		panic(fmt.Sprintf("write %s: %v", path, err))
	}
}

// ── post-run guidance ─────────────────────────────────────────────────────────

func printSummary() {
	fmt.Println(`
────────────────────────────────────────────────────────────────
  gRPC TLS certificates provisioned.
────────────────────────────────────────────────────────────────

  One Transport CA was created and signed all server certificates.

  ┌─ ca-service ─────────────────────────────────────────────────
  │  GRPC_SERVER_CERT_PATH = certs/grpc/ca-server.crt
  │  GRPC_SERVER_KEY_PATH  = certs/grpc/ca-server.key
  │
  ├─ kdc-service ────────────────────────────────────────────────
  │  KDC_GRPC_CERT_PATH    = certs/grpc/kdc-server.crt
  │  KDC_GRPC_KEY_PATH     = certs/grpc/kdc-server.key
  │  KDC_GRPC_CA_PATH      = certs/grpc/grpc-ca.crt
  │
  ├─ banking-service ────────────────────────────────────────────
  │  BANK_GRPC_CERT_PATH   = certs/grpc/bank-server.crt
  │  BANK_GRPC_KEY_PATH    = certs/grpc/bank-server.key
  │  BANK_GRPC_CA_PATH     = certs/grpc/grpc-ca.crt
  │
  └─ api-gateway ────────────────────────────────────────────────
     CA_CERT_PATH          = certs/grpc-ca.crt

  NOTE: paths above are relative to each service's working directory.
  The transport CA private key (ca-service/certs/grpc/grpc-ca.key)
  is only needed for re-issuance — keep it out of version control.
────────────────────────────────────────────────────────────────`)
}
