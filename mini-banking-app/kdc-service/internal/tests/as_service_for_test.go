package kdc_test

import (
	"kdc-service/internal/kdc"
)

// NewServiceForTest is a thin wrapper over kdc.NewServiceForTest so tests can
// build an AS service with injected dependencies (in-memory cert repo, replay
// store, clock) and ephemeral keys — no .env, key files, or real CA/Redis.
//
// Usage:
//
//	svc := NewServiceForTest(ASConfig{CertRepo: repo, ReplayStore: replay, Keys: keys})
func NewServiceForTest(cfg ASConfig) *ASService {
	return kdc.NewServiceForTest(cfg)
}
