import type { CertStatus, TxStatus } from "./data"

export function CertBadge({ status }: { status: CertStatus }) {
  const map = {
    active: { cls: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20", dot: "bg-emerald-400" },
    revoked: { cls: "bg-red-500/10 text-red-400 border-red-500/20", dot: "bg-red-400" },
    expired: { cls: "bg-amber-500/10 text-amber-400 border-amber-500/20", dot: "bg-amber-400" },
  }[status]
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-xs font-mono font-medium ${map.cls}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${map.dot}`} />
      {status}
    </span>
  )
}

export function TxBadge({ status }: { status: TxStatus }) {
  const map = {
    completed: { cls: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20", dot: "bg-emerald-400" },
    failed: { cls: "bg-red-500/10 text-red-400 border-red-500/20", dot: "bg-red-400" },
    pending: { cls: "bg-sky-500/10 text-sky-400 border-sky-500/20", dot: "bg-sky-400" },
  }[status]
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-xs font-mono ${map.cls}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${map.dot}`} />
      {status}
    </span>
  )
}

export function UserStatusBadge({ status }: { status: "active" | "locked" }) {
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-xs font-mono ${
      status === "active"
        ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
        : "bg-red-500/10 text-red-400 border-red-500/20"
    }`}>
      <span className={`w-1.5 h-1.5 rounded-full ${status === "active" ? "bg-emerald-400" : "bg-red-400"}`} />
      {status}
    </span>
  )
}

export const ACTION_STYLES: Record<string, string> = {
  issued: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
  revoked: "bg-red-500/10 text-red-400 border-red-500/20",
  looked_up: "bg-blue-500/10 text-blue-400 border-blue-500/20",
  revocation_checked: "bg-purple-500/10 text-purple-400 border-purple-500/20",
  transfer_completed: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
  transfer_rejected: "bg-red-500/10 text-red-400 border-red-500/20",
  replay_detected: "bg-orange-500/10 text-orange-400 border-orange-500/20",
  invalid_signature: "bg-red-500/10 text-red-400 border-red-500/20",
  insufficient_funds: "bg-amber-500/10 text-amber-400 border-amber-500/20",
  certificate_rejected: "bg-red-500/10 text-red-400 border-red-500/20",
}

export function ActionBadge({ action }: { action: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded border text-xs font-mono ${ACTION_STYLES[action] || "bg-muted text-muted-foreground border-border"}`}>
      {action.replace(/_/g, " ")}
    </span>
  )
}

export function StatCard({ label, value, sub, icon: Icon, color = "blue" }: {
  label: string; value: string; sub?: string
  icon: (props: { className?: string }) => JSX.Element
  color?: string
}) {
  const colors: Record<string, string> = {
    blue: "text-blue-400 bg-blue-500/10",
    cyan: "text-cyan-400 bg-cyan-500/10",
    emerald: "text-emerald-400 bg-emerald-500/10",
    amber: "text-amber-400 bg-amber-500/10",
    purple: "text-purple-400 bg-purple-500/10",
    red: "text-red-400 bg-red-500/10",
  }
  return (
    <div className="bg-card border border-border rounded-xl p-4 flex items-start gap-3">
      <div className={`p-2.5 rounded-lg shrink-0 ${colors[color]}`}>
        <Icon className="w-4 h-4" />
      </div>
      <div>
        <p className="text-xs text-muted-foreground mb-0.5">{label}</p>
        <p className="text-xl font-semibold text-foreground font-mono leading-tight">{value}</p>
        {sub && <p className="text-xs text-muted-foreground mt-0.5">{sub}</p>}
      </div>
    </div>
  )
}
