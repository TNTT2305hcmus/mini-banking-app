import { apiGet, apiPost } from "../api.service";

export type CertificateStatus = "active" | "revoked" | "expired" | "unknown";
export type CertificateType = "root_ca" | "intermediate_ca" | "service_tls" | "client";

export interface AdminCertificate {
  serial: string;
  cert_type: CertificateType | "";
  issuer_id: string;
  issuer_common_name: string;
  issuer_serial_number: string;
  owner_id: string;
  cn: string;
  email: string;
  chain_pem: string;
  chain_fingerprints: string[];
  is_ca: boolean;
  key_usage: string[];
  extended_key_usage: string[];
  fingerprint: string;
  status: CertificateStatus;
  not_before: number;
  not_after: number;
  issued_at: number;
  revoked_at: number | null;
  revocation_reason: string | null;
}

export interface CertificateListParams {
  status?: "all" | "active" | "revoked" | "expired";
  certType?: "all" | CertificateType;
  issuerId?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

export interface CertificateListResponse {
  items: AdminCertificate[];
  total: number;
  limit: number;
  offset: number;
}

export interface AdminLoginResponse {
  token: string;
  token_type: "Bearer";
  role: string;
  email: string;
  expires_in: number;
}

const TOKEN_KEY = "mini_banking_admin_ca_token";
const EMAIL_KEY = "mini_banking_admin_ca_email";

const envToken = () => (import.meta.env.VITE_ADMIN_CA_TOKEN as string | undefined) ?? "";

export const getStoredAdminToken = () => {
  if (typeof window === "undefined") {
    return envToken();
  }
  return localStorage.getItem(TOKEN_KEY) || envToken();
};

export const getStoredAdminEmail = () => {
  if (typeof window === "undefined") {
    return "";
  }
  return localStorage.getItem(EMAIL_KEY) || "";
};

export const storeAdminSession = (session: AdminLoginResponse) => {
  localStorage.setItem(TOKEN_KEY, session.token);
  localStorage.setItem(EMAIL_KEY, session.email);
};

export const clearAdminSession = () => {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(EMAIL_KEY);
};

const authHeaders = (token = getStoredAdminToken()) => ({
  Authorization: `Bearer ${token}`,
});

export const loginAdminCA = (email: string, password: string) =>
  apiPost<AdminLoginResponse>("/v1/admin-ca/auth", { email, password });

export const listAdminCertificates = (params: CertificateListParams = {}) => {
  const query = new URLSearchParams();
  query.set("limit", String(params.limit ?? 20));
  query.set("offset", String(params.offset ?? 0));

  if (params.status && params.status !== "all") {
    query.set("status", params.status);
  }
  if (params.certType && params.certType !== "all") {
    query.set("cert_type", params.certType);
  }
  if (params.issuerId?.trim()) {
    query.set("issuer_id", params.issuerId.trim());
  }

  const search = params.search?.trim() ?? "";
  if (search) {
    if (search.includes("@")) {
      query.set("email", search);
    } else {
      query.set("serial", search);
    }
  }

  return apiGet<CertificateListResponse>(
    `/v1/admin-ca/certificates?${query.toString()}`,
    authHeaders(),
  );
};

export const getAdminCertificateDetail = (serial: string) =>
  apiGet<AdminCertificate>(
    `/v1/admin-ca/certificates/${encodeURIComponent(serial)}`,
    authHeaders(),
  );

export const revokeAdminCertificate = (serial: string, reason: string) =>
  apiPost<AdminCertificate>(
    `/v1/admin-ca/certificates/${encodeURIComponent(serial)}/revoke`,
    { reason },
    authHeaders(),
  );

// ---- CA audit log (read-only) ----

// Khớp CHECK constraint của certificate_audit_log trong DB migration.
export type CaAuditAction =
  | "issuer_provisioned"
  | "issued"
  | "revoked"
  | "looked_up"
  | "verify_certificate"
  | "chain_verified";

export interface CaAuditEvent {
  serial_number: string;
  cert_type: string;
  issuer_id: string;
  action: CaAuditAction;
  performed_by: string;
  reason: string;
  performed_at: string; // ISO 8601
  metadata: Record<string, string>;
}

export interface CaAuditListParams {
  action?: "all" | CaAuditAction;
  serial?: string;
  performedBy?: string;
  limit?: number;
  offset?: number;
}

export interface CaAuditListResponse {
  items: CaAuditEvent[];
  total: number;
  limit: number;
  offset: number;
}

export const listAdminCaAudit = (params: CaAuditListParams = {}) => {
  const query = new URLSearchParams();
  query.set("limit", String(params.limit ?? 20));
  query.set("offset", String(params.offset ?? 0));
  if (params.action && params.action !== "all") {
    query.set("action", params.action);
  }
  if (params.serial?.trim()) {
    query.set("serial", params.serial.trim());
  }
  if (params.performedBy?.trim()) {
    query.set("performed_by", params.performedBy.trim());
  }
  return apiGet<CaAuditListResponse>(
    `/v1/admin-ca/audit?${query.toString()}`,
    authHeaders(),
  );
};
