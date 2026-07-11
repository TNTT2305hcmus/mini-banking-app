import { NextFunction, Request, Response } from "express";
import jwt from "jsonwebtoken";
import redis, { RedisKeys } from "../config/ioredis";
import ENV from "../config/env";
import {
  getCertificateDetail,
  listCaAuditEvents,
  listCertificates,
  recordRaAudit,
  registerUser,
  revokeCertificate,
} from "../services/ca.service";
import {
  createCaAdminSession,
  issueCaAdminCertificate,
} from "../services/admin-ca.service";
import {
  AdminActivationError,
  getPendingAdminByToken,
  markAdminActivated,
} from "../services/admin-activation.service";
import { IDENTITY_ROLES } from "../models/bank-admin";
import { checkUserEmail, createUserBankAccount } from "../services/bank.service";
import { CertificateMetadata, CertStatus, IdentityRole } from "../proto/ca";
import { enrichAudit } from "../lib/audit-semantics";
import {
  caAdminGrpcError,
  caGrpcError,
  bankGrpcError,
  httpError,
} from "../middleware/errorHandler";
import z from "zod";

const CA_ACTIONS = [
  "issuer_provisioned",
  "issued",
  "revoked",
  "looked_up",
  "verify_certificate",
  "chain_verified",
  "ra_otp_requested",
  "ra_otp_verified",
  "ra_otp_failed",
  "ra_registration_approved",
  "ra_registration_rejected",
  "admin_ca_login_success",
  "admin_ca_login_failed",
] as const;

const isoToUnix = z
  .string()
  .optional()
  .transform((value, ctx) => {
    if (value === undefined || value === "") return 0;
    const millis = Date.parse(value);
    if (Number.isNaN(millis)) {
      ctx.addIssue({ code: "custom", message: "must be an ISO 8601 datetime" });
      return z.NEVER;
    }
    return Math.floor(millis / 1000);
  });

const CaAuditQuerySchema = z.object({
  action: z.enum(CA_ACTIONS).optional(),
  serial: z.string().optional(),
  performed_by: z.string().optional(),
  request_id: z.string().optional(),
  limit: z.coerce.number().int().min(1).max(100).default(20),
  offset: z.coerce.number().int().min(0).default(0),
  from: isoToUnix,
  to: isoToUnix,
});

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

const normalizeQueryString = (value: unknown) => String(value ?? "").trim();

const mapCertificate = (cert: CertificateMetadata) => ({
  serial: cert.serialNumber,
  cert_type: cert.certType,
  issuer_id: cert.issuerId,
  issuer_common_name: cert.issuerCommonName,
  issuer_serial_number: cert.issuerSerialNumber,
  owner_id: cert.ownerId,
  cn: cert.subjectCn,
  email: cert.subjectEmail,
  chain_pem: cert.chainPem,
  chain_fingerprints: cert.chainFingerprints,
  is_ca: cert.isCa,
  key_usage: cert.keyUsage,
  extended_key_usage: cert.extendedKeyUsage,
  fingerprint: cert.fingerprintSha256,
  status: statusFromProto(cert.status),
  not_before: cert.notBeforeUnix,
  not_after: cert.notAfterUnix,
  issued_at: cert.issuedAtUnix,
  revoked_at: cert.revokedAtUnix || null,
  revocation_reason: cert.revocationReason || null,
});

const recordRegistrationRejected = (
  requestId: string,
  ownerId: string,
  email: string,
  reason: string,
) =>
  recordRaAudit(
    {
      action: "ra_registration_rejected",
      performedBy: "ra:register",
      reason,
      metadata: { owner_id: ownerId, email, request_id: requestId },
    },
    requestId,
  );

