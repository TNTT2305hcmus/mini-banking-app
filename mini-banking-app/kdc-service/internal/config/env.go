package config

import (
	"log"
	"net"
	"os"
	"strings"
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
 * @property {string} CAAddress - CA Service gRPC address.
 * @property {time.Duration} TGTExp - TGT expiration duration.
 * @property {string} KTGSPath - Filesystem path to K_tgs AES key.
 * @property {string} KDCPrivatePath - Filesystem path to KDC RSA private key.
 * @property {string} RedisURI - Redis connection URI.
 */
type EnvConfig struct {
	GRPCPort       string
	CAPort         string
	CAAddress      string
	TGTExp         time.Duration
	KTGSPath       string
	KDCPrivatePath string
	RedisURI       string
	// PostgresDSN is optional: when empty the KDC runs without a durable audit
	// log (audit becomes a no-op). Set DATABASE_URL to enable kdc_audit_log.
	PostgresDSN string
	// KDCSigningChainPath is optional: path to the PEM bundle (KDC signing leaf
	// cert + intermediate) that the client uses to verify the AS_REP signature
	// against the embedded Root CA. When empty, AS_REP omits kdc_cert_chain and
	// clients that require it will reject the response (fail-closed).
	KDCSigningChainPath string
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

	caPort := MustGetEnv("CA_PORT")

	// @note 3. Build configuration object
	cfg := &EnvConfig{
		GRPCPort: MustGetEnv("GRPC_PORT"),

		CAPort: caPort,

		CAAddress: resolveCAAddress(caPort),

		TGTExp: tgtExp,

		KTGSPath: MustGetEnv("K_TGS_PATH"),

		KDCPrivatePath: MustGetEnv(
			"KDC_PRIVATE_KEY_PATH",
		),

		RedisURI: MustGetEnv("REDIS_URI"),

		// Optional: no MustGetEnv so existing local runs without Postgres still boot.
		PostgresDSN: os.Getenv("DATABASE_URL"),

		// Optional: path to the KDC signing cert chain served in AS_REP.
		KDCSigningChainPath: os.Getenv("KDC_SIGNING_CHAIN_PATH"),
	}

	// @note 4. Validate environment configuration
	//validateEnv(cfg)

	return cfg
}

func resolveCAAddress(caPort string) string {
	if address := os.Getenv("CA_ADDRESS"); address != "" {
		return address
	}

	if strings.Contains(caPort, ":") {
		return caPort
	}

	host := os.Getenv("CA_HOST")
	if host == "" {
		host = "localhost"
	}

	return net.JoinHostPort(host, caPort)
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
