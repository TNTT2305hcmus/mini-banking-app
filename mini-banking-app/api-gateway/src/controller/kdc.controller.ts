import { Request, Response } from "express";
import { ASRequest, TGSRequest } from "../proto/kdc";
import { requestTgt, requestServiceTicket } from "../services/kdc.service";
import { asGrpcError, tgsGrpcError } from "../middleware/errorHandler";

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
