package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mini-banking/ca-service/internal/ca"
	"mini-banking/ca-service/internal/config"
	cagrpc "mini-banking/ca-service/internal/grpc"
)

// main là entry point của CA Service.
// Thứ tự khởi động:
//  1. Load config từ environment
//  2. Load hoặc tạo mới Root CA
//  3. Khởi tạo in-memory cert store
//  4. Khởi tạo CA Service (business logic)
//  5. Khởi tạo gRPC handler + server
//  6. Start server, chờ signal để graceful shutdown
func main() {
	fmt.Println("[CA] Starting CA Service...")

	// ── 1. Config ─────────────────────────────────────────────
	cfg := config.Load()
	fmt.Printf("[CA] Config: port=%s certValidity=%d days\n",
		cfg.GRPCPort, cfg.CertValidityDays)

	// ── 2. Root CA ────────────────────────────────────────────
	rootCA, err := ca.LoadOrCreate(cfg.RootCAKeyPath, cfg.RootCACertPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[CA] FATAL: cannot initialize Root CA: %v\n", err)
		os.Exit(1)
	}

	// ── 3. Cert Store ─────────────────────────────────────────
	// Lưu ý: store KHÔNG load lại cert từ disk sau restart.
	// Xem ca.NewStore() để hiểu lý do — liên quan đến an toàn revocation
	store := ca.NewStore()

	// TODO: Load existing certs từ disk vào store khi restart
	// Hiện tại store rỗng sau restart — acceptable cho scope đồ án
	// Production: load từ Postgres bảng user_certificates

	// ── 4. CA Service ─────────────────────────────────────────
	svc := ca.NewService(rootCA, store, cfg.IssuedCertsPath, cfg.CertValidityDays)

	// ── 5. gRPC ───────────────────────────────────────────────
	handler := cagrpc.NewHandler(svc)
	server := cagrpc.NewServer(handler, cfg.GRPCPort)

	// ── 6. Start + Graceful Shutdown ──────────────────────────
	// Chạy server trong goroutine riêng
	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			errCh <- err
		}
	}()

	// Chờ SIGINT (Ctrl+C) hoặc SIGTERM (Docker stop)
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
