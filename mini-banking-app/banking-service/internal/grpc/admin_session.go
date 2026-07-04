package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"mini-banking/banking-service/internal/bank"
)

var errAdminSessionNotFound = errors.New("admin session not found")

type adminSessionStore interface {
	Create(ctx context.Context, tokenHash string, session bank.AdminSession, ttl time.Duration) error
	Get(ctx context.Context, tokenHash string) (bank.AdminSession, error)
}

type redisAdminSessionStore struct {
	client *redis.Client
}

func (s redisAdminSessionStore) Create(ctx context.Context, tokenHash string, session bank.AdminSession, ttl time.Duration) error {
	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, bank.AdminSessionKeyPrefix+tokenHash, encoded, ttl).Err()
}

func (s redisAdminSessionStore) Get(ctx context.Context, tokenHash string) (bank.AdminSession, error) {
	encoded, err := s.client.Get(ctx, bank.AdminSessionKeyPrefix+tokenHash).Bytes()
	if errors.Is(err, redis.Nil) {
		return bank.AdminSession{}, errAdminSessionNotFound
	}
	if err != nil {
		return bank.AdminSession{}, err
	}
	var session bank.AdminSession
	if err := json.Unmarshal(encoded, &session); err != nil {
		return bank.AdminSession{}, err
	}
	return session, nil
}

func hashAdminSessionToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}
