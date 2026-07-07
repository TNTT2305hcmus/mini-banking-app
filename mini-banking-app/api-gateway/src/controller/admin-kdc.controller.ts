// Controller cho KDC audit read API GET /v1/admin-kdc/audit: validate query bằng
// zod, map sang gRPC request, chuẩn hóa response envelope theo contract admin.
// Key-issuance audit (AS/TGS) thuộc quyền xem của CA/security admin (RBAC).
import { Request, Response, NextFunction } from "express";
import { z } from "zod";
import { listKdcAuditEvents } from "../services/kdc.service";

// Enum action phải khớp CHECK constraint của kdc_audit_log trong
// db/kdc/migrations/001_init_kdc.sql.
const KDC_ACTIONS = [
  "as_ticket_issued",
  "as_rejected",
  "tgs_ticket_issued",
  "tgs_rejected",
] as const;

// from/to là ISO 8601; parse về unix seconds cho gRPC (0 nghĩa là không filter).
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

const KdcAuditQuerySchema = z.object({
  action: z.enum(KDC_ACTIONS).optional(),
  client_id: z.string().optional(),
  cert_serial: z.string().optional(),
  request_id: z.string().optional(),
  limit: z.coerce.number().int().min(1).max(100).default(20),
  offset: z.coerce.number().int().min(0).default(0),
  from: isoToUnix,
  to: isoToUnix,
});

const safeJson = (raw: string): unknown => {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
};

// GET /v1/admin-kdc/audit?action&client_id&cert_serial&request_id&from&to&limit&offset
export const handleListKdcAudit = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"] as string;

  const parsed = KdcAuditQuerySchema.safeParse(req.query);
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
    const result = await listKdcAuditEvents(
      {
        action: q.action ?? "",
        clientId: q.client_id ?? "",
        certSerial: q.cert_serial ?? "",
        requestId: q.request_id ?? "",
        fromUnix: q.from,
        toUnix: q.to,
        limit: q.limit,
        offset: q.offset,
      },
      requestId,
    );
    return res.json({
      success: true,
      data: {
        items: result.events.map((event) => ({
          id: event.id,
          action: event.action,
          client_id: event.clientId,
          cert_serial: event.certSerial,
          scope: event.scope,
          reason: event.reason,
          request_id: event.requestId,
          ip: event.ip,
          metadata: safeJson(event.metadataJson),
          created_at: new Date(event.createdAtUnix * 1000).toISOString(),
        })),
        total: result.total,
        limit: result.limit,
        offset: result.offset,
      },
      request_id: requestId,
      timestamp: new Date().toISOString(),
    });
  } catch (err) {
    return next(err);
  }
};
