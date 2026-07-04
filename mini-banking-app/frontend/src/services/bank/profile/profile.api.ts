// Lệnh gọi gateway cho luồng xem thông tin cá nhân: POST /v1/auth/me.
// Body: ticket_v + authenticator (AP read, scope balance:read) + request_id (UUID v4).

import { apiPost } from "../../api.service";

export interface ProfileReqParams {
  ticketV: string; // base64 Ticket_v opaque (scope balance:read)
  authenticator: string; // base64 E_{K_c_v}[...]
  requestId: string; // UUID v4
}

export interface ProfileRepResult {
  ap_rep: string; // base64 E_{K_c_v}[profile...]
  account_id: string;
}

export function postProfile(params: ProfileReqParams): Promise<ProfileRepResult> {
  return apiPost<ProfileRepResult>("/v1/auth/me", {
    ticket_v: params.ticketV,
    authenticator: params.authenticator,
    request_id: params.requestId,
  });
}
