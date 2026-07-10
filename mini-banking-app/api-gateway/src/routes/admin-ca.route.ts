import { Express, Router } from "express";
import {
  handleAdminCaActivate,
  handleAdminCaCertificateSession,
  handleAdminGetCertificateDetail,
  handleAdminListCertificates,
  handleAdminRevokeCertificate,
  handleListCaAudit,
} from "../controller/ca.controller";
import {
  rateLimitAdminCaActivationByIP,
  validateAdminCaActivation,
  validateAdminCaCertificateSession,
  requireAdminRole,
} from "../middleware/admin.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminCARouter = (app: Express) => {
  const router = Router();
  const requireCAAdmin = requireAdminRole(["admin-ca"]);

  app.post(
    "/v1/admin-ca/activate",
    validateHeaders,
    rateLimitAdminCaActivationByIP,
    validateAdminCaActivation,
    handleAdminCaActivate,
  );
  app.post(
    "/v1/admin-ca/session",
    validateHeaders,
    validateAdminCaCertificateSession,
    handleAdminCaCertificateSession,
  );

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
