package ca

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu              sync.RWMutex
	issuers         map[string]IssuerRecord
	certificates    map[string]CertificateRecord
	auditLog        []AuditEvent
	persistencePath string
}

type persistedState struct {
	Version      int                          `json:"version"`
	Issuers      map[string]IssuerRecord      `json:"issuers,omitempty"`
	Certificates map[string]CertificateRecord `json:"certificates"`
	AuditLog     []AuditEvent                 `json:"audit_log"`
}

func NewStore() *Store {
	return &Store{
		issuers:      make(map[string]IssuerRecord),
		certificates: make(map[string]CertificateRecord),
	}
}

func NewPersistentStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: CA_STORE_STATE_PATH is required", ErrInvalidInput)
	}
	store := &Store{
		issuers:         make(map[string]IssuerRecord),
		certificates:    make(map[string]CertificateRecord),
		persistencePath: path,
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) UpsertIssuer(_ context.Context, record IssuerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(record.IssuerID) == "" {
		return fmt.Errorf("%w: issuer_id is required", ErrInvalidInput)
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if existing, ok := s.issuers[record.IssuerID]; ok && !existing.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	if record.Status == "" {
		record.Status = IssuerStatusActive
	}
	s.issuers[record.IssuerID] = cloneIssuerRecord(record)
	return s.persistLocked()
}

func (s *Store) UpsertCertificate(_ context.Context, record CertificateRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(record.SerialNumber) == "" {
		return fmt.Errorf("%w: serial_number is required", ErrInvalidInput)
	}
	now := time.Now().UTC()
	if existing, ok := s.certificates[record.SerialNumber]; ok {
		if record.CreatedAt.IsZero() {
			record.CreatedAt = existing.CreatedAt
		}
		if existing.RevokedAt != nil || existing.Status == CertStatusRevoked {
			record.Status = CertStatusRevoked
			record.RevokedAt = existing.RevokedAt
			record.RevocationReason = existing.RevocationReason
		}
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	if record.Status == "" {
		record.Status = CertStatusActive
	}
	s.certificates[record.SerialNumber] = cloneCertificateRecord(record)
	return s.persistLocked()
}

func (s *Store) CreateCertificate(_ context.Context, record CertificateRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.certificates[record.SerialNumber]; exists {
		return fmt.Errorf("%w: serial_number=%s", ErrActiveCertificateExists, record.SerialNumber)
	}
	now := time.Now().UTC()
	for _, existing := range s.certificates {
		if record.CertType == CertTypeClient && existing.CertType == CertTypeClient && existing.OwnerID == record.OwnerID && isActive(existing, now) {
			return fmt.Errorf("%w: owner_id=%s", ErrActiveCertificateExists, record.OwnerID)
		}
	}
	s.certificates[record.SerialNumber] = cloneCertificateRecord(record)
	return s.persistLocked()
}

func (s *Store) GetCertificate(_ context.Context, serial string) (*CertificateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.certificates[serial]
	if !ok {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrCertificateNotFound, serial)
	}
	cloned := cloneCertificateRecord(record)
	return &cloned, nil
}

func (s *Store) ListCertificates(_ context.Context, filter ListFilter) ([]CertificateRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := strings.TrimSpace(strings.ToLower(filter.Status))
	certType := strings.TrimSpace(strings.ToLower(filter.CertType))
	issuerID := strings.TrimSpace(strings.ToLower(filter.IssuerID))
	ownerID := strings.TrimSpace(strings.ToLower(filter.OwnerID))
	email := strings.TrimSpace(strings.ToLower(filter.SubjectEmail))
	if email == "" {
		email = strings.TrimSpace(strings.ToLower(filter.SubjectEmail))
	}
	serial := strings.TrimSpace(strings.ToLower(filter.SerialNumber))
	now := time.Now().UTC()

	var matched []CertificateRecord
	for _, record := range s.certificates {
		resolvedStatus := resolveRecordStatus(record, now)
		if status != "" && string(resolvedStatus) != status {
			continue
		}
		if certType != "" && strings.ToLower(record.CertType) != certType {
			continue
		}
		if issuerID != "" && strings.ToLower(record.IssuerID) != issuerID {
			continue
		}
		if ownerID != "" && !strings.Contains(strings.ToLower(record.OwnerID), ownerID) {
			continue
		}
		if email != "" && !strings.Contains(strings.ToLower(record.SubjectEmail), email) {
			continue
		}
		if serial != "" && !strings.Contains(strings.ToLower(record.SerialNumber), serial) {
			continue
		}
		record.Status = resolvedStatus
		matched = append(matched, cloneCertificateRecord(record))
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].IssuedAt.After(matched[j].IssuedAt)
	})

	total := len(matched)
	offset := filter.Offset
	if offset > total {
		return nil, total, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (s *Store) RevokeCertificate(_ context.Context, serial, reason string, revokedAt time.Time) (*CertificateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.certificates[serial]
	if !ok {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrCertificateNotFound, serial)
	}
	if record.RevokedAt != nil || record.Status == CertStatusRevoked {
		return nil, fmt.Errorf("%w: serial_number=%s", ErrAlreadyRevoked, serial)
	}
	if record.CertType != CertTypeClient {
		return nil, fmt.Errorf("%w: cert_type=%s serial_number=%s", ErrCertificateNotRevokable, record.CertType, serial)
	}

	revokedAt = revokedAt.UTC()
	record.Status = CertStatusRevoked
	record.RevokedAt = &revokedAt
	record.RevocationReason = reason
	record.UpdatedAt = revokedAt
	s.certificates[serial] = record
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	cloned := cloneCertificateRecord(record)
	return &cloned, nil
}

