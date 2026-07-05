import { NextFunction, Request, Response } from "express";
import jwt from "jsonwebtoken";
import redis, { RedisKeys } from "../config/ioredis";
import ENV from "../config/env";
import {
  getCertificateDetail,
  listCertificates,
  registerUser,
  revokeCertificate,
} from "../services/ca.service";
import { createUserBankAccount } from "../services/bank.service";
import { CertificateMetadata, CertStatus } from "../proto/ca";
import {
  caAdminGrpcError,
  caGrpcError,
  httpError,
} from "../middleware/errorHandler";

const meta = (req: Request) => ({
  request_id: req.headers["x-request-id"] as string,
  timestamp: new Date().toISOString(),
});

const adminPerformedBy = (res: Response) =>
  (res.locals.adminCa?.performedBy as string | undefined) ?? "admin-ca:unknown";

const normalizeLimit = (value: unknown) => {
  const parsed = Number(value ?? 20);
  if (!Number.isInteger(parsed) || parsed < 1) {
    return 20;
  }
  return Math.min(parsed, 100);
};

const normalizeOffset = (value: unknown) => {
  const parsed = Number(value ?? 0);
  if (!Number.isInteger(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
};

const statusFromProto = (status: CertStatus) => {
  switch (status) {
    case CertStatus.CERT_STATUS_ACTIVE:
      return "active";
    case CertStatus.CERT_STATUS_REVOKED:
      return "revoked";
    case CertStatus.CERT_STATUS_EXPIRED:
      return "expired";
    default:
      return "unknown";
  }
};

const mapCertificate = (cert: CertificateMetadata) => ({
  serial: cert.serialNumber,
  owner_id: cert.ownerId,
  cn: cert.subjectCn,
  email: cert.subjectEmail,
  fingerprint: cert.fingerprintSha256,
  status: statusFromProto(cert.status),
  not_before: cert.notBeforeUnix,
  not_after: cert.notAfterUnix,
  issued_at: cert.issuedAtUnix,
  revoked_at: cert.revokedAtUnix || null,
  revocation_reason: cert.revocationReason || null,
});

export const handleRegister = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const m = meta(req);
  const authHeader = req.headers["authorization"] ?? "";
  const token = authHeader.startsWith("Bearer ") ? authHeader.slice(7) : "";

  let payload: any;
  try {
    payload = jwt.verify(token, ENV.GATEWAY_JWT_SECRET);
  } catch (e: any) {
    const expired = e?.name === "TokenExpiredError";
    return next(
      httpError(
        401,
        expired ? "REG_TOKEN_EXPIRED" : "REG_TOKEN_INVALID",
        e.message,
      ),
    );
  }

  if (payload.purpose !== "pki_registration") {
    return next(httpError(401, "REG_TOKEN_INVALID", "Invalid token purpose"));
  }

  const jtiKey = RedisKeys.REG_TOKEN + payload.jti;
  const jtiState = await redis.get(jtiKey);
  if (jtiState === "1") {
    return next(
      httpError(
        401,
        "REG_TOKEN_USED",
        "Registration token has already been used",
      ),
    );
  }

  const { csrPem, fullName } = req.body;
  const subjectEmail =
    typeof payload.sub === "string" ? payload.sub.toLowerCase() : "";
  const ownerId = typeof payload.owner_id === "string" ? payload.owner_id : "";

  if (!subjectEmail || !ownerId) {
    return next(
      httpError(
        401,
        "REG_TOKEN_INVALID",
        "Registration token is missing subject email or owner id",
      ),
    );
  }

  await redis.set(jtiKey, "1");

  try {
    const caResp = await registerUser({
      csrPem,
      ownerId,
      subjectEmail,
      fullName,
    } as any, m.request_id);

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
    return next(caGrpcError(err));
  }
};

export const handleAdminAuth = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const email = String(req.body?.email ?? ENV.ADMIN_CA_DEMO_EMAIL)
    .trim()
    .toLowerCase();
  const password = String(req.body?.password ?? "");

  if (email !== ENV.ADMIN_CA_DEMO_EMAIL || password !== ENV.ADMIN_CA_DEMO_PASSWORD) {
    return next(
      httpError(401, "ADMIN_CA_LOGIN_FAILED", "Invalid admin-ca credentials"),
    );
  }

  const token = jwt.sign(
    {
      sub: email,
      email,
      role: "admin-ca",
      purpose: "admin-ca",
    },
    ENV.GATEWAY_JWT_SECRET,
    { expiresIn: "8h" },
  );

  return res.status(200).json({
    success: true,
    data: {
      token,
      token_type: "Bearer",
      role: "admin-ca",
      email,
      expires_in: 8 * 60 * 60,
    },
    ...meta(req),
  });
};

export const handleAdminListCertificates = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const status = String(req.query.status ?? "").trim().toLowerCase();
  const normalizedStatus = status === "all" ? "" : status;
  const allowedStatuses = new Set(["", "active", "revoked", "expired"]);

  if (!allowedStatuses.has(normalizedStatus)) {
    return next(
      httpError(
        400,
        "INVALID_CERT_STATUS",
        "status must be one of: all, active, revoked, expired",
      ),
    );
  }

  const limit = normalizeLimit(req.query.limit);
  const offset = normalizeOffset(req.query.offset);

  try {
    const caResp = await listCertificates(
      {
        status: normalizedStatus,
        ownerId: String(req.query.owner_id ?? "").trim(),
        subjectEmail: String(req.query.email ?? "").trim(),
        serialNumber: String(req.query.serial ?? "").trim(),
        limit,
        offset,
        performedBy: adminPerformedBy(res),
      },
      meta(req).request_id,
    );

    return res.status(200).json({
      success: true,
      data: {
        items: caResp.certificates.map(mapCertificate),
        total: caResp.total,
        limit: caResp.limit,
        offset: caResp.offset,
      },
      ...meta(req),
    });
  } catch (err: any) {
    return next(caAdminGrpcError(err));
  }
};

export const handleAdminGetCertificateDetail = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const serial = String(req.params.serial ?? "").trim();
  if (!serial) {
    return next(
      httpError(400, "CERT_SERIAL_REQUIRED", "Certificate serial is required"),
    );
  }

  try {
    const caResp = await getCertificateDetail(
      {
        serialNumber: serial,
        performedBy: adminPerformedBy(res),
      },
      meta(req).request_id,
    );

    if (!caResp.certificate) {
      return next(httpError(404, "CERT_NOT_FOUND", "Certificate not found"));
    }

    return res.status(200).json({
      success: true,
      data: mapCertificate(caResp.certificate),
      ...meta(req),
    });
  } catch (err: any) {
    return next(caAdminGrpcError(err));
  }
};

export const handleAdminRevokeCertificate = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const serial = String(req.params.serial ?? "").trim();
  const reason = String(req.body?.reason ?? "").trim();

  if (!serial) {
    return next(
      httpError(400, "CERT_SERIAL_REQUIRED", "Certificate serial is required"),
    );
  }
  if (!reason) {
    return next(httpError(400, "REVOCATION_REASON_REQUIRED", "reason is required"));
  }

  try {
    const caResp = await revokeCertificate(
      {
        serialNumber: serial,
        reason,
        performedBy: adminPerformedBy(res),
      },
      meta(req).request_id,
    );

    if (!caResp.certificate) {
      return next(httpError(404, "CERT_NOT_FOUND", "Certificate not found"));
    }

    return res.status(200).json({
      success: true,
      data: mapCertificate(caResp.certificate),
      ...meta(req),
    });
  } catch (err: any) {
    return next(caAdminGrpcError(err));
  }
};
