// Trích xuất clientId (owner_id) từ X.509 certificate đã lưu trong IndexedDB.
//
// CA Service nhúng owner_id vào certificate dưới dạng URI SubjectAltName:
//   urn:mini-banking:owner:<url-escaped owner_id>
// (xem ca-service/internal/ca/service.go — template.URIs).
// KDC chỉ tin owner_id lấy từ CA DB; ở client ta dùng giá trị này làm ID_c để gửi AS_REQ,
// và chính KDC sẽ ràng buộc lại với owner_id authoritative của certificate (fail-closed binding).

// Một node ASN.1 DER theo dạng tag-length-value.
interface Tlv {
  tag: number;
  content: Uint8Array;
  end: number; // offset byte ngay sau node này trong buffer gốc
}

// OID subjectAltName 2.5.29.17
const SAN_OID = new Uint8Array([0x55, 0x1d, 0x11]);
const TAG_SEQUENCE = 0x30;
const TAG_OID = 0x06;
const TAG_OCTET_STRING = 0x04;
const TAG_URI = 0x86; // GeneralName uniformResourceIdentifier = [6] IA5String (primitive)

const OWNER_URI_PREFIX = "urn:mini-banking:owner:";

// Đọc một TLV bắt đầu tại offset `off`.
function readTlv(buf: Uint8Array, off: number): Tlv {
  const tag = buf[off];
  let i = off + 1;
  let len = buf[i++];
  if (len & 0x80) {
    const numBytes = len & 0x7f;
    len = 0;
    for (let k = 0; k < numBytes; k++) len = (len << 8) | buf[i++];
  }
  return { tag, content: buf.subarray(i, i + len), end: i + len };
}

// Đọc toàn bộ các TLV con nằm liền nhau trong `content`.
function readChildren(content: Uint8Array): Tlv[] {
  const out: Tlv[] = [];
  let off = 0;
  while (off < content.length) {
    const node = readTlv(content, off);
    out.push(node);
    off = node.end;
  }
  return out;
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function isConstructed(tag: number): boolean {
  return (tag & 0x20) !== 0;
}

// Tìm extnValue (OCTET STRING content) của extension có OID cho trước, duyệt đệ quy toàn cây.
function findExtensionValue(bytes: Uint8Array, oid: Uint8Array): Uint8Array | null {
  let off = 0;
  while (off < bytes.length) {
    const node = readTlv(bytes, off);
    if (node.tag === TAG_SEQUENCE) {
      const children = readChildren(node.content);
      if (
        children.length >= 2 &&
        children[0].tag === TAG_OID &&
        bytesEqual(children[0].content, oid)
      ) {
        const octet = children.find((c) => c.tag === TAG_OCTET_STRING);
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

// extnValue của SAN bọc một SEQUENCE OF GeneralName; trả về URI đầu tiên (tag [6]).
function findUriGeneralName(extnValue: Uint8Array): string | null {
  const seq = readTlv(extnValue, 0); // SubjectAltName ::= GeneralNames ::= SEQUENCE OF GeneralName
  for (const gn of readChildren(seq.content)) {
    if (gn.tag === TAG_URI) return new TextDecoder().decode(gn.content);
  }
  return null;
}

// Tách phần DER từ một certificate PEM.
function pemToDer(pem: string): Uint8Array {
  const body = pem
    .replace(/-----BEGIN CERTIFICATE-----/, "")
    .replace(/-----END CERTIFICATE-----/, "")
    .replace(/\s+/g, "");
  const binary = atob(body);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

// Trích owner_id (clientId) từ certificate PEM. Ném lỗi nếu không tìm thấy URI SAN owner.
export function extractOwnerIdFromCertificate(certificatePem: string): string {
  const der = pemToDer(certificatePem);
  const san = findExtensionValue(der, SAN_OID);
  if (!san) throw new Error("Certificate không có SubjectAltName");

  const uri = findUriGeneralName(san);
  if (!uri || !uri.startsWith(OWNER_URI_PREFIX)) {
    throw new Error("Certificate không chứa owner URI hợp lệ");
  }
  const ownerId = decodeURIComponent(uri.slice(OWNER_URI_PREFIX.length));
  if (!ownerId) throw new Error("Không đọc được owner_id từ certificate");
  return ownerId;
}
