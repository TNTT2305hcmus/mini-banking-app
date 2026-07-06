// Kho service ticket sau TGS Exchange — chỉ giữ trong RAM, KHÔNG persist.
// Mỗi Ticket_v gắn đúng 1 scope (không đa scope), nên lưu theo map keyed bằng scope.
// Ticket_v opaque (mã hóa bằng K_v của Bank Service); K_{c,v} là session key client↔Bank.

export type Scope = "balance:read" | "transfer:create" | "history:read" | "bank-admin:read";

export interface ServiceTicketSession {
  scope: Scope;
  serviceId: string;
  // Ticket_v (base64 ciphertext) — xuất trình nguyên trạng cho Bank Service ở AP Exchange
  ticketV: string;
  // K_{c,v}: session key AES-256 (32 byte) dùng mã hóa payload/Authenticator AP, giải mã AP_REP
  sessionKey: Uint8Array;
  // Unix seconds — thời điểm Ticket_v hết hạn
  ticketExpiresAt: number;
  // Định danh đi kèm để Bank/AP verify; lấy từ phiên TGT
  clientId: string;
  certSn: string;
}

const tickets = new Map<Scope, ServiceTicketSession>();

// Lưu ticket cho scope (zero ticket cũ cùng scope nếu có)
export function setServiceTicket(ticket: ServiceTicketSession): void {
  const old = tickets.get(ticket.scope);
  if (old) old.sessionKey.fill(0);
  tickets.set(ticket.scope, ticket);
}

// Lấy ticket theo scope (undefined nếu chưa có)
export function getServiceTicket(scope: Scope): ServiceTicketSession | undefined {
  return tickets.get(scope);
}

// Ticket cho scope còn hiệu lực không
export function hasValidServiceTicket(scope: Scope): boolean {
  const t = tickets.get(scope);
  return t !== undefined && t.ticketExpiresAt * 1000 > Date.now();
}

// Xóa toàn bộ ticket và zero K_{c,v} khỏi RAM (logout / dọn phiên)
export function clearServiceTickets(): void {
  for (const t of tickets.values()) t.sessionKey.fill(0);
  tickets.clear();
}
