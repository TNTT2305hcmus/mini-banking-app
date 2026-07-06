import { performAsExchange } from "../as-exchange"
import { buildApAuthenticator } from "../bank/ap-authenticator"
import { aesGcmDecrypt, base64ToBytes } from "../key.service"
import { getServiceTicket, performTgsExchange } from "../tgs-exchange"
import { postAdminSession } from "./admin-session.api"

const SCOPE = "bank-admin:read" as const

interface AdminSessionApRep {
  result: string
  nonce: string
  request_id: string
  role: string
  session_expires_at: number
}

export interface BankAdminSession {
  adminId: string
  role: "bank_admin"
  expiresAtUnix: number
}

export async function createAdminSession(pin: string): Promise<BankAdminSession> {
  const asResult = await performAsExchange(pin)
  await performTgsExchange(SCOPE, true)
  const ticket = getServiceTicket(SCOPE)
  if (!ticket) throw new Error("Không lấy được Ticket_v cho Bank Admin")

  const auth = await buildApAuthenticator(ticket.sessionKey, ticket.clientId)
  const response = await postAdminSession({
    ticketV: ticket.ticketV,
    authenticator: auth.authenticator,
  })
  const apRep = JSON.parse(
    new TextDecoder().decode(
      await aesGcmDecrypt(ticket.sessionKey, base64ToBytes(response.ap_rep)),
    ),
  ) as AdminSessionApRep

  if (apRep.result !== "ok") throw new Error("ADMIN_SESSION_INVALID")
  if (apRep.nonce !== auth.nonce || apRep.request_id !== auth.requestId) {
    throw new Error("AP_REP nonce không khớp — nghi ngờ MITM/replay")
  }
  if (apRep.role !== "bank_admin" || response.role !== "bank_admin") {
    throw new Error("ADMIN_ROLE_REQUIRED")
  }
  if (response.admin_id !== asResult.clientId) {
    throw new Error("ADMIN_SESSION_INVALID")
  }
  if (apRep.session_expires_at !== response.expires_at_unix) {
    throw new Error("ADMIN_SESSION_INVALID")
  }

  return {
    adminId: response.admin_id,
    role: "bank_admin",
    expiresAtUnix: response.expires_at_unix,
  }
}
