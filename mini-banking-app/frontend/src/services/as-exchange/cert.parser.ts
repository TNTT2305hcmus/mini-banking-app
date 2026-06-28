// Parser X.509 DER tối thiểu cho certificate PEM do CA Service cấp.
// Không phụ thuộc thư viện ngoài và chỉ trả về dữ liệu JSON-safe để UI sử dụng.

interface Tlv {
  tag: number;
  content: Uint8Array;
  end: number;
}

export interface ParsedDistinguishedName {
  commonName: string;
  organization: string;
  organizationalUnit: string;
  country: string;
  locality: string;
  stateOrProvince: string;
  emailAddress: string;
  attributes: Record<string, string[]>;
}

export interface ParsedCertificateJson {
  version: number;
  serialNumber: string;
  signatureAlgorithm: { oid: string; name: string };
  publicKeyAlgorithm: { oid: string; name: string };
  issuer: ParsedDistinguishedName;
  subject: ParsedDistinguishedName;
  validity: { notBefore: string; notAfter: string };
  subjectAltName: { emails: string[]; dnsNames: string[]; uris: string[]; ipAddresses: string[] };
  ownerId: string | null;
  fingerprintSha256: string;
}

const TAG_SEQUENCE = 0x30;
const TAG_SET = 0x31;
const TAG_INTEGER = 0x02;
const TAG_OID = 0x06;
const TAG_OCTET_STRING = 0x04;
const TAG_UTC_TIME = 0x17;
const TAG_GENERALIZED_TIME = 0x18;
const TAG_VERSION = 0xa0;
const TAG_EMAIL = 0x81;
const TAG_DNS = 0x82;
const TAG_URI = 0x86;
const TAG_IP = 0x87;

const SAN_OID = new Uint8Array([0x55, 0x1d, 0x11]); // 2.5.29.17
const OWNER_URI_PREFIX = "urn:mini-banking:owner:";

const DN_OIDS = {
  commonName: "2.5.4.3",
  country: "2.5.4.6",
  locality: "2.5.4.7",
  stateOrProvince: "2.5.4.8",
  organization: "2.5.4.10",
  organizationalUnit: "2.5.4.11",
  emailAddress: "1.2.840.113549.1.9.1",
} as const;

const ALGORITHM_NAMES: Record<string, string> = {
  "1.2.840.113549.1.1.1": "RSA",
  "1.2.840.113549.1.1.5": "SHA-1 with RSA",
  "1.2.840.113549.1.1.10": "RSA-PSS",
  "1.2.840.113549.1.1.11": "SHA-256 with RSA",
  "1.2.840.113549.1.1.12": "SHA-384 with RSA",
  "1.2.840.113549.1.1.13": "SHA-512 with RSA",
  "1.2.840.10045.2.1": "EC Public Key",
  "1.2.840.10045.4.3.2": "ECDSA with SHA-256",
  "1.2.840.10045.4.3.3": "ECDSA with SHA-384",
  "1.2.840.10045.4.3.4": "ECDSA with SHA-512",
};

function readTlv(buf: Uint8Array, off: number): Tlv {
  if (off < 0 || off + 2 > buf.length) throw new Error("Certificate DER bị cắt ngắn");
  const tag = buf[off];
  let i = off + 1;
  let len = buf[i++];

  if ((len & 0x80) !== 0) {
    const numBytes = len & 0x7f;
    if (numBytes === 0 || numBytes > 4 || i + numBytes > buf.length) {
      throw new Error("Độ dài ASN.1 không hợp lệ");
    }
    len = 0;
    for (let k = 0; k < numBytes; k++) len = len * 256 + buf[i++];
  }

  const end = i + len;
  if (end > buf.length) throw new Error("Nội dung ASN.1 vượt quá certificate");
  return { tag, content: buf.subarray(i, end), end };
}

function readChildren(content: Uint8Array): Tlv[] {
  const out: Tlv[] = [];
  let off = 0;
  while (off < content.length) {
    const node = readTlv(content, off);
    out.push(node);
    if (node.end <= off) throw new Error("Cấu trúc ASN.1 không hợp lệ");
    off = node.end;
  }
  return out;
}

