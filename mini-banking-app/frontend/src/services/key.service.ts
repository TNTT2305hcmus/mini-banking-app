// Cấu hình mã hóa RSA
const RSA_ALG = {
  name: "RSASSA-PKCS1-v1_5",
  modulusLength: 2048,
  publicExponent: new Uint8Array([0x01, 0x00, 0x01]), // 65537
  hash: "SHA-256",
} as const;

// Cấu hình tham số cho mã hóa Private Key
const PBKDF2_ITERATIONS = 210_000;
const SALT_BYTES = 16;
const IV_BYTES = 12;

export interface WrappedPrivateKey {
  v: 1;
  wrapped: ArrayBuffer;
  salt: Uint8Array;
  iv: Uint8Array;
  iterations: number;
}

// Sinh cặp khóa RSA
export function generateClientKeyPair(): Promise<CryptoKeyPair> {
  return crypto.subtle.generateKey(RSA_ALG, true, ["sign", "verify"]);
}

// Trích xuất Public Key ra mảng byte
export async function exportPublicKeySpki(publicKey: CryptoKey): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.exportKey("spki", publicKey));
}

// Trích xuất Public Key từ mảng byte sang PEM
export async function exportPublicKeyPem(publicKey: CryptoKey): Promise<string> {
  return derToPem(await exportPublicKeySpki(publicKey), "PUBLIC KEY");
}

// Tạo AES key pairs từ mã PIN của người dùng.
async function deriveWrapKey(pin: string, salt: Uint8Array, iterations: number): Promise<CryptoKey> {
  const pinKey = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(pin),
    "PBKDF2",
    false,
    ["deriveKey"],
  );
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt: salt as BufferSource, iterations, hash: "SHA-256" },
    pinKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["wrapKey", "unwrapKey"],
  );
}

// Mã hóa Private Key RSA bằng mã PIN
export async function wrapPrivateKey(privateKey: CryptoKey, pin: string): Promise<WrappedPrivateKey> {
  const salt = crypto.getRandomValues(new Uint8Array(SALT_BYTES));
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const wrapKey = await deriveWrapKey(pin, salt, PBKDF2_ITERATIONS);

  const wrapped = await crypto.subtle.wrapKey("pkcs8", privateKey, wrapKey, {
    name: "AES-GCM",
    iv: iv as BufferSource,
  });

  return { v: 1, wrapped, salt, iv, iterations: PBKDF2_ITERATIONS };
}

// Giải mã khối dữ liệu lưu trữ từ IndexedDB để lấy lại Priavte Key
export async function unwrapPrivateKey(blob: WrappedPrivateKey, pin: string): Promise<CryptoKey> {
  const wrapKey = await deriveWrapKey(pin, blob.salt, blob.iterations);
  return crypto.subtle.unwrapKey(
    "pkcs8",
    blob.wrapped,
    wrapKey,
    { name: "AES-GCM", iv: blob.iv as BufferSource },
    { name: RSA_ALG.name, hash: RSA_ALG.hash },
    false,
    ["sign"],
  );
}

// Giải mã cùng một wrapped private key nhưng import dưới dạng RSA-OAEP (usage decrypt).
// Cần thiết vì AS_REP wrap AES key bằng public key của client (RSA-OAEP); WebCrypto ràng buộc
// thuật toán theo từng CryptoKey nên không thể dùng key "sign" để decrypt.
export async function unwrapDecryptionKey(blob: WrappedPrivateKey, pin: string): Promise<CryptoKey> {
  const wrapKey = await deriveWrapKey(pin, blob.salt, blob.iterations);
  return crypto.subtle.unwrapKey(
    "pkcs8",
    blob.wrapped,
    wrapKey,
    { name: "AES-GCM", iv: blob.iv as BufferSource },
    { name: "RSA-OAEP", hash: "SHA-256" },
    false,
    ["decrypt"],
  );
}

