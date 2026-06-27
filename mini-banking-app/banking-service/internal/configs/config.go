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
	RedisURI          string
	BankKVKey         string
	CAServiceAddress  string
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

	dsn := os.Getenv("BANK_DATABASE_URL")
	if dsn == "" {
		log.Println("BANK_DATABASE_URL is not set")
	}

	port := os.Getenv("BANK_GRPC_PORT")
	if port == "" {
		port = "50053"
	}

	return &Config{
		PostgresDSN:       dsn,
		RedisURI:          getEnv("BANK_REDIS_URL", "redis://localhost:6379/0"),
		BankKVKey:         getEnv("BANK_KEY_K_V", ""),
		CAServiceAddress:  getEnv("CA_SERVICE_ADDRESS", "localhost:50051"),
		GRPCPort:          port,
		TLSServerCertPath: getEnv("BANK_CERT_PATH", "certs/bank-server.crt"),
		TLSServerKeyPath:  getEnv("BANK_KEY_PATH", "certs/bank-server.key"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
