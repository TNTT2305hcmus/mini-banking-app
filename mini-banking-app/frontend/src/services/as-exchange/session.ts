// Phiên Kerberos sau AS Exchange — chỉ giữ trong RAM, KHÔNG persist (IndexedDB/localStorage).
// TGT là opaque (mã hóa bằng K_tgs của KDC); K_{c,tgs} là session key client dùng cho TGS Exchange.

export interface AsSession {
  // owner_id (ID_c) lấy từ certificate
  clientId: string;
  // serial number certificate dùng cho AS/TGS request
  certSn: string;
  // TGT (base64 ciphertext) — xuất trình lại nguyên trạng ở TGS Exchange
  tgt: string;
  // K_{c,tgs}: session key AES-256 (32 byte) dùng mã hóa Authenticator/giải mã TGS_REP
  sessionKey: Uint8Array;
  // Unix seconds — thời điểm TGT hết hạn
  tgtExpiresAt: number;
  // Shared frontend trace id for a larger business flow such as login.
  operationId?: string;
}

let current: AsSession | null = null;

// Lưu phiên mới (ghi đè phiên cũ nếu có)
export function setSession(session: AsSession): void {
  clearSession();
  current = session;
}

// Lấy phiên hiện tại (null nếu chưa AS Exchange)
export function getSession(): AsSession | null {
  return current;
}

// TGT còn hiệu lực hay không
export function hasValidTgt(): boolean {
  return current !== null && current.tgtExpiresAt * 1000 > Date.now();
}

// Xóa phiên và zero K_{c,tgs} khỏi RAM (logout / TGT hết hạn)
export function clearSession(): void {
  if (current) current.sessionKey.fill(0);
  current = null;
}
