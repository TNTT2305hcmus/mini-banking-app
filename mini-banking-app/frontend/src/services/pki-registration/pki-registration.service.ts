// Orchestrate phần browser của PHA 3 (POST /v1/pki/register):
//   1. Sinh cặp khóa RSA (WebCrypto)
//   2. Dựng + ký CSR (proof-of-possession)
//   3. Wrap private key bằng PIN và lưu vào IndexedDB
//   4. Sau khi gateway trả 201, lưu certificate PEM và hồ sơ người dùng
// Private key không bao giờ rời browser dạng plaintext; chỉ CSR (public key + chữ ký PoP) gửi lên server.
// Phần network để component tự gọi: prepareEnrollment trả csrPem, nhận 201 thì gọi storeCertificate.

import { STORES, idbGet, idbPut, idbPutMany, idbDelete } from "../db.service";
import {
  generateClientKeyPair,
  wrapPrivateKey,
  unwrapPrivateKey,
  type WrappedPrivateKey,
} from "../key.service";
import { buildCsrPem, type CsrSubject } from "./csr";
import { registerPki } from "./registration.api";

// Các key trong store pki của IndexedDB
const KEYS = {
  wrappedPrivateKey: "wrapped_private_key",
  certificate: "certificate",
  clientProfile: "client_profile",
} as const;

// Record certificate lưu cạnh wrapped private key
export interface StoredCertificate {
  // X.509 certificate, định dạng PEM
  certificatePem: string;
  // serial number hex do CA cấp
  serialNumber: string;
  // Thời điểm hết hạn (ISO 8601 UTC) gateway trả về
  notAfter: string;
}

// Hồ sơ tối thiểu dùng để cá nhân hóa UI mà không cần yêu cầu lại email/username khi đăng nhập.
export interface StoredClientProfile {
  fullName: string;
}

export interface PrepareEnrollmentParams {
  // Họ tên khách hàng → CSR Common Name
  fullName: string;
  // Email đã xác minh → CSR subject emailAddress
  email: string;
  // PIN dùng để wrap private key, caller chỉ giữ tạm thời
  pin: string;
}

// Sinh key pair, dựng CSR, wrap private key bằng PIN, lưu wrapped key vào IndexedDB.
// Thứ tự quan trọng: lưu wrapped key TRƯỚC khi gửi CSR (step 9 trước step 10) để luôn có
// key local ứng với bất kỳ cert nào CA cấp. Trả về CSR PEM để gửi tới POST /v1/pki/register.
export async function prepareEnrollment(params: PrepareEnrollmentParams): Promise<{ csrPem: string }> {
  const subject: CsrSubject = { commonName: params.fullName, email: params.email };

  const keyPair = await generateClientKeyPair();
  const csrPem = await buildCsrPem(keyPair, subject);

  const wrapped = await wrapPrivateKey(keyPair.privateKey, params.pin);
  await idbPut<WrappedPrivateKey>(STORES.pki, KEYS.wrappedPrivateKey, wrapped);

  // Tham chiếu keyPair.privateKey bị bỏ khi return; chỉ còn bản wrapped và public key (trong CSR)
  return { csrPem };
}

// Lưu certificate đã cấp (gọi sau khi gateway trả 201)
export async function storeCertificate(cert: StoredCertificate): Promise<void> {
  await idbPut<StoredCertificate>(STORES.pki, KEYS.certificate, cert);
}

// Lưu hồ sơ người dùng độc lập để UI có thể đọc mà không cần parse certificate.
export async function storeClientProfile(profile: StoredClientProfile): Promise<void> {
  await idbPut<StoredClientProfile>(STORES.pki, KEYS.clientProfile, profile);
}

export interface EnrollAndRegisterParams extends PrepareEnrollmentParams {
  // reg_token nhận từ /v1/otp/verify (Bearer cho /v1/auth/register)
  regToken: string;
}

// Khép kín bước cuối: sinh key + CSR, lưu wrapped key, gọi gateway register,
// rồi lưu certificate và full name trong cùng transaction IndexedDB.
export async function enrollAndRegister(params: EnrollAndRegisterParams): Promise<StoredCertificate> {
  const { csrPem } = await prepareEnrollment({
    fullName: params.fullName,
    email: params.email,
    pin: params.pin,
  });

  const resp = await registerPki({
    csrPem,
    fullName: params.fullName,
    regToken: params.regToken,
  });

  const cert: StoredCertificate = {
    certificatePem: resp.cert_pem,
    serialNumber: resp.cert_serial,
    notAfter: new Date(resp.expires_at * 1000).toISOString(),
  };
  const profile: StoredClientProfile = { fullName: params.fullName.trim() };
  await idbPutMany(STORES.pki, [
    { key: KEYS.certificate, value: cert },
    { key: KEYS.clientProfile, value: profile },
  ]);
  return cert;
}

// Đọc record certificate đã lưu, undefined nếu chưa enroll
export function getStoredCertificate(): Promise<StoredCertificate | undefined> {
  return idbGet<StoredCertificate>(STORES.pki, KEYS.certificate);
}

// Đọc full name đã lưu để hiển thị trên màn hình đăng nhập và giao diện khách hàng.
export function getStoredClientProfile(): Promise<StoredClientProfile | undefined> {
  return idbGet<StoredClientProfile>(STORES.pki, KEYS.clientProfile);
}

// Đọc blob wrapped private key, undefined nếu chưa enroll
export function getWrappedPrivateKey(): Promise<WrappedPrivateKey | undefined> {
  return idbGet<WrappedPrivateKey>(STORES.pki, KEYS.wrappedPrivateKey);
}

// True khi đã có cả wrapped key lẫn certificate ở local
export async function isEnrolled(): Promise<boolean> {
  const [key, cert] = await Promise.all([getWrappedPrivateKey(), getStoredCertificate()]);
  return key !== undefined && cert !== undefined;
}

// Giải mã private key đã lưu bằng PIN, trả về signing key non-extractable
// cho việc ký AS_REQ / giao dịch sau này
export async function loadSigningKey(pin: string): Promise<CryptoKey> {
  const blob = await getWrappedPrivateKey();
  if (!blob) throw new Error("No enrolled private key found");
  return unwrapPrivateKey(blob, pin);
}

// Xóa toàn bộ PKI material đã lưu (vd: logout / enroll lại)
export async function clearEnrollment(): Promise<void> {
  await Promise.all([
    idbDelete(STORES.pki, KEYS.wrappedPrivateKey),
    idbDelete(STORES.pki, KEYS.certificate),
    idbDelete(STORES.pki, KEYS.clientProfile),
  ]);
}
