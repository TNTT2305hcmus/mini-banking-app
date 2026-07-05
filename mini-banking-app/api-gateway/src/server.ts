import express from "express";
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";

import ENV from "./config/env";
import { authRouter } from "./routes/auth.route";
import { otpRouter } from "./routes/otp.route";
import { bankRouter } from "./routes/bank.route";
import { adminCARouter } from "./routes/admin-ca.route";
import { errorHandler } from "./middleware/errorHandler";

// Start the BullMQ mail worker
import "./workers/mail.worker";

const app = express();

app.use(express.json());
app.use(cors());
app.use(helmet());
app.use(morgan("dev"));

otpRouter(app); // /v1/otp/*
authRouter(app); // /v1/auth/*
bankRouter(app); // /v1/bank/*
adminCARouter(app); // /v1/admin-ca/*


app.use(errorHandler);

app.listen(ENV.PORT, () => {
  console.log("API Gateway is running on port " + ENV.PORT);
});
