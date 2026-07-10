import crypto from "crypto";
import jwt from "jsonwebtoken";
import ENV from "../config/env";
import { CertStatus, IdentityRole } from "../proto/ca";
import { caGrpcError } from "../middleware/errorHandler";
import {
  recordRaAudit,
  registerUser,
  verifyCertificate,
} from "./ca.service";

const CHALLENGE_PREFIX = "admin-ca-login";
const CHALLENGE_MAX_AGE_SECONDS = 5 * 60;

export interface IssueCaAdminCertificateInput {
  csrPem: string;
  adminId: string;
  subjectEmail: string;
  fullName: string;
}

export const issueCaAdminCertificate = (
  input: IssueCaAdminCertificateInput,
) =>
  registerUser({
    csrPem: input.csrPem,
    ownerId: input.adminId,
    subjectEmail: input.subjectEmail,
    fullName: input.fullName,
    role: IdentityRole.IDENTITY_ROLE_CA_ADMIN,
  });

export interface CreateCaAdminSessionInput {
  certSerial: string;
  challenge: string;
  signature: string;
  requestId?: string;
}

export interface CaAdminSession {
  token: string;
  email: string;
  certSerial: string;
  ownerId: string;
  expiresIn: number;
}

const parseChallenge = (challenge: string, certSerial: string) => {
  const parts = challenge.split(":");
  if (parts.length !== 4 || parts[0] !== CHALLENGE_PREFIX) {
    throw new Error("ADMIN_CA_CHALLENGE_INVALID");
  }
  const [, serial, requestId, issuedAtRaw] = parts;
  const issuedAt = Number(issuedAtRaw);
  const now = Math.floor(Date.now() / 1000);
  if (serial !== certSerial || !requestId || !Number.isInteger(issuedAt)) {
    throw new Error("ADMIN_CA_CHALLENGE_INVALID");
  }
  if (Math.abs(now - issuedAt) > CHALLENGE_MAX_AGE_SECONDS) {
    throw new Error("ADMIN_CA_CHALLENGE_EXPIRED");
  }
};

const verifySignature = (
  publicKeyPem: string,
  challenge: string,
  signatureBase64: string,
) => {
  const signature = Buffer.from(signatureBase64, "base64");
  if (signature.length < 128) {
    throw new Error("ADMIN_CA_SIGNATURE_INVALID");
  }
  const verifier = crypto.createVerify("RSA-SHA256");
  verifier.update(challenge, "utf8");
  verifier.end();
  if (!verifier.verify(publicKeyPem, signature)) {
    throw new Error("ADMIN_CA_SIGNATURE_INVALID");
  }
};

export async function createCaAdminSession(
  input: CreateCaAdminSessionInput,
): Promise<CaAdminSession> {
  const certSerial = input.certSerial.trim().toLowerCase();
  parseChallenge(input.challenge, certSerial);

  let cert;
  try {
    cert = await verifyCertificate({
      serialNumber: certSerial,
      caller: "api-gateway:admin-ca-session",
      includeCertificatePem: false,
      includePublicKeyPem: true,
    });
  } catch (err: any) {
    throw caGrpcError(err);
  }

  if (cert.status !== CertStatus.CERT_STATUS_ACTIVE) {
    throw new Error("ADMIN_CA_CERT_NOT_ACTIVE");
  }
  if (cert.role !== IdentityRole.IDENTITY_ROLE_CA_ADMIN) {
    throw new Error("ADMIN_CA_CERT_ROLE_REQUIRED");
  }
  if (!cert.publicKeyPem) {
    throw new Error("ADMIN_CA_CERT_PUBLIC_KEY_MISSING");
  }

  verifySignature(cert.publicKeyPem, input.challenge, input.signature);

  const expiresIn = 8 * 60 * 60;
  const token = jwt.sign(
    {
      sub: cert.subjectEmail,
      email: cert.subjectEmail,
      role: "admin-ca",
      purpose: "admin-ca",
      cert_serial: certSerial,
      owner_id: cert.ownerId,
    },
    ENV.GATEWAY_JWT_SECRET,
    { expiresIn },
  );

  void recordRaAudit(
    {
      action: "admin_ca_login_success",
      serialNumber: certSerial,
      performedBy: `admin-ca:${cert.subjectEmail}`,
      metadata: {
        email: cert.subjectEmail,
        owner_id: cert.ownerId,
        request_id: input.requestId ?? "",
        method: "certificate",
      },
    },
    input.requestId,
  );

  return {
    token,
    email: cert.subjectEmail,
    certSerial,
    ownerId: cert.ownerId,
    expiresIn,
  };
}
