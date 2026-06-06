import express from "express";
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";

import ENV from "./config/env";
import { authRouter } from "./routes/auth.route";
import { errorHandler } from "./middleware/errorHandler";

const app = express();

app.use(express.json());
app.use(cors());
app.use(helmet());
app.use(morgan("dev"));

authRouter(app);

app.use(errorHandler);

app.listen(ENV.PORT, () => {
  console.log("API Gateway is running on port " + ENV.PORT);
});
