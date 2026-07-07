package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"kdc-service/internal/config"
	kdcgrpc "kdc-service/internal/grpc"
	"kdc-service/internal/kdc"
	capb "mini_banking/pkg/pb/ca"
)

func main() {
	// 1. Load Configuration
	env := config.LoadEnv()
	redisCfg := config.LoadRedisConfig()

	// 2. Initialize Redis Client
	opts, err := redis.ParseURL(redisCfg.URL)
	if err != nil {
		log.Fatalf("[FATAL] Failed to parse Redis URL: %v", err)
	}

	redisClient := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("[FATAL] Failed to connect to Redis: %v", err)
	}
	log.Println("[INFO] Successfully connected to Redis.")

	// 3. Initialize CA gRPC Client
	conn, err := grpc.NewClient(env.CAAddress, grpc.WithTransportCredentials(loadCATLSCredentials()))
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to CA Service: %v", err)
	}
	defer conn.Close()

	caClient := capb.NewCAServiceClient(conn)
	log.Printf("[INFO] Successfully initialized CA Service client at %s.", env.CAAddress)

	// 3b. Initialize Postgres for the KDC audit log (optional: audit is
	// best-effort infrastructure, so a missing/failed DB must not stop the KDC).
	auditDB := openAuditDB(env.PostgresDSN)
	if auditDB != nil {
		defer auditDB.Close()
	}

	// 4. Initialize Core KDC Service
	svc, err := kdc.NewService(caClient, redisClient, auditDB)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize KDC Service: %v", err)
	}

	// 5. Initialize gRPC Handler and Server
	handler := kdcgrpc.NewHandler(svc)
	server := kdcgrpc.NewServer(handler, env.GRPCPort)

	// 6. Start Server with Graceful Shutdown
	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Fatalf("[FATAL] gRPC server error: %v", err)
	case sig := <-sigCh:
		log.Printf("[INFO] Received signal %v, shutting down...", sig)
		server.Stop()
	}
}

// openAuditDB connects to Postgres for the KDC audit log. It returns nil (not a
// fatal error) when DATABASE_URL is unset or unreachable, so ticket issuance
// keeps working even without a durable audit store.
func openAuditDB(dsn string) *sql.DB {
	if dsn == "" {
		log.Println("[INFO] DATABASE_URL not set, KDC audit log is disabled")
		return nil
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Printf("[WARN] KDC audit DB open failed, audit disabled: %v", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("[WARN] KDC audit DB ping failed, audit disabled: %v", err)
		_ = db.Close()
		return nil
	}
	log.Println("[INFO] Successfully connected to KDC audit database.")
	return db
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
		ServerName: os.Getenv("CA_SERVER_NAME"),
	})
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("[ENV] missing required environment variable: %s", key)
	}
	return value
}
