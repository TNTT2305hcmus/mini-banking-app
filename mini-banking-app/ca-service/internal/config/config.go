package config

/**
 * @title CA Service - Configuration Manager
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Loading and validating environment variables (ports, paths, validity days) to configure the CA service safely.
 */

import (
	"log"
	"os"
	"strconv"
	"strings"
)

/**
 * @description Config contains all configurations for the CA service.
 * @note Values are read from environment variables, falling back to defaults if not set.
 *
 * @typedef {Object} Config
 * @property {string} GRPCPort - The port the gRPC server listens on.
 * @property {string} RootCAKeyPath - The file path to the Root CA's private key (PEM).
 * @property {string} RootCACertPath - The file path to the Root CA's certificate (PEM).
 * @property {string} ClientCAKeyPath - The file path to the Client CA's private key (PEM).
 * @property {string} ClientCACertPath - The file path to the Client CA's certificate (PEM).
 * @property {string} IssuedCertsPath - The folder where all issued certificates are saved.
 * @property {string} StoreBackend - The certificate repository backend: json or postgres.
 * @property {string} StoreStatePath - The file path for durable issued certificate/revocation state.
 * @property {string} DatabaseURL - The PostgreSQL connection URL for the postgres backend.
 * @property {int} CertValidityDays - The number of valid days for a newly issued certificate.
 * @property {[]string} CRLDistributionPoints - CRL URLs embedded in issued certificates.
 * @property {[]string} OCSPServers - OCSP responder URLs embedded in issued certificates.
 * @property {string} GRPCServerCertPath - The file path to the CA gRPC server certificate.
 * @property {string} GRPCServerKeyPath - The file path to the CA gRPC server private key.
 */
type Config struct {
	GRPCPort              string
	RootCAKeyPath         string
	RootCACertPath        string
	ClientCAKeyPath       string
	ClientCACertPath      string
	IssuedCertsPath       string
	StoreBackend          string
	StoreStatePath        string
	DatabaseURL           string
	CertValidityDays      int
	CRLDistributionPoints []string
	OCSPServers           []string
	GRPCServerCertPath    string
	GRPCServerKeyPath     string
}

/**
 * @description Load reads the configuration from environment variables (typically set in docker-compose.yml).
 * @note It uses relative paths by default to make local testing easier without needing Docker.
 *
 * @function Load
 * @returns {*Config} A pointer to the loaded Config struct.
 */
func Load() *Config {
	return &Config{
		GRPCPort:              getEnv("GRPC_PORT", "50051"),
		RootCAKeyPath:         getEnv("ROOT_CA_KEY_PATH", "certs/root-ca/ca.key"),
		RootCACertPath:        getEnv("ROOT_CA_CERT_PATH", "certs/root-ca/ca.crt"),
		ClientCAKeyPath:       getEnv("CLIENT_CA_KEY_PATH", "certs/intermediate/client-ca.key"),
		ClientCACertPath:      getEnv("CLIENT_CA_CERT_PATH", "certs/intermediate/client-ca.crt"),
		IssuedCertsPath:       getEnv("ISSUED_CERTS_PATH", "certs/issued"),
		StoreBackend:          strings.ToLower(getEnv("CA_STORE_BACKEND", "json")),
		StoreStatePath:        getEnv("CA_STORE_STATE_PATH", "certs/ca-store/state.json"),
		DatabaseURL:           getEnv("CA_DATABASE_URL", ""),
		CertValidityDays:      getEnvInt("CERT_VALIDITY_DAYS", 365),
		CRLDistributionPoints: getEnvCSV("CA_CRL_DISTRIBUTION_POINTS", ""),
		OCSPServers:           getEnvCSV("CA_OCSP_SERVERS", ""),
		GRPCServerCertPath:    getEnv("GRPC_SERVER_CERT_PATH", "certs/grpc/ca-server.crt"),
		GRPCServerKeyPath:     getEnv("GRPC_SERVER_KEY_PATH", "certs/grpc/ca-server.key"),
	}
}

/**
 * @description getEnv retrieves the value of the environment variable named by the key.
 * @note If the variable is not present or is empty, it returns the specified default value.
 *
 * @function getEnv
 * @param {string} key - The environment variable key to look up.
 * @param {string} defaultVal - The fallback value if the environment variable is not set.
 * @returns {string} The resolved environment variable or the default value.
 */
func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

/**
 * @description getEnvInt retrieves the integer value of the environment variable named by the key.
 * @note If the variable is not present, it returns the specified default value.
 *
 * @function getEnvInt
 * @param {string} key - The environment variable key to look up.
 * @param {int} defaultVal - The fallback integer value if the environment variable is not set.
 * @returns {int} The parsed integer value or the default value.
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

func getEnvCSV(key, defaultVal string) []string {
	value := getEnv(key, defaultVal)
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
