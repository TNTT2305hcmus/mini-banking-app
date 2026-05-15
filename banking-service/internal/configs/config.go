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
	PostgresDSN string
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
		log.Fatal("POSTGRES_CONNECTION_STRING is not set")
	}

	return &Config{
		PostgresDSN: dsn,
	}
}