function requireTag(node: Tlv, tag: number, label: string): Tlv {
  if (node.tag !== tag) throw new Error(`${label} không đúng định dạng ASN.1`);
  return node;
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function bytesToHex(bytes: Uint8Array, separator = ""): string {
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0").toUpperCase()).join(separator);
}

function integerToNumber(bytes: Uint8Array): number {
  let value = 0;
  for (const byte of bytes) value = value * 256 + byte;
  return value;
}

function decodeOid(bytes: Uint8Array): string {
  if (bytes.length === 0) throw new Error("OID rỗng");
  const first = bytes[0];
  const parts = [Math.min(2, Math.floor(first / 40)), first < 80 ? first % 40 : first - 80];
  let value = 0;
  for (let i = 1; i < bytes.length; i++) {
    value = value * 128 + (bytes[i] & 0x7f);
    if ((bytes[i] & 0x80) === 0) {
      parts.push(value);
      value = 0;
    }
  }
  if ((bytes[bytes.length - 1] & 0x80) !== 0) throw new Error("OID bị cắt ngắn");
  return parts.join(".");
}

function decodeString(node: Tlv): string {
  if (node.tag === 0x1e) {
    let value = "";
    for (let i = 0; i + 1 < node.content.length; i += 2) {
      value += String.fromCharCode((node.content[i] << 8) | node.content[i + 1]);
    }
    return value;
  }
  return new TextDecoder().decode(node.content);
}

function parseName(node: Tlv): ParsedDistinguishedName {
  requireTag(node, TAG_SEQUENCE, "Distinguished Name");
  const attributes: Record<string, string[]> = {};

  for (const set of readChildren(node.content)) {
    if (set.tag !== TAG_SET) continue;
    for (const sequence of readChildren(set.content)) {
      if (sequence.tag !== TAG_SEQUENCE) continue;
      const pair = readChildren(sequence.content);
      if (pair.length < 2 || pair[0].tag !== TAG_OID) continue;
      const oid = decodeOid(pair[0].content);
      const value = decodeString(pair[1]);
      if (!attributes[oid]) attributes[oid] = [];
      attributes[oid].push(value);
    }
  }

  const first = (oid: string) => attributes[oid]?.[0] ?? "";
  return {
    commonName: first(DN_OIDS.commonName),
    organization: first(DN_OIDS.organization),
    organizationalUnit: first(DN_OIDS.organizationalUnit),
    country: first(DN_OIDS.country),
    locality: first(DN_OIDS.locality),
    stateOrProvince: first(DN_OIDS.stateOrProvince),
    emailAddress: first(DN_OIDS.emailAddress),
    attributes,
  };
}

function parseAsn1Time(node: Tlv): string {
  const raw = new TextDecoder().decode(node.content);
  const match = node.tag === TAG_UTC_TIME
    ? raw.match(/^(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})Z$/)
    : node.tag === TAG_GENERALIZED_TIME
      ? raw.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(?:\.\d+)?Z$/)
      : null;
  if (!match) throw new Error(`Thời gian X.509 không hợp lệ: ${raw}`);

  const shortYear = node.tag === TAG_UTC_TIME;
  const parsedYear = Number(match[1]);
  const year = shortYear ? (parsedYear >= 50 ? 1900 + parsedYear : 2000 + parsedYear) : parsedYear;
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  return new Date(Date.UTC(year, month - 1, day, hour, minute, second)).toISOString();
}

function parseAlgorithmIdentifier(node: Tlv): { oid: string; name: string } {
  requireTag(node, TAG_SEQUENCE, "AlgorithmIdentifier");
  const oidNode = readChildren(node.content).find((child) => child.tag === TAG_OID);
  if (!oidNode) throw new Error("AlgorithmIdentifier không có OID");
  const oid = decodeOid(oidNode.content);
  return { oid, name: ALGORITHM_NAMES[oid] ?? oid };
}

function isConstructed(tag: number): boolean {
  return (tag & 0x20) !== 0;
}

function findExtensionValue(bytes: Uint8Array, oid: Uint8Array): Uint8Array | null {
  let off = 0;
  while (off < bytes.length) {
    const node = readTlv(bytes, off);
    if (node.tag === TAG_SEQUENCE) {
      const children = readChildren(node.content);
      if (children.length >= 2 && children[0].tag === TAG_OID && bytesEqual(children[0].content, oid)) {
        const octet = children.find((child) => child.tag === TAG_OCTET_STRING);
        if (octet) return octet.content;
      }
    }
    if (isConstructed(node.tag)) {
      const found = findExtensionValue(node.content, oid);
      if (found) return found;
    }
    off = node.end;
  }
  return null;
}

function parseSubjectAltName(der: Uint8Array): ParsedCertificateJson["subjectAltName"] {
  const result: ParsedCertificateJson["subjectAltName"] = {
    emails: [],
    dnsNames: [],
    uris: [],
    ipAddresses: [],
  };
  const extension = findExtensionValue(der, SAN_OID);
  if (!extension) return result;

  const sequence = requireTag(readTlv(extension, 0), TAG_SEQUENCE, "SubjectAltName");
  for (const name of readChildren(sequence.content)) {
    const value = new TextDecoder().decode(name.content);
    if (name.tag === TAG_EMAIL) result.emails.push(value);
    else if (name.tag === TAG_DNS) result.dnsNames.push(value);
    else if (name.tag === TAG_URI) result.uris.push(value);
    else if (name.tag === TAG_IP) result.ipAddresses.push(Array.from(name.content).join("."));
  }
  return result;
}

