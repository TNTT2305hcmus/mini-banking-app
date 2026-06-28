// Orchestrate TGS Exchange (Phase 3) phía browser:
//   1. Lấy phiên TGT (TGT + K_{c,tgs} + clientId + certSn) từ AS Exchange (RAM).
//   2. Sinh nonce2 + timestamp + request_id, dựng Authenticator và mã hóa bằng K_{c,tgs}.
//   3. POST /v1/auth/tgs-req, nhận TGS_REP đã mã hóa.
//   4. Giải mã TGS_REP bằng K_{c,tgs}, kiểm tra nonce2 & scope khớp.
//   5. Lưu Ticket_v + K_{c,v} (đúng scope) vào session memory (RAM) cho AP Exchange.
// Không dùng chữ ký số / certificate ở runtime; tin cậy đến từ bí mật chia sẻ K_{c,tgs}.

import { getSession, hasValidTgt } from "../as-exchange";
import { aesGcmEncrypt, aesGcmDecrypt, bytesToBase64, base64ToBytes } from "../key.service";
import { postTgsReq } from "./tgs-exchange.api";
import {
  getServiceTicket,
  hasValidServiceTicket,
  setServiceTicket,
  type Scope,
  type ServiceTicketSession,
} from "./session";

// nonce2: gateway yêu cầu base64 giải mã đúng 16 byte (auth.middleware.ts)
const NONCE_BYTES = 16;
// KDC chỉ cấu hình service "bank-service" (kdc service.go)
const SERVICE_ID = "bank-service";

// Payload TGS_REP do KDC marshal (types.go TGSReplyPlaintext) — Go []byte → base64 std
interface TgsReplyJson {
  k_c_v: string; // base64 K_{c,v}
  id_v: string; // service id
  ticket_v: string; // base64 Ticket_v opaque
  nonce2: string; // base64 nonce client đã gửi
  nonce_req: string;
  ts_4: number;
  expires_at: number;
  scope: string;
}

export interface TgsExchangeResult {
  scope: Scope;
  serviceId: string;
  ticketExpiresAt: number; // unix seconds
}

// Xin Ticket_v cho một scope. `force=false` sẽ tái dùng ticket còn hạn (reusable trong TTL).
export async function performTgsExchange(scope: Scope, force = false): Promise<TgsExchangeResult> {
  if (!force && hasValidServiceTicket(scope)) {
    const existing = getServiceTicket(scope)!;
    return { scope, serviceId: existing.serviceId, ticketExpiresAt: existing.ticketExpiresAt };
  }

  if (!hasValidTgt()) throw new Error("TGT đã hết hạn hoặc chưa đăng nhập — cần thực hiện lại AS Exchange");
  const tgtSession = getSession();
  if (!tgtSession) throw new Error("Không tìm thấy phiên TGT");

  const { clientId, certSn, tgt, sessionKey: kcTgs } = tgtSession;

  // --- Dựng Authenticator ---
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
  const nonceB64 = bytesToBase64(nonce);
  const timestamp = Math.floor(Date.now() / 1000);
  const requestId = crypto.randomUUID();

  // Field name khớp AuthenticatorPlaintext (types.go): client_id, ts_3, nonce_req,
  // request_id, requested_service, scope. KDC còn ràng buộc nonce_req == base64(proto nonce).
  const authenticatorPlain = JSON.stringify({
    client_id: clientId,
    ts_3: timestamp,
    nonce_req: nonceB64,
    request_id: requestId,
    requested_service: SERVICE_ID,
    scope,
  });
  const authenticator = await aesGcmEncrypt(kcTgs, new TextEncoder().encode(authenticatorPlain));

  // --- Gọi gateway ---
  const resp = await postTgsReq({
    serviceId: SERVICE_ID,
    tgtCiphertext: tgt,
    authenticator: bytesToBase64(authenticator),
    certSn,
    nonce: nonceB64,
    requestedScope: scope,
  });

  // --- Giải mã TGS_REP bằng K_{c,tgs} ---
  const reply = JSON.parse(
    new TextDecoder().decode(await aesGcmDecrypt(kcTgs, base64ToBytes(resp.encrypted_payload))),
  ) as TgsReplyJson;

  // Chống replay/MITM phía response + đảm bảo nhận đúng vé
  if (reply.nonce2 !== nonceB64) throw new Error("TGS_REP nonce không khớp — nghi ngờ MITM/replay");
  if (reply.scope !== scope) throw new Error("TGS_REP scope không khớp yêu cầu");

  // --- Lưu phiên dịch vụ (RAM) ---
  const ticket: ServiceTicketSession = {
    scope,
    serviceId: reply.id_v || SERVICE_ID,
    ticketV: reply.ticket_v,
    sessionKey: base64ToBytes(reply.k_c_v),
    ticketExpiresAt: resp.ticket_expiry,
    clientId,
    certSn,
  };
  setServiceTicket(ticket);

  return { scope, serviceId: ticket.serviceId, ticketExpiresAt: ticket.ticketExpiresAt };
}
