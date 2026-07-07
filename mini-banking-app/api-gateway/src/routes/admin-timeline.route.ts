// Cross-service audit views, mounted under /v1/admin/audit.
// Guarded by the security-admin (SOC) role: these span CA+KDC(+Bank), so they
// belong to Security Operations, not to any single domain admin. They always
// return CA + KDC events and fold in Bank events when the caller also carries a
// bank admin session cookie (a super-admin holding both sees the full trail).
// Read-only.
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
  const requireSecAdmin = requireAdminRole(["security-admin"]);

  router.get("/timeline", validateHeaders, requireSecAdmin, handleAuditTimeline);
  router.get("/verify", validateHeaders, requireSecAdmin, handleAuditVerify);
  router.get("/summary", validateHeaders, requireSecAdmin, handleAuditSummary);
  router.get("/export", validateHeaders, requireSecAdmin, handleAuditExport);

  app.use("/v1/admin/audit", router);
};
