// Server-owned identity contract for Bank Admin provisioning and activation.
// Values stored here are persisted as JSON in Gateway Redis in later steps.

export const IDENTITY_ROLES = {
  CUSTOMER: "customer",
  BANK_ADMIN: "bank_admin",
  CA_ADMIN: "ca_admin",
} as const;

export type IdentityRole =
  (typeof IDENTITY_ROLES)[keyof typeof IDENTITY_ROLES];

export const BANK_ADMIN_STATUSES = {
  PENDING_ACTIVATION: "pending_activation",
  ACTIVE: "active",
} as const;

export type BankAdminStatus =
  (typeof BANK_ADMIN_STATUSES)[keyof typeof BANK_ADMIN_STATUSES];

export type AdminStatus = BankAdminStatus;

export interface AdminIdentity {
  admin_id: string;
  email: string;
  full_name: string;
  role: Exclude<IdentityRole, typeof IDENTITY_ROLES.CUSTOMER>;
  status: AdminStatus;
  activation_token_hash: string;
  activation_expires_at: number;
  cert_serial?: string;
  created_at: number;
  activated_at?: number;
}

export type BankAdminIdentity = AdminIdentity & {
  role: typeof IDENTITY_ROLES.BANK_ADMIN;
};

export type CaAdminIdentity = AdminIdentity & {
  role: typeof IDENTITY_ROLES.CA_ADMIN;
};

export const ADMIN_BANK_IDENTITY_KEY_PREFIX = "admin:bank:identity:";
export const ADMIN_BANK_ACTIVATION_KEY_PREFIX = "admin:bank:activation:";
export const ADMIN_CA_IDENTITY_KEY_PREFIX = "admin:ca:identity:";
export const ADMIN_CA_ACTIVATION_KEY_PREFIX = "admin:ca:activation:";

