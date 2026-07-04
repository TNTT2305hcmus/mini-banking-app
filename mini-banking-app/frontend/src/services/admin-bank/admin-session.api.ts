import { apiPost } from "../api.service"

export interface AdminSessionResult {
  ap_rep: string
  expires_at_unix: number
  admin_id: string
  role: string
}

export function postAdminSession(params: {
  ticketV: string
  authenticator: string
}): Promise<AdminSessionResult> {
  return apiPost<AdminSessionResult>("/v1/admin/bank/session", {
    ticket_v: params.ticketV,
    authenticator: params.authenticator,
  })
}