const caActivationError = (err: any) => {
  if (err instanceof AdminActivationError) {
    return { status: err.status, code: err.code, message: err.message };
  }
  if (typeof err?.status === "number" && err?.error_code) {
    return { status: err.status, code: err.error_code, message: err.message };
  }
  const mapped = caGrpcError(err);
  return {
    status: mapped.status,
    code: mapped.error_code,
    message: mapped.message,
  };
};

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

  let existingUser;
  try {
    existingUser = await checkUserEmail({ subjectEmail }, m.request_id);
  } catch (bankErr: any) {
    const mappedBankErr = bankGrpcError(bankErr);
    void recordRegistrationRejected(
      m.request_id,
      ownerId,
      subjectEmail,
      "bank_email_check_failed",
    );
    return next(
      httpError(
        mappedBankErr.status,
        mappedBankErr.error_code,
        mappedBankErr.message,
      ),
    );
  }

  if (existingUser.exists) {
    void recordRegistrationRejected(
      m.request_id,
      ownerId,
      subjectEmail,
      "email_already_registered",
    );
    return next(
      httpError(409, "EMAIL_ALREADY_REGISTERED", "Email is already registered"),
    );
  }

  let caResp;
  try {
    caResp = await registerUser(
      {
        csrPem,
        ownerId,
        subjectEmail,
        fullName,
        role: IdentityRole.IDENTITY_ROLE_CUSTOMER,
      } as any,
      m.request_id as string,
    );
  } catch (err: any) {
    void recordRegistrationRejected(
      m.request_id,
      ownerId,
      subjectEmail,
      err?.details ?? err?.message ?? "registration_failed",
    );
    return next(caGrpcError(err));
  }

  try {
    await createUserBankAccount(
      {
        userId: ownerId,
        subjectEmail,
        fullName,
      },
      m.request_id,
    );
  } catch (bankErr: any) {
    const mappedBankErr = bankGrpcError(bankErr);
    const rollbackReason =
      mappedBankErr.error_code === "ALREADY_EXISTS"
        ? "email_already_registered"
        : "bank_create_user_failed";

    try {
      await revokeCertificate(
        {
          serialNumber: caResp.serialNumber,
          reason: "registration_rollback",
          performedBy: "ra:register",
        },
        m.request_id,
      );
    } catch (revokeErr) {
      console.warn(
        `[REGISTER] failed to revoke cert ${caResp.serialNumber} after bank rollback:`,
        (revokeErr as Error)?.message ?? revokeErr,
      );
    }

    void recordRegistrationRejected(
      m.request_id,
      ownerId,
      subjectEmail,
      rollbackReason,
    );

    if (mappedBankErr.error_code === "ALREADY_EXISTS") {
      return next(
        httpError(
          409,
          "EMAIL_ALREADY_REGISTERED",
          "Email is already registered",
        ),
      );
    }
    return next(
      httpError(
        mappedBankErr.status,
        mappedBankErr.error_code,
        mappedBankErr.message,
      ),
    );
  }

  const ttlSeconds =
    typeof payload.exp === "number"
      ? Math.max(1, payload.exp - Math.floor(Date.now() / 1000))
      : 600;
  try {
    await redis.set(jtiKey, "1", "EX", ttlSeconds);
  } catch (err) {
    console.warn(
      `[REGISTER] failed to mark registration token ${payload.jti} as used:`,
      (err as Error)?.message ?? err,
    );
  }

  // RA event: the gateway vetted the request and the CA issued a certificate.
  void recordRaAudit(
    {
      action: "ra_registration_approved",
      serialNumber: caResp.serialNumber,
      performedBy: "ra:register",
      metadata: { owner_id: ownerId, email: subjectEmail, request_id: m.request_id },
    },
    m.request_id,
  );

  return res.status(201).json({
    success: true,
    message: "X.509 certificate issued",
    data: {
      cert_pem: caResp.certificatePem,
      cert_serial: caResp.serialNumber,
      issued_at: Math.floor(Date.now() / 1000),
      expires_at: caResp.notAfterUnix,
      // Chuỗi cert (Client CA) để client verify cert cấp về Root CA nhúng (fail-closed).
      chain_pem: caResp.chainPem,
    },
    ...m,
  });
};

export const handleAdminCaActivate = async (req: Request, res: Response) => {
  const m = meta(req);
  try {
    const identity = await getPendingAdminByToken(
      req.body.activation_token,
      IDENTITY_ROLES.CA_ADMIN,
    );
    const issued = await issueCaAdminCertificate({
      csrPem: req.body.csr_pem,
      adminId: identity.admin_id,
      subjectEmail: identity.email,
      fullName: identity.full_name,
    });
    const active = await markAdminActivated({
      identity,
      certSerial: issued.serialNumber,
    });

    return res.status(201).json({
      success: true,
      message: "CA Admin certificate issued",
      data: {
        cert_pem: issued.certificatePem,
        cert_serial: issued.serialNumber,
        issued_at: issued.notBeforeUnix,
        expires_at: issued.notAfterUnix,
        admin_id: active.admin_id,
        email: active.email,
        full_name: active.full_name,
        role: active.role,
      },
      ...m,
    });
  } catch (err: any) {
    const mapped = caActivationError(err);
    return res.status(mapped.status).json({
      success: false,
      error_code: mapped.code,
      message: mapped.message,
      ...m,
    });
  }
};

