package ca

import "strings"

// IdentityRole is assigned by a trusted server-side enrollment flow. It must
// never be copied from an untrusted CSR field or frontend request body.
type IdentityRole string

const (
	IdentityRoleUnknown   IdentityRole = ""
	IdentityRoleCustomer  IdentityRole = "customer"
	IdentityRoleBankAdmin IdentityRole = "bank_admin"
	IdentityRoleCAAdmin   IdentityRole = "ca_admin"
)

// NormalizeIdentityRole preserves compatibility with callers and persisted
// certificates created before role existed: missing/unknown means customer.
func NormalizeIdentityRole(role IdentityRole) (IdentityRole, bool) {
	normalized := IdentityRole(strings.ToLower(strings.TrimSpace(string(role))))
	switch normalized {
	case IdentityRoleUnknown, "unknown":
		return IdentityRoleCustomer, true
	case IdentityRoleCustomer, IdentityRoleBankAdmin, IdentityRoleCAAdmin:
		return normalized, true
	default:
		return normalized, false
	}
}

func (r IdentityRole) Valid() bool {
	_, ok := NormalizeIdentityRole(r)
	return ok
}
