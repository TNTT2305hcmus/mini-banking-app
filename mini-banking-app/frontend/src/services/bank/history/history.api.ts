// Lệnh gọi gateway cho luồng xem lịch sử giao dịch:
// POST /v1/bank/accounts/{account_id}/transactions/query?limit&offset
// Body: ticket_v + authenticator (AP read, scope history:read) + request_id (UUID v4).

import { apiPost } from "../../api.service";
import { operationHeaders, type OperationId } from "../../operation-id";

export interface HistoryReqParams {
  accountId: string; // UUID tài khoản (lấy từ profile /auth/me)
  ticketV: string; // base64 Ticket_v (scope history:read)
  authenticator: string; // base64 E_{K_c_v}[...]
  requestId: string; // UUID v4
  limit: number;
  offset: number;
  operationId?: OperationId;
}

export interface HistoryItemDto {
  transaction_id: string;
  from_account_number: string;
  to_account_number: string;
  amount: number;
  currency: string;
  status: string; // pending | completed | failed
  description: string;
  scope: string;
  created_at_unix: number;
  completed_at_unix: number;
}

export interface HistoryRepResult {
  ap_rep: string;
  transactions: HistoryItemDto[];
  total: number;
  limit: number;
  offset: number;
}

export function postHistory(params: HistoryReqParams): Promise<HistoryRepResult> {
  const query = `?limit=${params.limit}&offset=${params.offset}`;
  return apiPost<HistoryRepResult>(
    `/v1/bank/accounts/${encodeURIComponent(params.accountId)}/transactions/query${query}`,
    {
      ticket_v: params.ticketV,
      authenticator: params.authenticator,
      request_id: params.requestId,
    },
    operationHeaders(params.operationId),
  );
}
