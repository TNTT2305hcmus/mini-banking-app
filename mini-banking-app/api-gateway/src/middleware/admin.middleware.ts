import { Request, Response, NextFunction } from "express";
import jwt from "jsonwebtoken";
import { z } from "zod";
import ENV from "../config/env";

type AdminCATokenPayload = {
  email?: string;
  role?: string;
  sub?: string;
};

const AdminCALoginBodySchema = z.object({
  email: z.email("email must be valid").optional(),
  password: z.string().min(1, "password is required"),
});

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
  const result = AdminCALoginBodySchema.safeParse(req.body);
  if (!result.success) {
    return reject(res, result.error.issues[0], {
      email: "INVALID_ADMIN_CA_EMAIL",
      password: "ADMIN_CA_PASSWORD_REQUIRED",
    });
  }

  req.body = result.data;
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

    // Static demo tokens, each scoped to exactly one role so a token can only
    // reach the surface it belongs to (admin-ca token cannot open SOC routes).
    const staticRole =
      token === ENV.ADMIN_CA_DEMO_TOKEN
        ? "admin-ca"
        : ENV.ADMIN_SEC_DEMO_TOKEN && token === ENV.ADMIN_SEC_DEMO_TOKEN
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
        staticRole === "admin-ca"
          ? ENV.ADMIN_CA_DEMO_EMAIL
          : ENV.ADMIN_SEC_DEMO_EMAIL ?? "security-admin";
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
