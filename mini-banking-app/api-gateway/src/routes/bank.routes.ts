import { Router, Request, Response, NextFunction } from "express";
import { bankClient } from "../grpc-clients/bank.client";
import { CreateUserRequest } from "../proto/bank";
import {
  validateTransferRequest,
  validateBalanceQuery,
  validateHistoryQuery,
} from "../middleware/bank.middleware";
import { rateLimitBankByIP } from "../middleware/rateLimiter";

export const bankRouter = Router();

bankRouter.post(
  "/bank/transfer",
  rateLimitBankByIP,
  validateTransferRequest,
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, cipher_payload, iv, request_id } =
      req.body;
    bankClient.transferMoney(
      {
        ticketV: Buffer.from(ticket_v, "base64"),
        authenticator: Buffer.from(authenticator, "base64"),
        cipherPayload: Buffer.from(cipher_payload, "base64"),
        iv: Buffer.from(iv, "base64"),
      },
      (err, response) => {
        if (err) return next(err);
        res.json(response);
      },
    );
  },
);

bankRouter.post(
  "/bank/accounts/:id/balance/query",
  rateLimitBankByIP,
  validateBalanceQuery,
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, request_id } = req.body;
    res.set("Cache-Control", "no-store");
    bankClient.getBalance(
      {
        ticketV: Buffer.from(ticket_v, "base64"),
        authenticator: Buffer.from(authenticator, "base64"),
        accountId: String(req.params.id),
      },
      (err, response) => {
        if (err) return next(err);
        res.json(response);
      },
    );
  },
);

bankRouter.post(
  "/bank/accounts/:id/transactions/query",
  rateLimitBankByIP,
  validateHistoryQuery,
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, request_id } = req.body;
    const { limit, offset } = req.query as Record<string, string>;
    res.set("Cache-Control", "no-store");
    bankClient.getHistory(
      {
        ticketV: Buffer.from(ticket_v, "base64"),
        authenticator: Buffer.from(authenticator, "base64"),
        accountId: String(req.params.id),
        limit: parseInt(limit ?? "20", 10),
        offset: parseInt(offset ?? "0", 10),
      },
      (err, response) => {
        if (err) return next(err);
        res.json(response);
      },
    );
  },
);

export function callCreateUser(req: CreateUserRequest): Promise<void> {
  return new Promise((resolve, reject) => {
    bankClient.createUser(req, (err) => {
      if (err) return reject(err);
      resolve();
    });
  });
}
