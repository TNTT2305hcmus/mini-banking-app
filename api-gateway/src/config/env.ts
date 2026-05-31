import dotenv from "dotenv";
import { z } from "zod";

dotenv.config();

const envSchema = z.object({
  PORT: z.string().min(1, "PORT is required").default("3000"),
  REDIS_URI: z.string().min(1, "REDIS_URI is required"),

  CA_CERT_PATH: z.string().min(1, "CA_CERT_PATH is required"),
  PRIVATE_KEY_PATH: z.string().min(1, "PRIVATE_KEY_PATH is required"),
  GATEWAY_CERT_PATH: z.string().min(1, "GATEWAY_CERT_PATH is required"),
});

const ENV = envSchema.parse(process.env);

export default ENV;
