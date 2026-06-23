package kdc

// NewServiceForTest builds an AS service from an injected ASConfig so tests can
// supply in-memory fakes (certificate repository, replay store, clock) and
// ephemeral keys instead of touching the filesystem or environment.
func NewServiceForTest(cfg ASConfig) *ASService {
	svc, err := NewASService(cfg)
	if err != nil {
		panic("NewServiceForTest: " + err.Error())
	}
	return svc
}
