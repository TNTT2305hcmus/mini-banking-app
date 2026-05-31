package ca

/**
 * @title CA Service - Certificate Store
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Managing a thread-safe storage for issued certificates and revocation status.
 */

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrActiveCertificateExists = errors.New("active certificate already exists for user")

/**
 * @description CertRecord stores information about an issued certificate.
 *
 * @typedef {Object} CertRecord
 * @property {string} UserID - The ID of the user owning the certificate.
 * @property {*x509.Certificate} Cert - The parsed x509 certificate.
 * @property {string} CertPEM - The PEM-encoded certificate string.
 * @property {*time.Time} RevokedAt - The timestamp of revocation, or nil if not revoked.
 * @property {string} RevokeReason - The reason for revocation.
 */
type CertRecord struct {
	UserID  string
	Cert    *x509.Certificate
	CertPEM string
	// @note If not revoked -> nil
	RevokedAt    *time.Time
	RevokeReason string
}

/**
 * @description Store manages issued certificates and revocation state.
 * @note Key: serial number as a hex string (e.g., "1a2b3c...").
 * @note When persistencePath is set, state is loaded from and written to a JSON file.
 *
 * @typedef {Object} Store
 * @property {sync.RWMutex} mu - Read-Write mutex for concurrent access.
 * @property {map[string]*CertRecord} records - Map of serial number to CertRecord.
 */
type Store struct {
	mu sync.RWMutex
	// @note serial -> record
	records         map[string]*CertRecord
	persistencePath string
}

/**
 * @description NewStore initializes an empty volatile store.
 *
 * @note Tests can use this store when persistence is not relevant.
 * @note Runtime code should use NewPersistentStore so issued certificates and
 * @note revocation state survive service restarts.
 *
 * @function NewStore
 * @returns {*Store} A pointer to the newly created Store.
 */
func NewStore() *Store {
	return &Store{
		records: make(map[string]*CertRecord),
	}
}

/**
 * @description NewPersistentStore initializes a store backed by a JSON state file.
 *
 * @function NewPersistentStore
 * @param {string} path - The state file path.
 * @returns {(*Store, error)} The loaded Store or an error.
 */
func NewPersistentStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("persistence path is required")
	}

	s := &Store{
		records:         make(map[string]*CertRecord),
		persistencePath: path,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

/**
 * @description Save stores a new certificate record into the store.
 *
 * @function Save
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @param {*CertRecord} record - The certificate record to save.
 * @returns {error} An error if durable persistence fails.
 */
func (s *Store) Save(serial string, record *CertRecord) error {
	if record == nil {
		return fmt.Errorf("certificate record is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, hadPrev := s.records[serial]
	s.records[serial] = cloneCertRecord(record)
	if err := s.persistLocked(); err != nil {
		if hadPrev {
			s.records[serial] = prev
		} else {
			delete(s.records, serial)
		}
		return err
	}
	return nil
}

/**
 * @description SaveIssued stores a newly issued certificate only when the user has no active cert.
 *
 * @function SaveIssued
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @param {*CertRecord} record - The certificate record to save.
 * @param {time.Time} now - Current time used for active/expired evaluation.
 * @returns {error} ErrActiveCertificateExists if the user already has an active cert.
 */
func (s *Store) SaveIssued(serial string, record *CertRecord, now time.Time) error {
	if record == nil {
		return fmt.Errorf("certificate record is required")
	}
	if record.Cert == nil {
		return fmt.Errorf("certificate record cert is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasActiveCertificateForUserLocked(record.UserID, now, serial) {
		return ErrActiveCertificateExists
	}

	prev, hadPrev := s.records[serial]
	s.records[serial] = cloneCertRecord(record)
	if err := s.persistLocked(); err != nil {
		if hadPrev {
			s.records[serial] = prev
		} else {
			delete(s.records, serial)
		}
		return err
	}
	return nil
}

/**
 * @description Get retrieves a certificate record by its serial number.
 * @note Returns a defensive copy so callers cannot mutate the store after the lock is released.
 *
 * @function Get
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @returns {*CertRecord|nil} The certificate record, or nil if not found.
 */
func (s *Store) Get(serial string) *CertRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCertRecord(s.records[serial])
}

/**
 * @description Revoke marks a certificate as revoked.
 *
 * @function Revoke
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @param {string} reason - The reason for revoking the certificate.
 * @returns {(bool, error)} True if revoked; false if missing/already revoked; error if persistence fails.
 */
func (s *Store) Revoke(serial, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[serial]
	if !ok || rec.RevokedAt != nil {
		return false, nil
	}

	now := time.Now().UTC()
	oldRevokedAt := rec.RevokedAt
	oldReason := rec.RevokeReason
	rec.RevokedAt = &now
	rec.RevokeReason = reason
	if err := s.persistLocked(); err != nil {
		rec.RevokedAt = oldRevokedAt
		rec.RevokeReason = oldReason
		return false, err
	}
	return true, nil
}

type persistedStore struct {
	Version int                        `json:"version"`
	Records map[string]persistedRecord `json:"records"`
}

type persistedRecord struct {
	UserID       string     `json:"user_id"`
	CertPEM      string     `json:"cert_pem"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.persistencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read CA store state: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var state persistedStore
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode CA store state: %w", err)
	}
	for serial, persisted := range state.Records {
		cert, err := parseCertificatePEM(persisted.CertPEM)
		if err != nil {
			return fmt.Errorf("load certificate serial=%s: %w", serial, err)
		}
		actualSerial := hex.EncodeToString(cert.SerialNumber.Bytes())
		if actualSerial != serial {
			return fmt.Errorf("load certificate serial=%s: PEM contains serial=%s", serial, actualSerial)
		}

		var revokedAt *time.Time
		if persisted.RevokedAt != nil {
			t := persisted.RevokedAt.UTC()
			revokedAt = &t
		}
		s.records[serial] = &CertRecord{
			UserID:       persisted.UserID,
			Cert:         cert,
			CertPEM:      persisted.CertPEM,
			RevokedAt:    revokedAt,
			RevokeReason: persisted.RevokeReason,
		}
	}
	if err := s.validateNoDuplicateActiveCertificates(time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Store) validateNoDuplicateActiveCertificates(now time.Time) error {
	activeByUser := make(map[string]string)
	for serial, rec := range s.records {
		if !isActiveCertificate(rec, now) {
			continue
		}
		if existingSerial, ok := activeByUser[rec.UserID]; ok {
			return fmt.Errorf("%w: user=%s serials=%s,%s", ErrActiveCertificateExists, rec.UserID, existingSerial, serial)
		}
		activeByUser[rec.UserID] = serial
	}
	return nil
}

func (s *Store) hasActiveCertificateForUserLocked(userID string, now time.Time, excludeSerial string) bool {
	for serial, rec := range s.records {
		if serial == excludeSerial {
			continue
		}
		if rec.UserID == userID && isActiveCertificate(rec, now) {
			return true
		}
	}
	return false
}

func isActiveCertificate(rec *CertRecord, now time.Time) bool {
	return rec != nil &&
		rec.Cert != nil &&
		rec.UserID != "" &&
		rec.RevokedAt == nil &&
		now.Before(rec.Cert.NotAfter)
}

func (s *Store) persistLocked() error {
	if s.persistencePath == "" {
		return nil
	}

	state := persistedStore{
		Version: 1,
		Records: make(map[string]persistedRecord, len(s.records)),
	}
	for serial, rec := range s.records {
		state.Records[serial] = persistedRecord{
			UserID:       rec.UserID,
			CertPEM:      rec.CertPEM,
			RevokedAt:    cloneTime(rec.RevokedAt),
			RevokeReason: rec.RevokeReason,
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode CA store state: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(s.persistencePath), 0750); err != nil {
		return fmt.Errorf("create CA store directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.persistencePath), filepath.Base(s.persistencePath)+".tmp-*")
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

func cloneCertRecord(rec *CertRecord) *CertRecord {
	if rec == nil {
		return nil
	}
	return &CertRecord{
		UserID:       rec.UserID,
		Cert:         cloneCertificate(rec.Cert),
		CertPEM:      rec.CertPEM,
		RevokedAt:    cloneTime(rec.RevokedAt),
		RevokeReason: rec.RevokeReason,
	}
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copied := t.UTC()
	return &copied
}

func cloneCertificate(cert *x509.Certificate) *x509.Certificate {
	if cert == nil {
		return nil
	}
	if len(cert.Raw) > 0 {
		cloned, err := x509.ParseCertificate(cert.Raw)
		if err == nil {
			return cloned
		}
	}
	copied := *cert
	return &copied
}

func parseCertificatePEM(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM type: %s", block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, nil
}
