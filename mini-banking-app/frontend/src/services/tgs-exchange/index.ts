// Bề mặt public của feature TGS Exchange (Phase 3 — Kerberos-like service ticket).
// Component nên import từ đây thay vì chọc vào từng file.

export { performTgsExchange, type TgsExchangeResult } from "./tgs-exchange.service";

export {
  getServiceTicket,
  hasValidServiceTicket,
  clearServiceTickets,
  type Scope,
  type ServiceTicketSession,
} from "./session";

export { postTgsReq, type TgsReqParams, type TgsRepResult } from "./tgs-exchange.api";
