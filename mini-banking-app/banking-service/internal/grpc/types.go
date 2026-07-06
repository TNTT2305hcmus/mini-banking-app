package grpc

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// replayStore records request fingerprints to detect replays. Backed by Redis in
// production, with a DB fallback handled by the Handler.
type replayStore interface {
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
}

type redisReplayStore struct {
	client *redis.Client
}

func (s redisReplayStore) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, value, ttl).Result()
}

// clock abstracts time so tests can inject a deterministic source.
type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

// ticketInfo is the decrypted, validated Ticket_V the Handler works with after
// parsing bank.ServiceTicketPlaintext.
type ticketInfo struct {
	clientID          string
	session           []byte
	certSN            string
	certType          string
	issuerID          string
	issuerCommonName  string
	issuerSerial      string
	chainFingerprints []string
	scope             string
	expires           time.Time
	publicKeyPEM      string
}

// authInfo is the result of a successful AP exchange: the ticket plus the
// authenticator fields and the resolved client public key.
type authInfo struct {
	ticket            ticketInfo
	sessionKey        []byte
	nonce             string
	requestID         string
	timestamp         time.Time
	publicKeyPEM      string
	certType          string
	issuerID          string
	issuerCommonName  string
	issuerSerial      string
	chainFingerprints []string
}

// transferPayload is the signed, encrypted transfer body the client sends inside
// the cipher payload.
type transferPayload struct {
	FromAccountNumber string `json:"from_account_number"`
	ToAccountNumber   string `json:"to_account_number"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	Description       string `json:"description,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
	Timestamp         int64  `json:"timestamp,omitempty"`
	RequestID         string `json:"request_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	Scope             string `json:"scope"`
}
