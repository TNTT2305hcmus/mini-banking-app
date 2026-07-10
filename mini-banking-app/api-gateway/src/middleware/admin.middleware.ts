import { Request, Response, NextFunction } from "express";
import jwt from "jsonwebtoken";
import { z } from "zod";
import ENV from "../config/env";
import redis from "../config/ioredis";

type AdminCATokenPayload = {
  email?: string;
  role?: string;
  sub?: string;
};

const AdminLoginBodySchema = z.object({
  email: z.email("email must be valid").optional(),
  password: z.string().min(1, "password is required"),
});

const base64Re = /^[A-Za-z0-9+/]+={0,2}$/;
const hexRe = /^[0-9a-f]+$/i;
const ADMIN_CA_ACTIVATION_RATE_PREFIX = "rate:admin-ca-activation:";

const AdminCaActivationBodySchema = z
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

const AdminCaCertificateSessionBodySchema = z
  .object({
    cert_serial: z.string().min(8).max(128).regex(hexRe, "cert_serial must be hexadecimal"),
    challenge: z.string().min(32).max(512),
    signature: z.string().min(128).max(1024).regex(base64Re, "signature must be base64"),
  })
  .strict();

const AuthHeaderSchema = z.object({
  authorization: z
    .string()
    .min(1, "Authorization header is required")
    .regex(/^Bearer\s+\S+$/, "Authorization must use Bearer token"),
});

const reject = (
  res: Response,
  issue: z.core.$ZodIssue,
  codeByField: Record<string, string> = {},
  status = 400,
) => {
  const field = issue.path[0] as string;
  return res.status(status).json({
    success: false,
    error_code: codeByField[field] ?? "INVALID_REQUEST",
    message: issue.message,
  });
};

export const validateAdminLoginRequest = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = AdminLoginBodySchema.safeParse(req.body);
  if (!result.success) {
    return reject(res, result.error.issues[0], {
      email: "INVALID_ADMIN_CA_EMAIL",
      password: "ADMIN_CA_PASSWORD_REQUIRED",
    });
  }

  req.body = result.data;
  next();
};

export const validateAdminCaActivation = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = AdminCaActivationBodySchema.safeParse(req.body);
  if (!result.success) {
    return reject(res, result.error.issues[0], {}, 400);
  }

  req.body = result.data;
  next();
};

export const rateLimitAdminCaActivationByIP = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  const key = ADMIN_CA_ACTIVATION_RATE_PREFIX + ip;
  try {
    const count = await redis.incr(key);
    if (count === 1) await redis.expire(key, 300);
    if (count > 10) {
      return res.status(429).json({
        success: false,
        error_code: "ADMIN_CA_ACTIVATION_RATE_LIMITED",
        message: "Too many activation attempts. Try again later.",
      });
    }
  } catch {
    // Keep activation reachable if Redis rate-limit storage is temporarily down.
  }
  next();
};

export const validateAdminCaCertificateSession = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = AdminCaCertificateSessionBodySchema.safeParse(req.body);
  if (!result.success) {
    return reject(res, result.error.issues[0], {}, 400);
  }

  req.body = {
    ...result.data,
    cert_serial: result.data.cert_serial.toLowerCase(),
  };
  next();
};

export const requireAdminRole =
  (allowedRoles: string[]) =>
  (req: Request, res: Response, next: NextFunction) => {
    const headers = AuthHeaderSchema.safeParse(req.headers);
    if (!headers.success) {
      return reject(
        res,
        headers.error.issues[0],
        { authorization: "ADMIN_CA_TOKEN_REQUIRED" },
        401,
      );
    }

    const token = headers.data.authorization
      .slice("Bearer ".length)
      .trim();

    // The SOC static token is a development fallback only. Admin CA no longer
    // accepts a static token; it must first create a cert-backed session.
    const staticRole =
      ENV.ADMIN_SEC_DEMO_TOKEN && token === ENV.ADMIN_SEC_DEMO_TOKEN
        ? "security-admin"
        : "";
    if (staticRole) {
      if (!allowedRoles.includes(staticRole)) {
        return res.status(403).json({
          success: false,
          error_code: "ADMIN_CA_ROLE_FORBIDDEN",
          message: "Admin role is not allowed for this endpoint",
        });
      }
      const email =
        ENV.ADMIN_SEC_DEMO_EMAIL ?? "security-admin";
      res.locals.adminCa = {
        email,
        role: staticRole,
        performedBy: `${staticRole}:${email}`,
      };
      return next();
    }

    try {
      const payload = jwt.verify(
        token,
        ENV.GATEWAY_JWT_SECRET,
      ) as AdminCATokenPayload;
      const role = payload.role ?? "";

      if (!allowedRoles.includes(role)) {
        return res.status(403).json({
          success: false,
          error_code: "ADMIN_CA_ROLE_FORBIDDEN",
          message: "Admin CA role is not allowed for this endpoint",
        });
      }

      const email = payload.email ?? payload.sub ?? "unknown";
      res.locals.adminCa = {
        email,
        role,
        performedBy: `admin-ca:${email}`,
      };
      return next();
    } catch (err: any) {
      return res.status(401).json({
        success: false,
        error_code: "ADMIN_CA_TOKEN_INVALID",
        message: err?.message ?? "Invalid admin-ca token",
      });
    }
  };
