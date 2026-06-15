// Khai báo các REST route Bank và nối chúng vào API Gateway dưới prefix /v1/bank.
import { Router, Express } from "express";
import {
  handleBalanceQuery,
  handleHistoryQuery,
  handleTransfer,
} from "../controller/bank.controller";
import {
  validateTransferRequest,
  validateBalanceQuery,
  validateHistoryQuery,
} from "../middleware/bank.middleware";
import { rateLimitBankByIP } from "../middleware/rateLimiter";
import { validateHeaders } from "../middleware/validateHeaders";

// Gắn toàn bộ endpoint Bank vào Express app theo cùng style với otp/pki/auth router.
export const bankRouter = (app: Express) => {
  // Router con chỉ giữ phần path sau /v1/bank để tránh lặp prefix ở từng endpoint.
  const router = Router();

  // Endpoint chuyển tiền: validate request rồi forward opaque payload sang Bank Service.
  router.post(
    "/transfer",
    validateHeaders,
    rateLimitBankByIP,
    validateTransferRequest,
    handleTransfer,
  );

  // Xem số dư: dùng POST để không đưa ticket/authenticator lên URL.
  router.post(
    "/accounts/:id/balance/query",
    validateHeaders,
    rateLimitBankByIP,
    validateBalanceQuery,
    handleBalanceQuery,
  );

  // Xem lịch sử giao dịch: validate phân trang rồi gọi Bank Service.
  router.post(
    "/accounts/:id/transactions/query",
    validateHeaders,
    rateLimitBankByIP,
    validateHistoryQuery,
    handleHistoryQuery,
  );

  // Mount router Bank vào app chính với prefix API public.
  app.use("/v1/bank", router);
};
