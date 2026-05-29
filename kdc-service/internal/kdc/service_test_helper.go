package kdc

import (
	capb "mini_banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
)

// NewServiceForTest builds an AS service with injected dependencies.
func NewServiceForTest(
	caClient capb.CAServiceClient,
	redisClient *redis.Client,
	keys *KDCKeys,
) *ASService {
	return &ASService{
		caClient:    caClient,
		redisClient: redisClient,
		kdcKeys:     keys,
	}
}