function pemToDer(pem: string): Uint8Array {
  if (!pem.includes("-----BEGIN CERTIFICATE-----") || !pem.includes("-----END CERTIFICATE-----")) {
    throw new Error("Certificate PEM không hợp lệ");
  }
  const body = pem
    .replace(/-----BEGIN CERTIFICATE-----/, "")
    .replace(/-----END CERTIFICATE-----/, "")
    .replace(/\s+/g, "");
  if (!body) throw new Error("Certificate PEM rỗng");

  let binary: string;
  try {
    binary = atob(body);
  } catch {
    throw new Error("Certificate PEM chứa base64 không hợp lệ");
  }
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

function ownerIdFromUris(uris: string[]): string | null {
  const uri = uris.find((value) => value.startsWith(OWNER_URI_PREFIX));
  if (!uri) return null;
  const encoded = uri.slice(OWNER_URI_PREFIX.length);
  if (!encoded) return null;
  try {
    return decodeURIComponent(encoded) || null;
  } catch {
    throw new Error("Owner URI trong certificate không hợp lệ");
  }
}

// Chuyển certificate PEM thành một object thuần có thể JSON.stringify trực tiếp.
export async function certificatePemToJson(certificatePem: string): Promise<ParsedCertificateJson> {
  const der = pemToDer(certificatePem);
  const certificate = requireTag(readTlv(der, 0), TAG_SEQUENCE, "Certificate");
  if (certificate.end !== der.length) throw new Error("Certificate DER có dữ liệu thừa");

  const certificateChildren = readChildren(certificate.content);
  if (certificateChildren.length < 3) throw new Error("Certificate X.509 thiếu thành phần bắt buộc");
  const tbsCertificate = requireTag(certificateChildren[0], TAG_SEQUENCE, "TBSCertificate");
  const tbs = readChildren(tbsCertificate.content);
  let index = 0;

  let version = 1;
  if (tbs[index]?.tag === TAG_VERSION) {
    const versionNode = requireTag(readTlv(tbs[index].content, 0), TAG_INTEGER, "Version");
    version = integerToNumber(versionNode.content) + 1;
    index++;
  }

  const serialNode = requireTag(tbs[index++], TAG_INTEGER, "Serial Number");
  const signatureAlgorithm = parseAlgorithmIdentifier(tbs[index++]);
  const issuer = parseName(tbs[index++]);
  const validityNode = requireTag(tbs[index++], TAG_SEQUENCE, "Validity");
  const validity = readChildren(validityNode.content);
  if (validity.length < 2) throw new Error("Certificate thiếu thời gian hiệu lực");
  const subject = parseName(tbs[index++]);
  const subjectPublicKeyInfo = requireTag(tbs[index++], TAG_SEQUENCE, "SubjectPublicKeyInfo");
  const spkiChildren = readChildren(subjectPublicKeyInfo.content);
  if (spkiChildren.length === 0) throw new Error("Certificate thiếu public key algorithm");

  let serialBytes = serialNode.content;
  while (serialBytes.length > 1 && serialBytes[0] === 0) serialBytes = serialBytes.subarray(1);
  const subjectAltName = parseSubjectAltName(der);
  const fingerprint = new Uint8Array(await crypto.subtle.digest("SHA-256", der as BufferSource));

  return {
    version,
    serialNumber: bytesToHex(serialBytes),
    signatureAlgorithm,
    publicKeyAlgorithm: parseAlgorithmIdentifier(spkiChildren[0]),
    issuer,
    subject,
    validity: {
      notBefore: parseAsn1Time(validity[0]),
      notAfter: parseAsn1Time(validity[1]),
    },
    subjectAltName,
    ownerId: ownerIdFromUris(subjectAltName.uris),
    fingerprintSha256: bytesToHex(fingerprint, ":"),
  };
}

// API đồng bộ được AS Exchange dùng để lấy ID_c trước khi gửi AS_REQ.
export function extractOwnerIdFromCertificate(certificatePem: string): string {
  const der = pemToDer(certificatePem);
  const ownerId = ownerIdFromUris(parseSubjectAltName(der).uris);
  if (!ownerId) throw new Error("Certificate không chứa owner URI hợp lệ");
  return ownerId;
}
