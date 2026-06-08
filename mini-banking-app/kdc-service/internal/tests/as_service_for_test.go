package kdc_test

import (
	"kdc-service/internal/kdc"

	capb "mini_banking/pkg/pb/ca"

	"github.com/redis/go-redis/v9"
)

// NewServiceForTest là constructor bổ sung dành cho unit test.
//
// Cho phép inject *KDCKeys trực tiếp (ephemeral, sinh trong test)
// thay vì đọc từ file system qua LoadKeys() — loại bỏ phụ thuộc vào
// .env, file key, và cấu hình môi trường khi chạy test.
//
// Sử dụng trong tests/:
//
//	svc := kdc.NewServiceForTest(fakeCA, nil, &kdc.KDCKeys{...})
func NewServiceForTest(
	caClient capb.CAServiceClient,
	redisClient *redis.Client,
	keys *KDCKeys,
) *ASService {
	return kdc.NewServiceForTest(caClient, redisClient, keys)
}
