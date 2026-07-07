// Shared fan-out fetch + normalisation for the aggregate audit endpoints
// (summary, export). Each source's paginated list RPC is walked within a time
// window up to a cap, and every event is flattened to one common shape so the
// callers can enrich/aggregate uniformly. Read-only.
import { listCaAuditEvents } from "../services/ca.service";
import { listKdcAuditEvents } from "../services/kdc.service";
import { listBankAdminAuditEvents } from "../services/admin-bank.service";
import { bankAuditActionToJSON } from "../proto/bank";

export interface NormalizedEvent {
  source: "ca" | "kdc" | "bank";
  action: string;
  actor: string;
  target: string;
  reason: string;
  request_id: string;
  ip: string;
  scope: string;
  timestamp: string; // ISO 8601
  ts: number; // epoch seconds
  metadata: unknown;
}

export interface WindowFilter {
  fromUnix: number; // 0 = no lower bound
  toUnix: number; // 0 = no upper bound
}

const PAGE = 100;

const safeJson = (raw: string): unknown => {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
};

const metaRequestId = (metadata: Record<string, string> | undefined): string =>
  (metadata && metadata["request_id"]) || "";

export async function fetchCaWindow(
  filter: WindowFilter,
  cap: number,
  requestId?: string,
): Promise<NormalizedEvent[]> {
  const out: NormalizedEvent[] = [];
  for (let offset = 0; out.length < cap; offset += PAGE) {
    const res = await listCaAuditEvents(
      {
        action: "",
        serialNumber: "",
        performedByFilter: "",
        performedBy: "",
        requestId: "",
        fromUnix: filter.fromUnix,
        toUnix: filter.toUnix,
        limit: PAGE,
        offset,
      },
      requestId,
    );
    for (const e of res.events) {
      out.push({
        source: "ca",
        action: e.action,
        actor: e.performedBy,
        target: e.serialNumber,
        reason: e.reason,
        request_id: metaRequestId(e.metadata),
        ip: "",
        scope: "",
        timestamp: new Date(e.performedAtUnix * 1000).toISOString(),
        ts: e.performedAtUnix,
        metadata: e.metadata,
      });
    }
    if (res.events.length < PAGE) break;
  }
  return out.slice(0, cap);
}

export async function fetchKdcWindow(
  filter: WindowFilter,
  cap: number,
  requestId?: string,
): Promise<NormalizedEvent[]> {
  const out: NormalizedEvent[] = [];
  for (let offset = 0; out.length < cap; offset += PAGE) {
    const res = await listKdcAuditEvents(
      {
        action: "",
        clientId: "",
        certSerial: "",
        requestId: "",
        fromUnix: filter.fromUnix,
        toUnix: filter.toUnix,
        limit: PAGE,
        offset,
      },
      requestId,
    );
    for (const e of res.events) {
      out.push({
        source: "kdc",
        action: e.action,
        actor: e.clientId,
        target: e.certSerial,
        reason: e.reason,
        request_id: e.requestId,
        ip: e.ip,
        scope: e.scope,
        timestamp: new Date(e.createdAtUnix * 1000).toISOString(),
        ts: e.createdAtUnix,
        metadata: safeJson(e.metadataJson),
      });
    }
    if (res.events.length < PAGE) break;
  }
  return out.slice(0, cap);
}

export async function fetchBankWindow(
  filter: WindowFilter,
  cap: number,
  adminSessionToken: string,
): Promise<NormalizedEvent[]> {
  const out: NormalizedEvent[] = [];
  for (let offset = 0; out.length < cap; offset += PAGE) {
    const res = await listBankAdminAuditEvents({
      action: 0, // BANK_AUDIT_ACTION_UNKNOWN = all
      userId: "",
      certSerial: "",
      requestId: "",
      fromUnix: filter.fromUnix,
      toUnix: filter.toUnix,
      limit: PAGE,
      offset,
      adminSessionToken,
    });
    for (const e of res.events) {
      out.push({
        source: "bank",
        action: bankAuditActionToJSON(e.action).toLowerCase(),
        actor: e.userId,
        target: e.transactionId || e.accountId || e.certSerial,
        reason: e.reason,
        request_id: e.requestId,
        ip: "",
        scope: "",
        timestamp: new Date(e.createdAtUnix * 1000).toISOString(),
        ts: e.createdAtUnix,
        metadata: safeJson(e.metadataJson),
      });
    }
    if (res.events.length < PAGE) break;
  }
  return out.slice(0, cap);
}

// Minimal cookie reader shared by the aggregate endpoints (bank opt-in).
export const BANK_SESSION_COOKIE = "bank_admin_session";
export const readCookie = (header: string | undefined, name: string): string => {
  if (!header) return "";
  for (const part of header.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return decodeURIComponent(v.join("="));
  }
  return "";
};

// Parses a window string like "24h" / "7d" / "90m" into seconds. Defaults 24h.
export const parseWindowSeconds = (value: string | undefined): number => {
  if (!value) return 24 * 3600;
  const m = /^(\d+)([smhd])$/.exec(value.trim());
  if (!m) return 24 * 3600;
  const n = parseInt(m[1], 10);
  const unit = { s: 1, m: 60, h: 3600, d: 86400 }[m[2]] ?? 3600;
  return n * unit;
};
