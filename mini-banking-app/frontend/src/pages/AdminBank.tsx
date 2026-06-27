import { useState } from "react"
import {
  Shield, BarChart3,
  Users, Database, Activity, Search, Hash, Lock,
} from "lucide-react"
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer,
} from "recharts"
import { CertBadge, TxBadge, UserStatusBadge, ActionBadge, StatCard } from "../lib/ui"
import { CERTS, USERS, TXS, B_AUDIT, CHART_DATA, formatVND, fmtDate, fmtDateTime, trunc } from "../lib/data"

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

// ─── Overview view ────────────────────────────────────────────────────────────
function Overview() {
  const totalBalance = USERS.reduce((s, u) => s + u.total_balance, 0)
  const activeUsers = USERS.filter(u => u.status === "active").length
  const completedTxs = TXS.filter(t => t.status === "completed").length
  const activeCerts = CERTS.filter(c => c.status === "active").length

  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard label="Tổng người dùng" value={USERS.length.toString()} sub={`${activeUsers} active`} icon={Users} color="blue" />
        <StatCard label="Tổng tài sản" value={`${(totalBalance / 1_000_000_000).toFixed(2)} tỷ ₫`} sub="toàn hệ thống" icon={Database} color="cyan" />
        <StatCard label="Giao dịch thành công" value={completedTxs.toString()} sub={`${TXS.length} tổng`} icon={Activity} color="emerald" />
        <StatCard label="Chứng chỉ active" value={activeCerts.toString()} sub={`${CERTS.filter(c => c.status === "revoked").length} revoked`} icon={Shield} color="purple" />
      </div>

      <div className="bg-card border border-border rounded-xl p-5">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-semibold text-foreground">Lưu lượng giao dịch — 14 ngày</h3>
          <div className="flex items-center gap-4 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5"><span className="w-3 h-0.5 bg-blue-400 inline-block rounded" /> Số GD</span>
            <span className="flex items-center gap-1.5"><span className="w-3 h-0.5 bg-cyan-400 inline-block rounded" /> Tổng (M₫)</span>
          </div>
        </div>
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={CHART_DATA} margin={{ top: 5, right: 10, bottom: 0, left: -20 }}>
            <defs>
              <linearGradient id="gbBlue" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.25} />
                <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
              </linearGradient>
              <linearGradient id="gbCyan" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#06b6d4" stopOpacity={0.25} />
                <stop offset="95%" stopColor="#06b6d4" stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis dataKey="day" tick={{ fill: "#64748b", fontSize: 11 }} axisLine={false} tickLine={false} />
            <YAxis tick={{ fill: "#64748b", fontSize: 11 }} axisLine={false} tickLine={false} />
            <Tooltip contentStyle={{ background: "#0d1520", border: "1px solid rgba(148,163,184,0.1)", borderRadius: "8px", color: "#e2e8f0", fontSize: 12 }} />
            <Area type="monotone" dataKey="txns" name="Số GD" stroke="#3b82f6" fill="url(#gbBlue)" strokeWidth={1.5} dot={false} />
            <Area type="monotone" dataKey="amount" name="Tổng (M₫)" stroke="#06b6d4" fill="url(#gbCyan)" strokeWidth={1.5} dot={false} />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="bg-card border border-border rounded-xl">
          <div className="px-4 py-3.5 border-b border-border">
            <h3 className="text-sm font-semibold text-foreground">Giao dịch mới nhất</h3>
          </div>
          <div className="divide-y divide-border">
            {TXS.slice(0, 4).map(tx => (
              <div key={tx.id} className="px-4 py-3 flex items-center justify-between hover:bg-accent/30 transition-colors">
                <div>
                  <p className="text-xs text-foreground">{tx.from_name} → {tx.to_name}</p>
                  <p className="text-xs text-muted-foreground font-mono">{fmtDate(tx.created_at)}</p>
                </div>
                <div className="text-right">
                  <p className="text-xs font-mono font-semibold text-foreground">{formatVND(tx.amount)}</p>
                  <TxBadge status={tx.status} />
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="bg-card border border-border rounded-xl">
          <div className="px-4 py-3.5 border-b border-border">
            <h3 className="text-sm font-semibold text-foreground">Sự kiện bảo mật</h3>
          </div>
          <div className="divide-y divide-border">
            {B_AUDIT.filter(e => ["replay_detected","invalid_signature","transfer_rejected","insufficient_funds"].includes(e.action)).slice(0, 4).map(e => (
              <div key={e.id} className="px-4 py-3 flex items-center justify-between hover:bg-accent/30 transition-colors">
                <div>
                  <ActionBadge action={e.action} />
                  <p className="text-xs text-muted-foreground mt-1">{e.user_email}</p>
                </div>
                <p className="text-xs text-muted-foreground font-mono shrink-0 ml-2">{fmtDate(e.created_at)}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Users view ───────────────────────────────────────────────────────────────
function UsersView() {
  const [search, setSearch] = useState("")
  const filtered = USERS.filter(u => !search || u.full_name.toLowerCase().includes(search.toLowerCase()) || u.email.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="space-y-4">
      <div className="relative max-w-xs">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
        <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Tìm người dùng..." className="w-full bg-card border border-border rounded-lg pl-9 pr-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/30" />
      </div>
      <div className="bg-card border border-border rounded-xl overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border">
              {["Người dùng","Status","Tài khoản","Tổng số dư","Certificate","Đăng ký"].map(h => (
                <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {filtered.map(user => {
              const cert = CERTS.find(c => c.owner_id === user.id)
              return (
                <tr key={user.id} className="hover:bg-accent/30 transition-colors">
                  <td className="px-4 py-3">
                    <p className="text-sm text-foreground">{user.full_name}</p>
                    <p className="text-xs text-muted-foreground">{user.email}</p>
                  </td>
                  <td className="px-4 py-3"><UserStatusBadge status={user.status} /></td>
                  <td className="px-4 py-3 text-sm font-mono text-foreground">{user.accounts}</td>
                  <td className="px-4 py-3">
                    {user.status === "locked"
                      ? <span className="text-xs text-muted-foreground/30 font-mono">—</span>
                      : <span className="text-sm font-mono text-foreground">{formatVND(user.total_balance)}</span>
                    }
                  </td>
                  <td className="px-4 py-3">
                    {cert ? <CertBadge status={cert.status} /> : <span className="text-xs text-muted-foreground/30">—</span>}
                  </td>
                  <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{fmtDate(user.created_at)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ─── Transactions view ────────────────────────────────────────────────────────
function TransactionsView() {
  return (
    <div className="bg-card border border-border rounded-xl overflow-hidden">
      <div className="px-5 py-4 border-b border-border flex items-center justify-between">
        <h3 className="text-sm font-semibold text-foreground">Immutable Ledger</h3>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Hash className="w-3.5 h-3.5" />
          <span className="font-mono">Hash-chained · {TXS.length} records</span>
        </div>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border">
              {["ID","Từ","Đến","Số tiền","Status","Cert Serial","Chain Hash","Thời gian"].map(h => (
                <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {TXS.map(tx => (
              <tr key={tx.id} className="hover:bg-accent/30 transition-colors">
                <td className="px-4 py-3">
                  <p className="text-xs font-mono text-muted-foreground">{tx.id}</p>
                  <p className="text-xs text-muted-foreground">{trunc(tx.description, 22)}</p>
                </td>
                <td className="px-4 py-3">
                  <p className="text-xs text-foreground">{tx.from_name}</p>
                  <p className="text-xs font-mono text-muted-foreground">{tx.from_acct}</p>
                </td>
                <td className="px-4 py-3">
                  <p className="text-xs text-foreground">{tx.to_name}</p>
                  <p className="text-xs font-mono text-muted-foreground">{tx.to_acct}</p>
                </td>
                <td className="px-4 py-3 text-sm font-mono font-semibold text-foreground whitespace-nowrap">{formatVND(tx.amount)}</td>
                <td className="px-4 py-3"><TxBadge status={tx.status} /></td>
                <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{tx.cert_serial.slice(0, 10)}…</td>
                <td className="px-4 py-3">
                  {tx.chain_hash
                    ? <span className="text-xs font-mono text-cyan-500/60">{tx.chain_hash}</span>
                    : <span className="text-xs text-muted-foreground/30">—</span>
                  }
                </td>
                <td className="px-4 py-3 text-xs font-mono text-muted-foreground whitespace-nowrap">{fmtDate(tx.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ─── Audit view ───────────────────────────────────────────────────────────────
function AuditView() {
  return (
    <div className="bg-card border border-border rounded-xl overflow-hidden">
      <div className="px-5 py-4 border-b border-border flex items-center justify-between">
        <h3 className="text-sm font-semibold text-foreground">Bank Security Audit Log</h3>
        <span className="text-xs text-muted-foreground font-mono">{B_AUDIT.length} events</span>
      </div>
      <table className="w-full">
        <thead>
          <tr className="border-b border-border">
            {["Event","User","Cert Serial","Reason","Timestamp"].map(h => (
              <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {B_AUDIT.map(e => (
            <tr key={e.id} className="hover:bg-accent/30 transition-colors">
              <td className="px-4 py-3"><ActionBadge action={e.action} /></td>
              <td className="px-4 py-3 text-xs text-foreground">{e.user_email}</td>
              <td className="px-4 py-3 text-xs font-mono text-muted-foreground">
                {e.cert_serial ? `${e.cert_serial.slice(0, 10)}…` : <span className="text-muted-foreground/30">—</span>}
              </td>
              <td className="px-4 py-3 text-xs text-muted-foreground">
                {e.reason ? trunc(e.reason, 46) : <span className="text-muted-foreground/30">—</span>}
              </td>
              <td className="px-4 py-3 text-xs font-mono text-muted-foreground whitespace-nowrap">{fmtDateTime(e.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── AdminBank page ───────────────────────────────────────────────────────────
type View = "overview" | "users" | "transactions" | "audit"
const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "overview", label: "Tổng quan", icon: BarChart3 },
  { id: "users", label: "Người dùng", icon: Users },
  { id: "transactions", label: "Ledger", icon: Database },
  { id: "audit", label: "Security Audit", icon: Activity },
]

export default function AdminBank() {
  const [view, setView] = useState<View>("overview")
  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-44 shrink-0 border-r border-border bg-card/20 flex flex-col py-3 px-2 gap-1">
          {NAV.map(n => {
            const Icon = n.icon
            return (
              <button key={n.id} onClick={() => setView(n.id)} className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-left transition-colors ${view === n.id ? "bg-cyan-600/15 text-cyan-400 border border-cyan-500/20" : "text-muted-foreground hover:text-foreground hover:bg-accent/50"}`}>
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{n.label}</span>
              </button>
            )
          })}
          <div className="mt-auto px-3 pt-4 border-t border-border">
            <p className="text-xs text-muted-foreground/40 font-mono">bank-admin</p>
          </div>
        </aside>
        <main className="flex-1 overflow-y-auto p-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {view === "overview" && <Overview />}
          {view === "users" && <UsersView />}
          {view === "transactions" && <TransactionsView />}
          {view === "audit" && <AuditView />}
        </main>
      </div>
    </div>
  )
}
