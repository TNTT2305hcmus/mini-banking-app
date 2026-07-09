import { Express, Router } from "express";
import {
  handleAdminAuth,
  handleAdminGetCertificateDetail,
  handleAdminListCertificates,
  handleAdminRevokeCertificate,
  handleListCaAudit,
} from "../controller/ca.controller";
import {
  requireAdminCAAuthConfigured,
  validateAdminLoginRequest,
  requireAdminRole,
} from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminCARouter = (app: Express) => {
  const router = Router();
  const requireCAAdmin = requireAdminRole(["admin-ca"]);

  const authMiddlewares = [
    validateHeaders,
    requireAdminCAAuthConfigured,
    validateAdminLoginRequest,
    handleAdminAuth,
  ];

  app.post("/v1/admin-ca/auth", ...authMiddlewares);

  router.get(
    "/certificates",
    validateHeaders,
    requireCAAdmin,
    handleAdminListCertificates,
  );
  router.get(
    "/certificates/:serial",
    validateHeaders,
    requireCAAdmin,
    handleAdminGetCertificateDetail,
  );
  router.post(
    "/certificates/:serial/revoke",
    validateHeaders,
    requireCAAdmin,
    handleAdminRevokeCertificate,
  );

  router.get("/audit", validateHeaders, requireCAAdmin, handleListCaAudit);

  app.use("/v1/admin-ca", router);
};
