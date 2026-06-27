import { useState } from "react"
import {
  Shield, ShieldCheck,
  Activity, Search, Eye, Ban, X, Lock,
} from "lucide-react"
import { CertBadge, ActionBadge } from "../lib/ui"
import { CERTS, CA_AUDIT, fmtDate, fmtDateTime, trunc, type Cert, type CertStatus } from "../lib/data"

function Header() {
  return (
    <header className="shrink-0 border-b border-border bg-card/60 backdrop-blur-sm z-10" style={{ height: "52px" }}>
      <div className="flex items-center px-5 h-full gap-3">
        <div className="w-7 h-7 bg-blue-600 rounded-lg flex items-center justify-center">
          <Lock className="w-3.5 h-3.5 text-white" />
        </div>
        <span className="text-sm font-semibold text-foreground font-mono">Mini Banking</span>
      </div>
    </header>
  )
}

// ─── Certificates view ────────────────────────────────────────────────────────
function Certificates({ onRevoke }: { onRevoke: (cert: Cert) => void }) {
  const [search, setSearch] = useState("")
  const [filter, setFilter] = useState<CertStatus | "all">("all")
  const [selected, setSelected] = useState<Cert | null>(null)

  const filtered = CERTS.filter(c => {
    const ok = filter === "all" || c.status === filter
    const q = search.toLowerCase()
    return ok && (!q || c.cn.toLowerCase().includes(q) || c.email.toLowerCase().includes(q) || c.serial.toLowerCase().includes(q))
  })

  return (
    <div className="flex gap-4">
      <div className="flex-1 min-w-0 space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative flex-1 min-w-48">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Tìm tên, email, serial..." className="w-full bg-card border border-border rounded-lg pl-9 pr-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/30" />
          </div>
          <div className="flex gap-1.5 shrink-0">
            {(["all","active","revoked","expired"] as const).map(f => (
              <button key={f} onClick={() => setFilter(f)} className={`px-2.5 py-1.5 rounded-lg text-xs font-mono transition-colors ${filter === f ? "bg-purple-600 text-white" : "bg-card border border-border text-muted-foreground hover:text-foreground"}`}>
                {f} ({f === "all" ? CERTS.length : CERTS.filter(c => c.status === f).length})
              </button>
            ))}
          </div>
        </div>
        <div className="bg-card border border-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                {["Subject","Serial","Status","Issued","Expires","Actions"].map(h => (
                  <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filtered.map(cert => (
                <tr key={cert.id} onClick={() => setSelected(cert)} className={`cursor-pointer transition-colors ${selected?.id === cert.id ? "bg-purple-500/5" : "hover:bg-accent/30"}`}>
                  <td className="px-4 py-3">
                    <p className="text-sm text-foreground">{cert.cn}</p>
                    <p className="text-xs text-muted-foreground">{cert.email}</p>
                  </td>
                  <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{cert.serial.slice(0, 12)}…</td>
                  <td className="px-4 py-3"><CertBadge status={cert.status} /></td>
                  <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{fmtDate(cert.issued_at)}</td>
                  <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{fmtDate(cert.not_after)}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-1">
                      <button onClick={e => { e.stopPropagation(); setSelected(cert) }} className="p-1.5 rounded hover:bg-accent transition-colors" title="View">
                        <Eye className="w-3.5 h-3.5 text-muted-foreground hover:text-foreground" />
                      </button>
                      {cert.status === "active" && (
                        <button onClick={e => { e.stopPropagation(); onRevoke(cert) }} className="p-1.5 rounded hover:bg-red-500/10 transition-colors" title="Revoke">
                          <Ban className="w-3.5 h-3.5 text-muted-foreground hover:text-red-400" />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {selected && (
        <div className="w-72 shrink-0">
          <div className="bg-card border border-border rounded-xl p-4 space-y-3 sticky top-0">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-semibold text-foreground">Certificate Detail</h4>
              <button onClick={() => setSelected(null)} className="p-1 hover:bg-accent rounded transition-colors">
                <X className="w-3.5 h-3.5 text-muted-foreground" />
              </button>
            </div>
            <div className="flex items-center gap-2.5">
              <div className="p-2 bg-cyan-500/10 rounded-lg shrink-0"><Shield className="w-4 h-4 text-cyan-400" /></div>
              <div>
                <p className="text-sm font-medium text-foreground">{selected.cn}</p>
                <CertBadge status={selected.status} />
              </div>
            </div>
            <div className="space-y-2">
              {[
                { label: "Serial", value: selected.serial },
                { label: "Email", value: selected.email },
                { label: "Owner ID", value: selected.owner_id },
                { label: "Not Before", value: fmtDate(selected.not_before) },
                { label: "Not After", value: fmtDate(selected.not_after) },
                { label: "Issued At", value: fmtDateTime(selected.issued_at) },
              ].map(f => (
                <div key={f.label} className="bg-background border border-border rounded-lg p-2.5">
                  <p className="text-xs text-muted-foreground mb-0.5">{f.label}</p>
                  <p className="text-xs font-mono text-foreground break-all">{f.value}</p>
                </div>
              ))}
            </div>
            <div className="bg-background border border-border rounded-lg p-2.5">
              <p className="text-xs text-muted-foreground mb-0.5">SHA-256 Fingerprint</p>
              <p className="text-xs font-mono text-foreground break-all leading-relaxed">{selected.fingerprint}</p>
            </div>
            {selected.status === "revoked" && (
              <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
                <p className="text-xs text-red-400 font-medium mb-1">Revoked at {fmtDateTime(selected.revoked_at!)}</p>
                <p className="text-xs text-muted-foreground">{selected.revocation_reason}</p>
              </div>
            )}
            {selected.status === "active" && (
              <button onClick={() => onRevoke(selected)} className="w-full bg-red-500/10 hover:bg-red-500/20 border border-red-500/20 text-red-400 rounded-lg py-2 text-xs font-medium transition-colors flex items-center justify-center gap-1.5">
                <Ban className="w-3.5 h-3.5" /> Revoke Certificate
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Audit log view ───────────────────────────────────────────────────────────
function AuditLog() {
  return (
    <div className="bg-card border border-border rounded-xl overflow-hidden">
      <div className="px-5 py-4 border-b border-border flex items-center justify-between">
        <h3 className="text-sm font-semibold text-foreground">CA Audit Log</h3>
        <span className="text-xs text-muted-foreground font-mono">{CA_AUDIT.length} entries</span>
      </div>
      <table className="w-full">
        <thead>
          <tr className="border-b border-border">
            {["Action","Serial","Performed By","Reason","Timestamp"].map(h => (
              <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {CA_AUDIT.map(e => (
            <tr key={e.id} className="hover:bg-accent/30 transition-colors">
              <td className="px-4 py-3"><ActionBadge action={e.action} /></td>
              <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{e.serial.slice(0, 12)}…</td>
              <td className="px-4 py-3 text-xs font-mono text-foreground">{e.performed_by}</td>
              <td className="px-4 py-3 text-xs text-muted-foreground">{e.reason ? trunc(e.reason, 44) : <span className="text-muted-foreground/30">—</span>}</td>
              <td className="px-4 py-3 text-xs font-mono text-muted-foreground whitespace-nowrap">{fmtDateTime(e.performed_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Revoke modal ─────────────────────────────────────────────────────────────
function RevokeModal({ cert, onClose }: { cert: Cert; onClose: () => void }) {
  const [reason, setReason] = useState("")
  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
      <div className="bg-card border border-border rounded-xl w-full max-w-md p-6 shadow-2xl">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-foreground flex items-center gap-2">
            <Ban className="w-4 h-4 text-red-400" /> Revoke Certificate
          </h3>
          <button onClick={onClose} className="p-1 hover:bg-accent rounded transition-colors">
            <X className="w-4 h-4 text-muted-foreground" />
          </button>
        </div>
        <div className="bg-background border border-border rounded-lg p-3.5 mb-4">
          <p className="text-xs text-muted-foreground mb-0.5">Subject</p>
          <p className="text-sm font-medium text-foreground">{cert.cn}</p>
          <p className="text-xs text-muted-foreground">{cert.email}</p>
          <p className="text-xs font-mono text-muted-foreground mt-1.5">Serial: {cert.serial}</p>
        </div>
        <div className="mb-4">
          <label className="block text-xs text-muted-foreground mb-1.5">Lý do thu hồi <span className="text-red-400">*</span></label>
          <textarea value={reason} onChange={e => setReason(e.target.value)} placeholder="Key compromise, administrative action, account suspended..." rows={3} className="w-full bg-background border border-border rounded-lg px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground/30 resize-none" />
        </div>
        <div className="bg-amber-500/5 border border-amber-500/15 rounded-lg p-3 mb-5">
          <p className="text-xs text-amber-400/70">Thao tác này không thể hoàn tác. Certificate sẽ bị thu hồi ngay lập tức và revocation cache sẽ được cập nhật.</p>
        </div>
        <div className="flex gap-2">
          <button onClick={onClose} className="flex-1 bg-background border border-border text-muted-foreground hover:text-foreground rounded-lg py-2.5 text-sm transition-colors">Hủy</button>
          <button onClick={onClose} disabled={!reason.trim()} className="flex-1 bg-red-600 hover:bg-red-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-lg py-2.5 text-sm font-medium transition-colors">Xác nhận Thu hồi</button>
        </div>
      </div>
    </div>
  )
}

// ─── AdminCA page ─────────────────────────────────────────────────────────────
type View = "certificates" | "audit"
const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "certificates", label: "Certificates", icon: ShieldCheck },
  { id: "audit", label: "Audit Log", icon: Activity },
]

export default function AdminCA() {
  const [view, setView] = useState<View>("certificates")
  const [revokeTarget, setRevokeTarget] = useState<Cert | null>(null)

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-44 shrink-0 border-r border-border bg-card/20 flex flex-col py-3 px-2 gap-1">
          {NAV.map(n => {
            const Icon = n.icon
            return (
              <button key={n.id} onClick={() => setView(n.id)} className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-left transition-colors ${view === n.id ? "bg-purple-600/15 text-purple-400 border border-purple-500/20" : "text-muted-foreground hover:text-foreground hover:bg-accent/50"}`}>
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{n.label}</span>
              </button>
            )
          })}
          <div className="mt-auto px-3 pt-4 border-t border-border">
            <p className="text-xs text-muted-foreground/40 font-mono">ca-admin</p>
          </div>
        </aside>
        <main className="flex-1 overflow-y-auto p-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {view === "certificates" && <Certificates onRevoke={setRevokeTarget} />}
          {view === "audit" && <AuditLog />}
        </main>
      </div>
      {revokeTarget && <RevokeModal cert={revokeTarget} onClose={() => setRevokeTarget(null)} />}
    </div>
  )
}
