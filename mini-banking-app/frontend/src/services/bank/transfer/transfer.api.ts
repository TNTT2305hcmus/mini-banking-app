// Lệnh gọi gateway cho luồng chuyển tiền (AP Exchange — Phase 4): POST /v1/bank/transfer.
// Body khớp TransferBodySchema (bank.middleware.ts): ticket_v, authenticator, cipher_payload,
// iv (base64, decode đúng 12 byte), request_id (UUID v4). Controller đã chuẩn hóa trả { success, data }.

import { apiPost } from "../../api.service";
import { operationHeaders, type OperationId } from "../../operation-id";

export interface TransferReqParams {
  ticketV: string; // base64 Ticket_v opaque
  authenticator: string; // base64 E_{K_c_v}[client_id, nonce, request_id, ts_3] (nonce-prefixed)
  cipherPayload: string; // base64 AES-GCM detached ciphertext (không kèm IV)
  iv: string; // base64 IV 12 byte của cipher_payload
  requestId: string; // UUID v4 — trùng với request_id trong authenticator/payload
  operationId?: OperationId;
}

// data trả về sau khi controller chuẩn hóa
export interface TransferRepResult {
  ap_rep: string; // base64 E_{K_c_v}[result, tx_id, nonce, request_id]
  transaction_id: string;
}

export function postTransfer(params: TransferReqParams): Promise<TransferRepResult> {
  return apiPost<TransferRepResult>("/v1/bank/transfer", {
    ticket_v: params.ticketV,
    authenticator: params.authenticator,
    cipher_payload: params.cipherPayload,
    iv: params.iv,
    request_id: params.requestId,
  }, operationHeaders(params.operationId));
}
