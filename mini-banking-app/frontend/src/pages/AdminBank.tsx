import { useState } from "react"
import { Activity, BarChart3, Database, Lock, Users } from "lucide-react"

type View = "overview" | "users" | "transactions" | "audit"

const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "overview", label: "Tổng quan", icon: BarChart3 },
  { id: "users", label: "Người dùng", icon: Users },
  { id: "transactions", label: "Ledger", icon: Database },
  { id: "audit", label: "Security Audit", icon: Activity },
]

const VIEW_COPY: Record<View, { title: string; message: string }> = {
  overview: {
    title: "Chưa có dữ liệu tổng quan",
    message: "Các chỉ số hệ thống sẽ hiển thị khi frontend được kết nối với API quản trị Bank.",
  },
  users: {
    title: "Chưa có dữ liệu người dùng",
    message: "Danh sách người dùng và tài khoản sẽ được tải từ Bank Service.",
  },
  transactions: {
    title: "Chưa có dữ liệu ledger",
    message: "Giao dịch và hash chain sẽ được tải từ Bank Service.",
  },
  audit: {
    title: "Chưa có nhật ký bảo mật",
    message: "Các sự kiện bảo mật sẽ được tải từ Bank Service.",
  },
}

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

export default function AdminBank() {
  const [view, setView] = useState<View>("overview")
  const copy = VIEW_COPY[view]

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
        <main className="flex-1 p-5 flex items-center justify-center">
          <div className="w-full max-w-lg bg-card border border-border rounded-xl p-8 text-center">
            <div className="w-12 h-12 bg-cyan-500/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <Database className="w-6 h-6 text-cyan-400" />
            </div>
            <h1 className="text-base font-semibold text-foreground">{copy.title}</h1>
            <p className="text-sm text-muted-foreground mt-2">{copy.message}</p>
          </div>
        </main>
      </div>
    </div>
  )
}
