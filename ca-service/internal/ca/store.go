package ca

/**
 * @title CA Service - Certificate Store
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Managing a thread-safe, in-memory storage for issued certificates and tracking their revocation status.
 */

import (
	"crypto/x509"
	"sync"
	"time"
)

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
 * @description Store is an in-memory store for all issued certificates.
 * @note Key: serial number as a hex string (e.g., "1a2b3c...")
 * @note Production: Replace this with a Postgres query to the user_certificates table.
 * @note This store is sufficient for the project scope because it is thread-safe thanks to RWMutex.
 *
 * @typedef {Object} Store
 * @property {sync.RWMutex} mu - Read-Write mutex for concurrent access.
 * @property {map[string]*CertRecord} records - Map of serial number to CertRecord.
 */
type Store struct {
	mu sync.RWMutex
	// @note serial -> record
	records map[string]*CertRecord
}

/**
 * @description NewStore initializes an empty store.
 *
 * @note Important limitation: The store MUST NOT be reloaded from PEM files on disk
 * @note after a restart because standard X.509 .pem files do not store revocation state.
 * @note Reloading would cause revoked certificates to reappear with a VALID status —
 * @note which is a severe security vulnerability.
 *
 * @note Consequence: After the CA Service restarts, previously issued certificates will
 * @note return codes.NotFound when KDC/Bank calls GetCertificate or CheckRevocation.
 *
 * @note Production fix: Replace this store with a database query to the user_certificates table
 * @note This table has a status column ('active'/'revoked'/'expired') that persists across restarts.
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
 * @description Save stores a new certificate record into the store.
 *
 * @function Save
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @param {*CertRecord} record - The certificate record to save.
 */
func (s *Store) Save(serial string, record *CertRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[serial] = record
}

/**
 * @description Get retrieves a certificate record by its serial number.
 *
 * @function Get
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @returns {*CertRecord|nil} The certificate record, or nil if not found.
 */
func (s *Store) Get(serial string) *CertRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.records[serial]
}

/**
 * @description Revoke marks a certificate as revoked.
 *
 * @function Revoke
 * @memberof Store
 * @param {string} serial - The hex string representation of the certificate's serial number.
 * @param {string} reason - The reason for revoking the certificate.
 * @returns {bool} True if successfully revoked, false if the serial does not exist or was already revoked.
 */
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
