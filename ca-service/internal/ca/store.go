package ca

import (
	"crypto/x509"
	"sync"
	"time"
)

// CertRecord lưu thông tin của một certificate đã cấp.
type CertRecord struct {
	UserID       string
	Cert         *x509.Certificate
	CertPEM      string
	RevokedAt    *time.Time // nil nếu chưa bị revoke
	RevokeReason string
}

// Store là in-memory store cho tất cả cert đã cấp.
// Key: serial number dạng hex string (vd: "1a2b3c...")
//
// Production: thay bằng Postgres query vào bảng user_certificates.
// Store này đủ dùng cho scope đồ án vì:
//  1. CA restart sẽ load lại từ file PEM trong IssuedCertsPath
//  2. Thread-safe nhờ RWMutex
type Store struct {
	mu      sync.RWMutex
	records map[string]*CertRecord // serial -> record
}

// NewStore khởi tạo store rỗng.
//
// Giới hạn quan trọng: store KHÔNG được load lại từ file PEM trên disk
// sau khi restart vì file .pem chuẩn X.509 không lưu trạng thái revocation.
// Load lại sẽ khiến cert đã revoke xuất hiện trở lại với trạng thái VALID —
// đây là lỗ hổng bảo mật nghiêm trọng.
//
// Hệ quả: sau khi CA Service restart, các cert đã cấp trước đó sẽ trả
// codes.NotFound khi KDC/Bank gọi GetCertificate hoặc CheckRevocation.
//
// Production fix: thay store này bằng Postgres query vào bảng user_certificates
// — bảng này có cột status ('active'/'revoked'/'expired') persist qua restart.
func NewStore() *Store {
	return &Store{
		records: make(map[string]*CertRecord),
	}
}

// Save lưu một cert record mới vào store.
func (s *Store) Save(serial string, record *CertRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[serial] = record
}

// Get trả về cert record theo serial. Trả nil nếu không tìm thấy.
func (s *Store) Get(serial string) *CertRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.records[serial]
}

// Revoke đánh dấu cert bị thu hồi.
// Trả false nếu serial không tồn tại hoặc đã bị revoke trước đó.
func (s *Store) Revoke(serial, reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[serial]
	if !ok || rec.RevokedAt != nil {
		return false
	}

	now := time.Now().UTC()
	rec.RevokedAt = &now
	rec.RevokeReason = reason
	return true
}
