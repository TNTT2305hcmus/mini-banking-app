package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
	caPort := env.CAPort
	caAddr := "localhost:" + caPort

	// Assuming CA runs locally for now
	conn, err := grpc.NewClient(caAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to CA Service: %v", err)
	}
	defer conn.Close()

	caClient := capb.NewCAServiceClient(conn)
	log.Println("[INFO] Successfully initialized CA Service client.")

	// 4. Initialize Core KDC Service
	svc := kdc.NewService(caClient, redisClient)

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
