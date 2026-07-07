// Security Operations (SOC) console. Owns the cross-cutting security views that
// do not belong to any single domain admin: KDC key-issuance audit and the
// cross-service timeline / integrity verify / summary / export. Guarded by the
// security-admin identity (JWT from /v1/admin-sec/auth).
import { FormEvent, useEffect, useMemo, useState } from "react"
import {
  Activity,
  ChevronLeft,
  ChevronRight,
  Download,
  KeyRound,
  Loader2,
  Lock,
  RefreshCw,
  Search,
  ShieldCheck,
  X,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"
import {
  clearSocSession,
  downloadAuditExport,
  getAuditSummary,
  getAuditTimeline,
  getStoredSocEmail,
  getStoredSocToken,
  listKdcAudit,
  loginSecAdmin,
  storeSocSession,
  verifyAuditChains,
} from "../services/admin/soc-admin.api"
import type {
  AuditSummary,
  AuditVerifyResult,
  KdcAuditAction,
} from "../services/admin/soc-admin.api"
import { AuditTimeline, toAuditVM } from "../components/AuditTimeline"
import type { AuditEventVM } from "../components/AuditTimeline"
import { ApiError } from "../services/api.service"

type View = "kdc" | "cross"
const PAGE_SIZE = 20

const NAV: { id: View; label: string; icon: LucideIcon }[] = [
  { id: "kdc", label: "Key Issuance", icon: KeyRound },
  { id: "cross", label: "Cross-service", icon: Activity },
]

const KDC_ACTIONS: ("all" | KdcAuditAction)[] = [
  "all",
  "as_ticket_issued",
  "as_rejected",
  "tgs_ticket_issued",
  "tgs_rejected",
]

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return err instanceof Error ? err.message : "Unexpected error"
}

function Header({ email, onLogout }: { email: string; onLogout: () => void }) {
  return (
    <header className="shrink-0 border-b border-border bg-card/80 backdrop-blur-sm z-10" style={{ height: "52px" }}>
      <div className="flex items-center justify-between px-5 h-full gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-7 h-7 bg-red-600 rounded-md flex items-center justify-center">
            <ShieldCheck className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="text-sm font-semibold text-foreground font-mono">Security Operations</span>
        </div>
        <div className="flex items-center gap-3 min-w-0">
          <span className="text-xs text-muted-foreground truncate max-w-48">{email || "security-admin"}</span>
          <button onClick={onLogout} className="h-8 px-3 rounded-md border border-border text-xs text-muted-foreground hover:text-foreground hover:bg-accent">
            Sign out
          </button>
        </div>
      </div>
    </header>
  )
}

function LoginPanel({ onLogin }: { onLogin: (email: string) => void }) {
  const [email, setEmail] = useState(getStoredSocEmail() || "soc.admin@demo.local")
  const [password, setPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError("")
    try {
      const session = await loginSecAdmin(email.trim().toLowerCase(), password)
      storeSocSession(session)
      onLogin(session.email)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <form onSubmit={submit} className="w-full max-w-sm bg-card border border-border rounded-lg p-6 space-y-4">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 bg-red-600 rounded-md flex items-center justify-center"><Lock className="w-4 h-4 text-white" /></div>
          <h1 className="text-base font-semibold text-foreground">Security Operations</h1>
        </div>
        <div className="space-y-2">
          <input value={email} onChange={e => setEmail(e.target.value)} placeholder="Email" className="w-full h-10 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-red-500" />
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="Password" className="w-full h-10 rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-red-500" />
        </div>
        {error && <p className="text-xs text-red-300">{error}</p>}
        <button disabled={submitting} className="w-full h-10 rounded-md bg-red-600 text-white text-sm font-medium hover:bg-red-500 disabled:opacity-50">
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  )
}

function SessionDrawer({ requestId, onClose }: { requestId: string; onClose: () => void }) {
  const [items, setItems] = useState<AuditEventVM[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    setLoading(true)
    setError("")
    getAuditTimeline(requestId)
      .then(res => { if (live) setItems(res.items.map((it, i) => toAuditVM(it, i))) })
      .catch(err => { if (live) setError(errorMessage(err)) })
      .finally(() => { if (live) setLoading(false) })
    return () => { live = false }
  }, [requestId])

  return (
    <div className="fixed inset-0 z-30 flex justify-end bg-black/50" onClick={onClose}>
      <div className="w-full max-w-2xl h-full bg-background border-l border-border overflow-auto" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 h-14 border-b border-border">
          <div>
            <h2 className="text-sm font-semibold text-foreground">Session timeline</h2>
            <p className="text-xs text-muted-foreground font-mono">{requestId}</p>
          </div>
          <button onClick={onClose} className="w-8 h-8 rounded-md border border-border flex items-center justify-center hover:bg-accent"><X className="w-4 h-4" /></button>
        </div>
        <div className="p-5">
          <AuditTimeline events={items} loading={loading} error={error} showSource emptyLabel="No events for this request id" />
        </div>
      </div>
    </div>
  )
}

