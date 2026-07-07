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
import { createUserBankAccount } from "../services/bank.service";
import { CertificateMetadata, CertStatus, IdentityRole } from "../proto/ca";
import { enrichAudit } from "../lib/audit-semantics";
import {
  caAdminGrpcError,
  caGrpcError,
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
    const caResp = await registerUser(
      {
        csrPem,
        ownerId,
        subjectEmail,
        fullName,
        role: IdentityRole.IDENTITY_ROLE_CUSTOMER,
      } as any,
      m.request_id as string,
    );

    await createUserBankAccount({
      userId: ownerId,
      subjectEmail,
      fullName,
    });

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
      },
      ...m,
    });
  } catch (err: any) {
    // RA event: registration could not be completed (CA/bank rejected it).
    void recordRaAudit(
      {
        action: "ra_registration_rejected",
        performedBy: "ra:register",
        reason: err?.details ?? err?.message ?? "registration_failed",
        metadata: { owner_id: ownerId, email: subjectEmail, request_id: m.request_id },
      },
      m.request_id,
    );
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

  if (
    email !== ENV.ADMIN_CA_DEMO_EMAIL ||
    password !== ENV.ADMIN_CA_DEMO_PASSWORD
  ) {
    void recordRaAudit(
      {
        action: "admin_ca_login_failed",
        performedBy: `admin-ca:${email || "unknown"}`,
        reason: "invalid_credentials",
        metadata: { email, request_id: meta(req).request_id },
      },
      meta(req).request_id,
    );
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

  // Admin authentication event, recorded in the CA audit domain.
  void recordRaAudit(
    {
      action: "admin_ca_login_success",
      performedBy: `admin-ca:${email}`,
      metadata: { email, request_id: meta(req).request_id },
    },
    meta(req).request_id,
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
