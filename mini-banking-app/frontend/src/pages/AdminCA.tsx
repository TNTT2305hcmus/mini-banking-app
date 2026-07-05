import { FormEvent, useEffect, useMemo, useState } from "react"
import {
  Activity,
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Clipboard,
  Database,
  Loader2,
  Lock,
  RefreshCw,
  Search,
  ShieldCheck,
  X,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import {
  AdminCertificate,
  CertificateStatus,
  clearAdminSession,
  getAdminCertificateDetail,
  getStoredAdminEmail,
  getStoredAdminToken,
  listAdminCertificates,
  loginAdminCA,
  revokeAdminCertificate,
  storeAdminSession,
} from "../services/admin/ca-admin.api"
import { ApiError } from "../services/api.service"

type View = "certificates" | "audit"
type StatusFilter = "all" | "active" | "revoked" | "expired"

const NAV: { id: View; label: string; icon: LucideIcon }[] = [
  { id: "certificates", label: "Certificates", icon: ShieldCheck },
  { id: "audit", label: "Audit Log", icon: Activity },
]

const STATUS_OPTIONS: StatusFilter[] = ["all", "active", "revoked", "expired"]
const PAGE_SIZE = 20

function Header({ email, onLogout }: { email: string; onLogout: () => void }) {
  return (
    <header className="shrink-0 border-b border-border bg-card/80 backdrop-blur-sm z-10" style={{ height: "52px" }}>
      <div className="flex items-center justify-between px-5 h-full gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-7 h-7 bg-cyan-600 rounded-md flex items-center justify-center">
            <Lock className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="text-sm font-semibold text-foreground font-mono">Mini Banking</span>
        </div>
        <div className="flex items-center gap-3 min-w-0">
          <span className="text-xs text-muted-foreground truncate max-w-48">{email || "admin-ca"}</span>
          <button onClick={onLogout} className="h-8 px-3 rounded-md border border-border text-xs text-muted-foreground hover:text-foreground hover:bg-accent">
            Sign out
          </button>
        </div>
      </div>
    </header>
  )
}

function statusClass(status: CertificateStatus) {
  switch (status) {
    case "active":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
    case "revoked":
      return "border-red-500/30 bg-red-500/10 text-red-400"
    case "expired":
      return "border-amber-500/30 bg-amber-500/10 text-amber-300"
    default:
      return "border-border bg-muted text-muted-foreground"
  }
}

function formatUnix(value?: number | null) {
  if (!value) return "-"
  return new Intl.DateTimeFormat("en-GB", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value * 1000))
}

function shortValue(value: string, keep = 10) {
  if (!value) return "-"
  if (value.length <= keep * 2 + 3) return value
  return `${value.slice(0, keep)}...${value.slice(-keep)}`
}

function errorMessage(err: unknown) {
  if (err instanceof ApiError) {
    return `${err.code}: ${err.message}`
  }
  if (err instanceof Error) {
    return err.message
  }
  return "Request failed"
}

