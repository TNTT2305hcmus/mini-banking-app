// Lệnh gọi gateway cho TGS Exchange: POST /v1/auth/tgs-req.
// Body khớp TGSRequestSchema của gateway (auth.middleware.ts) và controller handleRequestTGS:
//   serviceId ("bank-service"), tgtCiphertext (base64), authenticator (base64),
//   certSn (hex), nonce (base64 16 byte), requestedScope (enum).

import { apiPost } from "../api.service";
import { operationHeaders, type OperationId } from "../operation-id";
import type { Scope } from "./session";

export interface TgsReqParams {
  serviceId: string;
  tgtCiphertext: string; // base64 TGT opaque
  authenticator: string; // base64 E_{K_c_tgs}[...]
  certSn: string; // hex
  nonce: string; // base64, phải khớp nonce_req trong authenticator
  requestedScope: Scope;
  operationId?: OperationId;
}

// Kết quả /v1/auth/tgs-req: TGS_REP (base64 AES-GCM bằng K_{c,tgs}) + hạn Ticket_v
export interface TgsRepResult {
  encrypted_payload: string; // base64 ciphertext TGSReplyPlaintext
  ticket_expiry: number; // unix seconds
}

export function postTgsReq(params: TgsReqParams): Promise<TgsRepResult> {
  return apiPost<TgsRepResult>("/v1/auth/tgs-req", {
    serviceId: params.serviceId,
    tgtCiphertext: params.tgtCiphertext,
    authenticator: params.authenticator,
    certSn: params.certSn,
    nonce: params.nonce,
    requestedScope: params.requestedScope,
  }, operationHeaders(params.operationId));
}
