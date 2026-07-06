package kdc

import (
	"context"
	"crypto/rand"
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
	env := config.LoadEnv()

	keys, err := LoadKeys(env.KTGSPath, env.KDCPrivatePath)
	if err != nil {
		return nil, fmt.Errorf("load_kdc_keys_failed: %w", err)
	}

	// Shared collaborators wired once and used by BOTH the AS and TGS exchanges.
	certRepo := CACertificateRepository{Client: caClient}
	replayStore := RedisReplayStore{Client: redisClient}
	clock := SystemClock{}

	asService, err := NewASService(ASConfig{
		CertRepo:        certRepo,
		ReplayStore:     replayStore,
		Clock:           clock,
		Random:          rand.Reader,
		Keys:            keys,
		TGTTTL:          env.TGTExp,
		TimestampWindow: 5 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("init AS service: %w", err)
	}

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
		TGSKey: keys.KTGSKey,
		ServiceKeys: map[string][]byte{
			serviceID: serviceKey,
		},
		ReplayStore: replayStore,
		CertRepo:    certRepo,
		ScopeAuthorizer: StaticScopeAuthorizer{
			serviceID: {
				"transfer:create": true,
				"balance:read":    true,
				"history:read":    true,
				"bank-admin:read": true,
			},
		},
		Clock:           clock,
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
	resp, err := r.Client.VerifyCertificate(ctx, &capb.VerifyCertificateRequest{
		SerialNumber:          certSN,
		Caller:                "kdc-service",
		IncludePublicKeyPem:   true,
		IncludeCertificatePem: true,
	})
	if status.Code(err) == codes.NotFound {
		return Certificate{}, ErrCertificateMissing
	}
	if err != nil {
		return Certificate{}, err
	}

	publicKeyPEM := resp.PublicKeyPem
	var subjectCN string
	var notBefore time.Time
	var notAfter time.Time
	if resp.NotBeforeUnix != 0 {
		notBefore = time.Unix(resp.NotBeforeUnix, 0)
	}
	if resp.NotAfterUnix != 0 {
		notAfter = time.Unix(resp.NotAfterUnix, 0)
	}
	if resp.CertificatePem != "" {
		block, _ := pem.Decode([]byte(resp.CertificatePem))
		if block == nil {
			return Certificate{}, fmt.Errorf("decode certificate pem failed")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return Certificate{}, err
		}
		subjectCN = cert.Subject.CommonName
		if notBefore.IsZero() {
			notBefore = cert.NotBefore
		}
		if notAfter.IsZero() {
			notAfter = cert.NotAfter
		}
		if publicKeyPEM == "" {
			pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
			if err != nil {
				return Certificate{}, err
			}
			publicKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
		}
	}

	return Certificate{
		Serial:            certSN,
		CertType:          resp.GetCertType(),
		IssuerID:          resp.GetIssuerId(),
		IssuerCommonName:  resp.GetIssuerCommonName(),
		IssuerSerial:      resp.GetIssuerSerialNumber(),
		ChainPEM:          resp.GetChainPem(),
		ChainFingerprints: append([]string(nil), resp.GetChainFingerprints()...),
		OwnerID:           resp.OwnerId,
		Role:              mapCAIdentityRole(resp.GetRole()),
		SubjectCN:         subjectCN,
		PublicKeyPEM:      publicKeyPEM,
		Status:            mapCACertStatus(resp.Status),
		NotBefore:         notBefore,
		NotAfter:          notAfter,
	}, nil
}

// Missing/UNKNOWN is treated as customer during rollout so certificates
// issued before the role migration keep the existing Client behavior.
func mapCAIdentityRole(role capb.IdentityRole) IdentityRole {
	if role == capb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN {
		return IdentityRoleBankAdmin
	}
	return IdentityRoleCustomer
}

/**
 * Maps CA protobuf certificate status values to the KDC domain status type.
 *
 * @param {capb.CertStatus} st - The certificate status returned by CA service.
 * @returns {CertificateStatus} The equivalent KDC certificate status.
 */
func mapCACertStatus(st capb.CertStatus) CertificateStatus {
	switch st {
	case capb.CertStatus_CERT_STATUS_ACTIVE:
		return CertificateActive
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
