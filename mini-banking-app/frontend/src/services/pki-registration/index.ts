// Bề mặt public của feature PKI registration (browser-side PHA 3).
// Component nên import từ đây thay vì chọc vào từng file.

export {
  prepareEnrollment,
  enrollAndRegister,
  storeCertificate,
  storeClientProfile,
  getStoredCertificate,
  getStoredClientProfile,
  getWrappedPrivateKey,
  isEnrolled,
  loadSigningKey,
  clearEnrollment,
  type EnrollmentScope,
  type PrepareEnrollmentParams,
  type EnrollAndRegisterParams,
  type StoredCertificate,
  type StoredClientProfile,
} from "./pki-registration.service";

export { buildCsrPem, buildCsrDer, type CsrSubject } from "./csr";

export {
  requestOtp,
  verifyOtp,
  registerPki,
  type RequestOtpResult,
  type VerifyOtpResult,
  type RegisterPkiResult,
} from "./registration.api";
