package config

import (
	"log"
	"os"
	"strconv"
)

/**
 * @description config contains all configurations of ca service
 * @note read from environment var, which has default value
 * @note gRPCPort is the port of gRPC that server listen to
 * @note RootCAKeyPath is the path of file private key of Root CA (PEM)
 * @note RootCACertsPath is the path of file certificate of Root CA (PEM)
 * @note IssuedCertsPath is the folder that had saved all certificates had been given
 * @note CertValidityDays is the number of valid days of given certificate
 */
type Config struct {
	GRPCPort         string
	RootCAKeyPath    string
	RootCACertPath   string
	IssuedCertsPath  string
	CertValidityDays int
}

/**
 * @description Read configuration from environment variables which have been set in docker-compose.yml
 * @note Use relative component path to test on local easier, which don't need to use Docker
 */
func Load() *Config {
	return &Config{
		GRPCPort:         getEnv("GRPC_PORT", "50051"),
		RootCAKeyPath:    getEnv("ROOT_CA_KEY_PATH", "certs/root-ca/ca.key"),
		RootCACertPath:   getEnv("ROOT_CA_CERT_PATH", "certs/root-ca/ca.crt"),
		IssuedCertsPath:  getEnv("ISSUED_CERTS_PATH", "certs/issued"),
		CertValidityDays: getEnvInt("CERT_VALIDITY_DAYS", 365),
	}
}

/**
 * @description
 */
func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

/**
 * @description
 * @note If strconv.Atoi return err -> crash
 */
func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("[Config] Invalid value for %s=%q: must be an integer (%v)", key, v, err)
		}
		return i
	}
	return defaultVal
}
