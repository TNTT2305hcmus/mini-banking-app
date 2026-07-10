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
import type { OperationId } from "../operation-id";

// Các key trong store pki của IndexedDB
const KEYS = {
  wrappedPrivateKey: "wrapped_private_key",
  certificate: "certificate",
  clientProfile: "client_profile",
} as const;

export type EnrollmentScope = "customer" | "bank_admin" | "ca_admin";

const scopedKey = (key: (typeof KEYS)[keyof typeof KEYS], scope: EnrollmentScope = "customer") =>
  `${scope}:${key}`;

// Record certificate lưu cạnh wrapped private key
export interface StoredCertificate {
  // X.509 certificate, định dạng PEM
  certificatePem: string;
  // serial number hex do Client CA cấp
  serialNumber: string;
  // Thời điểm hết hạn (ISO 8601 UTC) gateway trả về
  notAfter: string;
  // Vai trò local của cert. Cert customer đời cũ có thể chưa có field này.
  role?: EnrollmentScope;
}

// Hồ sơ tối thiểu dùng để cá nhân hóa UI mà không cần yêu cầu lại email/username khi đăng nhập.
export interface StoredClientProfile {
  fullName: string;
  role?: EnrollmentScope;
}

export interface PrepareEnrollmentParams {
  // Họ tên khách hàng → CSR Common Name
  fullName: string;
  // Email đã xác minh → CSR subject emailAddress
  email: string;
  // PIN dùng để wrap private key, caller chỉ giữ tạm thời
  pin: string;
  // Phân vùng local credential trong IndexedDB.
  scope?: EnrollmentScope;
}

// Sinh key pair, dựng CSR, wrap private key bằng PIN, lưu wrapped key vào IndexedDB.
// Thứ tự quan trọng: lưu wrapped key TRƯỚC khi gửi CSR (step 9 trước step 10) để luôn có
// key local ứng với bất kỳ client cert nào Client CA cấp. Trả về CSR PEM để gửi tới POST /v1/pki/register.
export async function prepareEnrollment(params: PrepareEnrollmentParams): Promise<{ csrPem: string }> {
  const subject: CsrSubject = { commonName: params.fullName, email: params.email };

  const keyPair = await generateClientKeyPair();
  const csrPem = await buildCsrPem(keyPair, subject);

  const wrapped = await wrapPrivateKey(keyPair.privateKey, params.pin);
  await idbPut<WrappedPrivateKey>(STORES.pki, scopedKey(KEYS.wrappedPrivateKey, params.scope), wrapped);

  // Tham chiếu keyPair.privateKey bị bỏ khi return; chỉ còn bản wrapped và public key (trong CSR)
  return { csrPem };
}

// Lưu certificate đã cấp (gọi sau khi gateway trả 201)
export async function storeCertificate(
  cert: StoredCertificate,
  scope: EnrollmentScope = "customer",
): Promise<void> {
  await idbPut<StoredCertificate>(STORES.pki, scopedKey(KEYS.certificate, scope), {
    ...cert,
    role: cert.role ?? scope,
  });
}

// Lưu hồ sơ người dùng độc lập để UI có thể đọc mà không cần parse certificate.
export async function storeClientProfile(
  profile: StoredClientProfile,
  scope: EnrollmentScope = "customer",
): Promise<void> {
  await idbPut<StoredClientProfile>(STORES.pki, scopedKey(KEYS.clientProfile, scope), {
    ...profile,
    role: profile.role ?? scope,
  });
}

export interface EnrollAndRegisterParams extends PrepareEnrollmentParams {
  // reg_token nhận từ /v1/otp/verify (Bearer cho /v1/auth/register)
  regToken: string;
  operationId?: OperationId;
}

// Khép kín bước cuối: sinh key + CSR, lưu wrapped key, gọi gateway register,
// rồi lưu certificate và full name trong cùng transaction IndexedDB.
export async function enrollAndRegister(params: EnrollAndRegisterParams): Promise<StoredCertificate> {
  const { csrPem } = await prepareEnrollment({
    fullName: params.fullName,
    email: params.email,
    pin: params.pin,
    scope: "customer",
  });

  const resp = await registerPki({
    csrPem,
    fullName: params.fullName,
    regToken: params.regToken,
    operationId: params.operationId,
  });

  const cert: StoredCertificate = {
    certificatePem: resp.cert_pem,
    serialNumber: resp.cert_serial,
    notAfter: new Date(resp.expires_at * 1000).toISOString(),
    role: "customer",
  };
  const profile: StoredClientProfile = { fullName: params.fullName.trim(), role: "customer" };
  await idbPutMany(STORES.pki, [
    { key: scopedKey(KEYS.certificate, "customer"), value: cert },
    { key: scopedKey(KEYS.clientProfile, "customer"), value: profile },
  ]);
  return cert;
}

// Đọc record certificate đã lưu, undefined nếu chưa enroll
export function getStoredCertificate(
  scope: EnrollmentScope = "customer",
): Promise<StoredCertificate | undefined> {
  return idbGet<StoredCertificate>(STORES.pki, scopedKey(KEYS.certificate, scope));
}

// Đọc full name đã lưu để hiển thị trên màn hình đăng nhập và giao diện khách hàng.
export function getStoredClientProfile(
  scope: EnrollmentScope = "customer",
): Promise<StoredClientProfile | undefined> {
  return idbGet<StoredClientProfile>(STORES.pki, scopedKey(KEYS.clientProfile, scope));
}

// Đọc blob wrapped private key, undefined nếu chưa enroll
export function getWrappedPrivateKey(
  scope: EnrollmentScope = "customer",
): Promise<WrappedPrivateKey | undefined> {
  return idbGet<WrappedPrivateKey>(STORES.pki, scopedKey(KEYS.wrappedPrivateKey, scope));
}

// True khi đã có cả wrapped key lẫn certificate ở local
export async function isEnrolled(scope: EnrollmentScope = "customer"): Promise<boolean> {
  const [key, cert] = await Promise.all([getWrappedPrivateKey(scope), getStoredCertificate(scope)]);
  return key !== undefined && cert !== undefined;
}

// Giải mã private key đã lưu bằng PIN, trả về signing key non-extractable
// cho việc ký AS_REQ / giao dịch sau này
export async function loadSigningKey(
  pin: string,
  scope: EnrollmentScope = "customer",
): Promise<CryptoKey> {
  const blob = await getWrappedPrivateKey(scope);
  if (!blob) throw new Error("No enrolled private key found");
  return unwrapPrivateKey(blob, pin);
}

// Xóa toàn bộ PKI material đã lưu (vd: logout / enroll lại)
export async function clearEnrollment(scope: EnrollmentScope = "customer"): Promise<void> {
  await Promise.all([
    idbDelete(STORES.pki, scopedKey(KEYS.wrappedPrivateKey, scope)),
    idbDelete(STORES.pki, scopedKey(KEYS.certificate, scope)),
    idbDelete(STORES.pki, scopedKey(KEYS.clientProfile, scope)),
  ]);
}
