// Orchestrate Bank Transfer (AP Exchange — Phase 4) phía browser.
// Tự nối luồng TGS: đảm bảo có Ticket_v scope transfer:create (reuse trong TTL) trước khi giao dịch.
//
// Pipeline client:
//   1. performTgsExchange("transfer:create") → Ticket_v + K_{c,v} (RAM).
//   2. Dựng Authenticator = E_{K_c_v}[client_id, nonce3, request_id, ts_3] (AES-GCM nonce-prefixed).
//   3. Dựng canonical payload, ký RSA-PSS bằng privKeyRSA_c (unwrap bằng PIN).
//   4. CipherPayload = AES-GCM detached_{K_c_v}[{ payload, client_signature }] (IV 12B tách riêng).
//   5. POST /v1/bank/transfer; giải mã AP_REP bằng K_{c,v}, verify nonce3 + result=ok.
// Private key plaintext không rời browser; chỉ chữ ký + ciphertext được gửi đi.

import { performTgsExchange, getServiceTicket } from "../../tgs-exchange";
import { getWrappedPrivateKey } from "../../pki-registration";
import {
  unwrapPssSigningKey,
  signRsaPss,
  aesGcmEncrypt,
  aesGcmDecrypt,
  bytesToBase64,
  base64ToBytes,
} from "../../key.service";
import { postTransfer } from "./transfer.api";

const SCOPE = "transfer:create" as const;
const NONCE_BYTES = 32; // nonce3 theo transfer-flow.md
const IV_BYTES = 12;

export interface TransferParams {
  fromAccountId: string;
  toAccountId: string;
  amount: number; // đơn vị nhỏ nhất (cents) — gửi thẳng vào field amount
  description?: string;
  currency?: string; // mặc định VND
  pin: string;
}

export interface TransferResult {
  transactionId: string;
}

// Payload AP_REP do Bank marshal (handler.go encryptAPRep)
interface ApRepJson {
  result: string;
  tx_id: string;
  nonce: string;
  request_id: string;
}

// Canonical JSON khớp Go json.Marshal: keys sort tăng dần, compact, escape <>& và U+2028/2029.
// Bank tái canonical hóa payload nhận được rồi verify chữ ký trên đúng chuỗi này.
// Dùng codepoint để tránh nhúng ký tự separator "vô hình" vào source.
function canonicalStringify(obj: Record<string, string | number>): string {
  const json = JSON.stringify(obj, Object.keys(obj).sort());
  let out = "";
  for (const ch of json) {
    const code = ch.codePointAt(0)!;
    if (ch === "<") out += "\\u003c";
    else if (ch === ">") out += "\\u003e";
    else if (ch === "&") out += "\\u0026";
    else if (code === 0x2028) out += "\\u2028";
    else if (code === 0x2029) out += "\\u2029";
    else out += ch;
  }
  return out;
}

export async function performTransfer(params: TransferParams): Promise<TransferResult> {
  const currency = params.currency ?? "VND";

  // 1) Đảm bảo có Ticket_v transfer:create (reuse nếu còn hạn; ném nếu TGT/đăng nhập hết hạn)
  await performTgsExchange(SCOPE);
  const ticket = getServiceTicket(SCOPE);
  if (!ticket) throw new Error("Không lấy được Ticket_v cho scope transfer:create");

  const blob = await getWrappedPrivateKey();
  if (!blob) throw new Error("Không tìm thấy private key đã lưu");

  const { ticketV, sessionKey: kcv, clientId } = ticket;

  // 2) Authenticator (nonce-prefixed AES-GCM bằng K_{c,v})
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
  const nonceB64 = bytesToBase64(nonce);
  const ts3 = Math.floor(Date.now() / 1000);
  const requestId = crypto.randomUUID();

  const authenticatorPlain = JSON.stringify({
    client_id: clientId,
    nonce: nonceB64,
    request_id: requestId,
    ts_3: ts3,
  });
  const authenticator = await aesGcmEncrypt(kcv, new TextEncoder().encode(authenticatorPlain));

  // 3) Canonical payload + chữ ký RSA-PSS
  const payload: Record<string, string | number> = {
    from_account_id: params.fromAccountId,
    to_account_id: params.toAccountId,
    amount: params.amount,
    currency,
    request_id: requestId,
    idempotency_key: crypto.randomUUID(),
    scope: SCOPE,
  };
  if (params.description) payload.description = params.description;

  const canonical = canonicalStringify(payload);
  const signingKey = await unwrapPssSigningKey(blob, params.pin); // ném nếu PIN sai
  const signature = await signRsaPss(signingKey, new TextEncoder().encode(canonical));

  // 4) CipherPayload detached: gửi payload đúng bytes canonical đã ký để Bank tái canonical khớp tuyệt đối
  const envelope = `{"payload":${canonical},"client_signature":${JSON.stringify(bytesToBase64(signature))}}`;
  const embedded = await aesGcmEncrypt(kcv, new TextEncoder().encode(envelope));
  const iv = embedded.subarray(0, IV_BYTES);
  const cipherPayload = embedded.subarray(IV_BYTES);

  // 5) Gửi & giải mã AP_REP
  const resp = await postTransfer({
    ticketV,
    authenticator: bytesToBase64(authenticator),
    cipherPayload: bytesToBase64(cipherPayload),
    iv: bytesToBase64(iv),
    requestId,
  });

  const apRep = JSON.parse(
    new TextDecoder().decode(await aesGcmDecrypt(kcv, base64ToBytes(resp.ap_rep))),
  ) as ApRepJson;

  if (apRep.result !== "ok") throw new Error("Giao dịch bị từ chối bởi Bank Service");
  if (apRep.nonce !== nonceB64) throw new Error("AP_REP nonce không khớp — nghi ngờ MITM/replay");

  return { transactionId: resp.transaction_id || apRep.tx_id };
}
