// API client cho Security Operations (SOC) admin. Danh tính `security-admin`
// (JWT từ /v1/admin-sec/auth) sở hữu: KDC key-issuance audit + các view xuyên
// domain (timeline / verify / summary / export). Tách hẳn khỏi Admin CA/Bank.
import { apiGet, apiPost } from "../api.service";

const BASE_URL = ((import.meta.env.VITE_API_BASE_URL as string) ?? "").replace(/\/$/, "");

const TOKEN_KEY = "mini_banking_admin_soc_token";
const EMAIL_KEY = "mini_banking_admin_soc_email";

const envToken = () => (import.meta.env.VITE_ADMIN_SOC_TOKEN as string | undefined) ?? "";

export const getStoredSocToken = () =>
  typeof window === "undefined" ? envToken() : localStorage.getItem(TOKEN_KEY) || envToken();

export const getStoredSocEmail = () =>
  typeof window === "undefined" ? "" : localStorage.getItem(EMAIL_KEY) || "";

export interface SocLoginResponse {
  token: string;
  token_type: "Bearer";
  role: string;
  email: string;
  expires_in: number;
}

export const storeSocSession = (session: SocLoginResponse) => {
  localStorage.setItem(TOKEN_KEY, session.token);
  localStorage.setItem(EMAIL_KEY, session.email);
};

export const clearSocSession = () => {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(EMAIL_KEY);
};

const authHeaders = (token = getStoredSocToken()) => ({ Authorization: `Bearer ${token}` });

export const loginSecAdmin = (email: string, password: string) =>
  apiPost<SocLoginResponse>("/v1/admin-sec/auth", { email, password });

// ---- Shared semantic fields (attached by the gateway) ----
export interface AuditSemanticFields {
  category:
    | "key_issuance"
    | "cert_lifecycle"
    | "authentication"
    | "resource_access"
    | "admin_action";
  severity: "info" | "warning" | "critical";
  outcome: "success" | "denied";
  actor: { type: string; id: string; display: string };
  description: string;
}

// ---- KDC key-issuance audit ----
export type KdcAuditAction =
  | "as_ticket_issued"
  | "as_rejected"
  | "tgs_ticket_issued"
  | "tgs_rejected";

export interface KdcAuditEvent extends AuditSemanticFields {
  id: string;
  action: KdcAuditAction;
  client_id: string;
  cert_serial: string;
  scope: string;
  reason: string;
  request_id: string;
  ip: string;
  metadata: unknown;
  created_at: string; // ISO 8601
}

export interface KdcAuditListParams {
  action?: "all" | KdcAuditAction;
  clientId?: string;
  certSerial?: string;
  requestId?: string;
  limit?: number;
  offset?: number;
}

export interface KdcAuditListResponse {
  items: KdcAuditEvent[];
  total: number;
  limit: number;
  offset: number;
}

export const listKdcAudit = (params: KdcAuditListParams = {}) => {
  const query = new URLSearchParams();
  query.set("limit", String(params.limit ?? 20));
  query.set("offset", String(params.offset ?? 0));
  if (params.action && params.action !== "all") query.set("action", params.action);
  if (params.clientId?.trim()) query.set("client_id", params.clientId.trim());
  if (params.certSerial?.trim()) query.set("cert_serial", params.certSerial.trim());
  if (params.requestId?.trim()) query.set("request_id", params.requestId.trim());
  return apiGet<KdcAuditListResponse>(`/v1/admin-kdc/audit?${query.toString()}`, authHeaders());
};

// ---- Cross-service views ----
export interface AuditSummary {
  window_seconds: number;
  from: string;
  to: string;
  sources: Record<string, boolean>;
  total: number;
  capped: boolean;
  by_severity: { info: number; warning: number; critical: number };
  by_category: Record<string, number>;
  by_outcome: { success: number; denied: number };
  security_events: number;
  top_reasons: { reason: string; count: number }[];
  anomalies: { actor: string; denied_count: number }[];
}

export const getAuditSummary = (window = "24h") =>
  apiGet<AuditSummary>(`/v1/admin/audit/summary?window=${encodeURIComponent(window)}`, authHeaders());

export interface AuditChainStatus {
  checked: boolean;
  ok?: boolean;
  verified?: number;
  broken_seq?: number;
  broken_id?: string;
  detail?: string;
}

export interface AuditVerifyResult {
  ok: boolean;
  sources: Record<string, AuditChainStatus>;
}

export const verifyAuditChains = () =>
  apiGet<AuditVerifyResult>("/v1/admin/audit/verify", authHeaders());

export interface TimelineItem extends AuditSemanticFields {
  source: "ca" | "kdc" | "bank";
  action: string;
  target: string;
  reason: string;
  request_id: string;
  timestamp: string;
  metadata: unknown;
}

export interface AuditTimelineResult {
  request_id: string;
  count: number;
  sources: Record<string, { included: boolean; reason?: string }>;
  items: TimelineItem[];
}

export const getAuditTimeline = (requestId: string) =>
  apiGet<AuditTimelineResult>(
    `/v1/admin/audit/timeline?request_id=${encodeURIComponent(requestId)}`,
    authHeaders(),
  );

// Export needs the auth header, so fetch as a blob and trigger a download
// instead of opening a bare URL (which could not carry the Bearer token).
export const downloadAuditExport = async (opts: {
  source?: "all" | "ca" | "kdc" | "bank";
  format?: "csv" | "json";
  from?: string;
  to?: string;
}) => {
  const query = new URLSearchParams();
  query.set("source", opts.source ?? "all");
  query.set("format", opts.format ?? "csv");
  if (opts.from) query.set("from", opts.from);
  if (opts.to) query.set("to", opts.to);

  const res = await fetch(`${BASE_URL}/v1/admin/audit/export?${query.toString()}`, {
    headers: { "X-Request-ID": crypto.randomUUID(), ...authHeaders() },
  });
  if (!res.ok) throw new Error(`Export failed (HTTP ${res.status})`);

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit-${opts.source ?? "all"}.${opts.format ?? "csv"}`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
};
