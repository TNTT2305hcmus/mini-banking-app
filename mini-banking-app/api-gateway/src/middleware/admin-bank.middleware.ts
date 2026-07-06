import type { NextFunction, Request, Response } from "express";
import { z } from "zod";
import redis from "../config/ioredis";

const ADMIN_ACTIVATION_RATE_PREFIX = "rate:admin-bank-activation:";
export const BANK_ADMIN_SESSION_COOKIE = "bank_admin_session";

const base64Re = /^[A-Za-z0-9+/]+={0,2}$/;
const base64UrlRe = /^[A-Za-z0-9_-]+$/;
const hexRe = /^[0-9a-f]+$/i;

const b64 = z.string().regex(base64Re, "must be a non-empty base64 string");
const pagination = {
  limit: z.number().int().min(1).max(100).default(20),
  offset: z.number().int().min(0).default(0),
};
const unixRange = {
  from_unix: z.number().int().nonnegative().default(0),
  to_unix: z.number().int().nonnegative().default(0),
};

const adminSessionBodySchema = z
  .object({ ticket_v: b64, authenticator: b64 })
  .strict();
const emptyQuerySchema = z.object({}).strict();
const usersQuerySchema = z
  .object({
    email: z.string().trim().min(1).max(254).optional(),
    status: z.enum(["active", "locked"]).optional(),
    ...pagination,
  })
  .strict();
const transactionsQuerySchema = z
  .object({
    account_id: z.uuid("account_id must be a valid UUID").optional(),
    status: z.enum(["pending", "completed", "failed"]).optional(),
    ...unixRange,
    ...pagination,
  })
  .strict();
const auditQuerySchema = z
  .object({
    action: z
      .enum([
        "transfer_completed",
        "transfer_rejected",
        "replay_detected",
        "invalid_signature",
        "certificate_rejected",
        "forbidden_ownership",
        "insufficient_funds",
      ])
      .optional(),
    user_id: z.uuid("user_id must be a valid UUID").optional(),
    cert_serial: z.string().regex(hexRe, "cert_serial must be hexadecimal").optional(),
    request_id: z.uuid("request_id must be a valid UUID").optional(),
    ...unixRange,
    ...pagination,
  })
  .strict();

const activationBodySchema = z
  .object({
    activation_token: z.string().min(32).max(256),
    csr_pem: z
      .string()
      .min(128)
      .refine(
        (value) =>
          value.startsWith("-----BEGIN CERTIFICATE REQUEST-----") &&
          value.includes("-----END CERTIFICATE REQUEST-----"),
        "csr_pem must be a PEM certificate request",
      ),
  })
  .strict();

export const validateAdminActivation = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = activationBodySchema.safeParse(req.body);
  if (!result.success) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_ADMIN_ACTIVATION_REQUEST",
      message: result.error.issues[0].message,
    });
  }
  req.body = result.data;
  next();
};

// Dedicated counter avoids sharing the existing AS/Client rate-limit bucket.
export const rateLimitAdminActivationByIP = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  const key = ADMIN_ACTIVATION_RATE_PREFIX + ip;
  try {
    const count = await redis.incr(key);
    if (count === 1) await redis.expire(key, 300);
    if (count > 10) {
      return res.status(429).json({
        success: false,
        error_code: "ADMIN_ACTIVATION_RATE_LIMITED",
        message: "Too many activation attempts. Try again later.",
      });
    }
  } catch {
    // Keep the same fail-open behavior as the existing Gateway rate limiter.
  }
  next();
};

const metadata = (req: Request) => ({
  request_id: String(req.headers["x-request-id"] ?? ""),
  timestamp: new Date().toISOString(),
});

const reject = (req: Request, res: Response, code: string, message: string) =>
  res.status(400).json({
    success: false,
    error_code: code,
    message,
    ...metadata(req),
  });

