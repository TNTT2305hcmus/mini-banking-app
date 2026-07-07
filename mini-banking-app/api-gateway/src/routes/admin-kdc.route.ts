// REST route đọc KDC key-issuance audit, mount dưới /v1/admin-kdc/audit.
// Guard bằng vai trò security-admin (SOC): key issuance là mối quan tâm bảo mật
// cross-cutting, không thuộc bất kỳ domain admin đơn lẻ (CA/Bank) nào.
// Endpoint read-only, không ghi audit.
import { Express, Router } from "express";
import { handleListKdcAudit } from "../controller/admin-kdc.controller";
import { requireAdminRole } from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminKDCRouter = (app: Express) => {
  const router = Router();

  router.get(
    "/audit",
    validateHeaders,
    requireAdminRole(["security-admin"]),
    handleListKdcAudit,
  );

  app.use("/v1/admin-kdc", router);
};
