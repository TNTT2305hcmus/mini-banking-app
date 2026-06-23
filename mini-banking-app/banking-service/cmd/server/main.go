package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "mini-banking/banking-service/internal/configs"
	bankgrpc "mini-banking/banking-service/internal/grpc"
	capb "mini-banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	fmt.Println("[Bank] Starting Bank Service...")

	cfg := config.LoadConfig()
	fmt.Printf("[Bank] Config: grpc_port=%s\n", cfg.GRPCPort)

	db, err := sql.Open("pgx", cfg.PostgresDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to ping database: %v\n", err)
		os.Exit(1)
	}

	redisOpt, err := redis.ParseURL(cfg.RedisURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to parse REDIS_URI: %v\n", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	caConn, err := grpc.NewClient(cfg.CAServiceAddress, grpc.WithTransportCredentials(loadCATLSCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to create CA gRPC client: %v\n", err)
		os.Exit(1)
	}
	defer caConn.Close()

	bankKVKey, err := loadBankKey(cfg.BankKVKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Invalid BANK_KEY: %v\n", err)
		os.Exit(1)
	}

	handler := bankgrpc.NewHandler(db, redisClient, capb.NewCAServiceClient(caConn), bankKVKey)
	server, err := bankgrpc.NewServer(handler, cfg.GRPCPort, bankgrpc.SecurityConfig{
		ServerCertPath: cfg.TLSServerCertPath,
		ServerKeyPath:  cfg.TLSServerKeyPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to configure gRPC server: %v\n", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "[Bank] Server error: %v\n", err)
		os.Exit(1)
	case sig := <-quit:
		fmt.Printf("[Bank] Received signal %s, shutting down...\n", sig)
		server.Stop()
	}

	fmt.Println("[Bank] Bank Service stopped.")
}

// loadBankKey resolves the 32-byte service key K_v used to decrypt Ticket_v.
//
// BANK_KEY may be either a path to a 32-byte key file (preferred — this is the
// same file the KDC encrypts service tickets with, BANK_SERVICE_KEY_PATH) or
// inline key material as hex / base64 / a raw 32-byte string.
func loadBankKey(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("BANK_KEY is required")
	}

	// Preferred: a key file path holding the raw 32-byte key. Trailing newline
	// handling mirrors the KDC key loader so both sides derive identical bytes.
	if raw, err := os.ReadFile(value); err == nil {
		if n := len(raw); n > 0 && raw[n-1] == '\n' {
			raw = raw[:n-1]
		}
		if n := len(raw); n > 0 && raw[n-1] == '\r' {
			raw = raw[:n-1]
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("key file %q must contain a 32-byte key, got %d bytes", value, len(raw))
		}
		return raw, nil
	}

	// Fallback: inline key material (hex / base64 / raw 32-byte string).
	if b, err := hex.DecodeString(value); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(value); err == nil && len(b) == 32 {
		return b, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("must be a 32-byte key file path or hex/base64/raw 32-byte key")
}

func loadCATLSCredentials() credentials.TransportCredentials {
	caCertPath := requiredEnv("CA_CERT_PATH")
	caPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to read CA server CA certificate: %v", err)
	}
	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(caPEM) {
		log.Fatalf("[FATAL] Failed to parse CA server CA certificate")
	}

	return credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootCAs,
		ServerName: os.Getenv("CA_TLS_SERVER_NAME"),
	})
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("[ENV] missing required environment variable: %s", key)
	}
	return value
}
