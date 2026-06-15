import { Router, Express, Request, Response, NextFunction } from "express";
import { getBalance, getHistory, transferMoney } from "../services/bank.service";
import {
  validateTransferRequest,
  validateBalanceQuery,
  validateHistoryQuery,
} from "../middleware/bank.middleware";
import { rateLimitBankByIP } from "../middleware/rateLimiter";

export const bankRouter = (app: Express) => {
  const router = Router();

  router.post(
    "/transfer",
    rateLimitBankByIP,
    validateTransferRequest,
    async (req: Request, res: Response, next: NextFunction) => {
      const { ticket_v, authenticator, cipher_payload, iv } = req.body;
      try {
        const response = await transferMoney({
          ticketV: Buffer.from(ticket_v, "base64"),
          authenticator: Buffer.from(authenticator, "base64"),
          cipherPayload: Buffer.from(cipher_payload, "base64"),
          iv: Buffer.from(iv, "base64"),
        });
        res.json(response);
      } catch (err) {
        next(err);
      }
    },
  );

  router.post(
    "/accounts/:id/balance/query",
    rateLimitBankByIP,
    validateBalanceQuery,
    async (req: Request, res: Response, next: NextFunction) => {
      const { ticket_v, authenticator } = req.body;
      res.set("Cache-Control", "no-store");
      try {
        const response = await getBalance({
          ticketV: Buffer.from(ticket_v, "base64"),
          authenticator: Buffer.from(authenticator, "base64"),
          accountId: String(req.params.id),
        });
        res.json(response);
      } catch (err) {
        next(err);
      }
    },
  );

  router.post(
    "/accounts/:id/transactions/query",
    rateLimitBankByIP,
    validateHistoryQuery,
    async (req: Request, res: Response, next: NextFunction) => {
      const { ticket_v, authenticator } = req.body;
      const { limit, offset } = req.query as Record<string, string>;
      res.set("Cache-Control", "no-store");
      try {
        const response = await getHistory({
          ticketV: Buffer.from(ticket_v, "base64"),
          authenticator: Buffer.from(authenticator, "base64"),
          accountId: String(req.params.id),
          limit: parseInt(limit ?? "20", 10),
          offset: parseInt(offset ?? "0", 10),
        });
        res.json(response);
      } catch (err) {
        next(err);
      }
    },
  );

  app.use("/v1/bank", router);
};
