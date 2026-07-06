package kdc

// roleAllowsScope is an authorization boundary applied after CA certificate
// verification. The existing service/scope allowlist still runs afterwards.
func roleAllowsScope(role IdentityRole, scope string) bool {
	switch role {
	case IdentityRoleBankAdmin:
		return scope == "bank-admin:read"
	case IdentityRoleCustomer:
		switch scope {
		case "transfer:create", "balance:read", "history:read":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
