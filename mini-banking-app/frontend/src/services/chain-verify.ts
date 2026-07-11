// X.509 chain verification tối thiểu, chạy trong browser (WebCrypto), không thư viện ngoài.
//
// Mục tiêu: xác minh một chuỗi cert [leaf, intermediate...] thực sự chain về Root CA
// mà frontend NẠP RUNTIME từ file same-origin (trust-anchors.ts → loadRootCaPem),
// rồi trả về public key của leaf đã được xác thực để bước sau verify chữ ký dữ liệu
// (vd kdc_signature trên AS_REP).
//
// Phạm vi có chủ đích (đủ cho PKI nội bộ của dự án, không phải thư viện X.509 tổng quát):
//   - Chỉ hỗ trợ chữ ký cert RSA PKCS#1 v1.5 / SHA-256 (đúng thuật toán CA đang cấp).
//   - Kiểm: chữ ký từng mắt xích, khớp issuer/subject DN, thời hạn hiệu lực,
//     intermediate phải là CA (basicConstraints), và chain kết thúc ở Root nhúng sẵn.
//   - KHÔNG kiểm CRL/OCSP (thu hồi) — xem plan GĐ 4.3.

import { loadRootCaPem } from "../config/trust-anchors";

// ── ASN.1 DER TLV reader ────────────────────────────────────────────────────
interface Tlv {
  tag: number;
  contentStart: number; // offset của byte nội dung đầu tiên
  end: number; // offset sau phần tử (exclusive)
  raw: Uint8Array; // toàn bộ bytes TLV (tag + length + content) — dùng để hash/so khớp
}

function readTlv(buf: Uint8Array, off: number): Tlv {
  if (off + 2 > buf.length) throw new Error("DER bị cắt ngắn");
  const tag = buf[off];
  let i = off + 1;
  let len = buf[i++];
  if ((len & 0x80) !== 0) {
    const n = len & 0x7f;
    if (n === 0 || n > 4 || i + n > buf.length)
      throw new Error("Độ dài DER không hợp lệ");
    len = 0;
    for (let k = 0; k < n; k++) len = len * 256 + buf[i++];
  }
  const contentStart = i;
  const end = i + len;
  if (end > buf.length) throw new Error("Nội dung DER vượt quá buffer");
  return { tag, contentStart, end, raw: buf.subarray(off, end) };
}

// Đọc lần lượt các phần tử con trong `content` (từ contentStart..end của cha).
function children(buf: Uint8Array, parent: Tlv): Tlv[] {
  const out: Tlv[] = [];
  let off = parent.contentStart;
  while (off < parent.end) {
    const node = readTlv(buf, off);
    out.push(node);
    if (node.end <= off) throw new Error("Vòng lặp DER không tiến triển");
    off = node.end;
  }
  return out;
}

function decodeOid(content: Uint8Array): string {
  if (content.length === 0) throw new Error("OID rỗng");
  const first = content[0];
  const parts = [Math.floor(first / 40), first % 40];
  let value = 0;
  for (let i = 1; i < content.length; i++) {
    value = value * 128 + (content[i] & 0x7f);
    if ((content[i] & 0x80) === 0) {
      parts.push(value);
      value = 0;
    }
  }
  return parts.join(".");
}

// Decode OID từ một node TLV có tag OID (dùng đúng phần content của node).
function oidOf(buf: Uint8Array, node: Tlv): string {
  return decodeOid(buf.subarray(node.contentStart, node.end));
}

// ── Cert PEM → DER ──────────────────────────────────────────────────────────
function pemBlocks(pem: string): Uint8Array[] {
  const re = /-----BEGIN CERTIFICATE-----([\s\S]*?)-----END CERTIFICATE-----/g;
  const out: Uint8Array[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(pem)) !== null) {
    const b64 = m[1].replace(/\s+/g, "");
    if (!b64) continue;
    const bin = atob(b64);
    const der = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) der[i] = bin.charCodeAt(i);
    out.push(der);
  }
  return out;
}

const OID_SHA256_RSA = "1.2.840.113549.1.1.11";
const OID_BASIC_CONSTRAINTS = "2.5.29.19";
const OID_CN = "2.5.4.3";
const TAG_BOOLEAN = 0x01;
const TAG_OID = 0x06;
const TAG_SEQUENCE = 0x30;
const TAG_EXTENSIONS = 0xa3; // [3] EXPLICIT

interface ParsedCert {
  der: Uint8Array;
  tbsRaw: Uint8Array; // bytes được ký
  sigAlgOid: string;
  signature: Uint8Array;
  issuerName: string;
  subjectName: string;
  spkiRaw: Uint8Array; // SubjectPublicKeyInfo DER — importKey("spki")
  notBefore: Date;
  notAfter: Date;
  subjectCN: string;
  isCA: boolean;
}

