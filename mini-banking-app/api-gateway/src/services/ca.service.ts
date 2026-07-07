import { Metadata } from "@grpc/grpc-js";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";
import {
  AppendAuditEventRequest,
  CAServiceClient,
  GetCertificateDetailRequest,
  GetCertificateDetailResponse,
  ListAuditEventsRequest,
  ListAuditEventsResponse,
  ListCertificatesRequest,
  ListCertificatesResponse,
  RegisterUserRequest,
  RegisterUserResponse,
  RevokeCertificateRequest,
  RevokeCertificateResponse,
  VerifyCertificateRequest,
  VerifyCertificateResponse,
} from "../proto/ca";

const caServiceClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);

const requestMetadata = (requestId?: string) => {
  const metadata = new Metadata();
  if (requestId) {
    metadata.set("x-request-id", requestId);
  }
  return metadata;
};

export const registerUser = (
  payload: RegisterUserRequest,
  requestId?: string,
): Promise<RegisterUserResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.registerUser(
      payload,
      requestMetadata(requestId),
      (err, res) => {
        if (err) {
          console.log(err);
          reject(err);
        } else resolve(res);
      },
    );
  });

export const verifyCertificate = (
  payload: VerifyCertificateRequest,
): Promise<VerifyCertificateResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.verifyCertificate(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const listCertificates = (
  payload: ListCertificatesRequest,
  requestId?: string,
): Promise<ListCertificatesResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.listCertificates(
      payload,
      requestMetadata(requestId),
      (err, res) => {
        if (err) reject(err);
        else resolve(res);
      },
    );
  });

export const getCertificateDetail = (
  payload: GetCertificateDetailRequest,
  requestId?: string,
): Promise<GetCertificateDetailResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.getCertificateDetail(
      payload,
      requestMetadata(requestId),
      (err, res) => {
        if (err) reject(err);
        else resolve(res);
      },
    );
  });

export const revokeCertificate = (
  payload: RevokeCertificateRequest,
  requestId?: string,
): Promise<RevokeCertificateResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.revokeCertificate(
      payload,
      requestMetadata(requestId),
      (err, res) => {
        if (err) reject(err);
        else resolve(res);
      },
    );
  });

const traceMetadata = (requestId?: string): Metadata => {
  const md = new Metadata();
  if (requestId) md.set("x-request-id", requestId);
  return md;
};

export const listCaAuditEvents = (
  payload: ListAuditEventsRequest,
  requestId?: string,
): Promise<ListAuditEventsResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.listAuditEvents(
      payload,
      traceMetadata(requestId),
      (err, res) => {
        if (err) return reject(err);
        resolve(res);
      },
    );
  });

// The Gateway acts as the PKI Registration Authority: it records OTP vetting,
// registration decisions and admin-ca logins into the CA audit trail. Only
// whitelisted ra_*/admin_ca_login_* actions are accepted by the CA.
const appendCaAuditEvent = (
  payload: AppendAuditEventRequest,
  requestId?: string,
): Promise<void> =>
  new Promise((resolve, reject) => {
    caServiceClient.appendAuditEvent(
      payload,
      traceMetadata(requestId),
      (err) => {
        if (err) return reject(err);
        resolve();
      },
    );
  });

// recordRaAudit is best-effort: an audit failure must never break the RA flow
// (OTP / registration / admin login), so errors are logged and swallowed.
export const recordRaAudit = async (
  event: {
    action: string;
    serialNumber?: string;
    performedBy: string;
    reason?: string;
    metadata?: Record<string, string>;
  },
  requestId?: string,
): Promise<void> => {
  try {
    await appendCaAuditEvent(
      {
        action: event.action,
        serialNumber: event.serialNumber ?? "",
        performedBy: event.performedBy,
        reason: event.reason ?? "",
        metadata: event.metadata ?? {},
      },
      requestId,
    );
  } catch (err) {
    console.warn(
      `[RA-AUDIT] failed to record ${event.action}:`,
      (err as Error)?.message ?? err,
    );
  }
};
