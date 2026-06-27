// ─── Types ────────────────────────────────────────────────────────────────────
export type CertStatus = "active" | "revoked" | "expired"
export type TxStatus = "completed" | "failed" | "pending"

export interface Cert {
  id: string; serial: string; owner_id: string; cn: string; email: string
  fingerprint: string; not_before: string; not_after: string
  status: CertStatus; issued_at: string; revoked_at?: string; revocation_reason?: string
}
export interface BankUser {
  id: string; email: string; full_name: string
  status: "active" | "locked"; accounts: number
  total_balance: number; created_at: string
}
export interface Transaction {
  id: string; from_acct: string; to_acct: string
  from_name: string; to_name: string; amount: number
  status: TxStatus; description: string
  created_at: string; cert_serial: string; chain_hash: string
}
export interface CAuditEntry {
  id: string; action: string; performed_by: string
  serial: string; reason?: string; performed_at: string
}
export interface BAuditEntry {
  id: string; action: string; user_email: string
  cert_serial?: string; reason?: string; created_at: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────
export const CERTS: Cert[] = [
  { id: "c1", serial: "1A2B3C4D5E6F7A8B", owner_id: "u1", cn: "Alice Nguyen", email: "alice@minibank.vn", fingerprint: "a3:f1:2b:9c:4d:5e:6f:7a:8b:9c:0d:1e:2f:3a:4b:5c", not_before: "2025-01-15", not_after: "2026-01-15", status: "active", issued_at: "2025-01-15T08:00:00Z" },
  { id: "c2", serial: "2B3C4D5E6F7A8B9C", owner_id: "u2", cn: "Bob Tran", email: "bob@minibank.vn", fingerprint: "b4:c2:3d:0e:1f:2a:3b:4c:5d:6e:7f:8a:9b:0c:1d:2e", not_before: "2025-02-10", not_after: "2026-02-10", status: "active", issued_at: "2025-02-10T09:00:00Z" },
  { id: "c3", serial: "3C4D5E6F7A8B9C0D", owner_id: "u3", cn: "Charlie Le", email: "charlie@minibank.vn", fingerprint: "c5:d3:4e:1f:2a:3b:4c:5d:6e:7f:8a:9b:0c:1d:2e:3f", not_before: "2024-06-01", not_after: "2025-06-01", status: "revoked", issued_at: "2024-06-01T10:00:00Z", revoked_at: "2025-03-15T14:22:00Z", revocation_reason: "Key compromise reported by user" },
  { id: "c4", serial: "4D5E6F7A8B9C0D1E", owner_id: "u4", cn: "Diana Pham", email: "diana@minibank.vn", fingerprint: "d6:e4:5f:2a:3b:4c:5d:6e:7f:8a:9b:0c:1d:2e:3f:4a", not_before: "2024-03-01", not_after: "2025-03-01", status: "expired", issued_at: "2024-03-01T07:00:00Z" },
  { id: "c5", serial: "5E6F7A8B9C0D1E2F", owner_id: "u5", cn: "Eric Vo", email: "eric@minibank.vn", fingerprint: "e7:f5:6a:3b:4c:5d:6e:7f:8a:9b:0c:1d:2e:3f:4a:5b", not_before: "2025-03-20", not_after: "2026-03-20", status: "active", issued_at: "2025-03-20T11:00:00Z" },
  { id: "c6", serial: "6F7A8B9C0D1E2F3A", owner_id: "u6", cn: "Frank Hoang", email: "frank@minibank.vn", fingerprint: "f8:a6:7b:4c:5d:6e:7f:8a:9b:0c:1d:2e:3f:4a:5b:6c", not_before: "2025-01-05", not_after: "2026-01-05", status: "revoked", issued_at: "2025-01-05T09:30:00Z", revoked_at: "2025-04-10T16:45:00Z", revocation_reason: "Unilateral admin revocation — account suspended" },
  { id: "c7", serial: "7A8B9C0D1E2F3A4B", owner_id: "u7", cn: "Grace Dang", email: "grace@minibank.vn", fingerprint: "a9:b7:8c:5d:6e:7f:8a:9b:0c:1d:2e:3f:4a:5b:6c:7d", not_before: "2025-04-01", not_after: "2026-04-01", status: "active", issued_at: "2025-04-01T08:15:00Z" },
  { id: "c8", serial: "8B9C0D1E2F3A4B5C", owner_id: "u8", cn: "Hoa Nguyen", email: "hoa@minibank.vn", fingerprint: "ba:c8:9d:6e:7f:8a:9b:0c:1d:2e:3f:4a:5b:6c:7d:8e", not_before: "2024-01-10", not_after: "2025-01-10", status: "expired", issued_at: "2024-01-10T10:00:00Z" },
]

export const USERS: BankUser[] = [
  { id: "u1", email: "alice@minibank.vn", full_name: "Alice Nguyen", status: "active", accounts: 2, total_balance: 125_500_000, created_at: "2025-01-15" },
  { id: "u2", email: "bob@minibank.vn", full_name: "Bob Tran", status: "active", accounts: 1, total_balance: 48_200_000, created_at: "2025-02-10" },
  { id: "u3", email: "charlie@minibank.vn", full_name: "Charlie Le", status: "locked", accounts: 1, total_balance: 0, created_at: "2024-06-01" },
  { id: "u4", email: "diana@minibank.vn", full_name: "Diana Pham", status: "active", accounts: 2, total_balance: 310_000_000, created_at: "2024-03-01" },
  { id: "u5", email: "eric@minibank.vn", full_name: "Eric Vo", status: "active", accounts: 1, total_balance: 73_800_000, created_at: "2025-03-20" },
  { id: "u6", email: "frank@minibank.vn", full_name: "Frank Hoang", status: "locked", accounts: 1, total_balance: 0, created_at: "2025-01-05" },
  { id: "u7", email: "grace@minibank.vn", full_name: "Grace Dang", status: "active", accounts: 3, total_balance: 891_300_000, created_at: "2025-04-01" },
  { id: "u8", email: "hoa@minibank.vn", full_name: "Hoa Nguyen", status: "active", accounts: 1, total_balance: 22_100_000, created_at: "2024-01-10" },
]

export const TXS: Transaction[] = [
  { id: "t1", from_acct: "VN0001001", to_acct: "VN0002001", from_name: "Alice Nguyen", to_name: "Bob Tran", amount: 5_000_000, status: "completed", description: "Thanh toán tiền thuê nhà tháng 5", created_at: "2025-06-20T09:15:00Z", cert_serial: "1A2B3C4D5E6F7A8B", chain_hash: "a1b2c3d4e5f6" },
  { id: "t2", from_acct: "VN0005001", to_acct: "VN0001001", from_name: "Eric Vo", to_name: "Alice Nguyen", amount: 2_500_000, status: "completed", description: "Hoàn tiền ăn trưa", created_at: "2025-06-19T14:30:00Z", cert_serial: "5E6F7A8B9C0D1E2F", chain_hash: "b2c3d4e5f6a1" },
  { id: "t3", from_acct: "VN0001001", to_acct: "VN0007001", from_name: "Alice Nguyen", to_name: "Grace Dang", amount: 10_000_000, status: "completed", description: "Chia sẻ chi phí dự án", created_at: "2025-06-18T16:45:00Z", cert_serial: "1A2B3C4D5E6F7A8B", chain_hash: "c3d4e5f6a1b2" },
  { id: "t4", from_acct: "VN0004001", to_acct: "VN0001002", from_name: "Diana Pham", to_name: "Alice Nguyen", amount: 15_000_000, status: "completed", description: "Thanh toán hợp đồng tư vấn", created_at: "2025-06-17T10:00:00Z", cert_serial: "4D5E6F7A8B9C0D1E", chain_hash: "d4e5f6a1b2c3" },
  { id: "t5", from_acct: "VN0003001", to_acct: "VN0002001", from_name: "Charlie Le", to_name: "Bob Tran", amount: 1_000_000, status: "failed", description: "Từ chối — certificate đã bị thu hồi", created_at: "2025-06-16T08:22:00Z", cert_serial: "3C4D5E6F7A8B9C0D", chain_hash: "" },
  { id: "t6", from_acct: "VN0007001", to_acct: "VN0005001", from_name: "Grace Dang", to_name: "Eric Vo", amount: 8_200_000, status: "completed", description: "Phí dịch vụ thiết kế", created_at: "2025-06-15T13:10:00Z", cert_serial: "7A8B9C0D1E2F3A4B", chain_hash: "e5f6a1b2c3d4" },
  { id: "t7", from_acct: "VN0002001", to_acct: "VN0004002", from_name: "Bob Tran", to_name: "Diana Pham", amount: 3_500_000, status: "completed", description: "Hoàn ứng trước", created_at: "2025-06-14T11:55:00Z", cert_serial: "2B3C4D5E6F7A8B9C", chain_hash: "f6a1b2c3d4e5" },
  { id: "t8", from_acct: "VN0001002", to_acct: "VN0008001", from_name: "Alice Nguyen", to_name: "Hoa Nguyen", amount: 500_000, status: "completed", description: "Quà tặng sinh nhật", created_at: "2025-06-13T19:30:00Z", cert_serial: "1A2B3C4D5E6F7A8B", chain_hash: "a1b2c3d4f5e6" },
]

export const CA_AUDIT: CAuditEntry[] = [
  { id: "ca1", action: "issued", performed_by: "system:ca-service", serial: "7A8B9C0D1E2F3A4B", performed_at: "2025-04-01T08:15:00Z" },
  { id: "ca2", action: "revocation_checked", performed_by: "system:bank-service", serial: "1A2B3C4D5E6F7A8B", performed_at: "2025-06-20T09:14:58Z" },
  { id: "ca3", action: "revoked", performed_by: "admin:admin@minibank.vn", serial: "6F7A8B9C0D1E2F3A", reason: "Unilateral admin revocation — account suspended", performed_at: "2025-04-10T16:45:00Z" },
  { id: "ca4", action: "looked_up", performed_by: "admin:admin@minibank.vn", serial: "4D5E6F7A8B9C0D1E", performed_at: "2025-06-22T09:00:00Z" },
  { id: "ca5", action: "revocation_checked", performed_by: "system:kdc-service", serial: "5E6F7A8B9C0D1E2F", performed_at: "2025-06-19T14:30:00Z" },
  { id: "ca6", action: "issued", performed_by: "system:ca-service", serial: "5E6F7A8B9C0D1E2F", performed_at: "2025-03-20T11:00:00Z" },
  { id: "ca7", action: "revoked", performed_by: "admin:admin@minibank.vn", serial: "3C4D5E6F7A8B9C0D", reason: "Key compromise reported by user", performed_at: "2025-03-15T14:22:00Z" },
  { id: "ca8", action: "looked_up", performed_by: "admin:admin@minibank.vn", serial: "8B9C0D1E2F3A4B5C", performed_at: "2025-06-22T10:30:00Z" },
]

export const B_AUDIT: BAuditEntry[] = [
  { id: "ba1", action: "transfer_completed", user_email: "alice@minibank.vn", cert_serial: "1A2B3C4D5E6F7A8B", created_at: "2025-06-20T09:15:00Z" },
  { id: "ba2", action: "transfer_rejected", user_email: "charlie@minibank.vn", cert_serial: "3C4D5E6F7A8B9C0D", reason: "certificate_rejected — certificate is revoked", created_at: "2025-06-16T08:22:00Z" },
  { id: "ba3", action: "transfer_completed", user_email: "grace@minibank.vn", cert_serial: "7A8B9C0D1E2F3A4B", created_at: "2025-06-15T13:10:00Z" },
  { id: "ba4", action: "replay_detected", user_email: "frank@minibank.vn", cert_serial: "6F7A8B9C0D1E2F3A", reason: "Nonce already used within replay window", created_at: "2025-04-12T11:20:00Z" },
  { id: "ba5", action: "invalid_signature", user_email: "bob@minibank.vn", cert_serial: "2B3C4D5E6F7A8B9C", reason: "RSA-PSS signature verification failed", created_at: "2025-06-10T08:05:00Z" },
  { id: "ba6", action: "transfer_completed", user_email: "alice@minibank.vn", cert_serial: "1A2B3C4D5E6F7A8B", created_at: "2025-06-18T16:45:00Z" },
  { id: "ba7", action: "insufficient_funds", user_email: "eric@minibank.vn", cert_serial: "5E6F7A8B9C0D1E2F", reason: "Balance 800.000 ₫ < requested 1.000.000 ₫", created_at: "2025-06-08T15:33:00Z" },
  { id: "ba8", action: "transfer_completed", user_email: "diana@minibank.vn", cert_serial: "4D5E6F7A8B9C0D1E", created_at: "2025-06-17T10:00:00Z" },
]

export const CHART_DATA = [
  { day: "10/6", txns: 3, amount: 12 }, { day: "11/6", txns: 5, amount: 28 },
  { day: "12/6", txns: 2, amount: 8 }, { day: "13/6", txns: 7, amount: 35 },
  { day: "14/6", txns: 4, amount: 18 }, { day: "15/6", txns: 6, amount: 42 },
  { day: "16/6", txns: 1, amount: 3 }, { day: "17/6", txns: 8, amount: 51 },
  { day: "18/6", txns: 5, amount: 29 }, { day: "19/6", txns: 9, amount: 67 },
  { day: "20/6", txns: 11, amount: 89 }, { day: "21/6", txns: 4, amount: 22 },
  { day: "22/6", txns: 7, amount: 44 }, { day: "23/6", txns: 3, amount: 15 },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────
export const formatVND = (n: number) => new Intl.NumberFormat("vi-VN").format(n) + " ₫"

export const fmtDate = (iso: string) => {
  const [y, m, d] = iso.split("T")[0].split("-")
  return `${d}/${m}/${y}`
}

export const fmtDateTime = (iso: string) => {
  const dt = new Date(iso)
  const d = String(dt.getUTCDate()).padStart(2, "0")
  const mo = String(dt.getUTCMonth() + 1).padStart(2, "0")
  const h = String(dt.getUTCHours()).padStart(2, "0")
  const mi = String(dt.getUTCMinutes()).padStart(2, "0")
  return `${d}/${mo}/${dt.getUTCFullYear()} ${h}:${mi}`
}

export const trunc = (s: string, n: number) => s.length > n ? s.slice(0, n) + "…" : s