function parseAsn1Time(node: Tlv, buf: Uint8Array): Date {
  const raw = new TextDecoder().decode(
    buf.subarray(node.contentStart, node.end),
  );
  // UTCTime (0x17): YYMMDDHHMMSSZ ; GeneralizedTime (0x18): YYYYMMDDHHMMSSZ
  let m: RegExpMatchArray | null;
  let year: number;
  if (node.tag === 0x17) {
    m = raw.match(/^(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})Z$/);
    if (!m) throw new Error(`UTCTime không hợp lệ: ${raw}`);
    const yy = Number(m[1]);
    year = yy >= 50 ? 1900 + yy : 2000 + yy;
  } else if (node.tag === 0x18) {
    m = raw.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})Z$/);
    if (!m) throw new Error(`GeneralizedTime không hợp lệ: ${raw}`);
    year = Number(m[1]);
  } else {
    throw new Error("Kiểu thời gian X.509 không hỗ trợ");
  }
  return new Date(
    Date.UTC(
      year,
      Number(m[2]) - 1,
      Number(m[3]),
      Number(m[4]),
      Number(m[5]),
      Number(m[6]),
    ),
  );
}

// Parse Name ::= SEQUENCE OF RDN(SET OF AttributeTypeAndValue(SEQ{OID, value})).
// Trả về CN (giữ nguyên hoa/thường để hiển thị & kiểm CN=kdc-service) và dạng chuẩn
// hóa "oid=value" (đã trim/lowercase/gộp khoảng trắng) để chain name matching bền vững
// trước khác biệt encoding giữa các bộ tạo cert.
function parseName(
  buf: Uint8Array,
  name: Tlv,
): { cn: string; canonical: string } {
  const parts: string[] = [];
  let cn = "";
  for (const rdn of children(buf, name)) {
    for (const atv of children(buf, rdn)) {
      const pair = children(buf, atv);
      if (pair.length < 2 || pair[0].tag !== TAG_OID) continue;
      const oid = oidOf(buf, pair[0]);
      const value = new TextDecoder().decode(
        buf.subarray(pair[1].contentStart, pair[1].end),
      );
      parts.push(`${oid}=${value.trim().toLowerCase().replace(/\s+/g, " ")}`);
      if (oid === OID_CN && !cn) cn = value;
    }
  }
  return { cn, canonical: parts.join(",") };
}

function parseBasicConstraintsCA(
  buf: Uint8Array,
  extensions: Tlv | null,
): boolean {
  if (!extensions) return false;
  // extensions [3] EXPLICIT SEQUENCE OF Extension
  const seq = children(buf, extensions)[0];
  if (!seq) return false;
  for (const ext of children(buf, seq)) {
    const parts = children(buf, ext);
    if (parts.length < 2 || parts[0].tag !== TAG_OID) continue;
    if (oidOf(buf, parts[0]) !== OID_BASIC_CONSTRAINTS) continue;
    // extnValue là OCTET STRING (có thể sau BOOLEAN critical) bọc SEQUENCE{cA BOOLEAN?...}
    const octet = parts[parts.length - 1];
    const inner = readTlv(buf, octet.contentStart);
    if (inner.tag !== TAG_SEQUENCE) return false;
    const bcParts = children(buf, inner);
    if (bcParts.length >= 1 && bcParts[0].tag === TAG_BOOLEAN) {
      return buf[bcParts[0].contentStart] !== 0x00;
    }
    return false; // cA mặc định FALSE
  }
  return false;
}

function parseCert(der: Uint8Array): ParsedCert {
  const cert = readTlv(der, 0);
  const kids = children(der, cert);
  if (kids.length < 3) throw new Error("Certificate thiếu thành phần");
  const tbs = kids[0];
  const sigAlg = kids[1];
  const sigBit = kids[2];

  // signatureAlgorithm OID (phần tử con đầu của SEQUENCE AlgorithmIdentifier)
  const sigAlgOid = oidOf(der, children(der, sigAlg)[0]);
  // signatureValue: BIT STRING, byte đầu = số bit thừa (0)
  const signature = der.subarray(sigBit.contentStart + 1, sigBit.end);

  // TBS children theo thứ tự: [version]? serial sigAlg issuer validity subject spki ...ext
  const tbsKids = children(der, tbs);
  let idx = 0;
  if (tbsKids[idx].tag === 0xa0) idx++; // version [0]
  idx++; // serialNumber
  idx++; // signature AlgorithmIdentifier
  const issuer = tbsKids[idx++];
  const validity = tbsKids[idx++];
  const subject = tbsKids[idx++];
  const spki = tbsKids[idx++];
  let extensions: Tlv | null = null;
  for (let j = idx; j < tbsKids.length; j++) {
    if (tbsKids[j].tag === TAG_EXTENSIONS) {
      extensions = tbsKids[j];
      break;
    }
  }

  const vKids = children(der, validity);
  const notBefore = parseAsn1Time(vKids[0], der);
  const notAfter = parseAsn1Time(vKids[1], der);

  const issuerName = parseName(der, issuer);
  const subjectName = parseName(der, subject);

  return {
    der,
    tbsRaw: tbs.raw,
    sigAlgOid,
    signature,
    issuerName: issuerName.canonical,
    subjectName: subjectName.canonical,
    spkiRaw: spki.raw,
    notBefore,
    notAfter,
    subjectCN: subjectName.cn,
    isCA: parseBasicConstraintsCA(der, extensions),
  };
}

