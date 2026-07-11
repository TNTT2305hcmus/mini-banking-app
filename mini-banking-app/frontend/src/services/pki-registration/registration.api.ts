// Các lệnh gọi gateway cho luồng đăng ký: OTP request → OTP verify → PKI register.

import { apiPost } from "../api.service";
import { operationHeaders, type OperationId } from "../operation-id";

// Kết quả /v1/otp/request
export interface RequestOtpResult {
  email_masked: string;
  expires_in: number;
}

// Kết quả /v1/otp/verify
export interface VerifyOtpResult {
  reg_token: string;
  owner_id: string;
  expires_in: number;
}

// Kết quả /v1/auth/register (client cert vừa được Client CA cấp)
export interface RegisterPkiResult {
  cert_pem: string;
  cert_serial: string;
  issued_at: number;
  expires_at: number; // unix seconds (notAfter)
  chain_pem: string; // chuỗi cert (Client CA...) để client verify cert cấp về Root nhúng
}

// Bước 1: yêu cầu gửi OTP về email
export function requestOtp(email: string, operationId?: OperationId): Promise<RequestOtpResult> {
  return apiPost<RequestOtpResult>("/v1/otp/request", { email }, operationHeaders(operationId));
}

// Bước 2: xác thực OTP, nhận reg_token dùng 1 lần
export function verifyOtp(email: string, otp: string, operationId?: OperationId): Promise<VerifyOtpResult> {
  return apiPost<VerifyOtpResult>("/v1/otp/verify", { email, otp }, operationHeaders(operationId));
}

// Bước 3: gửi CSR kèm reg_token (Bearer), nhận X.509 client certificate
export function registerPki(params: {
  csrPem: string;
  fullName: string;
  regToken: string;
  operationId?: OperationId;
}): Promise<RegisterPkiResult> {
  return apiPost<RegisterPkiResult>(
    "/v1/auth/register",
    { csrPem: params.csrPem, fullName: params.fullName },
    { ...operationHeaders(params.operationId), Authorization: `Bearer ${params.regToken}` },
  );
}
