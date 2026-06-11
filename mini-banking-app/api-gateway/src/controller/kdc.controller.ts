import { Request, Response } from "express";
import { status as grpcStatus } from "@grpc/grpc-js";
import { ASRequest, TGSRequest } from "../proto/kdc";
import { requestTgt, requestServiceTicket } from "../services/kdc.service";

type HttpError = { status: number; error_code: string; message: string };

const asGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "KDC unavailable";

  switch (code) {
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: "INVALID_REQUEST", message: msg };
    case grpcStatus.UNAUTHENTICATED:
      return { status: 401, error_code: "SIGNATURE_INVALID", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 409, error_code: "REPLAY_DETECTED", message: msg };
    case grpcStatus.NOT_FOUND:
      return { status: 401, error_code: "CERT_NOT_FOUND", message: msg };
    default:
      return { status: 502, error_code: "KDC_UNAVAILABLE", message: msg };
  }
};

const tgsGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "KDC unavailable";

  switch (code) {
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: "INVALID_TGT_FORMAT", message: msg };
    case grpcStatus.UNAUTHENTICATED:
      return { status: 401, error_code: "TGT_EXPIRED", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 403, error_code: "SCOPE_DENIED", message: msg };
    default:
      return { status: 502, error_code: "KDC_UNAVAILABLE", message: msg };
  }
};

export const handleRequestTGT = async (req: Request, res: Response) => {
  const requestId = req.headers["x-request-id"] as string;
  const { clientId, nonce, timestamp, certSn, preAuthSignature } = req.body;
  const ts = new Date().toISOString();

  try {
    const grpcReq = ASRequest.fromJSON({
      idC: clientId,
      certSn,
      nonce,
      timestamp,
      signature: preAuthSignature,
    });

    const resp = await requestTgt(grpcReq);

    return res.status(200).json({
      success: true,
      message: "AS_REP generated",
      data: {
        encrypted_payload: Buffer.from(resp.asRep).toString("base64"),
        tgt_expiry: resp.tgtExpiresAtUnix,
      },
      request_id: requestId,
      timestamp: ts,
    });
  } catch (err: any) {
    const { status, error_code, message } = asGrpcError(err);
    return res.status(status).json({
      success: false,
      error_code,
      message,
      request_id: requestId,
      timestamp: ts,
    });
  }
};

export const handleRequestTGS = async (req: Request, res: Response) => {
  const requestId = req.headers["x-request-id"] as string;
  const {
    serviceId,
    tgtCiphertext,
    authenticator,
    certSn,
    nonce,
    requestedScope,
  } = req.body;
  const ts = new Date().toISOString();

  try {
    const grpcReq = TGSRequest.fromJSON({
      tgt: tgtCiphertext,
      authenticator,
      scope: requestedScope,
      serviceId,
    });
    const resp = await requestServiceTicket(grpcReq);

    return res.status(200).json({
      success: true,
      message: "TGS_REP generated",
      data: {
        encrypted_payload: Buffer.from(resp.tgsRep).toString("base64"),
        ticket_expiry: resp.ticketExpiresAtUnix,
      },
      request_id: requestId,
      timestamp: ts,
    });
  } catch (err: any) {
    const { status, error_code, message } = tgsGrpcError(err);
    return res.status(status).json({
      success: false,
      error_code,
      message,
      request_id: requestId,
      timestamp: ts,
    });
  }
};
