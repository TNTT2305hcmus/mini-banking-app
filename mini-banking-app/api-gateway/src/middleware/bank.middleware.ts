import { Request, Response, NextFunction } from "express";
import { z } from "zod";

const base64Re = /^[A-Za-z0-9+/]+={0,2}$/;
const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const uuidV4Re =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const decodesTo = (bytes: number) => (b64: string) => {
  try {
    return Buffer.from(b64, "base64").length === bytes;
  } catch {
    return false;
  }
};

const b64 = (label: string) =>
  z.string().regex(base64Re, `${label} must be a non-empty base64 string`);

const requestId = z
  .string()
  .regex(uuidV4Re, "request_id must be a valid UUID v4");

// POST /v1/bank/transfer
const TransferBodySchema = z.object({
  ticket_v: b64("ticket_v"),
  authenticator: b64("authenticator"),
  cipher_payload: b64("cipher_payload"),
  iv: b64("iv").refine(decodesTo(12), "iv must decode to exactly 12 bytes"),
  request_id: requestId,
});

// POST /v1/bank/accounts/{id}/balance/query
// POST /v1/bank/accounts/{id}/transactions/query
const ReadBodySchema = z.object({
  ticket_v: b64("ticket_v"),
  authenticator: b64("authenticator"),
  request_id: requestId,
});

// account_id path param (UUID)
const AccountIdSchema = z
  .string()
  .regex(uuidRe, "account_id must be a valid UUID");

// limit/offset query params for history pagination
const PaginationSchema = z.object({
  limit: z.coerce.number().int().min(1).max(100).optional(),
  offset: z.coerce.number().int().min(0).optional(),
});

// Build a 400 response from the first Zod issue, mapping field → error_code.
const reject = (
  res: Response,
  issue: z.core.$ZodIssue,
  codeByField: Record<string, string> = {},
) => {
  const field = issue.path[0] as string;
  return res.status(400).json({
    success: false,
    error_code: codeByField[field] ?? "INVALID_REQUEST",
    message: issue.message,
  });
};

export const validateTransferRequest = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = TransferBodySchema.safeParse(req.body);
  if (!result.success) {
    return reject(res, result.error.issues[0], {
      ticket_v: "INVALID_TICKET",
      iv: "INVALID_IV",
    });
  }
  next();
};

export const validateBalanceQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const id = AccountIdSchema.safeParse(req.params.id);
  if (!id.success) {
    return reject(res, id.error.issues[0]);
  }

  const body = ReadBodySchema.safeParse(req.body);
  if (!body.success) {
    return reject(res, body.error.issues[0], { ticket_v: "INVALID_TICKET" });
  }
  next();
};

export const validateHistoryQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const id = AccountIdSchema.safeParse(req.params.id);
  if (!id.success) {
    return reject(res, id.error.issues[0]);
  }

  const body = ReadBodySchema.safeParse(req.body);
  if (!body.success) {
    return reject(res, body.error.issues[0], { ticket_v: "INVALID_TICKET" });
  }

  const page = PaginationSchema.safeParse(req.query);
  if (!page.success) {
    return reject(res, page.error.issues[0], {
      limit: "INVALID_PAGINATION",
      offset: "INVALID_PAGINATION",
    });
  }
  next();
};
