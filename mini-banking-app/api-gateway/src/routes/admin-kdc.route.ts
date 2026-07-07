// REST route đọc KDC key-issuance audit cho admin, mount dưới /v1/admin-kdc/audit.
// Guard bằng vai trò admin-ca: theo RBAC của kế hoạch, CA/security admin xem cả
// cert_lifecycle (CA) lẫn key_issuance (KDC). Endpoint read-only, không ghi audit.
import { Express, Router } from "express";
import { handleListKdcAudit } from "../controller/admin-kdc.controller";
import { requireAdminRole } from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminKDCRouter = (app: Express) => {
  const router = Router();

  router.get(
    "/audit",
    validateHeaders,
    requireAdminRole(["admin-ca"]),
    handleListKdcAudit,
  );

  app.use("/v1/admin-kdc", router);
};
