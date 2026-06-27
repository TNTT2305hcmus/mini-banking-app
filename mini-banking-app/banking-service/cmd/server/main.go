package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "mini-banking/banking-service/internal/configs"
	bankgrpc "mini-banking/banking-service/internal/grpc"
	capb "mini-banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
		fmt.Fprintf(os.Stderr, "[Bank] Failed to parse BANK_REDIS_URL: %v\n", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	caConn, err := grpc.NewClient(cfg.CAServiceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Failed to create CA gRPC client: %v\n", err)
		os.Exit(1)
	}
	defer caConn.Close()

	bankKVKey, err := parseBankKey(cfg.BankKVKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Bank] Invalid BANK_KEY_K_V: %v\n", err)
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

func parseBankKey(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("BANK_KEY_K_V is required")
	}
	if b, err := hex.DecodeString(value); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(value); err == nil && len(b) == 32 {
		return b, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("must decode to exactly 32 bytes")
}
