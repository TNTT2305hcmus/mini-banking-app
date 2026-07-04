import { Request, Response } from "express";
import jwt from "jsonwebtoken";
import { status as grpcStatus } from "@grpc/grpc-js";
import redis, { RedisKeys } from "../config/ioredis";
import ENV from "../config/env";
import { registerUser } from "../services/ca.service";
import { createUserBankAccount } from "../services/bank.service";

const meta = (req: Request) => ({
  request_id: req.headers["x-request-id"] as string,
  timestamp: new Date().toISOString(),
});

const caHttpError = (err: any) => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "CA service unavailable";
  switch (code) {
    case grpcStatus.ALREADY_EXISTS:
      return {
        status: 409,
        error_code: "IDENTITY_ALREADY_REGISTERED",
        message: msg,
      };
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: "INVALID_CSR_FORMAT", message: msg };
    case grpcStatus.NOT_FOUND:
      return { status: 404, error_code: "CERT_NOT_FOUND", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 403, error_code: "CERT_NOT_OWNED", message: msg };
    default:
      return {
        status: 502,
        error_code: "CA_SERVICE_UNAVAILABLE",
        message: msg,
      };
  }
};

// ── POST /v1/pki/register ─────────────────────────────────────────────────────

export const handleRegister = async (req: Request, res: Response) => {
  const m = meta(req);
  const authHeader = req.headers["authorization"] ?? "";
  const token = authHeader.startsWith("Bearer ") ? authHeader.slice(7) : "";

  // 1. Verify JWT — check signature + expiry
  let payload: any;
  try {
    payload = jwt.verify(token, ENV.GATEWAY_JWT_SECRET);
  } catch (e: any) {
    const expired = e?.name === "TokenExpiredError";
    return res.status(401).json({
      success: false,
      error_code: expired ? "REG_TOKEN_EXPIRED" : "REG_TOKEN_INVALID",
      message: e.message,
      ...m,
    });
  }

  // 2. Validate purpose claim
  if (payload.purpose !== "pki_registration") {
    return res.status(401).json({
      success: false,
      error_code: "REG_TOKEN_INVALID",
      message: "Invalid token purpose",
      ...m,
    });
  }

  // 3. Single-use check
  const jtiKey = RedisKeys.REG_TOKEN + payload.jti;
  const jtiState = await redis.get(jtiKey);
  if (jtiState === "1") {
    return res.status(401).json({
      success: false,
      error_code: "REG_TOKEN_USED",
      message: "Registration token has already been used",
      ...m,
    });
  }

  // 4. Identity comes from the verified registration token, not from client body.
  const { csrPem, fullName } = req.body;
  const subjectEmail =
    typeof payload.sub === "string" ? payload.sub.toLowerCase() : "";
  const ownerId = typeof payload.owner_id === "string" ? payload.owner_id : "";

  if (!subjectEmail || !ownerId) {
    return res.status(401).json({
      success: false,
      error_code: "REG_TOKEN_INVALID",
      message: "Registration token is missing subject email or owner id",
      ...m,
    });
  }

  // 5. Consume token BEFORE gRPC call — prevents double-submit race condition
  await redis.set(jtiKey, "1");

  // 6. Forward to CA service
  try {
    // Truyền requestId vào để set Header
    const caResp = await registerUser({
      csrPem,
      ownerId,
      subjectEmail,
      fullName,
    } as any);

    await createUserBankAccount({
      userId: ownerId,
      subjectEmail,
      fullName,
    });

    return res.status(201).json({
      success: true,
      message: "X.509 certificate issued",
      data: {
        cert_pem: caResp.certificatePem,
        cert_serial: caResp.serialNumber,
        issued_at: Math.floor(Date.now() / 1000),
        expires_at: caResp.notAfterUnix,
      },
      ...m,
    });
  } catch (err: any) {
    const { status, error_code, message } = caHttpError(err);
    return res
      .status(status)
      .json({ success: false, error_code, message, ...m });
  }
};
