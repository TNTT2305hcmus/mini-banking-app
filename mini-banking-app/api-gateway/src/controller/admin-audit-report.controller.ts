// Aggregate audit reporting: security summary + export.
//
//   GET /v1/admin/audit/summary?window=24h
//   GET /v1/admin/audit/export?source=all&from&to&format=csv|json
//
// Both fan out to the per-service audit reads over a time window, enrich each
// event with canonical semantics, and either aggregate (summary) or serialise
// (export). CA + KDC are always included; Bank folds in when a bank admin
// session cookie is present. Read-only.
import { Request, Response, NextFunction } from "express";
import { z } from "zod";
import {
  BANK_SESSION_COOKIE,
  fetchBankWindow,
  fetchCaWindow,
  fetchKdcWindow,
  NormalizedEvent,
  parseWindowSeconds,
  readCookie,
} from "../lib/audit-fetch";
import {
  AuditCategory,
  AuditSeverity,
  enrichAudit,
} from "../lib/audit-semantics";

const SUMMARY_CAP = 2000;
const EXPORT_CAP = 5000;
// Flag an actor with at least this many denied events in the window.
const ANOMALY_THRESHOLD = 5;

const SummaryQuerySchema = z.object({
  window: z.string().optional(),
});

const isoToUnix = z
  .string()
  .optional()
  .transform((value, ctx) => {
    if (value === undefined || value === "") return 0;
    const millis = Date.parse(value);
    if (Number.isNaN(millis)) {
      ctx.addIssue({ code: "custom", message: "must be an ISO 8601 datetime" });
      return z.NEVER;
    }
    return Math.floor(millis / 1000);
  });

const ExportQuerySchema = z.object({
  source: z.enum(["all", "ca", "kdc", "bank"]).default("all"),
  format: z.enum(["json", "csv"]).default("json"),
  from: isoToUnix,
  to: isoToUnix,
});

interface EnrichedEvent extends NormalizedEvent {
  category: AuditCategory;
  severity: AuditSeverity;
  outcome: string;
  actor_type: string;
  actor_display: string;
  description: string;
}

const enrich = (e: NormalizedEvent): EnrichedEvent => {
  const s = enrichAudit({
    source: e.source,
    action: e.action,
    actor: e.actor,
    reason: e.reason,
    target: e.target,
  });
  return {
    ...e,
    category: s.category,
    severity: s.severity,
    outcome: s.outcome,
    actor_type: s.actor.type,
    actor_display: s.actor.display,
    description: s.description,
  };
};

// Collects events from the authorised sources within [fromUnix, toUnix).
async function collect(
  req: Request,
  fromUnix: number,
  toUnix: number,
  cap: number,
  source: "all" | "ca" | "kdc" | "bank",
): Promise<{ events: EnrichedEvent[]; sources: Record<string, boolean> }> {
  const requestId = req.headers["x-request-id"] as string;
  const filter = { fromUnix, toUnix };
  const events: NormalizedEvent[] = [];
  const sources: Record<string, boolean> = {};

  if (source === "all" || source === "ca") {
    try {
      events.push(...(await fetchCaWindow(filter, cap, requestId)));
      sources.ca = true;
    } catch {
      sources.ca = false;
    }
  }
  if (source === "all" || source === "kdc") {
    try {
      events.push(...(await fetchKdcWindow(filter, cap, requestId)));
      sources.kdc = true;
    } catch {
      sources.kdc = false;
    }
  }
  if (source === "all" || source === "bank") {
    const bankToken = readCookie(req.headers.cookie, BANK_SESSION_COOKIE);
    if (bankToken) {
      try {
        events.push(...(await fetchBankWindow(filter, cap, bankToken)));
        sources.bank = true;
      } catch {
        sources.bank = false;
      }
    } else {
      sources.bank = false;
    }
  }

  events.sort((a, b) => a.ts - b.ts);
  return { events: events.slice(0, cap).map(enrich), sources };
}

