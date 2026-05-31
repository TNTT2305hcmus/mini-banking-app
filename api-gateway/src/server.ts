import express from "express";

// Middleware
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";

// Config
import ENV from "./config/env";

const app = express();

app.use(express.json());
app.use(cors());
app.use(helmet());
app.use(morgan("dev"));

app.get("/", (req, res) => {
  res.send("Hello from API Gateway!");
});

app.listen(ENV.PORT, () => {
  console.log("API Gateway is running on port " + ENV.PORT);
});