const validateBody = (schema: z.ZodType, code: string) =>
  (req: Request, res: Response, next: NextFunction) => {
    const result = schema.safeParse(req.body ?? {});
    if (!result.success) {
      return reject(req, res, code, result.error.issues[0].message);
    }
    req.body = result.data;
    next();
  };

export const validateAdminSessionRequest = validateBody(
  adminSessionBodySchema,
  "INVALID_REQUEST",
);
export const validateAdminOverviewQuery = validateBody(
  emptyQuerySchema,
  "INVALID_FILTER",
);
export const validateAdminUsersQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = usersQuerySchema.safeParse(req.body ?? {});
  if (!result.success) {
    const field = String(result.error.issues[0].path[0] ?? "");
    const code = field === "limit" || field === "offset"
      ? "INVALID_PAGINATION"
      : "INVALID_FILTER";
    return reject(req, res, code, result.error.issues[0].message);
  }
  req.body = result.data;
  next();
};

export const validateAdminTransactionsQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = transactionsQuerySchema.safeParse(req.body ?? {});
  if (!result.success) {
    const field = String(result.error.issues[0].path[0] ?? "");
    const code = field === "limit" || field === "offset"
      ? "INVALID_PAGINATION"
      : "INVALID_FILTER";
    return reject(req, res, code, result.error.issues[0].message);
  }
  if (
    result.data.from_unix > 0 &&
    result.data.to_unix > 0 &&
    result.data.from_unix > result.data.to_unix
  ) {
    return reject(
      req,
      res,
      "INVALID_DATE_RANGE",
      "from_unix must not exceed to_unix",
    );
  }
  req.body = result.data;
  next();
};

export const validateAdminAuditQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = auditQuerySchema.safeParse(req.body ?? {});
  if (!result.success) {
    const field = String(result.error.issues[0].path[0] ?? "");
    const code = field === "limit" || field === "offset"
      ? "INVALID_PAGINATION"
      : "INVALID_FILTER";
    return reject(req, res, code, result.error.issues[0].message);
  }
  if (
    result.data.from_unix > 0 &&
    result.data.to_unix > 0 &&
    result.data.from_unix > result.data.to_unix
  ) {
    return reject(
      req,
      res,
      "INVALID_DATE_RANGE",
      "from_unix must not exceed to_unix",
    );
  }
  req.body = result.data;
  next();
};

export const validateAdminUserAccountsQuery = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const userId = z.uuid("userId must be a valid UUID").safeParse(req.params.userId);
  if (!userId.success) {
    return reject(req, res, "INVALID_FILTER", userId.error.issues[0].message);
  }
  const body = emptyQuerySchema.safeParse(req.body ?? {});
  if (!body.success) {
    return reject(req, res, "INVALID_FILTER", body.error.issues[0].message);
  }
  next();
};

const cookieValue = (
  header: string | undefined,
  name: string,
): string | undefined => {
  if (!header) return undefined;
  for (const part of header.split(";")) {
    const separator = part.indexOf("=");
    if (separator < 0 || part.slice(0, separator).trim() !== name) continue;
    const value = part.slice(separator + 1).trim();
    try {
      return decodeURIComponent(value);
    } catch {
      return undefined;
    }
  }
  return undefined;
};

export const requireBankAdminSessionCookie = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const token = cookieValue(req.headers.cookie, BANK_ADMIN_SESSION_COOKIE);
  if (!token) {
    return res.status(401).json({
      success: false,
      error_code: "ADMIN_SESSION_REQUIRED",
      message: "Bank Admin session cookie is required",
      ...metadata(req),
    });
  }
  if (token.length > 256 || !base64UrlRe.test(token)) {
    return res.status(401).json({
      success: false,
      error_code: "ADMIN_SESSION_INVALID",
      message: "Bank Admin session cookie is invalid",
      ...metadata(req),
    });
  }
  res.locals.bankAdminSessionToken = token;
  next();
};
