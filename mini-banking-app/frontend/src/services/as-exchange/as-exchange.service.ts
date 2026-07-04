// Orchestrate AS Exchange (Phase 2) phía browser:
//   1. Lấy certificate + wrapped private key từ IndexedDB; clientId (owner_id) đọc từ certificate.
//   2. Sinh nonce1 + timestamp, dựng canonical AS_REQ payload và ký bằng privKeyRSA_c.
//   3. POST /v1/auth/as-req, nhận AS_REP đã mã hóa.
//   4. Giải mã AS_REP (RSA-OAEP unwrap AES key → AES-256-GCM mở payload), kiểm tra nonce1 khớp.
//   5. Lưu TGT + K_{c,tgs} vào session memory (RAM) cho TGS Exchange.
// Private key plaintext không rời browser; chỉ chữ ký được gửi đi.

import { getStoredCertificate, getWrappedPrivateKey } from "../pki-registration";
import {
  unwrapPrivateKey,
  unwrapDecryptionKey,
  signRsaSha256,
  rsaOaepDecrypt,
  aesGcmDecrypt,
  bytesToBase64,
  base64ToBytes,
} from "../key.service";
import { extractOwnerIdFromCertificate } from "./cert.parser";
import { postAsReq } from "./as-exchange.api";
import { setSession, type AsSession } from "./session";

// nonce1: gateway yêu cầu base64 giải mã đúng 16 byte (auth.middleware.ts)
const NONCE_BYTES = 16;
// Label OAEP phải khớp KDC (as_service.go: rsa.EncryptOAEP(..., []byte("AS_REP")))
const OAEP_LABEL = "AS_REP";

// Cấu trúc AS_REP do KDC marshal (types.go ASResponse) — Go []byte → base64 std
interface ASResponseJson {
  kdc_signature: string;
  encrypted_key: string;
  encrypted_payload: string;
}

// Payload bên trong AS_REP (types.go ASRepPayload)
interface ASRepPayloadJson {
  k_c_tgs: string; // base64 K_{c,tgs}
  tgt: string; // base64 TGT ciphertext
  nonce_1: string; // base64 nonce client đã gửi
}

export interface AsExchangeResult {
  clientId: string;
  certSn: string;
  tgtExpiresAt: number; // unix seconds
}

export async function performAsExchange(pin: string): Promise<AsExchangeResult> {
  const cert = await getStoredCertificate();
  if (!cert) throw new Error("Chưa đăng ký PKI: không tìm thấy certificate");
  const blob = await getWrappedPrivateKey();
  if (!blob) throw new Error("Không tìm thấy private key đã lưu");

  const clientId = extractOwnerIdFromCertificate(cert.certificatePem);
  const certSn = cert.serialNumber;

  // --- Dựng AS_REQ ---
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
  const nonceB64 = bytesToBase64(nonce);
  const timestamp = Math.floor(Date.now() / 1000);

  // Canonical payload phải khớp byte-for-byte với buildASCanonicalPayload (Go json.Marshal):
  // thứ tự field cert_sn, owner_id, nonce, timestamp; compact, không space.
  const canonical = JSON.stringify({
    cert_sn: certSn,
    owner_id: clientId,
    nonce: nonceB64,
    timestamp,
  });
  const canonicalBytes = new TextEncoder().encode(canonical);

  // Ký AS_REQ (RSASSA-PKCS1-v1_5/SHA-256 — KDC verifySignature chấp nhận PKCS1v15)
  const signingKey = await unwrapPrivateKey(blob, pin); // ném nếu PIN sai
  const signature = await signRsaSha256(signingKey, canonicalBytes);

  // --- Gọi gateway ---
  const resp = await postAsReq({
    clientId,
    certSn,
    nonce: nonceB64,
    timestamp,
    preAuthSignature: bytesToBase64(signature),
  });

  // --- Giải mã AS_REP ---
  const asRep = JSON.parse(
    new TextDecoder().decode(base64ToBytes(resp.encrypted_payload)),
  ) as ASResponseJson;

  // Unwrap AES key (RSA-OAEP, label "AS_REP") rồi mở payload (AES-256-GCM, nonce-prefixed)
  const decryptionKey = await unwrapDecryptionKey(blob, pin);
  const aesKey = await rsaOaepDecrypt(decryptionKey, base64ToBytes(asRep.encrypted_key), OAEP_LABEL);
  const payload = JSON.parse(
    new TextDecoder().decode(await aesGcmDecrypt(aesKey, base64ToBytes(asRep.encrypted_payload))),
  ) as ASRepPayloadJson;

  // Mutual auth + chống replay phía response: nonce1 trong AS_REP phải khớp nonce đã gửi
  if (payload.nonce_1 !== nonceB64) {
    throw new Error("AS_REP nonce không khớp — nghi ngờ MITM/replay");
  }

  // --- Lưu phiên (RAM) ---
  const session: AsSession = {
    clientId,
    certSn,
    tgt: payload.tgt,
    sessionKey: base64ToBytes(payload.k_c_tgs),
    tgtExpiresAt: resp.tgt_expiry,
  };
  setSession(session);

  return { clientId, certSn, tgtExpiresAt: resp.tgt_expiry };
}
