// Security Operations (SOC) admin login. Issues a JWT with role
// "security-admin", the identity that owns the KDC audit and the cross-service
// audit views (timeline/verify/summary/export). Kept separate from Admin CA and
// Admin Bank so no single domain admin can read across domains.
import { Request, Response, NextFunction } from "express";
import jwt from "jsonwebtoken";
import ENV from "../config/env";
import { httpError } from "../middleware/errorHandler";

const meta = (req: Request) => ({
  request_id: req.headers["x-request-id"] as string,
  timestamp: new Date().toISOString(),
});

// POST /v1/admin-sec/auth
export const handleSecAdminAuth = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  // Fail closed when unconfigured: no fake defaults, so the login simply stays
  // disabled until ADMIN_SEC_DEMO_* are set.
  if (!ENV.ADMIN_SEC_DEMO_EMAIL || !ENV.ADMIN_SEC_DEMO_PASSWORD) {
    return next(
      httpError(
        503,
        "ADMIN_SEC_NOT_CONFIGURED",
        "Security-admin login is not configured",
      ),
    );
  }

  const email = String(req.body?.email ?? ENV.ADMIN_SEC_DEMO_EMAIL)
    .trim()
    .toLowerCase();
  const password = String(req.body?.password ?? "");

  if (
    email !== ENV.ADMIN_SEC_DEMO_EMAIL ||
    password !== ENV.ADMIN_SEC_DEMO_PASSWORD
  ) {
    return next(
      httpError(401, "ADMIN_SEC_LOGIN_FAILED", "Invalid security-admin credentials"),
    );
  }

  const token = jwt.sign(
    { sub: email, email, role: "security-admin", purpose: "admin-sec" },
    ENV.GATEWAY_JWT_SECRET,
    { expiresIn: "8h" },
  );

  return res.status(200).json({
    success: true,
    data: {
      token,
      token_type: "Bearer",
      role: "security-admin",
      email,
      expires_in: 8 * 60 * 60,
    },
    ...meta(req),
  });
};
