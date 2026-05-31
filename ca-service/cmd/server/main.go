package main

/**
 * @title CA Service - Main Entry Point
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Bootstrapping configurations, initializing Root CA, store, core service, and starting the gRPC server with graceful shutdown.
 */

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mini-banking/ca-service/internal/ca"
	"mini-banking/ca-service/internal/config"
	cagrpc "mini-banking/ca-service/internal/grpc"
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
	rootCA, err := ca.LoadOrCreate(cfg.RootCAKeyPath, cfg.RootCACertPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize Root CA: %v\n", err)
		os.Exit(1)
	}

	// =======================================================
	// ================== CERTIFICATE STORE ==================
	// =======================================================
	store, err := ca.NewPersistentStore(cfg.StoreStatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize certificate store: %v\n", err)
		os.Exit(1)
	}

	// =======================================================
	// ======================= CA Service ====================
	// =======================================================
	svc := ca.NewServiceWithExtensionConfig(rootCA, store, cfg.IssuedCertsPath, cfg.CertValidityDays, ca.CertificateExtensionConfig{
		CRLDistributionPoints: cfg.CRLDistributionPoints,
		OCSPServers:           cfg.OCSPServers,
	})

	// =======================================================
	// ========================== GRPC =======================
	// =======================================================
	handler := cagrpc.NewHandler(svc)
	server, err := cagrpc.NewServer(handler, cfg.GRPCPort, cagrpc.SecurityConfig{
		ServerCertPath:         cfg.GRPCServerCertPath,
		ServerKeyPath:          cfg.GRPCServerKeyPath,
		ClientCACertPath:       cfg.GRPCClientCACertPath,
		RevokeAllowedClientCNs: cfg.RevokeAllowedClientCNs,
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