function KdcPanel({ onAuthError, onViewSession }: { onAuthError: () => void; onViewSession: (id: string) => void }) {
  const [items, setItems] = useState<AuditEventVM[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [action, setAction] = useState<"all" | KdcAuditAction>("all")
  const [search, setSearch] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  const canPrev = offset > 0
  const canNext = offset + PAGE_SIZE < total
  const pageLabel = useMemo(() => (total === 0 ? "No events" : `${offset + 1}-${Math.min(offset + PAGE_SIZE, total)} of ${total}`), [offset, total])

  async function load(nextOffset: number) {
    setLoading(true)
    setError("")
    try {
      const resp = await listKdcAudit({ action, certSerial: search, limit: PAGE_SIZE, offset: nextOffset })
      setItems(resp.items.map((e, i) => toAuditVM({ ...e, source: "kdc" }, i)))
      setTotal(resp.total)
      setOffset(resp.offset)
    } catch (err) {
      setError(errorMessage(err))
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) onAuthError()
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    const t = window.setTimeout(() => load(0), 250)
    return () => window.clearTimeout(t)
  }, [action, search])

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-foreground">Key Issuance (KDC)</h1>
          <p className="text-xs text-muted-foreground mt-1">{pageLabel}</p>
        </div>
        <div className="flex items-center gap-2">
          <button disabled={!canPrev} onClick={() => load(Math.max(0, offset - PAGE_SIZE))} className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent disabled:opacity-40"><ChevronLeft className="w-4 h-4" /></button>
          <button disabled={!canNext} onClick={() => load(offset + PAGE_SIZE)} className="w-9 h-9 rounded-md border border-border flex items-center justify-center hover:bg-accent disabled:opacity-40"><ChevronRight className="w-4 h-4" /></button>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-sm">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Filter by cert serial" className="w-full h-9 rounded-md border border-border bg-card pl-9 pr-3 text-sm outline-none focus:border-red-500" />
        </div>
        <select value={action} onChange={e => setAction(e.target.value as "all" | KdcAuditAction)} className="h-9 rounded-md border border-border bg-card px-3 text-xs outline-none focus:border-red-500">
          {KDC_ACTIONS.map(o => <option key={o} value={o}>{o === "all" ? "All actions" : o}</option>)}
        </select>
      </div>
      <AuditTimeline events={items} loading={loading} error={error} onRetry={() => load(offset)} onRefresh={() => load(offset)} onViewSession={onViewSession} emptyLabel="No key-issuance events found" />
    </div>
  )
}

