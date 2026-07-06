package bank

import "time"

// IdentityRole comes from CA VerifyCertificate when Bank opens an Admin
// session. It is not trusted from the frontend or copied from Ticket_v.
type IdentityRole string

const (
	IdentityRoleCustomer  IdentityRole = "customer"
	IdentityRoleBankAdmin IdentityRole = "bank_admin"
)

func (r IdentityRole) Valid() bool {
	switch r {
	case IdentityRoleCustomer, IdentityRoleBankAdmin:
		return true
	default:
		return false
	}
}

// AdminSession is persisted in Bank Redis under the SHA-256 hash of an opaque
// session token. The raw token is held only by the Gateway's HttpOnly cookie.
type AdminSession struct {
	AdminID   string       `json:"admin_id"`
	CertSN    string       `json:"cert_sn"`
	Role      IdentityRole `json:"role"`
	ExpiresAt int64        `json:"expires_at"`
}

const AdminSessionKeyPrefix = "bank:admin:session:"

type AdminOverview struct {
	TotalUsers            int64
	ActiveUsers           int64
	TotalAccounts         int64
	TotalBalance          int64
	TotalTransactions     int64
	CompletedTransactions int64
	FailedTransactions    int64
	AuditEvents24h        int64
}

type AdminUser struct {
	UserID       string
	Email        string
	FullName     string
	Status       string
	AccountCount int64
	TotalBalance int64
	CreatedAt    time.Time
}

type AdminAccount struct {
	AccountID     string
	AccountNumber string
	Balance       int64
	Currency      string
	Status        string
	CreatedAt     time.Time
}

type AdminTransaction struct {
	TransactionID     string
	FromAccountNumber string
	ToAccountNumber   string
	Amount            int64
	Currency          string
	Status            string
	Description       string
	CertSerial        string
	CurrentHash       string
	CreatedAt         time.Time
}

type AdminAuditEvent struct {
	EventID       string
	Action        string
	UserID        string
	AccountID     string
	TransactionID string
	CertSerial    string
	RequestID     string
	Reason        string
	MetadataJSON  string
	CreatedAt     time.Time
}

type AdminUsersFilter struct {
	Email  string
	Status string
	Limit  int
	Offset int
}

type AdminTransactionsFilter struct {
	AccountID string
	Status    string
	FromUnix  int64
	ToUnix    int64
	Limit     int
	Offset    int
}

type AdminAuditFilter struct {
	Action     string
	UserID     string
	CertSerial string
	RequestID  string
	FromUnix   int64
	ToUnix     int64
	Limit      int
	Offset     int
}

type AdminUsersResult struct {
	Users  []AdminUser
	Total  int64
	Limit  int
	Offset int
}

type AdminTransactionsResult struct {
	Transactions []AdminTransaction
	Total        int64
	Limit        int
	Offset       int
}

type AdminAuditResult struct {
	Events []AdminAuditEvent
	Total  int64
	Limit  int
	Offset int
}
