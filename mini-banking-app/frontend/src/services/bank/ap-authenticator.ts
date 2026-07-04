// Dựng Authenticator cho AP Exchange (read path: balance/history/profile).
// Authenticator = E_{K_c_v}[client_id, nonce, request_id, ts_3] (AES-256-GCM nonce-prefixed),
// khớp bank.Authenticator (banking-service/internal/bank/types.go) — bank giải mã bằng K_{c,v}.

import { aesGcmEncrypt, bytesToBase64 } from "../key.service";

export interface ApAuthenticator {
  authenticator: string; // base64
  nonce: string; // base64, echo lại trong ap_rep để client verify
  requestId: string; // UUID v4
}

export async function buildApAuthenticator(
  kcv: Uint8Array,
  clientId: string,
): Promise<ApAuthenticator> {
  const nonce = bytesToBase64(crypto.getRandomValues(new Uint8Array(16)));
  const requestId = crypto.randomUUID();
  const ts3 = Math.floor(Date.now() / 1000);
  const plain = JSON.stringify({
    client_id: clientId,
    nonce,
    request_id: requestId,
    ts_3: ts3,
  });
  const authenticator = bytesToBase64(await aesGcmEncrypt(kcv, new TextEncoder().encode(plain)));
  return { authenticator, nonce, requestId };
}
