// Dựng và tự ký CSR PKCS#10 ngay trong browser. Chữ ký trên CertificationRequestInfo
// chính là proof-of-possession: chứng minh client giữ private key tương ứng public key
// trong request. CA (Go x509.ParseCertificateRequest) verify chữ ký này trước khi cấp cert.

import { exportPublicKeySpki, signRsaSha256, derToPem } from "../key.service";
import { der, OID } from "./asn1";

export interface CsrSubject {
  // Common Name — thường là họ tên đầy đủ của khách hàng
  commonName: string;
  // emailAddress của subject
  email: string;
}

// Dựng subject Name: SEQUENCE các RDN một thuộc tính (CN, email)
function buildSubjectName(subject: CsrSubject): Uint8Array {
  const cnRdn = der.set(der.sequence(der.oid(OID.commonName), der.utf8String(subject.commonName)));
  const emailRdn = der.set(der.sequence(der.oid(OID.emailAddress), der.ia5String(subject.email)));
  return der.sequence(cnRdn, emailRdn);
}

// Dựng attributes [0] chứa extensionRequest -> subjectAltName(rfc822Name = email).
// CA (Go x509) chỉ đọc email từ SAN (csr.EmailAddresses), nên email BẮT BUỘC phải nằm ở đây.
function buildAttributes(subject: CsrSubject): Uint8Array {
  // SubjectAltName ::= GeneralNames ::= SEQUENCE OF GeneralName
  const sanValue = der.sequence(der.rfc822Name(subject.email));
  // Extension ::= SEQUENCE { extnID, extnValue OCTET STRING(DER của SAN) }
  const sanExtension = der.sequence(der.oid(OID.subjectAltName), der.octetString(sanValue));
  // Extensions ::= SEQUENCE OF Extension
  const extensions = der.sequence(sanExtension);
  // Attribute ::= SEQUENCE { type = extensionRequest, values SET { Extensions } }
  const attribute = der.sequence(der.oid(OID.extensionRequest), der.set(extensions));
  // attributes [0] IMPLICIT SET OF Attribute (chỉ một attribute)
  return der.context0(attribute);
}

// Dựng và ký CSR, trả về DER
export async function buildCsrDer(keyPair: CryptoKeyPair, subject: CsrSubject): Promise<Uint8Array> {
  const spki = await exportPublicKeySpki(keyPair.publicKey);

  // CertificationRequestInfo — phần dữ liệu sẽ được ký
  const requestInfo = der.sequence(
    der.integer(0), // version v1 (0)
    buildSubjectName(subject),
    der.raw(spki), // SubjectPublicKeyInfo, đã là DER hợp lệ
    buildAttributes(subject), // attributes [0] — extensionRequest mang SAN email
  );

  // Proof-of-possession: ký phần info bằng private key
  const signature = await signRsaSha256(keyPair.privateKey, requestInfo);

  const signatureAlgorithm = der.sequence(der.oid(OID.sha256WithRSA), der.null());

  return der.sequence(requestInfo, signatureAlgorithm, der.bitString(signature));
}

// Dựng và ký CSR, trả về PEM (-----BEGIN CERTIFICATE REQUEST-----)
export async function buildCsrPem(keyPair: CryptoKeyPair, subject: CsrSubject): Promise<string> {
  const derBytes = await buildCsrDer(keyPair, subject);
  return derToPem(derBytes, "CERTIFICATE REQUEST");
}
