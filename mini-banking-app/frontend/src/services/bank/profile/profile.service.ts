// Orchestrate luồng xem thông tin cá nhân (user + tài khoản) qua AP Exchange (read).
//   1. performTgsExchange("balance:read") → Ticket_v + K_{c,v} (RAM).
//   2. Dựng Authenticator bằng K_{c,v} (không cần PIN/chữ ký — đây là read path).
//   3. POST /v1/auth/me; Bank resolve tài khoản theo client_id và trả profile trong ap_rep.
//   4. Giải mã ap_rep bằng K_{c,v}, verify nonce + result=ok.

import { performTgsExchange, getServiceTicket } from "../../tgs-exchange";
import { aesGcmDecrypt, base64ToBytes } from "../../key.service";
import { buildApAuthenticator } from "../ap-authenticator";
import { postProfile } from "./profile.api";

const SCOPE = "balance:read" as const;

// Các field profile bank nhồi vào ap_rep (handler.go GetBalance)
interface ProfileApRepJson {
  result: string;
  nonce: string;
  account_id: string;
  full_name: string;
  email: string;
  user_status: string;
  account_number: string;
  balance: number;
  currency: string;
  account_status: string;
  daily_transfer_limit: number;
  daily_transfer_used: number; // tổng đã chuyển hôm nay (completed), 0 nếu chưa có
}

export interface Profile {
  accountId: string;
  fullName: string;
  email: string;
  userStatus: string;
  accountNumber: string;
  balance: number; // VND, giữ nguyên giá trị Bank Service trả về
  currency: string;
  accountStatus: string;
  dailyTransferLimit: number; // VND, giữ nguyên giá trị Bank Service trả về
  dailyTransferUsed: number;  // VND, tổng đã chuyển hôm nay (completed)
}

export async function fetchProfile(): Promise<Profile> {
  await performTgsExchange(SCOPE);
  const ticket = getServiceTicket(SCOPE);
  if (!ticket) throw new Error("Không lấy được Ticket_v cho scope balance:read");

  const { sessionKey: kcv, clientId, ticketV } = ticket;
  const auth = await buildApAuthenticator(kcv, clientId);

  const resp = await postProfile({
    ticketV,
    authenticator: auth.authenticator,
    requestId: auth.requestId,
  });

  const apRep = JSON.parse(
    new TextDecoder().decode(await aesGcmDecrypt(kcv, base64ToBytes(resp.ap_rep))),
  ) as ProfileApRepJson;

  if (apRep.result !== "ok") throw new Error("Không lấy được thông tin tài khoản");
  if (apRep.nonce !== auth.nonce) throw new Error("AP_REP nonce không khớp — nghi ngờ MITM/replay");

  return {
    accountId: apRep.account_id,
    fullName: apRep.full_name,
    email: apRep.email,
    userStatus: apRep.user_status,
    accountNumber: apRep.account_number,
    balance: apRep.balance,
    currency: apRep.currency,
    accountStatus: apRep.account_status,
    dailyTransferLimit: apRep.daily_transfer_limit,
    dailyTransferUsed: apRep.daily_transfer_used ?? 0,
  };
}
