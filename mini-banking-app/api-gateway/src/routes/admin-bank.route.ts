import { Router, type Express } from "express";
import {
  handleAdminActivate,
  handleAdminAuditQuery,
  handleAdminBankError,
  handleAdminOverviewQuery,
  handleAdminSession,
  handleAdminTransactionsQuery,
  handleAdminUserAccountsQuery,
  handleAdminUsersQuery,
} from "../controller/admin-bank.controller";
import {
  rateLimitAdminActivationByIP,
  requireBankAdminSessionCookie,
  validateAdminActivation,
  validateAdminAuditQuery,
  validateAdminOverviewQuery,
  validateAdminSessionRequest,
  validateAdminTransactionsQuery,
  validateAdminUserAccountsQuery,
  validateAdminUsersQuery,
} from "../middleware/admin-bank.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

export const adminBankRouter = (app: Express) => {
  const router = Router();

  router.post(
    "/activate",
    validateHeaders,
    rateLimitAdminActivationByIP,
    validateAdminActivation,
    handleAdminActivate,
  );

  router.post(
    "/session",
    validateHeaders,
    validateAdminSessionRequest,
    handleAdminSession,
  );

  router.post(
    "/overview/query",
    validateHeaders,
    requireBankAdminSessionCookie,
    validateAdminOverviewQuery,
    handleAdminOverviewQuery,
  );

  router.post(
    "/users/query",
    validateHeaders,
    requireBankAdminSessionCookie,
    validateAdminUsersQuery,
    handleAdminUsersQuery,
  );

  router.post(
    "/users/:userId/accounts/query",
    validateHeaders,
    requireBankAdminSessionCookie,
    validateAdminUserAccountsQuery,
    handleAdminUserAccountsQuery,
  );

  router.post(
    "/transactions/query",
    validateHeaders,
    requireBankAdminSessionCookie,
    validateAdminTransactionsQuery,
    handleAdminTransactionsQuery,
  );

  router.post(
    "/audit/query",
    validateHeaders,
    requireBankAdminSessionCookie,
    validateAdminAuditQuery,
    handleAdminAuditQuery,
  );

  router.use(handleAdminBankError);

  app.use("/v1/admin/bank", router);
};
