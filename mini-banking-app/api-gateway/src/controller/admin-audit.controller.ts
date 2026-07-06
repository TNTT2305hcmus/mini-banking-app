// Controller cho audit read API /v1/admin/audit/{ca,bank}: validate query bằng
// zod, map sang gRPC request, chuẩn hóa response envelope theo contract admin.
import { Request, Response, NextFunction } from "express";
import { z } from "zod";
import {
  listCaAuditEvents,
  listBankAuditEvents,
} from "../services/admin-audit.service";
import { performedBy } from "../middleware/admin.middleware";

// Enum action phải khớp CHECK constraint trong DB migration của từng service.
const CA_ACTIONS = [
  "issued",
  "revoked",
  "looked_up",
  "revocation_checked",
  "verify_certificate",
] as const;
const BANK_ACTIONS = [
  "transfer_completed",
  "transfer_rejected",
  "replay_detected",
  "invalid_signature",
  "certificate_rejected",
  "forbidden_ownership",
  "insufficient_funds",
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

const pagination = {
  limit: z.coerce.number().int().min(1).max(100).default(20),
  offset: z.coerce.number().int().min(0).default(0),
  from: isoToUnix,
  to: isoToUnix,
};

const CaAuditQuerySchema = z.object({
  action: z.enum(CA_ACTIONS).optional(),
  serial: z.string().optional(),
  performed_by: z.string().optional(),
  ...pagination,
});

const BankAuditQuerySchema = z.object({
  action: z.enum(BANK_ACTIONS).optional(),
  user_id: z.uuid("user_id must be a UUID").optional(),
  cert_serial: z.string().optional(),
  request_id: z.string().optional(),
  ...pagination,
});

type ParsedRange = { from: number; to: number };

const validRange = (q: ParsedRange, res: Response, requestId?: string) => {
  if (q.from > 0 && q.to > 0 && q.to < q.from) {
    res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: "to must not be before from",
      request_id: requestId,
    });
    return false;
  }
  return true;
};

const badQuery = (res: Response, message: string, requestId?: string) =>
  res.status(400).json({
    success: false,
    error_code: "INVALID_REQUEST",
    message,
    request_id: requestId,
  });

const envelope = (res: Response, requestId: string | undefined, data: unknown) =>
  res.json({
    success: true,
    data,
    request_id: requestId,
    timestamp: new Date().toISOString(),
  });

// GET /v1/admin/audit/ca?action&serial&performed_by&from&to&limit&offset
export const handleListCaAudit = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const parsed = CaAuditQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return badQuery(res, parsed.error.issues[0].message, req.requestId);
  }
  const q = parsed.data;
  if (!validRange(q, res, req.requestId)) return;

  try {
    const result = await listCaAuditEvents(
      {
        action: q.action ?? "",
        serialNumber: q.serial ?? "",
        performedByFilter: q.performed_by ?? "",
        fromUnix: q.from,
        toUnix: q.to,
        limit: q.limit,
        offset: q.offset,
        performedBy: performedBy(req),
      },
      req.requestId,
    );
    return envelope(res, req.requestId, {
      items: result.events.map((event) => ({
        serial_number: event.serialNumber,
        action: event.action,
        performed_by: event.performedBy,
        reason: event.reason,
        performed_at: new Date(event.performedAtUnix * 1000).toISOString(),
        metadata: event.metadata,
      })),
      total: result.total,
      limit: result.limit,
      offset: result.offset,
    });
  } catch (err) {
    return next(err);
  }
};

// GET /v1/admin/audit/bank?action&user_id&cert_serial&request_id&from&to&limit&offset
export const handleListBankAudit = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const parsed = BankAuditQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return badQuery(res, parsed.error.issues[0].message, req.requestId);
  }
  const q = parsed.data;
  if (!validRange(q, res, req.requestId)) return;

  try {
    const result = await listBankAuditEvents(
      {
        action: q.action ?? "",
        userId: q.user_id ?? "",
        certSerial: q.cert_serial ?? "",
        // Filter domain: khớp cột bank_audit_log.request_id (AP flow), khác với
        // trace id của Gateway vốn đi qua gRPC metadata.
        requestId: q.request_id ?? "",
        fromUnix: q.from,
        toUnix: q.to,
        limit: q.limit,
        offset: q.offset,
      },
      req.requestId,
    );
    return envelope(res, req.requestId, {
      items: result.events.map((event) => ({
        id: event.id,
        action: event.action,
        user_id: event.userId,
        account_id: event.accountId,
        transaction_id: event.transactionId,
        cert_serial: event.certSerial,
        request_id: event.requestId,
        reason: event.reason,
        metadata: safeJson(event.metadataJson),
        created_at: new Date(event.createdAtUnix * 1000).toISOString(),
      })),
      total: result.total,
      limit: result.limit,
      offset: result.offset,
    });
  } catch (err) {
    return next(err);
  }
};

const safeJson = (raw: string): unknown => {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
};
