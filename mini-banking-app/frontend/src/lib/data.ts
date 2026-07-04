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
