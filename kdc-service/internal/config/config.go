package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	RedisURL string
	GRPCPort string
}

func Load() *Config {
	// Try loading from .env, ignore if not found
	_ = godotenv.Load("internal/.env")

	redisStr := os.Getenv("REDIS_DATABASE_CONNECTION_STRING")
	if redisStr == "" {
		log.Println("Warning: REDIS_DATABASE_CONNECTION_STRING is empty")
	}

	// Clean up the upstash copy-paste string if it exists
	// "redis-cli --tls -u redis://default:..." -> "rediss://default:..."
	var redisURL string
	if strings.Contains(redisStr, "redis-cli") {
		parts := strings.Split(redisStr, "-u ")
		if len(parts) > 1 {
			rawURL := strings.TrimSpace(parts[1])
			// remove quotes if any
			rawURL = strings.Trim(rawURL, `"'`)
			if strings.Contains(redisStr, "--tls") {
				rawURL = strings.Replace(rawURL, "redis://", "rediss://", 1)
			}
			redisURL = rawURL
		}
	} else {
		redisURL = strings.Trim(redisStr, `"'`)
	}

	return &Config{
		RedisURL: redisURL,
		GRPCPort: GetEnv("GRPC_PORT", ":50052"), // Default port for KDC
	}
}

func GetEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
