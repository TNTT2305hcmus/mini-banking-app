// Read-only semantic enrichment for audit events.
//
// Every audit source (CA, KDC, Bank) stores raw action strings. This module
// derives the canonical meaning — category, severity, outcome, a normalised
// actor and a human-readable description — purely from (source, action, actor,
// reason). It never touches the database or how events are written, so it can
// be layered on any audit read endpoint.

export type AuditSource = "ca" | "kdc" | "bank";
export type AuditCategory =
  | "key_issuance"
  | "cert_lifecycle"
  | "authentication"
  | "resource_access"
  | "admin_action";
export type AuditSeverity = "info" | "warning" | "critical";
export type AuditOutcome = "success" | "denied";
export type ActorType = "user" | "admin" | "ra" | "service" | "system";

export interface RawAuditFields {
  source: AuditSource;
  action: string;
  actor?: string; // performed_by (CA) / client_id (KDC) / user_id (Bank)
  reason?: string;
  target?: string; // serial / cert_serial / transaction id
}

export interface AuditActor {
  type: ActorType;
  id: string;
  display: string;
}

export interface AuditSemantics {
  category: AuditCategory;
  severity: AuditSeverity;
  outcome: AuditOutcome;
  actor: AuditActor;
  description: string;
}

// Category by action; falls back to the source's primary domain when unknown.
const CATEGORY_BY_ACTION: Record<string, AuditCategory> = {
  // CA — certificate lifecycle
  issued: "cert_lifecycle",
  verify_certificate: "cert_lifecycle",
  issuer_provisioned: "cert_lifecycle",
  chain_verified: "cert_lifecycle",
  ra_registration_approved: "cert_lifecycle",
  ra_registration_rejected: "cert_lifecycle",
  // CA — admin management actions
  looked_up: "admin_action",
  revoked: "admin_action",
  // Authentication (RA vetting + admin logins)
  ra_otp_requested: "authentication",
  ra_otp_verified: "authentication",
  ra_otp_failed: "authentication",
  admin_ca_login_success: "authentication",
  admin_ca_login_failed: "authentication",
  // KDC — key issuance
  as_ticket_issued: "key_issuance",
  as_rejected: "key_issuance",
  tgs_ticket_issued: "key_issuance",
  tgs_rejected: "key_issuance",
  // Bank — resource access
  transfer_completed: "resource_access",
  transfer_rejected: "resource_access",
  replay_detected: "resource_access",
  invalid_signature: "resource_access",
  certificate_rejected: "resource_access",
  forbidden_ownership: "resource_access",
  insufficient_funds: "resource_access",
};

const FALLBACK_CATEGORY: Record<AuditSource, AuditCategory> = {
  ca: "cert_lifecycle",
  kdc: "key_issuance",
  bank: "resource_access",
};

// High-impact security events, regardless of whether they succeeded or not.
const CRITICAL = new Set([
  "revoked",
  "certificate_rejected",
  "replay_detected",
  "invalid_signature",
]);

// A denied outcome: the operation was refused. `revoked` succeeded (it is just
// high-impact), so it is not counted as denied.
function isDenied(action: string): boolean {
  if (action === "revoked") return false;
  if (action.endsWith("_rejected") || action.endsWith("_failed")) return true;
  return [
    "forbidden_ownership",
    "insufficient_funds",
    "replay_detected",
    "invalid_signature",
    "certificate_rejected",
  ].includes(action);
}

function severityOf(action: string): AuditSeverity {
  if (CRITICAL.has(action)) return "critical";
  if (isDenied(action)) return "warning";
  return "info";
}

function shorten(value: string, keep = 8): string {
  return value.length > keep * 2 + 1
    ? `${value.slice(0, keep)}…${value.slice(-keep)}`
    : value;
}

function actorOf(source: AuditSource, actor: string): AuditActor {
  const a = (actor ?? "").trim();
  if (a === "") {
    if (source === "kdc") return { type: "user", id: "", display: "unknown client" };
    if (source === "bank") return { type: "user", id: "", display: "unknown user" };
    return { type: "system", id: "", display: "system" };
  }
  const idx = a.indexOf(":");
  if (idx >= 0) {
    const prefix = a.slice(0, idx);
    const suffix = a.slice(idx + 1);
    if (prefix === "ra") return { type: "ra", id: a, display: `RA (${suffix || "gateway"})` };
    if (prefix === "admin-ca" || prefix === "admin" || prefix === "bank_admin") {
      return { type: "admin", id: a, display: suffix || a };
    }
    if (prefix === "system" || prefix === "service" || prefix === "legacy") {
      return { type: "service", id: a, display: suffix || a };
    }
  }
  // No known prefix → a raw identity (UUID owner_id / client_id / user_id).
  return { type: "user", id: a, display: shorten(a) };
}

function describe(raw: RawAuditFields, actor: AuditActor): string {
  const who = actor.display;
  const target = raw.target ? ` ${shorten(raw.target)}` : "";
  const reason = raw.reason ? ` (${raw.reason})` : "";
  switch (raw.action) {
    case "issued":
      return `${who} issued certificate${target}`;
    case "revoked":
      return `${who} revoked certificate${target}${reason}`;
    case "looked_up":
      return `${who} viewed certificate${target}`;
    case "verify_certificate":
      return `certificate${target} was verified`;
    case "chain_verified":
      return `certificate chain${target} was verified`;
    case "issuer_provisioned":
      return `issuer${target} was provisioned`;
    case "ra_otp_requested":
      return `OTP challenge issued for registration`;
    case "ra_otp_verified":
      return `email ownership verified for registration`;
    case "ra_otp_failed":
      return `OTP verification failed${reason}`;
    case "ra_registration_approved":
      return `registration approved, certificate${target} issued`;
    case "ra_registration_rejected":
      return `registration rejected${reason}`;
    case "admin_ca_login_success":
      return `${who} signed in to Admin CA`;
    case "admin_ca_login_failed":
      return `Admin CA sign-in failed${reason}`;
    case "as_ticket_issued":
      return `${who} was granted a TGT (AS exchange)`;
    case "as_rejected":
      return `AS exchange denied${reason}`;
    case "tgs_ticket_issued":
      return `${who} was granted a service ticket${target}`;
    case "tgs_rejected":
      return `TGS exchange denied${reason}`;
    case "transfer_completed":
      return `${who} completed a transfer${target}`;
    case "transfer_rejected":
      return `transfer rejected${reason}`;
    case "replay_detected":
      return `replay attack detected${reason}`;
    case "invalid_signature":
      return `invalid payload signature${reason}`;
    case "certificate_rejected":
      return `certificate rejected${reason}`;
    case "forbidden_ownership":
      return `ownership check failed${reason}`;
    case "insufficient_funds":
      return `transfer failed: insufficient funds`;
    default:
      return `${raw.action}${reason}`;
  }
}

export function enrichAudit(raw: RawAuditFields): AuditSemantics {
  const action = (raw.action ?? "").toLowerCase();
  const category = CATEGORY_BY_ACTION[action] ?? FALLBACK_CATEGORY[raw.source];
  const actor = actorOf(raw.source, raw.actor ?? "");
  return {
    category,
    severity: severityOf(action),
    outcome: isDenied(action) ? "denied" : "success",
    actor,
    description: describe({ ...raw, action }, actor),
  };
}
