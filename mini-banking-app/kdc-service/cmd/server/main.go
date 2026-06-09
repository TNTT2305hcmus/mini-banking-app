package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// 4. Initialize Core KDC Service
	svc, err := kdc.NewService(caClient, redisClient)
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

func loadCATLSCredentials() credentials.TransportCredentials {
	caCertPath := requiredEnv("CA_TLS_CA_CERT_PATH")
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
