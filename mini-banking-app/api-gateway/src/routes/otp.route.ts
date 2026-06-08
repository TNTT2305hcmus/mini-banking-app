import { Router, Express } from "express";
import {
  handleOtpRequest,
  handleOtpVerify,
} from "../controller/otp.controller";
import {
  validateOtpRequest,
  validateOtpVerify,
} from "../middleware/otp.middleware";
import { rateLimitOtpByEmail } from "../middleware/rateLimiter";
import { validateHeaders } from "../middleware/validateHeaders";

const router = Router();

export const otpRouter = (app: Express) => {
  router.post(
    "/request",
    validateHeaders,
    rateLimitOtpByEmail,
    validateOtpRequest,
    handleOtpRequest,
  );
  router.post("/verify", validateHeaders, validateOtpVerify, handleOtpVerify);
  app.use("/v1/otp", router);
};
