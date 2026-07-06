import dotenv from "dotenv";
import { z } from "zod";

dotenv.config();

const envSchema = z.object({
  PORT: z.string().min(1, "PORT is required").default("3000"),
  FRONTEND_BASE_URL: z.string().url().default("http://localhost:5173"),
  GATEWAY_REDIS_URL: z.string().min(1, "GATEWAY_REDIS_URL is required"),

  CA_CERT_PATH: z.string().min(1, "CA_CERT_PATH is required"),
  ADMIN_CA_DEMO_EMAIL: z.email().default("ADMIN_CA_DEMO_EMAIL is required"),
  ADMIN_CA_DEMO_PASSWORD: z.string().min(1).default("ADMIN_CA_DEMO_PASSWORD is required"),
  ADMIN_CA_DEMO_TOKEN: z.string().min(1).default("ADMIN_CA_DEMO_TOKEN is required"),

  EMAIL_USER: z.string().min(1, "EMAIL_USER is required"),
  EMAIL_PASS: z.string().min(1, "EMAIL_PASS is required"),

  OTP_MAX_ATTEMPTS: z.coerce.number(),
  OTP_EXPIRES_IN: z.coerce.number(),
  OTP_COOLDOWN: z.coerce.number(),

  GATEWAY_OTP_SECRET: z.string().min(1, "GATEWAY_OTP_SECRET is required"),
  GATEWAY_JWT_SECRET: z.string().min(1, "GATEWAY_JWT_SECRET is required"),

  CA_GRPC_ADDR: z.string().default("localhost:50051"),
  KDC_GRPC_ADDR: z.string().default("localhost:50052"),
  BANK_GRPC_ADDR: z.string().default("localhost:50053"),

  SMTP_HOST: z.string().default("smtp.gmail.com"),
  SMTP_PORT: z.coerce.number().default(587),
});

const ENV = envSchema.parse(process.env);

export default ENV;
