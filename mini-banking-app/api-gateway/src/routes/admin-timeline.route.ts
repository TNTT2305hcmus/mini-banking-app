// Cross-service audit timeline, mounted at /v1/admin/audit/timeline.
// Guarded by the admin-ca role: it always returns CA + KDC events, and folds in
// Bank events when the caller also carries a bank admin session cookie
// (i.e. a super-admin holding both credentials sees the full trail). Read-only.
import { Express, Router } from "express";
import {
  handleAuditTimeline,
  handleAuditVerify,
} from "../controller/admin-timeline.controller";
import {
  handleAuditSummary,
  handleAuditExport,
} from "../controller/admin-audit-report.controller";
import { requireAdminRole } from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminTimelineRouter = (app: Express) => {
  const router = Router();
  const requireCAAdmin = requireAdminRole(["admin-ca"]);

  router.get("/timeline", validateHeaders, requireCAAdmin, handleAuditTimeline);
  router.get("/verify", validateHeaders, requireCAAdmin, handleAuditVerify);
  router.get("/summary", validateHeaders, requireCAAdmin, handleAuditSummary);
  router.get("/export", validateHeaders, requireCAAdmin, handleAuditExport);

  app.use("/v1/admin/audit", router);
};
