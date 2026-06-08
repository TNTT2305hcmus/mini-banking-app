package kdc

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"time"

	"kdc-service/internal/config"
	capb "mini_banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

/**
 * Represents the production KDC facade used by the gRPC layer.
 *
 * @property {*ASService} ASService - The embedded AS exchange service.
 * @property {*TGSService} tgsService - The TGS exchange service used to issue service tickets.
 */
type Service struct {
	*ASService
	tgsService *TGSService
}

/**
 * Creates the production KDC service facade with AS and TGS dependencies wired together.
 *
 * @param {capb.CAServiceClient} caClient - The CA gRPC client used by AS and TGS flows.
 * @param {*redis.Client} redisClient - The Redis client used for replay protection.
 * @returns {*Service} A production KDC service instance.
 */
func NewService(caClient capb.CAServiceClient, redisClient *redis.Client) (*Service, error) {
	asService, err := NewASService(caClient, redisClient)
	if err != nil {
		return nil, err
	}

	env := config.LoadEnv()

	serviceID := getEnvDefault("BANK_SERVICE_ID", "bank-service")
	serviceKeyPath := getEnvDefault("BANK_SERVICE_KEY_PATH", env.KTGSPath)
	if serviceKeyPath == env.KTGSPath {
		log.Printf("[KDC] BANK_SERVICE_KEY_PATH is not set, using K_TGS_PATH for %s demo key", serviceID)
	}

	serviceKey, err := loadAES256Key(serviceKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load service key: %w", err)
	}

	tgsService, err := NewTGSService(Config{
		TGSKey: asService.kdcKeys.KTGSKey,
		ServiceKeys: map[string][]byte{
			serviceID: serviceKey,
		},
		ReplayStore: RedisReplayStore{Client: redisClient},
		CertRepo:    CACertificateRepository{Client: caClient},
		ScopeAuthorizer: StaticScopeAuthorizer{
			serviceID: {
				"transfer:internal": true,
				"account:read":      true,
			},
		},
		TicketTTL:       5 * time.Minute,
		TimestampWindow: 5 * time.Minute,
		ReplayTTL:       5 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("init TGS service: %w", err)
	}

	return &Service{
		ASService:  asService,
		tgsService: tgsService,
	}, nil
}

/**
 * Requests a service ticket through the wrapped TGS service.
 *
 * @param {context.Context} ctx - The request context.
 * @param {TGSRequest} req - The service-ticket request payload.
 * @returns {TGSResponse} The encrypted TGS reply and ticket expiration metadata.
 * @returns {error} A domain error when validation or ticket issuance fails.
 */
func (s *Service) RequestServiceTicket(ctx context.Context, req TGSRequest) (TGSResponse, error) {
	return s.tgsService.RequestServiceTicket(ctx, req)
}

/**
 * Adapts a Redis client to the ReplayStore interface required by the TGS service.
 *
 * @property {*redis.Client} Client - The Redis client used for SET NX replay markers.
 */
type RedisReplayStore struct {
	Client *redis.Client
}

/**
 * Stores a replay marker using Redis SET NX semantics.
 *
 * @param {context.Context} ctx - The request context.
 * @param {string} key - The replay marker key.
 * @param {string} value - The marker value to store.
 * @param {time.Duration} ttl - The expiration time for the marker.
 * @returns {bool} True when the marker was inserted, false when it already existed.
 * @returns {error} A Redis error when the write fails.
 */
func (s RedisReplayStore) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return s.Client.SetNX(ctx, key, value, ttl).Result()
}

/**
 * Adapts the CA gRPC client to the CertificateRepository interface required by the TGS service.
 *
 * @property {capb.CAServiceClient} Client - The CA gRPC client used for certificate status and public key lookup.
 */
type CACertificateRepository struct {
	Client capb.CAServiceClient
}

/**
 * Loads certificate metadata from CA service for the TGS validation flow.
 *
 * @param {context.Context} ctx - The request context.
 * @param {string} certSN - The certificate serial number to look up.
 * @returns {Certificate} The certificate metadata required by the TGS service.
 * @returns {error} ErrCertificateMissing or a lower-level CA/parsing error.
 */
func (r CACertificateRepository) GetCertificate(ctx context.Context, certSN string) (Certificate, error) {
	rev, err := r.Client.CheckRevocation(ctx, &capb.CheckRevocationRequest{
		SerialNumber: certSN,
	})
	if status.Code(err) == codes.NotFound {
		return Certificate{}, ErrCertificateMissing
	}
	if err != nil {
		return Certificate{}, err
	}

	resp, err := r.Client.GetCertificate(ctx, &capb.GetCertificateRequest{
		SerialNumber: certSN,
	})
	if status.Code(err) == codes.NotFound {
		return Certificate{}, ErrCertificateMissing
	}
	if err != nil {
		return Certificate{}, err
	}

	block, _ := pem.Decode([]byte(resp.CertificatePem))
	if block == nil {
		return Certificate{}, fmt.Errorf("decode certificate pem failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Certificate{}, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return Certificate{}, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	return Certificate{
		Serial:       certSN,
		SubjectCN:    cert.Subject.CommonName,
		PublicKeyPEM: string(pubPEM),
		Status:       mapCACertStatus(rev.Status),
		NotAfter:     cert.NotAfter,
	}, nil
}

/**
 * Maps CA protobuf certificate status values to the KDC domain status type.
 *
 * @param {capb.CertStatus} st - The certificate status returned by CA service.
 * @returns {CertificateStatus} The equivalent KDC certificate status.
 */
func mapCACertStatus(st capb.CertStatus) CertificateStatus {
	switch st {
	case capb.CertStatus_CERT_STATUS_VALID:
		return CertificateValid
	case capb.CertStatus_CERT_STATUS_REVOKED:
		return CertificateRevoked
	case capb.CertStatus_CERT_STATUS_EXPIRED:
		return CertificateExpired
	default:
		return ""
	}
}

/**
 * Reads and validates a raw AES-256 key from disk.
 *
 * @param {string} path - The filesystem path to the raw 32-byte key file.
 * @returns {[]byte} The AES-256 key bytes.
 * @returns {error} An error when the file cannot be read or the key length is invalid.
 */
func loadAES256Key(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key = cleanNewline(key)
	if len(key) != aes256KeySize {
		return nil, fmt.Errorf("AES-256 key at %s must be 32 bytes, got %d", path, len(key))
	}
	return key, nil
}

/**
 * Reads an environment variable with a fallback value.
 *
 * @param {string} key - The environment variable name.
 * @param {string} fallback - The fallback value when the variable is empty.
 * @returns {string} The resolved environment value.
 */
func getEnvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
