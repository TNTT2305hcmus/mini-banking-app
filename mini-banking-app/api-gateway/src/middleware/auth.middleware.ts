import { Request, Response, NextFunction } from "express";
import { z } from "zod";

const CLOCK_SKEW = 300;
const base64Re = /^[A-Za-z0-9+/]*={0,2}$/;
const hexRe = /^[0-9A-Fa-f]+$/;

const is16Bytes = (b64: string) => {
  try {
    return Buffer.from(b64, "base64").length === 16;
  } catch {
    return false;
  }
};

const ASRequestSchema = z.object({
  clientId: z.uuid("Invalid client ID"),
  nonce: z
    .string()
    .regex(base64Re, "nonce1 must be valid base64")
    .refine(is16Bytes, "nonce1 must decode to exactly 16 bytes"),
  timestamp: z.number().int(),
  certSn: z.string().regex(hexRe, "cert_sn must be a hex string"),
  preAuthSignature: z
    .string()
    .regex(base64Re, "pre_auth_signature must be valid base64"),
});

const TGSRequestSchema = z.object({
  serviceId: z.literal("bank-service"),
  tgtCiphertext: z
    .string()
    .regex(base64Re, "tgt_ciphertext must be valid base64"),
  authenticator: z
    .string()
    .regex(base64Re, "authenticator must be valid base64"),
  certSn: z.string().regex(hexRe, "cert_sn must be a hex string"),
  nonce: z
    .string()
    .regex(base64Re, "nonce2 must be valid base64")
    .refine(is16Bytes, "nonce2 must decode to exactly 16 bytes"),
  requestedScope: z.enum(["transfer:create", "balance:read", "history:read"]),
});

export const validateASRequest = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = ASRequestSchema.safeParse(req.body);
  if (!result.success) {
    const err = result.error.issues[0];
    const field = err.path[0] as string;
    return res.status(400).json({
      success: false,
      error_code: field === "nonce1" ? "INVALID_NONCE" : "INVALID_REQUEST",
      message: err.message,
    });
  }

  const now = Math.floor(Date.now() / 1000);
  if (Math.abs(now - result.data.timestamp) > CLOCK_SKEW) {
    return res.status(408).json({
      success: false,
      error_code: "REQUEST_EXPIRED",
      message: "Request timestamp is outside the allowed ±5 minute window",
    });
  }

  next();
};

export const validateTGSRequest = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = TGSRequestSchema.safeParse(req.body);
  if (!result.success) {
    const err = result.error.issues[0];
    const field = err.path[0] as string;
    let code = "INVALID_REQUEST";
    if (field === "tgt_ciphertext") code = "INVALID_TGT_FORMAT";
    if (field === "authenticator") code = "INVALID_AUTH_C";
    return res.status(400).json({
      success: false,
      error_code: code,
      message: err.message,
    });
  }
  next();
};