func (s *Store) AppendAudit(_ context.Context, event AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.PerformedAt.IsZero() {
		event.PerformedAt = time.Now().UTC()
	}
	event.PerformedAt = event.PerformedAt.UTC()
	s.auditLog = append(s.auditLog, cloneAuditEvent(event))
	return s.persistLocked()
}

func (s *Store) ListAudit(_ context.Context, filter AuditFilter) ([]AuditEvent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	serial := strings.TrimSpace(strings.ToLower(filter.SerialNumber))
	action := strings.TrimSpace(strings.ToLower(filter.Action))
	performedBy := strings.TrimSpace(strings.ToLower(filter.PerformedBy))

	var matched []AuditEvent
	for _, event := range s.auditLog {
		if serial != "" && !strings.Contains(strings.ToLower(event.SerialNumber), serial) {
			continue
		}
		if action != "" && string(event.Action) != action {
			continue
		}
		if performedBy != "" && !strings.Contains(strings.ToLower(event.PerformedBy), performedBy) {
			continue
		}
		if !filter.From.IsZero() && event.PerformedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && !event.PerformedAt.Before(filter.To) {
			continue
		}
		matched = append(matched, cloneAuditEvent(event))
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].PerformedAt.After(matched[j].PerformedAt)
	})

	total := len(matched)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		return nil, total, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (s *Store) AuditEvents() []AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]AuditEvent, len(s.auditLog))
	for i, event := range s.auditLog {
		out[i] = cloneAuditEvent(event)
	}
	return out
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.persistencePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read CA store state: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode CA store state: %w", err)
	}
	if state.Certificates != nil {
		s.certificates = state.Certificates
	}
	if state.Issuers != nil {
		s.issuers = state.Issuers
	}
	if state.AuditLog != nil {
		s.auditLog = state.AuditLog
	}
	return s.validateLoadedState()
}

func (s *Store) validateLoadedState() error {
	now := time.Now().UTC()
	activeByOwner := map[string]string{}
	for serial, record := range s.certificates {
		if serial == "" || record.SerialNumber == "" || serial != record.SerialNumber {
			return fmt.Errorf("invalid CA store state: serial key %q does not match record serial %q", serial, record.SerialNumber)
		}
		if record.CertType == "" {
			record.CertType = CertTypeClient
			s.certificates[serial] = record
		}
		if record.SubjectCN == "" {
			return fmt.Errorf("invalid CA store state: certificate %s is missing subject metadata", serial)
		}
		if record.CertType == CertTypeClient && (record.OwnerID == "" || record.SubjectEmail == "") {
			return fmt.Errorf("invalid CA store state: client certificate %s is missing owner/email metadata", serial)
		}
		if record.CertType == CertTypeClient && isActive(record, now) {
			if previous, ok := activeByOwner[record.OwnerID]; ok {
				return fmt.Errorf("%w: owner_id=%s serials=%s,%s", ErrActiveCertificateExists, record.OwnerID, previous, serial)
			}
			activeByOwner[record.OwnerID] = serial
		}
	}
	return nil
}

func (s *Store) persistLocked() error {
	if s.persistencePath == "" {
		return nil
	}

	state := persistedState{
		Version:      2,
		Issuers:      s.issuers,
		Certificates: s.certificates,
		AuditLog:     s.auditLog,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode CA store state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.persistencePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create CA store directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(s.persistencePath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create CA store temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write CA store temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close CA store temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.persistencePath); err != nil {
		return fmt.Errorf("replace CA store state: %w", err)
	}
	return nil
}

func isActive(record CertificateRecord, now time.Time) bool {
	return resolveRecordStatus(record, now) == CertStatusActive
}

func resolveRecordStatus(record CertificateRecord, now time.Time) CertStatus {
	if record.RevokedAt != nil || record.Status == CertStatusRevoked {
		return CertStatusRevoked
	}
	if !record.NotAfter.IsZero() && now.After(record.NotAfter) {
		return CertStatusExpired
	}
	if record.Status == "" || record.Status == CertStatusUnknown {
		return CertStatusUnknown
	}
	return CertStatusActive
}

func cloneCertificateRecord(record CertificateRecord) CertificateRecord {
	record.NotBefore = record.NotBefore.UTC()
	record.NotAfter = record.NotAfter.UTC()
	record.IssuedAt = record.IssuedAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	if record.RevokedAt != nil {
		revokedAt := record.RevokedAt.UTC()
		record.RevokedAt = &revokedAt
	}
	record.ChainFingerprints = cloneStrings(record.ChainFingerprints)
	record.KeyUsage = cloneStrings(record.KeyUsage)
	record.ExtendedKeyUsage = cloneStrings(record.ExtendedKeyUsage)
	return record
}

func cloneIssuerRecord(record IssuerRecord) IssuerRecord {
	record.NotBefore = record.NotBefore.UTC()
	record.NotAfter = record.NotAfter.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	return record
}

func cloneAuditEvent(event AuditEvent) AuditEvent {
	event.PerformedAt = event.PerformedAt.UTC()
	if event.Metadata != nil {
		metadata := make(map[string]string, len(event.Metadata))
		for key, value := range event.Metadata {
			metadata[key] = value
		}
		event.Metadata = metadata
	}
	return event
}
