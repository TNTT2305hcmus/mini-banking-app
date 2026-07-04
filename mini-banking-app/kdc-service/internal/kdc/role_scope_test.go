package kdc

import (
	"testing"

	capb "mini_banking/pkg/pb/ca"
)

func TestRoleAllowsScope(t *testing.T) {
	tests := []struct {
		name  string
		role  IdentityRole
		scope string
		want  bool
	}{
		{name: "customer transfer", role: IdentityRoleCustomer, scope: "transfer:create", want: true},
		{name: "customer balance", role: IdentityRoleCustomer, scope: "balance:read", want: true},
		{name: "customer history", role: IdentityRoleCustomer, scope: "history:read", want: true},
		{name: "customer admin denied", role: IdentityRoleCustomer, scope: "bank-admin:read", want: false},
		{name: "admin read", role: IdentityRoleBankAdmin, scope: "bank-admin:read", want: true},
		{name: "admin customer scope denied", role: IdentityRoleBankAdmin, scope: "balance:read", want: false},
		{name: "unknown denied", role: "", scope: "bank-admin:read", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleAllowsScope(tt.role, tt.scope); got != tt.want {
				t.Fatalf("roleAllowsScope(%q, %q) = %t, want %t", tt.role, tt.scope, got, tt.want)
			}
		})
	}
}

func TestMapCAIdentityRoleDefaultsToCustomer(t *testing.T) {
	if got := mapCAIdentityRole(capb.IdentityRole_IDENTITY_ROLE_UNKNOWN); got != IdentityRoleCustomer {
		t.Fatalf("UNKNOWN mapped to %q, want customer", got)
	}
	if got := mapCAIdentityRole(capb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN); got != IdentityRoleBankAdmin {
		t.Fatalf("BANK_ADMIN mapped to %q, want bank_admin", got)
	}
}
