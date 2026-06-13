import { Request, Response, NextFunction } from "express";
import { z } from "zod";

const emailSchema = z.email().max(254).toLowerCase().trim();
const otpSchema = z
  .string()
  .regex(/^[0-9]{6}$/, "OTP must be exactly 6 digits");
const csrPemSchema = z
  .string()
  .startsWith(
    "-----BEGIN CERTIFICATE REQUEST-----",
    "csr_pem must be a PEM-encoded PKCS#10 CSR",
  );

const fail = (res: Response, code: string, msg: string, status = 400) =>
  res.status(status).json({ success: false, error_code: code, message: msg });

export const validateOtpRequest = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = z.object({ email: emailSchema }).safeParse(req.body);
  if (!result.success) {
    const err = result.error.issues[0];
    return fail(
      res,
      err.path[0] === "email" ? "INVALID_EMAIL" : "MISSING_REQUIRED_FIELD",
      err.message,
    );
  }
  req.body.email = result.data.email;
  next();
};

export const validateOtpVerify = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const result = z
    .object({ email: emailSchema, otp: otpSchema })
    .safeParse(req.body);
  if (!result.success) {
    const err = result.error.issues[0];
    const field = err.path[0] as string;
    const code =
      field === "otp" ? "INVALID_OTP_FORMAT" : "MISSING_REQUIRED_FIELD";
    return fail(res, code, err.message);
  }
  req.body.email = result.data.email;
  next();
};

export const validateRegister = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const schema = z.object({
    csrPem: csrPemSchema,
    idC: emailSchema,
    fullName: z.string().min(3, "Must be a valid name"),
  });
  const result = schema.safeParse(req.body);
  if (!result.success) {
    const err = result.error.issues[0];
    const field = err.path[0] as string;
    const code =
      field === "csr_pem" ? "INVALID_CSR_FORMAT" : "MISSING_REQUIRED_FIELD";
    return fail(res, code, err.message);
  }
  req.body = result.data;
  next();
};
