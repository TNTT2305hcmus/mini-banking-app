// REST route đọc audit log cho admin, mount dưới prefix /v1/admin/audit.
// Các endpoint này read-only và bản thân chúng không ghi audit (tránh vòng lặp).
import { Router, Express } from "express";
import {
  handleListCaAudit,
  handleListBankAudit,
} from "../controller/admin-audit.controller";
import {
  ensureRequestId,
  requireAdmin,
} from "../middleware/admin.middleware";

export const adminAuditRouter = (app: Express) => {
  const router = Router();

  // Audit CA: certificate_audit_log qua CA gRPC ListAuditEvents.
  router.get("/ca", requireAdmin(["ca_admin"]), handleListCaAudit);

  // Audit Bank: bank_audit_log qua Bank gRPC ListAuditEvents.
  router.get("/bank", requireAdmin(["bank_admin"]), handleListBankAudit);

  app.use("/v1/admin/audit", ensureRequestId, router);
};