// ── Verify một cert được ký bởi issuer ──────────────────────────────────────
async function verifySignedBy(
  child: ParsedCert,
  issuer: ParsedCert,
): Promise<void> {
  if (child.sigAlgOid !== OID_SHA256_RSA) {
    throw new Error(`Thuật toán chữ ký cert không hỗ trợ: ${child.sigAlgOid}`);
  }
  if (child.issuerName !== issuer.subjectName) {
    throw new Error("issuer DN không khớp subject của cert cấp trên");
  }
  const key = await crypto.subtle.importKey(
    "spki",
    issuer.spkiRaw as BufferSource,
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["verify"],
  );
  const ok = await crypto.subtle.verify(
    { name: "RSASSA-PKCS1-v1_5" },
    key,
    child.signature as BufferSource,
    child.tbsRaw as BufferSource,
  );
  if (!ok) throw new Error("Chữ ký cert không hợp lệ (không do issuer ký)");
}

function assertValidityWindow(
  cert: ParsedCert,
  now: Date,
  label: string,
): void {
  if (now < cert.notBefore) throw new Error(`${label}: cert chưa có hiệu lực`);
  if (now > cert.notAfter) throw new Error(`${label}: cert đã hết hạn`);
}

export interface VerifiedLeaf {
  subjectCN: string;
  // Public key của leaf, đã import sẵn để verify chữ ký RSA-PSS/SHA-256 trên dữ liệu.
  publicKeyPss: CryptoKey;
  // SubjectPublicKeyInfo (DER) của leaf — để so khớp byte với khóa client vừa sinh
  // (chống tráo cert: cert phải bind đúng public key của mình).
  publicKeySpki: Uint8Array;
}

// Root CA nạp từ ngoài (trust-anchors.ts → fetch), parse & cache một lần.
let cachedAnchor: ParsedCert | null = null;
async function anchor(): Promise<ParsedCert> {
  if (!cachedAnchor) {
    const pem = await loadRootCaPem();
    const blocks = pemBlocks(pem);
    if (blocks.length === 0) throw new Error("Root CA PEM rỗng/không hợp lệ");
    cachedAnchor = parseCert(blocks[0]);
  }
  return cachedAnchor;
}

/**
 * Xác minh chuỗi [leaf, intermediate...] chain về Root CA nạp runtime (same-origin).
 * @param leafPem  PEM chứa leaf cert ĐẦU TIÊN, có thể kèm luôn các intermediate phía
 *   sau trong cùng chuỗi (KDC gửi bundle leaf+Client CA); hàm tách mọi block.
 * @param intermediatePems  0..n PEM intermediate bổ sung (mỗi phần tử có thể nhiều block).
 * @returns publicKey của leaf (import cho RSA-PSS verify) + subject CN.
 * @throws nếu bất kỳ mắt xích nào không verify được, sai DN, hết hạn, hoặc không chain
 *   về đúng Root nhúng.
 */
export async function verifyChainToRoot(
  leafPem: string,
  intermediatePems: string[] = [],
): Promise<VerifiedLeaf> {
  // leafPem có thể là bundle nhiều cert (leaf + intermediate). Lấy TẤT CẢ block theo
  // thứ tự, rồi nối thêm intermediate rời (nếu có).
  const derChain: Uint8Array[] = [...pemBlocks(leafPem)];
  if (derChain.length === 0)
    throw new Error("Không tìm thấy leaf certificate trong PEM");
  for (const pem of intermediatePems) derChain.push(...pemBlocks(pem));

  const certs = derChain.map(parseCert);
  const root = await anchor();
  const now = new Date();

  // Verify từng mắt xích: certs[i] được ký bởi certs[i+1], mắt xích cuối bởi Root nhúng.
  for (let i = 0; i < certs.length; i++) {
    const issuer = i + 1 < certs.length ? certs[i + 1] : root;
    assertValidityWindow(
      certs[i],
      now,
      i === 0 ? "leaf" : `intermediate[${i}]`,
    );
    // Mọi issuer trung gian (không phải leaf) phải là CA hợp lệ.
    if (i > 0 && !certs[i].isCA)
      throw new Error(`intermediate[${i}] không phải CA`);
    await verifySignedBy(certs[i], issuer);
  }

  // Kết thúc đúng ở Root nhúng: mắt xích trên cùng phải do Root ký (đã verify ở trên
  // vì issuer cuối = root), và Root nhúng là anchor tin cậy tiên đề.
  assertValidityWindow(root, now, "root");

  const leaf = certs[0];
  const publicKeyPss = await crypto.subtle.importKey(
    "spki",
    leaf.spkiRaw as BufferSource,
    { name: "RSA-PSS", hash: "SHA-256" },
    false,
    ["verify"],
  );

  return { subjectCN: leaf.subjectCN, publicKeyPss, publicKeySpki: leaf.spkiRaw };
}