export const handleAdminCaCertificateSession = async (
  req: Request,
  res: Response,
) => {
  const m = meta(req);
  try {
    const session = await createCaAdminSession({
      certSerial: req.body.cert_serial,
      challenge: req.body.challenge,
      signature: req.body.signature,
      requestId: m.request_id,
    });

    return res.status(200).json({
      success: true,
      data: {
        token: session.token,
        token_type: "Bearer",
        role: "admin-ca",
        email: session.email,
        cert_serial: session.certSerial,
        owner_id: session.ownerId,
        expires_in: session.expiresIn,
      },
      ...m,
    });
  } catch (err: any) {
    const rawMessage = typeof err?.message === "string" ? err.message : "";
    const code =
      err?.error_code ??
      (rawMessage.startsWith("ADMIN_CA_")
        ? rawMessage
        : "ADMIN_CA_CERT_LOGIN_FAILED");
    const status = typeof err?.status === "number" ? err.status : 401;
    void recordRaAudit(
      {
        action: "admin_ca_login_failed",
        serialNumber: String(req.body?.cert_serial ?? ""),
        performedBy: "admin-ca:certificate",
        reason: code,
        metadata: { request_id: m.request_id, method: "certificate" },
      },
      m.request_id,
    );
    return res.status(status).json({
      success: false,
      error_code: code,
      message: "Admin CA certificate login failed",
      ...m,
    });
  }
};

export const handleAdminListCertificates = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const status = String(req.query.status ?? "")
    .trim()
    .toLowerCase();
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
  const certType = normalizeQueryString(req.query.cert_type).toLowerCase();
  const normalizedCertType = certType === "all" ? "" : certType;
  const allowedCertTypes = new Set([
    "",
    "root_ca",
    "intermediate_ca",
    "service_tls",
    "client",
  ]);

  if (!allowedCertTypes.has(normalizedCertType)) {
    return next(
      httpError(
        400,
        "INVALID_CERT_TYPE",
        "cert_type must be one of: all, root_ca, intermediate_ca, service_tls, client",
      ),
    );
  }

  try {
    const caResp = await listCertificates(
      {
        status: normalizedStatus,
        certType: normalizedCertType,
        issuerId: normalizeQueryString(req.query.issuer_id),
        ownerId: normalizeQueryString(req.query.owner_id),
        subjectEmail: normalizeQueryString(req.query.email),
        serialNumber: normalizeQueryString(req.query.serial),
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
    return next(
      httpError(400, "REVOCATION_REASON_REQUIRED", "reason is required"),
    );
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

export const handleListCaAudit = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = meta(req).request_id;

  const parsed = CaAuditQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: parsed.error.issues[0].message,
      request_id: requestId,
    });
  }
  const q = parsed.data;
  if (q.from > 0 && q.to > 0 && q.to < q.from) {
    return res.status(400).json({
      success: false,
      error_code: "INVALID_REQUEST",
      message: "to must not be before from",
      request_id: requestId,
    });
  }

  try {
    const result = await listCaAuditEvents(
      {
        action: q.action ?? "",
        serialNumber: q.serial ?? "",
        performedByFilter: q.performed_by ?? "",
        requestId: q.request_id ?? "",
        fromUnix: q.from,
        toUnix: q.to,
        limit: q.limit,
        offset: q.offset,
        performedBy: adminPerformedBy(res),
      },
      requestId,
    );
    return res.json({
      success: true,
      data: {
        items: result.events.map((event) => ({
          serial_number: event.serialNumber,
          cert_type: event.certType,
          issuer_id: event.issuerId,
          action: event.action,
          performed_by: event.performedBy,
          reason: event.reason,
          performed_at: new Date(event.performedAtUnix * 1000).toISOString(),
          metadata: event.metadata,
          ...enrichAudit({
            source: "ca",
            action: event.action,
            actor: event.performedBy,
            reason: event.reason,
            target: event.serialNumber,
          }),
        })),
        total: result.total,
        limit: result.limit,
        offset: result.offset,
      },
      request_id: requestId,
      timestamp: new Date().toISOString(),
    });
  } catch (err) {
    return next(err);
  }
};
