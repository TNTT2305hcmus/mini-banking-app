package ca

import "testing"

func TestNormalizeIdentityRole(t *testing.T) {
	tests := []struct {
		name  string
		input IdentityRole
		want  IdentityRole
		ok    bool
	}{
		{name: "missing defaults to customer", input: IdentityRoleUnknown, want: IdentityRoleCustomer, ok: true},
		{name: "legacy unknown defaults to customer", input: "unknown", want: IdentityRoleCustomer, ok: true},
		{name: "customer remains customer", input: IdentityRoleCustomer, want: IdentityRoleCustomer, ok: true},
		{name: "bank admin remains bank admin", input: IdentityRoleBankAdmin, want: IdentityRoleBankAdmin, ok: true},
		{name: "normalizes case and spaces", input: "  BANK_ADMIN ", want: IdentityRoleBankAdmin, ok: true},
		{name: "rejects unsupported role", input: "pki_admin", want: "pki_admin", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeIdentityRole(tt.input)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NormalizeIdentityRole(%q) = (%q, %t), want (%q, %t)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}
