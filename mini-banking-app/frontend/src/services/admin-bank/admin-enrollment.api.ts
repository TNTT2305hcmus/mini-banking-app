import { apiPost } from "../api.service"

export interface AdminActivationResult {
  cert_pem: string
  cert_serial: string
  issued_at: number
  expires_at: number
  admin_id: string
  email: string
  full_name: string
  role: string
}

export function postAdminActivation(params: {
  activationToken: string
  csrPem: string
}): Promise<AdminActivationResult> {
  return apiPost<AdminActivationResult>("/v1/admin/bank/activate", {
    activation_token: params.activationToken,
    csr_pem: params.csrPem,
  })
}