function CrossServicePanel({ onAuthError, onViewSession }: { onAuthError: () => void; onViewSession: (id: string) => void }) {
  const [summary, setSummary] = useState<AuditSummary | null>(null)
  const [verify, setVerify] = useState<AuditVerifyResult | null>(null)
  const [verifying, setVerifying] = useState(false)
  const [lookup, setLookup] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    getAuditSummary("24h").then(setSummary).catch(err => {
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) onAuthError()
    })
  }, [])

  async function runVerify() {
    setVerifying(true)
    setError("")
    try {
      setVerify(await verifyAuditChains())
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setVerifying(false)
    }
  }

  const chainOk = verify && Object.values(verify.sources).every(s => !s.checked || s.ok !== false)
  const cards = summary ? [
    { label: "Events (24h)", value: summary.total, tone: "text-foreground" },
    { label: "Security events", value: summary.security_events, tone: "text-amber-300" },
    { label: "Critical", value: summary.by_severity.critical, tone: "text-red-400" },
    { label: "Denied", value: summary.by_outcome.denied, tone: "text-amber-300" },
    { label: "Anomalies", value: summary.anomalies.length, tone: summary.anomalies.length ? "text-red-400" : "text-muted-foreground" },
  ] : []

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Cross-service security</h1>
        <div className="flex items-center gap-2">
          <button onClick={runVerify} disabled={verifying} className="h-9 px-3 rounded-md border border-border text-xs flex items-center gap-1.5 hover:bg-accent disabled:opacity-50">
            <ShieldCheck className={`w-4 h-4 ${verifying ? "animate-pulse" : ""}`} /> Verify integrity
          </button>
          <button onClick={() => downloadAuditExport({ format: "csv" }).catch(e => setError(errorMessage(e)))} className="h-9 px-3 rounded-md border border-border text-xs flex items-center gap-1.5 hover:bg-accent">
            <Download className="w-4 h-4" /> CSV
          </button>
          <button onClick={() => downloadAuditExport({ format: "json" }).catch(e => setError(errorMessage(e)))} className="h-9 px-3 rounded-md border border-border text-xs flex items-center gap-1.5 hover:bg-accent">
            <Download className="w-4 h-4" /> JSON
          </button>
        </div>
      </div>

      {cards.length > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
          {cards.map(c => (
            <div key={c.label} className="rounded-lg border border-border bg-card px-4 py-3">
              <p className="text-xs text-muted-foreground">{c.label}</p>
              <p className={`text-xl font-semibold mt-1 ${c.tone}`}>{c.value}</p>
            </div>
          ))}
        </div>
      )}

      {error && <p className="text-xs text-red-300">{error}</p>}

      {verify && (
        <div className={`rounded-md border px-3 py-2 text-xs ${chainOk ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300" : "border-red-500/30 bg-red-500/10 text-red-300"}`}>
          {chainOk ? "Audit hash chain verified — no tampering detected." : "Audit chain integrity check FAILED."}{" "}
          {Object.entries(verify.sources).map(([src, s]) => (
            <span key={src} className="ml-2">{src.toUpperCase()}: {s.checked ? (s.ok ? `ok (${s.verified})` : `broken @${s.broken_seq}`) : (s.detail ?? "skipped")}</span>
          ))}
        </div>
      )}

      {summary && summary.anomalies.length > 0 && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/5 p-4">
          <p className="text-sm font-medium text-red-300 mb-2">Anomalies (≥5 denied / window)</p>
          <ul className="text-xs text-muted-foreground space-y-1">
            {summary.anomalies.map(a => <li key={a.actor}><span className="font-mono text-foreground">{a.actor}</span> — {a.denied_count} denied</li>)}
          </ul>
        </div>
      )}

      <div className="rounded-lg border border-border bg-card p-4">
        <p className="text-sm font-medium text-foreground mb-2">Trace a session by request id</p>
        <div className="flex gap-2">
          <input value={lookup} onChange={e => setLookup(e.target.value)} placeholder="request_id (UUID)" className="flex-1 h-9 rounded-md border border-border bg-background px-3 text-sm font-mono outline-none focus:border-red-500" />
          <button disabled={!lookup.trim()} onClick={() => onViewSession(lookup.trim())} className="h-9 px-4 rounded-md bg-red-600 text-white text-xs hover:bg-red-500 disabled:opacity-40">Open timeline</button>
        </div>
      </div>
    </div>
  )
}

export default function AdminSOC() {
  const [token, setToken] = useState(getStoredSocToken())
  const [email, setEmail] = useState(getStoredSocEmail())
  const [view, setView] = useState<View>("kdc")
  const [session, setSession] = useState<string | null>(null)

  function logout() {
    clearSocSession()
    setToken("")
    setEmail("")
  }

  if (!token) {
    return <LoginPanel onLogin={next => { setEmail(next); setToken(getStoredSocToken()) }} />
  }

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header email={email} onLogout={logout} />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-44 shrink-0 border-r border-border bg-card/30 flex flex-col py-3 px-2 gap-1">
          {NAV.map(n => {
            const Icon = n.icon
            return (
              <button key={n.id} onClick={() => setView(n.id)} className={`flex items-center gap-2.5 px-3 py-2 rounded-md text-left transition-colors ${view === n.id ? "bg-red-600/15 text-red-300 border border-red-500/20" : "text-muted-foreground hover:text-foreground hover:bg-accent/50"}`}>
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{n.label}</span>
              </button>
            )
          })}
          <div className="mt-auto px-3 pt-4 border-t border-border"><p className="text-xs text-muted-foreground/60 font-mono">security-admin</p></div>
        </aside>
        <main className="flex-1 overflow-auto p-5">
          {view === "kdc"
            ? <KdcPanel onAuthError={logout} onViewSession={setSession} />
            : <CrossServicePanel onAuthError={logout} onViewSession={setSession} />}
        </main>
      </div>
      {session && <SessionDrawer requestId={session} onClose={() => setSession(null)} />}
    </div>
  )
}
