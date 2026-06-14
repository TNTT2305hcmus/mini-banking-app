import { Router, Express } from "express";
import { requireBodyFields } from "../middleware/validate";

export const router = Router();

const bankRouter = (app: Express) => {
  router.post("/transfer");

  router.post("/accounts/:id/balance");

  router.post("/accounts/:id/transactions");

  app.use("v1/bank", router);
};
