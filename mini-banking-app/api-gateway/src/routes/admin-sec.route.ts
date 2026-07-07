// Security Operations (SOC) admin login route, mounted at /v1/admin-sec/auth.
// The SOC audit surfaces (KDC audit + cross-service views) are re-guarded to the
// security-admin role in a follow-up; this route only issues the token.
import { Express } from "express";
import { handleSecAdminAuth } from "../controller/admin-sec.controller";
import { validateAdminLoginRequest } from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminSecRouter = (app: Express) => {
  app.post(
    "/v1/admin-sec/auth",
    validateHeaders,
    validateAdminLoginRequest,
    handleSecAdminAuth,
  );
};
