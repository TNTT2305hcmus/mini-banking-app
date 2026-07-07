// Unified audit timeline: GET /v1/admin/audit/timeline?request_id=<id>
//
// Reconstructs a single request/session across services by correlating on the
// trace id. It fans out to the per-service audit reads (CA, KDC, and — when a
// bank admin session is present — Bank), merges the events and sorts them by
// time. Aggregation is best-effort: a source that errors is reported in
// `sources` but does not fail the whole timeline.
import { Request, Response, NextFunction } from "express";
import { z } from "zod";
import { listCaAuditEvents, verifyCaAuditChain } from "../services/ca.service";
import { listKdcAuditEvents, verifyKdcAuditChain } from "../services/kdc.service";
import {
  listBankAdminAuditEvents,
  verifyBankAuditChain,
} from "../services/admin-bank.service";
import { bankAuditActionToJSON } from "../proto/bank";
import { enrichAudit } from "../lib/audit-semantics";

const TimelineQuerySchema = z.object({
  request_id: z.string().min(1, "request_id is required"),
  limit: z.coerce.number().int().min(1).max(200).default(100),
});

// One row of the merged timeline. Semantic enrichment (category/severity) is
// layered on separately; here we only normalise the shared fields + trace id.
interface TimelineItem {
  source: "ca" | "kdc" | "bank";
  action: string;
  actor: string;
  target: string;
  reason: string;
  request_id: string;
  timestamp: string; // ISO 8601
  ts: number; // epoch seconds, for sorting
  metadata: unknown;
}

const safeJson = (raw: string): unknown => {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
};

// Minimal cookie reader so the timeline can opt-in to bank events without
// requiring the bank session (a plain CA admin still gets CA+KDC).
const BANK_SESSION_COOKIE = "bank_admin_session";
const readCookie = (header: string | undefined, name: string): string => {
  if (!header) return "";
  for (const part of header.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return decodeURIComponent(v.join("="));
  }
  return "";
};

export const handleAuditTimeline = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"] as string;

  const parsed = TimelineQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: parsed.error.issues[0].message,
      request_id: requestId,
    });
  }
  const traceId = parsed.data.request_id;
  const limit = parsed.data.limit;

  const items: TimelineItem[] = [];
  const sources: Record<string, { included: boolean; reason?: string }> = {};

  // --- CA (cert lifecycle + RA/auth events) ---
  try {
    const ca = await listCaAuditEvents(
      {
        action: "",
        serialNumber: "",
        performedByFilter: "",
        requestId: traceId,
        fromUnix: 0,
        toUnix: 0,
        limit,
        offset: 0,
        performedBy: "",
      },
      requestId,
    );
    for (const e of ca.events) {
      items.push({
        source: "ca",
        action: e.action,
        actor: e.performedBy,
        target: e.serialNumber,
        reason: e.reason,
        request_id: traceId,
        timestamp: new Date(e.performedAtUnix * 1000).toISOString(),
        ts: e.performedAtUnix,
        metadata: e.metadata,
      });
    }
    sources.ca = { included: true };
  } catch (err) {
    sources.ca = { included: false, reason: (err as Error)?.message ?? "ca_error" };
  }

  // --- KDC (key issuance: AS/TGS) ---
  try {
    const kdc = await listKdcAuditEvents(
      {
        action: "",
        clientId: "",
        certSerial: "",
        requestId: traceId,
        fromUnix: 0,
        toUnix: 0,
        limit,
        offset: 0,
      },
      requestId,
    );
    for (const e of kdc.events) {
      items.push({
        source: "kdc",
        action: e.action,
        actor: e.clientId,
        target: e.certSerial,
        reason: e.reason,
        request_id: traceId,
        timestamp: new Date(e.createdAtUnix * 1000).toISOString(),
        ts: e.createdAtUnix,
        metadata: { ...safeJsonObject(e.metadataJson), scope: e.scope, ip: e.ip },
      });
    }
    sources.kdc = { included: true };
  } catch (err) {
    sources.kdc = { included: false, reason: (err as Error)?.message ?? "kdc_error" };
  }

  // --- Bank (resource access) — only when a bank admin session is present ---
  const bankToken = readCookie(req.headers.cookie, BANK_SESSION_COOKIE);
  if (bankToken) {
    try {
      const bank = await listBankAdminAuditEvents({
        action: 0, // BANK_AUDIT_ACTION_UNKNOWN = all actions
        userId: "",
        certSerial: "",
        requestId: traceId,
        fromUnix: 0,
        toUnix: 0,
        limit,
        offset: 0,
        adminSessionToken: bankToken,
      });
      for (const e of bank.events) {
        items.push({
          source: "bank",
          action: bankAuditActionToJSON(e.action).toLowerCase(),
          actor: e.userId,
          target: e.transactionId || e.accountId || e.certSerial,
          reason: e.reason,
          request_id: traceId,
          timestamp: new Date(e.createdAtUnix * 1000).toISOString(),
          ts: e.createdAtUnix,
          metadata: safeJson(e.metadataJson),
        });
      }
      sources.bank = { included: true };
    } catch (err) {
      sources.bank = { included: false, reason: (err as Error)?.message ?? "bank_error" };
    }
  } else {
    sources.bank = { included: false, reason: "bank_admin_session_required" };
  }

  items.sort((a, b) => a.ts - b.ts);

  // Enrich every row with canonical category/severity/actor/outcome + a
  // human-readable description so the client renders a meaningful timeline.
  const enriched = items.map(({ ts, ...rest }) => {
    const semantics = enrichAudit({
      source: rest.source,
      action: rest.action,
      actor: rest.actor,
      reason: rest.reason,
      target: rest.target,
    });
    return { ...rest, ...semantics };
  });

  return res.json({
    success: true,
    data: {
      request_id: traceId,
      count: enriched.length,
      sources,
      items: enriched,
    },
    request_id: requestId,
    timestamp: new Date().toISOString(),
  });
};

