package config

import (
	"log"
	"strings"
)

type RedisConfig struct {
	URL string
}

func LoadRedisConfig() *RedisConfig {
	redisStr := LoadEnv().RedisURI

	if redisStr == "" {
		log.Fatal("KDC_REDIS_URL is missing")
	}

	var redisURL string

	// Support Upstash copy-paste format
	if strings.Contains(redisStr, "redis-cli") {
		parts := strings.Split(redisStr, "-u ")

		if len(parts) > 1 {
			rawURL := strings.TrimSpace(parts[1])

			rawURL = strings.Trim(rawURL, `"'`)

			if strings.Contains(redisStr, "--tls") {
				rawURL = strings.Replace(rawURL, "redis://", "rediss://", 1)
			}

			redisURL = rawURL
		}
	} else {
		redisURL = strings.Trim(redisStr, `"'`)
	}

	return &RedisConfig{
		URL: redisURL,
	}
}
