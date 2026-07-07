import {
  ASRequest,
  ASResponse,
  KDCServiceClient,
  ListAuditEventsRequest as KdcListAuditRequest,
  ListAuditEventsResponse as KdcListAuditResponse,
  TGSRequest,
  TGSResponse,
  VerifyAuditChainResponse as KdcVerifyAuditChainResponse,
} from "../proto/kdc";
import { sslCredentials } from "../config/grpc";
import { Metadata } from "@grpc/grpc-js";

const KDC_ADDR = process.env.KDC_GRPC_ADDR ?? "localhost:50052";

const kdcClient = new KDCServiceClient(KDC_ADDR, sslCredentials);

// Trace id travels as gRPC metadata "x-request-id" (transport concern), so the
// KDC audit trail correlates with the same request across services.
const traceMetadata = (requestId?: string): Metadata => {
  const md = new Metadata();
  if (requestId) md.set("x-request-id", requestId);
  return md;
};

export const requestTgt = (payload: ASRequest): Promise<ASResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.requestTgt(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const requestServiceTicket = (payload: TGSRequest): Promise<TGSResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.requestServiceTicket(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

// Admin read-only: key-issuance audit log (AS/TGS ticket grants and rejections).
export const listKdcAuditEvents = (
  payload: KdcListAuditRequest,
  requestId?: string,
): Promise<KdcListAuditResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.listAuditEvents(payload, traceMetadata(requestId), (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });

export const verifyKdcAuditChain = (
  requestId?: string,
): Promise<KdcVerifyAuditChainResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.verifyAuditChain({}, traceMetadata(requestId), (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });
