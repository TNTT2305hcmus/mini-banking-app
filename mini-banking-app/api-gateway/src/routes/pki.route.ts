import { Router, Express } from "express";
import { handleRegister } from "../controller/pki.controller";
import { validateRegister } from "../middleware/otp.middleware";
import { validateHeaders } from "../middleware/validateHeaders";

const router = Router();

export const pkiRouter = (app: Express) => {
  router.post("/register", validateHeaders, validateRegister, handleRegister);
  app.use("/v1/pki", router);
};