const increment = (m: Record<string, number>, key: string) => {
  m[key] = (m[key] ?? 0) + 1;
};

export const handleAuditSummary = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"] as string;
  const parsed = SummaryQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: parsed.error.issues[0].message,
      request_id: requestId,
    });
  }

  const windowSeconds = parseWindowSeconds(parsed.data.window);
  const toUnix = Math.floor(Date.now() / 1000);
  const fromUnix = toUnix - windowSeconds;

  try {
    const { events, sources } = await collect(req, fromUnix, toUnix, SUMMARY_CAP, "all");

    const bySeverity: Record<string, number> = { info: 0, warning: 0, critical: 0 };
    const byCategory: Record<string, number> = {};
    const byOutcome: Record<string, number> = { success: 0, denied: 0 };
    const reasonCounts: Record<string, number> = {};
    const deniedByActor: Record<string, number> = {};

    for (const e of events) {
      increment(bySeverity, e.severity);
      increment(byCategory, e.category);
      increment(byOutcome, e.outcome);
      if (e.outcome === "denied") {
        if (e.reason) increment(reasonCounts, e.reason);
        const actorKey = `${e.source}:${e.actor || "unknown"}`;
        increment(deniedByActor, actorKey);
      }
    }

    const topReasons = Object.entries(reasonCounts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 10)
      .map(([reason, count]) => ({ reason, count }));

    // Simple anomaly detection: identities with many denied events in-window.
    const anomalies = Object.entries(deniedByActor)
      .filter(([, count]) => count >= ANOMALY_THRESHOLD)
      .sort((a, b) => b[1] - a[1])
      .map(([actor, count]) => ({ actor, denied_count: count }));

    return res.json({
      success: true,
      data: {
        window_seconds: windowSeconds,
        from: new Date(fromUnix * 1000).toISOString(),
        to: new Date(toUnix * 1000).toISOString(),
        sources,
        total: events.length,
        capped: events.length >= SUMMARY_CAP,
        by_severity: bySeverity,
        by_category: byCategory,
        by_outcome: byOutcome,
        security_events: bySeverity.warning + bySeverity.critical,
        top_reasons: topReasons,
        anomalies,
      },
      request_id: requestId,
      timestamp: new Date().toISOString(),
    });
  } catch (err) {
    return next(err);
  }
};

const CSV_COLUMNS: (keyof EnrichedEvent)[] = [
  "timestamp",
  "source",
  "category",
  "severity",
  "outcome",
  "action",
  "actor_type",
  "actor_display",
  "actor",
  "target",
  "reason",
  "request_id",
  "ip",
];

const csvCell = (value: unknown): string => {
  const s = value === undefined || value === null ? "" : String(value);
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
};

export const handleAuditExport = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"] as string;
  const parsed = ExportQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: parsed.error.issues[0].message,
      request_id: requestId,
    });
  }
  const q = parsed.data;
  if (q.from > 0 && q.to > 0 && q.to < q.from) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: "to must not be before from",
      request_id: requestId,
    });
  }

  try {
    const { events } = await collect(req, q.from, q.to, EXPORT_CAP, q.source);
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");

    if (q.format === "csv") {
      const header = CSV_COLUMNS.join(",");
      const rows = events.map((e) => CSV_COLUMNS.map((c) => csvCell(e[c])).join(","));
      res.setHeader("Content-Type", "text/csv; charset=utf-8");
      res.setHeader(
        "Content-Disposition",
        `attachment; filename="audit-${q.source}-${stamp}.csv"`,
      );
      return res.send([header, ...rows].join("\n"));
    }

    res.setHeader("Content-Type", "application/json; charset=utf-8");
    res.setHeader(
      "Content-Disposition",
      `attachment; filename="audit-${q.source}-${stamp}.json"`,
    );
    return res.send(
      JSON.stringify(
        { count: events.length, capped: events.length >= EXPORT_CAP, items: events },
        null,
        2,
      ),
    );
  } catch (err) {
    return next(err);
  }
};
