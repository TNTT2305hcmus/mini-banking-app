package kdc

// IdentityRole is the CA-authoritative role used to authorize a requested TGS
// scope. It is not persisted in the TGT or service ticket.
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
