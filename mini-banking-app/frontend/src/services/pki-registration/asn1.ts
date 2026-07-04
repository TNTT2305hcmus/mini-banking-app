// Bộ mã hóa ASN.1 DER tối thiểu, đủ để dựng CSR PKCS#10 trong browser
// WebCrypto không hỗ trợ CSR sẵn

// Các tag DER dùng cho CSR
const TAG = {
  INTEGER: 0x02,
  BIT_STRING: 0x03,
  OCTET_STRING: 0x04,
  NULL: 0x05,
  OID: 0x06,
  UTF8_STRING: 0x0c,
  IA5_STRING: 0x16,
  SEQUENCE: 0x30,
  SET: 0x31,
  // context-specific [n] constructed = 0xA0 | n
  CONTEXT_CONSTRUCTED_0: 0xa0,
  // GeneralName rfc822Name = [1] IA5String, primitive = 0x80 | 1
  CONTEXT_PRIMITIVE_1: 0x81,
} as const;

// Nối các mảng byte DER
function concat(parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const p of parts) {
    out.set(p, offset);
    offset += p.length;
  }
  return out;
}

// Mã hóa độ dài DER (short form < 128, còn lại long form)
function encodeLength(len: number): Uint8Array {
  if (len < 0x80) return new Uint8Array([len]);
  const bytes: number[] = [];
  let n = len;
  while (n > 0) {
    bytes.unshift(n & 0xff);
    n >>= 8;
  }
  return new Uint8Array([0x80 | bytes.length, ...bytes]);
}

// Tạo bộ ba tag-length-value
function tlv(tag: number, content: Uint8Array): Uint8Array {
  return concat([new Uint8Array([tag]), encodeLength(content.length), content]);
}

// Các builder DER
export const der = {
  sequence: (...items: Uint8Array[]): Uint8Array => tlv(TAG.SEQUENCE, concat(items)),
  set: (...items: Uint8Array[]): Uint8Array => tlv(TAG.SET, concat(items)),
  // INTEGER nhỏ không âm (đủ cho trường version của CSR)
  integer: (value: number): Uint8Array => tlv(TAG.INTEGER, new Uint8Array([value])),
  // OID từ phần content đã được mã hóa sẵn
  oid: (encodedContent: Uint8Array): Uint8Array => tlv(TAG.OID, encodedContent),
  null: (): Uint8Array => tlv(TAG.NULL, new Uint8Array(0)),
  octetString: (content: Uint8Array): Uint8Array => tlv(TAG.OCTET_STRING, content),
  utf8String: (s: string): Uint8Array => tlv(TAG.UTF8_STRING, new TextEncoder().encode(s)),
  // IA5String (ASCII — dùng cho emailAddress)
  ia5String: (s: string): Uint8Array => tlv(TAG.IA5_STRING, new TextEncoder().encode(s)),
  // GeneralName rfc822Name [1] IA5String (primitive) — email trong SubjectAltName
  rfc822Name: (s: string): Uint8Array => tlv(TAG.CONTEXT_PRIMITIVE_1, new TextEncoder().encode(s)),
  // BIT STRING với 0 bit thừa (thêm octet 0x00 ở đầu)
  bitString: (content: Uint8Array): Uint8Array =>
    tlv(TAG.BIT_STRING, concat([new Uint8Array([0x00]), content])),
  // Wrapper context-specific [0] constructed (cho attributes của CSR)
  context0: (content: Uint8Array): Uint8Array => tlv(TAG.CONTEXT_CONSTRUCTED_0, content),
  // Giữ nguyên byte đã là DER (vd: SPKI export)
  raw: (bytes: Uint8Array): Uint8Array => bytes,
};

// Phần content của OID đã mã hóa sẵn (tag/length do der.oid thêm vào)
export const OID = {
  // id-at-commonName 2.5.4.3
  commonName: new Uint8Array([0x55, 0x04, 0x03]),
  // emailAddress (PKCS#9) 1.2.840.113549.1.9.1
  emailAddress: new Uint8Array([0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x01]),
  // extensionRequest (PKCS#9) 1.2.840.113549.1.9.14
  extensionRequest: new Uint8Array([0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x0e]),
  // subjectAltName 2.5.29.17
  subjectAltName: new Uint8Array([0x55, 0x1d, 0x11]),
  // sha256WithRSAEncryption 1.2.840.113549.1.1.11
  sha256WithRSA: new Uint8Array([0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0b]),
} as const;