function LoginPanel({ onLogin }: { onLogin: (email: string) => void }) {
  const [email, setEmail] = useState(getStoredAdminEmail() || "ca.admin@demo.local")
  const [password, setPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError("")
    try {
      const session = await loginAdminCA(email, password)
      storeAdminSession(session)
      onLogin(session.email)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="h-screen bg-background flex items-center justify-center p-5">
      <form onSubmit={submit} className="w-full max-w-sm border border-border bg-card rounded-lg p-5 space-y-4">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-md bg-cyan-600 flex items-center justify-center">
            <ShieldCheck className="w-5 h-5 text-white" />
          </div>
          <div>
            <h1 className="text-base font-semibold text-foreground">Admin CA</h1>
            <p className="text-xs text-muted-foreground">Certificate dashboard</p>
          </div>
        </div>
        <div className="space-y-2">
          <label className="text-xs text-muted-foreground" htmlFor="email">Email</label>
          <input id="email" value={email} onChange={e => setEmail(e.target.value)} className="w-full h-9 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-cyan-500" />
        </div>
        <div className="space-y-2">
          <label className="text-xs text-muted-foreground" htmlFor="password">Password</label>
          <input id="password" type="password" value={password} onChange={e => setPassword(e.target.value)} className="w-full h-9 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-cyan-500" />
        </div>
        {error && <p className="text-xs text-red-400">{error}</p>}
        <button disabled={submitting} className="w-full h-9 rounded-md bg-cyan-600 text-white text-sm font-medium hover:bg-cyan-500 disabled:opacity-60">
          {submitting ? "Signing in..." : "Sign in"}
        </button>
      </form>
    </div>
  )
}

function DetailPanel({
  certificate,
  loading,
  onClose,
  onCopy,
  onRevoke,
}: {
  certificate: AdminCertificate | null
  loading: boolean
  onClose: () => void
  onCopy: (value: string, label: string) => void
  onRevoke: (cert: AdminCertificate) => void
}) {
  if (!certificate && !loading) return null

  return (
    <div className="fixed inset-y-0 right-0 w-full max-w-md border-l border-border bg-card shadow-xl z-30 flex flex-col">
      <div className="h-13 px-4 py-3 border-b border-border flex items-center justify-between">
        <h2 className="text-sm font-semibold text-foreground">Certificate Detail</h2>
        <button onClick={onClose} className="w-8 h-8 rounded-md flex items-center justify-center hover:bg-accent" aria-label="Close">
          <X className="w-4 h-4" />
        </button>
      </div>
      {loading || !certificate ? (
        <div className="flex-1 flex items-center justify-center text-sm text-muted-foreground">
          <Loader2 className="w-4 h-4 mr-2 animate-spin" /> Loading
        </div>
      ) : (
        <div className="flex-1 overflow-auto p-4 space-y-4">
          <div className="flex items-center justify-between gap-3">
            <span className={`px-2 py-1 rounded-md border text-xs ${statusClass(certificate.status)}`}>{certificate.status}</span>
            <button
              disabled={certificate.status !== "active"}
              onClick={() => onRevoke(certificate)}
              className="h-8 px-3 rounded-md bg-red-600 text-white text-xs font-medium hover:bg-red-500 disabled:opacity-40 disabled:hover:bg-red-600"
            >
              Revoke
            </button>
          </div>
          {[
            ["Serial", certificate.serial],
            ["Fingerprint", certificate.fingerprint],
            ["Common name", certificate.cn],
            ["Email", certificate.email],
            ["Owner ID", certificate.owner_id],
            ["Valid from", formatUnix(certificate.not_before)],
            ["Valid until", formatUnix(certificate.not_after)],
            ["Issued at", formatUnix(certificate.issued_at)],
            ["Revoked at", formatUnix(certificate.revoked_at)],
            ["Revocation reason", certificate.revocation_reason || "-"],
          ].map(([label, value]) => (
            <div key={label} className="space-y-1">
              <p className="text-xs text-muted-foreground">{label}</p>
              <div className="flex items-start gap-2">
                <p className="text-sm text-foreground break-all flex-1">{value}</p>
                {(label === "Serial" || label === "Fingerprint") && value !== "-" && (
                  <button onClick={() => onCopy(value, label)} className="w-8 h-8 rounded-md flex items-center justify-center hover:bg-accent" aria-label={`Copy ${label}`}>
                    <Clipboard className="w-4 h-4" />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function RevokeDialog({
  certificate,
  submitting,
  error,
  onCancel,
  onConfirm,
}: {
  certificate: AdminCertificate | null
  submitting: boolean
  error: string
  onCancel: () => void
  onConfirm: (reason: string) => void
}) {
  const [reason, setReason] = useState("")

  useEffect(() => {
    setReason("")
  }, [certificate?.serial])

  if (!certificate) return null

  return (
    <div className="fixed inset-0 z-40 bg-black/55 flex items-center justify-center p-4">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-5 space-y-4">
        <div className="flex items-start gap-3">
          <div className="w-9 h-9 rounded-md bg-red-500/10 flex items-center justify-center">
            <AlertTriangle className="w-5 h-5 text-red-400" />
          </div>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-foreground">Revoke certificate</h2>
            <p className="text-xs text-muted-foreground break-all mt-1">{certificate.serial}</p>
          </div>
        </div>
        <div className="space-y-2">
          <label className="text-xs text-muted-foreground" htmlFor="reason">Reason</label>
          <textarea id="reason" value={reason} onChange={e => setReason(e.target.value)} rows={3} className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-red-500 resize-none" />
        </div>
        <p className="text-xs text-amber-300">This action cannot be undone.</p>
        {error && <p className="text-xs text-red-400">{error}</p>}
        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="h-9 px-3 rounded-md border border-border text-sm hover:bg-accent">Cancel</button>
          <button
            disabled={submitting || !reason.trim()}
            onClick={() => onConfirm(reason)}
            className="h-9 px-3 rounded-md bg-red-600 text-white text-sm font-medium hover:bg-red-500 disabled:opacity-50"
          >
            {submitting ? "Revoking..." : "Confirm"}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function AdminCA() {
  const [view, setView] = useState<View>("certificates")
  const [token, setToken] = useState(getStoredAdminToken())
  const [email, setEmail] = useState(getStoredAdminEmail())
  const [items, setItems] = useState<AdminCertificate[]>([])
  const [total, setTotal] = useState(0)
  const [status, setStatus] = useState<StatusFilter>("all")
  const [search, setSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const [message, setMessage] = useState("")
  const [selected, setSelected] = useState<AdminCertificate | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<AdminCertificate | null>(null)
  const [revokeError, setRevokeError] = useState("")
  const [revokeSubmitting, setRevokeSubmitting] = useState(false)

  const canPrev = offset > 0
  const canNext = offset + PAGE_SIZE < total
  const pageLabel = useMemo(() => {
    if (total === 0) return "0 of 0"
    return `${offset + 1}-${Math.min(offset + PAGE_SIZE, total)} of ${total}`
  }, [offset, total])

  async function loadCertificates(nextOffset = offset) {
    if (!getStoredAdminToken()) return
    setLoading(true)
    setError("")
    try {
      const resp = await listAdminCertificates({
        status,
        search,
        limit: PAGE_SIZE,
        offset: nextOffset,
      })
      setItems(resp.items)
      setTotal(resp.total)
      setOffset(resp.offset)
    } catch (err) {
      const msg = errorMessage(err)
      setError(msg)
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
        clearAdminSession()
        setToken("")
      }
    } finally {
      setLoading(false)
    }
  }

  async function openDetail(cert: AdminCertificate) {
    setSelected(cert)
    setDetailLoading(true)
    setError("")
    try {
      setSelected(await getAdminCertificateDetail(cert.serial))
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setDetailLoading(false)
    }
  }

  async function confirmRevoke(reason: string) {
    if (!revokeTarget) return
    setRevokeSubmitting(true)
    setRevokeError("")
    try {
      const revoked = await revokeAdminCertificate(revokeTarget.serial, reason)
      setMessage("Certificate revoked")
      setRevokeTarget(null)
      setSelected(revoked)
      await loadCertificates(offset)
    } catch (err) {
      setRevokeError(errorMessage(err))
    } finally {
      setRevokeSubmitting(false)
    }
  }

  async function copy(value: string, label: string) {
    await navigator.clipboard.writeText(value)
    setMessage(`${label} copied`)
  }

  useEffect(() => {
    if (token) {
      loadCertificates(0)
    }
  }, [token])

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      if (token) {
        loadCertificates(0)
      }
    }, 250)
    return () => window.clearTimeout(timeout)
  }, [status, search])

  useEffect(() => {
    if (!message) return
    const timeout = window.setTimeout(() => setMessage(""), 2500)
    return () => window.clearTimeout(timeout)
  }, [message])

  if (!token) {
    return (
      <LoginPanel
        onLogin={nextEmail => {
          setEmail(nextEmail)
          setToken(getStoredAdminToken())
        }}
      />
    )
  }

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header
        email={email}
        onLogout={() => {
          clearAdminSession()
          setToken("")
          setEmail("")
        }}
      />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-44 shrink-0 border-r border-border bg-card/30 flex flex-col py-3 px-2 gap-1">
          {NAV.map(n => {
            const Icon = n.icon
            return (
              <button key={n.id} onClick={() => setView(n.id)} className={`flex items-center gap-2.5 px-3 py-2 rounded-md text-left transition-colors ${view === n.id ? "bg-cyan-600/15 text-cyan-300 border border-cyan-500/20" : "text-muted-foreground hover:text-foreground hover:bg-accent/50"}`}>
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{n.label}</span>
              </button>
            )
          })}
          <div className="mt-auto px-3 pt-4 border-t border-border">
            <p className="text-xs text-muted-foreground/60 font-mono">admin-ca</p>
          </div>
        </aside>

        {view === "audit" ? (
          <main className="flex-1 p-5 flex items-center justify-center">
            <div className="w-full max-w-lg bg-card border border-border rounded-lg p-8 text-center">
              <div className="w-12 h-12 bg-amber-500/10 rounded-md flex items-center justify-center mx-auto mb-4">
                <Database className="w-6 h-6 text-amber-300" />
              </div>
              <h1 className="text-base font-semibold text-foreground">Audit endpoint pending</h1>
              <p className="text-sm text-muted-foreground mt-2">CA audit will appear here when the audit read endpoint is available.</p>
            </div>
          </main>
        ) : (
          <main className="flex-1 overflow-hidden flex flex-col">
            <div className="p-5 border-b border-border bg-background">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h1 className="text-lg font-semibold text-foreground">Certificates</h1>
                  <p className="text-xs text-muted-foreground mt-1">{pageLabel}</p>
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => loadCertificates(offset)} className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent" aria-label="Refresh">
                    <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
                  </button>
                  <button disabled={!canPrev} onClick={() => loadCertificates(Math.max(0, offset - PAGE_SIZE))} className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent disabled:opacity-40" aria-label="Previous page">
                    <ChevronLeft className="w-4 h-4" />
                  </button>
                  <button disabled={!canNext} onClick={() => loadCertificates(offset + PAGE_SIZE)} className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent disabled:opacity-40" aria-label="Next page">
                    <ChevronRight className="w-4 h-4" />
                  </button>
                </div>
              </div>
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <div className="relative w-full max-w-sm">
                  <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                  <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Email or serial" className="w-full h-9 rounded-md border border-border bg-card pl-9 pr-3 text-sm outline-none focus:border-cyan-500" />
                </div>
                <div className="flex rounded-md border border-border overflow-hidden">
                  {STATUS_OPTIONS.map(option => (
                    <button key={option} onClick={() => setStatus(option)} className={`h-9 px-3 text-xs capitalize border-r border-border last:border-r-0 ${status === option ? "bg-cyan-600 text-white" : "bg-card text-muted-foreground hover:text-foreground"}`}>
                      {option}
                    </button>
                  ))}
                </div>
              </div>
              {message && <p className="text-xs text-emerald-400 mt-3">{message}</p>}
              {error && (
                <div className="mt-3 flex items-center justify-between gap-3 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2">
                  <p className="text-xs text-red-300">{error}</p>
                  <button onClick={() => loadCertificates(offset)} className="text-xs text-red-100 underline">Retry</button>
                </div>
              )}
            </div>

            <div className="flex-1 overflow-auto p-5">
              <div className="overflow-hidden rounded-lg border border-border bg-card">
                <table className="w-full text-sm">
                  <thead className="bg-muted/40 text-xs text-muted-foreground">
                    <tr>
                      <th className="text-left font-medium px-4 py-3">Serial</th>
                      <th className="text-left font-medium px-4 py-3">Email</th>
                      <th className="text-left font-medium px-4 py-3">Owner ID</th>
                      <th className="text-left font-medium px-4 py-3">Status</th>
                      <th className="text-left font-medium px-4 py-3">Issued</th>
                      <th className="text-left font-medium px-4 py-3">Expires</th>
                      <th className="text-right font-medium px-4 py-3">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {loading ? (
                      <tr>
                        <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">
                          <Loader2 className="w-5 h-5 animate-spin mx-auto mb-2" /> Loading certificates
                        </td>
                      </tr>
                    ) : items.length === 0 ? (
                      <tr>
                        <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">No certificates found</td>
                      </tr>
                    ) : (
                      items.map(cert => (
                        <tr key={cert.serial} className="border-t border-border hover:bg-accent/30">
                          <td className="px-4 py-3 font-mono text-xs">{shortValue(cert.serial, 8)}</td>
                          <td className="px-4 py-3">{cert.email}</td>
                          <td className="px-4 py-3 font-mono text-xs">{shortValue(cert.owner_id, 8)}</td>
                          <td className="px-4 py-3">
                            <span className={`px-2 py-1 rounded-md border text-xs ${statusClass(cert.status)}`}>{cert.status}</span>
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">{formatUnix(cert.issued_at)}</td>
                          <td className="px-4 py-3 text-muted-foreground">{formatUnix(cert.not_after)}</td>
                          <td className="px-4 py-3">
                            <div className="flex justify-end gap-2">
                              <button onClick={() => openDetail(cert)} className="h-8 px-3 rounded-md border border-border text-xs hover:bg-accent">Detail</button>
                              <button disabled={cert.status !== "active"} onClick={() => setRevokeTarget(cert)} className="h-8 px-3 rounded-md bg-red-600 text-white text-xs hover:bg-red-500 disabled:opacity-40 disabled:hover:bg-red-600">Revoke</button>
                            </div>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </main>
        )}
      </div>

      <DetailPanel certificate={selected} loading={detailLoading} onClose={() => setSelected(null)} onCopy={copy} onRevoke={setRevokeTarget} />
      <RevokeDialog certificate={revokeTarget} submitting={revokeSubmitting} error={revokeError} onCancel={() => setRevokeTarget(null)} onConfirm={confirmRevoke} />
    </div>
  )
}
