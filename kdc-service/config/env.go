package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	TGT_EXP string
	K_TGS   []byte
}

// func validate(cfg *Config) {
// 	required := map[string]string{
// 		"JWT_SECRET": cfg.JWTSecret,
// 		"DB_HOST":    cfg.DBHost,
// 		"DB_PORT":    cfg.DBPort,
// 	}

// 	for key, value := range required {
// 		if value == "" {
// 			log.Fatalf("Missing required env: %s", key)
// 		}
// 	}

// kTgs, err := hex.DecodeString(hexKey)
// if err != nil || len(kTgs) != 32 {
// 	log.Fatal("K_TGS_MASTER must be 32 bytes")
// }

// }

func LoadEnv() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	cfg := &Config{
		TGT_EXP: os.Getenv("TGT_EXP"),
		K_TGS:   []byte(os.Getenv("K_TGS")),
	}

	//validate(cfg)

	return cfg
}
