package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

/**
 * @struct Config
 * @description Represents the application configuration.
 * @property {string} PostgresDSN - The connection string for the PostgreSQL database.
 */
type Config struct {
	PostgresDSN       string
	GRPCPort          string
	TLSServerCertPath string
	TLSServerKeyPath  string
}

/**
 * @function LoadConfig
 * @description Loads the .env file and returns a Config struct.
 * @returns {*Config} The loaded configuration object.
 */
func LoadConfig() *Config {
	// Load .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Println(".env file not found, using system environment variables")
	}

	dsn := os.Getenv("POSTGRES_CONNECTION_STRING")
	if dsn == "" {
		log.Println("POSTGRES_CONNECTION_STRING is not set")
	}

	port := os.Getenv("BANK_GRPC_PORT")
	if port == "" {
		port = "50053"
	}

	return &Config{
		PostgresDSN:       dsn,
		GRPCPort:          port,
		TLSServerCertPath: getEnv("BANK_TLS_CERT_PATH", "certs/grpc/bank-server.crt"),
		TLSServerKeyPath:  getEnv("BANK_TLS_KEY_PATH", "certs/grpc/bank-server.key"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