function safeJsonObject(raw: string): Record<string, unknown> {
  const v = safeJson(raw);
  return v && typeof v === "object" ? (v as Record<string, unknown>) : {};
}

// GET /v1/admin/audit/verify — replay each audit hash chain and report tampering.
// CA + KDC are always checked (admin-ca scope); Bank is checked when a bank
// admin session cookie is present.
export const handleAuditVerify = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"] as string;
  const results: Record<
    string,
    { checked: boolean; ok?: boolean; verified?: number; broken_seq?: number; broken_id?: string; detail?: string }
  > = {};

  try {
    const ca = await verifyCaAuditChain(requestId);
    results.ca = {
      checked: true,
      ok: ca.ok,
      verified: ca.checked,
      broken_seq: ca.ok ? undefined : ca.brokenSeq,
      broken_id: ca.ok ? undefined : ca.brokenId,
      detail: ca.ok ? undefined : ca.detail,
    };
  } catch (err) {
    results.ca = { checked: false, detail: (err as Error)?.message ?? "ca_error" };
  }

  try {
    const kdc = await verifyKdcAuditChain(requestId);
    results.kdc = {
      checked: true,
      ok: kdc.ok,
      verified: kdc.checked,
      broken_seq: kdc.ok ? undefined : kdc.brokenSeq,
      broken_id: kdc.ok ? undefined : kdc.brokenId,
      detail: kdc.ok ? undefined : kdc.detail,
    };
  } catch (err) {
    results.kdc = { checked: false, detail: (err as Error)?.message ?? "kdc_error" };
  }

  const bankToken = readCookie(req.headers.cookie, BANK_SESSION_COOKIE);
  if (bankToken) {
    try {
      const bank = await verifyBankAuditChain({ adminSessionToken: bankToken });
      results.bank = {
        checked: true,
        ok: bank.ok,
        verified: bank.checked,
        broken_seq: bank.ok ? undefined : bank.brokenSeq,
        broken_id: bank.ok ? undefined : bank.brokenId,
        detail: bank.ok ? undefined : bank.detail,
      };
    } catch (err) {
      results.bank = { checked: false, detail: (err as Error)?.message ?? "bank_error" };
    }
  } else {
    results.bank = { checked: false, detail: "bank_admin_session_required" };
  }

  const allOk = Object.values(results).every((r) => !r.checked || r.ok !== false);

  return res.json({
    success: true,
    data: { ok: allOk, sources: results },
    request_id: requestId,
    timestamp: new Date().toISOString(),
  });
};
