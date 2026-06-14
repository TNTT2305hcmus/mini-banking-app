import express from "express";
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";

import ENV from "./config/env";
import { authRouter } from "./routes/auth.route";
import { otpRouter }  from "./routes/otp.route";
import { pkiRouter }  from "./routes/pki.route";
import { bankRouter } from "./routes/bank.routes";
import { errorHandler } from "./middleware/errorHandler";

// Start the BullMQ mail worker
import "./workers/mail.worker";

const app = express();

app.use(express.json());
app.use(cors());
app.use(helmet());
app.use(morgan("dev"));

otpRouter(app);   // /v1/otp/*
pkiRouter(app);   // /v1/pki/*
authRouter(app);  // /v1/auth/*
app.use("/v1", bankRouter); // /v1/bank/*

app.use(errorHandler);

app.listen(ENV.PORT, () => {
  console.log("API Gateway is running on port " + ENV.PORT);
});
