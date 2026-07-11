import dotenv from "dotenv";
import { z } from "zod";

dotenv.config();

const booleanFlag = z
  .preprocess(
    (value) => value === undefined ? "0" : String(value).toLowerCase(),
    z.enum(["0", "1", "true", "false"]),
  )
  .transform((value) => value === "1" || value === "true");

const optionalNonEmpty = z.preprocess(
  (value) => value === undefined || String(value).trim() === "" ? undefined : String(value).trim(),
  z.string().optional(),
);

const envSchema = z.object({
  PORT: z.string().min(1, "PORT is required").default("3000"),
  FRONTEND_BASE_URL: z.string().url().default("http://localhost:5173"),
  GATEWAY_REDIS_URL: z.string().min(1, "GATEWAY_REDIS_URL is required"),

  CA_CERT_PATH: z.string().min(1, "CA_CERT_PATH is required"),

  // Security Operations (SOC) admin. Optional so the gateway still boots when
  // unset — the /v1/admin-sec/auth login returns 503 until these are configured
  // (no fake defaults, so a missing value can never become a guessable token).
  ADMIN_SEC_DEMO_EMAIL: z.email().optional(),
  ADMIN_SEC_DEMO_PASSWORD: z.string().min(1).optional(),
  ADMIN_SEC_DEMO_TOKEN: z.string().min(1).optional(),

  EMAIL_USER: z.string().min(1, "EMAIL_USER is required"),
  EMAIL_PASS: z.string().min(1, "EMAIL_PASS is required"),

  OTP_MAX_ATTEMPTS: z.coerce.number(),
  OTP_EXPIRES_IN: z.coerce.number(),
  OTP_COOLDOWN: z.coerce.number(),
  DEMO_OTP: optionalNonEmpty.pipe(
    z.string().regex(/^[0-9]{6}$/, "DEMO_OTP must be exactly 6 digits").optional(),
  ),

  RATE_LIMIT_DISABLED: booleanFlag,
  RATE_LIMIT_AS_WINDOW_SECONDS: z.coerce.number().int().positive().default(300),
  RATE_LIMIT_AS_MAX: z.coerce.number().int().positive().default(10),
  RATE_LIMIT_TGS_WINDOW_SECONDS: z.coerce.number().int().positive().default(300),
  RATE_LIMIT_TGS_MAX: z.coerce.number().int().positive().default(10),
  RATE_LIMIT_BANK_WINDOW_SECONDS: z.coerce.number().int().positive().default(60),
  RATE_LIMIT_BANK_MAX: z.coerce.number().int().positive().default(20),
  RATE_LIMIT_OTP_WINDOW_SECONDS: z.coerce.number().int().positive().default(600),
  RATE_LIMIT_OTP_MAX: z.coerce.number().int().positive().default(3),

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
