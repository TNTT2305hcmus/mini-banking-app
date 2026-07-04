// Lệnh gọi gateway cho AS Exchange: POST /v1/auth/as-req.
// Body khớp ASRequestSchema của gateway (auth.middleware.ts):
//   clientId (UUID), certSn (hex), nonce (base64 16 byte), timestamp (unix s), preAuthSignature (base64).

import { apiPost } from "../api.service";

export interface AsReqParams {
  clientId: string;
  certSn: string;
  nonce: string; // base64
  timestamp: number; // unix seconds
  preAuthSignature: string; // base64
}

// Kết quả /v1/auth/as-req: AS_REP (base64 JSON {kdc_signature, encrypted_key, encrypted_payload}) + hạn TGT
export interface AsRepResult {
  encrypted_payload: string; // base64 của ASResponse JSON do KDC trả
  tgt_expiry: number; // unix seconds
}

export function postAsReq(params: AsReqParams): Promise<AsRepResult> {
  return apiPost<AsRepResult>("/v1/auth/as-req", {
    clientId: params.clientId,
    certSn: params.certSn,
    nonce: params.nonce,
    timestamp: params.timestamp,
    preAuthSignature: params.preAuthSignature,
  });
}
