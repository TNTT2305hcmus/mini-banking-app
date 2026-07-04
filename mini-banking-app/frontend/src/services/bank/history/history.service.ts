// Orchestrate luồng xem lịch sử giao dịch (AP Exchange — read, scope history:read).
//   1. performTgsExchange("history:read") → Ticket_v + K_{c,v} (RAM).
//   2. Dựng Authenticator bằng K_{c,v} (read path — không cần PIN/chữ ký).
//   3. POST /v1/bank/accounts/{accountId}/transactions/query → Bank lọc theo ownership.
//   4. Giải mã ap_rep bằng K_{c,v}, verify nonce + result=ok (mutual auth/chống replay response).
// accountId lấy từ profile (/auth/me); Bank vẫn check ownership account.user_id == ID_c.

import { performTgsExchange, getServiceTicket } from "../../tgs-exchange";
import { aesGcmDecrypt, base64ToBytes } from "../../key.service";
import { buildApAuthenticator } from "../ap-authenticator";
import { postHistory, type HistoryItemDto } from "./history.api";

const SCOPE = "history:read" as const;
const DEFAULT_LIMIT = 50;

export interface HistoryItem {
  transactionId: string;
  fromAccountNumber: string;
  toAccountNumber: string;
  amount: number; // VND, giữ nguyên giá trị Bank trả về
  currency: string;
  status: string; // pending | completed | failed
  description: string;
  scope: string;
  createdAtUnix: number;
  completedAtUnix: number;
}

export interface HistoryResult {
  items: HistoryItem[];
  total: number;
  limit: number;
  offset: number;
}

function mapItem(t: HistoryItemDto): HistoryItem {
  return {
    transactionId: t.transaction_id,
    fromAccountNumber: t.from_account_number,
    toAccountNumber: t.to_account_number,
    amount: t.amount,
    currency: t.currency,
    status: t.status,
    description: t.description,
    scope: t.scope,
    createdAtUnix: t.created_at_unix,
    completedAtUnix: t.completed_at_unix,
  };
}

export async function fetchHistory(params: {
  accountId: string;
  limit?: number;
  offset?: number;
}): Promise<HistoryResult> {
  if (!params.accountId) throw new Error("Thiếu account_id để xem lịch sử");

  await performTgsExchange(SCOPE);
  const ticket = getServiceTicket(SCOPE);
  if (!ticket) throw new Error("Không lấy được Ticket_v cho scope history:read");

  const { sessionKey: kcv, clientId, ticketV } = ticket;
  const auth = await buildApAuthenticator(kcv, clientId);

  const resp = await postHistory({
    accountId: params.accountId,
    ticketV,
    authenticator: auth.authenticator,
    requestId: auth.requestId,
    limit: params.limit ?? DEFAULT_LIMIT,
    offset: params.offset ?? 0,
  });

  const apRep = JSON.parse(
    new TextDecoder().decode(await aesGcmDecrypt(kcv, base64ToBytes(resp.ap_rep))),
  ) as { result: string; nonce: string };

  if (apRep.result !== "ok") throw new Error("Không lấy được lịch sử giao dịch");
  if (apRep.nonce !== auth.nonce) throw new Error("AP_REP nonce không khớp — nghi ngờ MITM/replay");

  return {
    items: (resp.transactions ?? []).map(mapItem),
    total: resp.total,
    limit: resp.limit,
    offset: resp.offset,
  };
}