// Giải mã RSA-OAEP (SHA-256). `label` khớp với label KDC dùng khi mã hóa (vd "AS_REP").
export async function rsaOaepDecrypt(
  privateKey: CryptoKey,
  data: Uint8Array,
  label?: string,
): Promise<Uint8Array> {
  const params: RsaOaepParams = { name: "RSA-OAEP" };
  if (label) params.label = new TextEncoder().encode(label) as BufferSource;
  return new Uint8Array(await crypto.subtle.decrypt(params, privateKey, data as BufferSource));
}

// Giải mã AES-256-GCM với định dạng nonce-prefixed (12 byte IV ở đầu, phần còn lại là ciphertext+tag).
// Khớp với encryptBytes phía KDC (Go): append(nonce, gcm.Seal(...)...).
export async function aesGcmDecrypt(
  keyBytes: Uint8Array,
  ivAndCiphertext: Uint8Array,
): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey("raw", keyBytes as BufferSource, { name: "AES-GCM" }, false, [
    "decrypt",
  ]);
  const iv = ivAndCiphertext.subarray(0, IV_BYTES);
  const ciphertext = ivAndCiphertext.subarray(IV_BYTES);
  return new Uint8Array(
    await crypto.subtle.decrypt({ name: "AES-GCM", iv: iv as BufferSource }, key, ciphertext as BufferSource),
  );
}

// Mã hóa AES-256-GCM, trả về định dạng nonce-prefixed (12 byte IV ngẫu nhiên + ciphertext+tag).
// Khớp với decryptBytes phía KDC (Go) — dùng để dựng Authenticator mã hóa bằng K_{c,tgs}.
export async function aesGcmEncrypt(
  keyBytes: Uint8Array,
  plaintext: Uint8Array,
): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey("raw", keyBytes as BufferSource, { name: "AES-GCM" }, false, [
    "encrypt",
  ]);
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const ciphertext = new Uint8Array(
    await crypto.subtle.encrypt({ name: "AES-GCM", iv: iv as BufferSource }, key, plaintext as BufferSource),
  );
  const out = new Uint8Array(iv.length + ciphertext.length);
  out.set(iv, 0);
  out.set(ciphertext, iv.length);
  return out;
}

// Ký bằng Private Key
export async function signRsaSha256(privateKey: CryptoKey, data: Uint8Array): Promise<Uint8Array> {
  return new Uint8Array(
    await crypto.subtle.sign({ name: RSA_ALG.name }, privateKey, data as BufferSource),
  );
}

// Giải mã cùng wrapped private key nhưng import dưới dạng RSA-PSS (usage sign).
// Cần thiết vì Bank Service verify chữ ký giao dịch bằng RSA-PSS (rsa.VerifyPSS),
// trong khi key gốc được wrap cho RSASSA-PKCS1-v1_5; WebCrypto ràng buộc thuật toán theo từng key.
export async function unwrapPssSigningKey(blob: WrappedPrivateKey, pin: string): Promise<CryptoKey> {
  const wrapKey = await deriveWrapKey(pin, blob.salt, blob.iterations);
  return crypto.subtle.unwrapKey(
    "pkcs8",
    blob.wrapped,
    wrapKey,
    { name: "AES-GCM", iv: blob.iv as BufferSource },
    { name: "RSA-PSS", hash: "SHA-256" },
    false,
    ["sign"],
  );
}

// Ký RSA-PSS/SHA-256 với saltLength = 32 (= độ dài hash), khớp PSSSaltLengthEqualsHash phía Go.
export async function signRsaPss(privateKey: CryptoKey, data: Uint8Array): Promise<Uint8Array> {
  return new Uint8Array(
    await crypto.subtle.sign({ name: "RSA-PSS", saltLength: 32 }, privateKey, data as BufferSource),
  );
}

// Chuyển thành Base64 (standard, khớp base64.StdEncoding của Go)
export function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

// Giải Base64 (standard) về mảng byte
export function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

// Chuyển từ byte[] thành PEM
export function derToPem(der: Uint8Array, label: string): string {
  const b64 = bytesToBase64(der);
  const lines = b64.match(/.{1,64}/g) ?? [];
  return `-----BEGIN ${label}-----\n${lines.join("\n")}\n-----END ${label}-----\n`;
}
