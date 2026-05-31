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
)

/**
 * @description Config contains all configurations for the CA service.
 * @note Values are read from environment variables, falling back to defaults if not set.
 *
 * @typedef {Object} Config
 * @property {string} GRPCPort - The port the gRPC server listens on.
 * @property {string} RootCAKeyPath - The file path to the Root CA's private key (PEM).
 * @property {string} RootCACertPath - The file path to the Root CA's certificate (PEM).
 * @property {string} IssuedCertsPath - The folder where all issued certificates are saved.
 * @property {int} CertValidityDays - The number of valid days for a newly issued certificate.
 */
type Config struct {
	GRPCPort         string
	RootCAKeyPath    string
	RootCACertPath   string
	IssuedCertsPath  string
	CertValidityDays int
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
		GRPCPort:         getEnv("GRPC_PORT", "50051"),
		RootCAKeyPath:    getEnv("ROOT_CA_KEY_PATH", "certs/root-ca/ca.key"),
		RootCACertPath:   getEnv("ROOT_CA_CERT_PATH", "certs/root-ca/ca.crt"),
		IssuedCertsPath:  getEnv("ISSUED_CERTS_PATH", "certs/issued"),
		CertValidityDays: getEnvInt("CERT_VALIDITY_DAYS", 365),
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
