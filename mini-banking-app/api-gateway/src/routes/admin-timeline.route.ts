// Cross-service audit timeline, mounted at /v1/admin/audit/timeline.
// Guarded by the admin-ca role: it always returns CA + KDC events, and folds in
// Bank events when the caller also carries a bank admin session cookie
// (i.e. a super-admin holding both credentials sees the full trail). Read-only.
import { Express, Router } from "express";
import {
  handleAuditTimeline,
  handleAuditVerify,
} from "../controller/admin-timeline.controller";
import { requireAdminRole } from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminTimelineRouter = (app: Express) => {
  const router = Router();

  router.get(
    "/timeline",
    validateHeaders,
    requireAdminRole(["admin-ca"]),
    handleAuditTimeline,
  );

  router.get(
    "/verify",
    validateHeaders,
    requireAdminRole(["admin-ca"]),
    handleAuditVerify,
  );

  app.use("/v1/admin/audit", router);
};
