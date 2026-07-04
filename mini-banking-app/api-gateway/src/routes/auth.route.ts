import { Router, Express } from "express";
import {
  handleRequestTGT,
  handleRequestTGS,
} from "../controller/kdc.controller";
import {
  validateASRequest,
  validateTGSRequest,
} from "../middleware/auth.middleware";
import { rateLimitByIP, rateLimitByCertSn } from "../middleware/rateLimiter";
import { validateHeaders } from "../middleware/validateHeaders";
import { validateRegister } from "../middleware/otp.middleware";
import { handleRegister } from "../controller/ca.controller";
import { validateProfileRequest } from "../middleware/bank.middleware";
import { handleProfile } from "../controller/bank.controller";

const router = Router();

export const authRouter = (app: Express) => {
  router.post("/register", validateHeaders, validateRegister, handleRegister);
  router.post(
    "/as-req",
    validateHeaders,
    rateLimitByIP,
    validateASRequest,
    handleRequestTGT,
  );
  router.post(
    "/tgs-req",
    validateHeaders,
    rateLimitByCertSn,
    validateTGSRequest,
    handleRequestTGS,
  );

  // /auth/me: client xuất trình Ticket_v (scope balance:read) + Authenticator (AP exchange);
  // Bank resolve tài khoản theo client_id trong ticket và trả profile user + tài khoản (trong ap_rep).
  router.post(
    "/me",
    validateHeaders,
    rateLimitByIP,
    validateProfileRequest,
    handleProfile,
  );

  app.use("/v1/auth", router);
};
