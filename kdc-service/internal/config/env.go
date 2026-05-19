package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

/**
 * @description EnvConfig stores all required environment
 * configuration values for the KDC service.
 *
 * @typedef {Object} EnvConfig
 * @property {string} GRPCPort - Port used by KDC gRPC server.
 * @property {string} CAPort - CA Service gRPC port/address.
 * @property {time.Duration} TGTExp - TGT expiration duration.
 * @property {string} KTGSPath - Filesystem path to K_tgs AES key.
 * @property {string} KDCPrivatePath - Filesystem path to KDC RSA private key.
 * @property {string} RedisURI - Redis connection URI.
 */
type EnvConfig struct {
	GRPCPort       string
	CAPort         string
	TGTExp         time.Duration
	KTGSPath       string
	KDCPrivatePath string
	RedisURI       string
}

/**
 * @description LoadEnv loads all required environment variables
 * from .env file or system environment variables.
 *
 * @note If the .env file is missing, the application falls back
 * to OS environment variables.
 *
 * @function LoadEnv
 * @returns {*EnvConfig} Loaded environment configuration.
 */
func LoadEnv() *EnvConfig {

	// @note 1. Load .env file if available
	err := godotenv.Load(".env")

	if err != nil {
		log.Println(
			"[WARN] .env file not found, using system environment variables",
		)
	}

	// @note 2. Read and validate TGT expiration duration
	tgtExpStr := MustGetEnv("TGT_EXP")

	tgtExp, err := time.ParseDuration(tgtExpStr)

	if err != nil {
		log.Fatalf(
			"[ENV] invalid TGT_EXP duration format (%s): %v",
			tgtExpStr,
			err,
		)
	}

	// @note 3. Build configuration object
	cfg := &EnvConfig{
		GRPCPort: MustGetEnv("GRPC_PORT"),

		CAPort: MustGetEnv("CA_PORT"),

		TGTExp: tgtExp,

		KTGSPath: MustGetEnv("K_TGS_PATH"),

		KDCPrivatePath: MustGetEnv(
			"KDC_PRIVATE_KEY_PATH",
		),

		RedisURI: MustGetEnv("REDIS_URI"),
	}

	// @note 4. Validate environment configuration
	//validateEnv(cfg)

	return cfg
}

/**
 * @description MustGetEnv retrieves a required environment variable.
 * @note The application exits immediately if the variable is missing.
 *
 * @function MustGetEnv
 * @param {string} key - Environment variable name.
 * @returns {string} Environment variable value.
 */
func MustGetEnv(key string) string {

	val := os.Getenv(key)

	if val == "" {
		log.Fatalf(
			"[ENV] missing required environment variable: %s",
			key,
		)
	}

	return val
}
