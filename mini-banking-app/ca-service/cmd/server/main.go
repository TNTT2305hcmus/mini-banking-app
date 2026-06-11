package main

/**
 * @title CA Service - Main Entry Point
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Bootstrapping configurations, initializing Root CA, store, core service, and starting the gRPC server with graceful shutdown.
 */

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mini-banking/ca-service/internal/ca"
	"mini-banking/ca-service/internal/config"
	cagrpc "mini-banking/ca-service/internal/grpc"

	_ "github.com/jackc/pgx/v5/stdlib"
)

/**
 * @description main is the entry point of the CA Service.
 *
 * @function main
 * @returns {void}
 */
func main() {
	fmt.Println("[CA] Starting CA Service...")

	// =======================================================
	// ======================= CONFIG ========================
	// =======================================================
	cfg := config.Load()
	fmt.Printf("[CA] Config: port=%s certValidity=%d days\n",
		cfg.GRPCPort, cfg.CertValidityDays)

	// =======================================================
	// ======================= Root CA =======================
	// =======================================================
	rootCA, err := ca.LoadKeyAndCert(cfg.RootCAKeyPath, cfg.RootCACertPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize Root CA: %v\n", err)
		os.Exit(1)
	}

	// =======================================================
	// ================== CERTIFICATE STORE ==================
	// =======================================================
	repository, cleanup, err := newRepository(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize certificate store: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// =======================================================
	// ======================= CA Service ====================
	// =======================================================
	svc := ca.NewServiceWithExtensionConfig(rootCA, repository, cfg.IssuedCertsPath, cfg.CertValidityDays, ca.CertificateExtensionConfig{
		CRLDistributionPoints: cfg.CRLDistributionPoints,
		OCSPServers:           cfg.OCSPServers,
	})

	// =======================================================
	// ========================== GRPC =======================
	// =======================================================
	handler := cagrpc.NewHandler(svc)
	server, err := cagrpc.NewServer(handler, cfg.GRPCPort, cagrpc.SecurityConfig{
		ServerCertPath: cfg.GRPCServerCertPath,
		ServerKeyPath:  cfg.GRPCServerKeyPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize secure gRPC server: %v\n", err)
		os.Exit(1)
	}

	// =======================================================
	// ================== Start + Graceful Shutdown ==========
	// =======================================================
	// Run the server in a separate goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errCh <- err
		}
	}()

	// Wait for SIGINT (Ctrl+C) or SIGTERM (Docker stop)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "[CA] Server error: %v\n", err)
		os.Exit(1)
	case sig := <-quit:
		fmt.Printf("[CA] Received signal %s, shutting down...\n", sig)
		server.Stop()
	}

	fmt.Println("[CA] CA Service stopped.")
}

func newRepository(cfg *config.Config) (ca.Repository, func(), error) {
	switch strings.ToLower(cfg.StoreBackend) {
	case "", "json":
		store, err := ca.NewPersistentStore(cfg.StoreStatePath)
		return store, func() {}, err
	case "postgres":
		if cfg.DatabaseURL == "" {
			return nil, func() {}, fmt.Errorf("CA_DATABASE_URL is required when CA_STORE_BACKEND=postgres")
		}
		db, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			return nil, func() {}, fmt.Errorf("open CA database: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, func() {}, fmt.Errorf("ping CA database: %w", err)
		}
		return ca.NewPostgresStore(db), func() { _ = db.Close() }, nil
	default:
		return nil, func() {}, fmt.Errorf("unsupported CA_STORE_BACKEND=%q; use json or postgres", cfg.StoreBackend)
	}
}
