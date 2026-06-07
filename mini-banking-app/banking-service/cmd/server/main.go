package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mini-banking/banking-service/internal/configs"
	bankgrpc "mini-banking/banking-service/internal/grpc"
)

func main() {
	fmt.Println("[Bank] Starting Bank Service...")

	cfg := config.LoadConfig()
	fmt.Printf("[Bank] Config: grpc_port=%s\n", cfg.GRPCPort)

	handler := bankgrpc.NewHandler()
	server := bankgrpc.NewServer(handler, cfg.GRPCPort)

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
