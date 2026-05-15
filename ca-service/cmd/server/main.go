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
	// Note: The store DOES NOT reload certs from disk after a restart.
	// See ca.NewStore() to understand why — related to revocation safety.
	store := ca.NewStore()

	// TODO: Load existing certs from disk into the store on restart.
	// Currently, the store is empty after a restart — acceptable for the project scope.
	// Production (Implement in the future): load from Postgres user_certificates table.

	// =======================================================
	// ======================= CA Service ====================
	// =======================================================
	svc := ca.NewService(rootCA, store, cfg.IssuedCertsPath, cfg.CertValidityDays)

	// =======================================================
	// ========================== GRPC =======================
	// =======================================================
	handler := cagrpc.NewHandler(svc)
	server := cagrpc.NewServer(handler, cfg.GRPCPort)

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
