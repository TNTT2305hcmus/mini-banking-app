// Bề mặt public của feature AS Exchange (Phase 2 — Kerberos-like authentication).
// Component nên import từ đây thay vì chọc vào từng file.

export { performAsExchange, type AsExchangeResult } from "./as-exchange.service";

export { getSession, hasValidTgt, clearSession, type AsSession } from "./session";

export { extractOwnerIdFromCertificate } from "./cert.parser";

export { postAsReq, type AsReqParams, type AsRepResult } from "./as-exchange.api";
