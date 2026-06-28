import { useState } from "react"
import { Activity, Database, Lock, ShieldCheck } from "lucide-react"

type View = "certificates" | "audit"

const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "certificates", label: "Certificates", icon: ShieldCheck },
  { id: "audit", label: "Audit Log", icon: Activity },
]

const VIEW_COPY: Record<View, { title: string; message: string }> = {
  certificates: {
    title: "Chưa có dữ liệu chứng chỉ",
    message: "Danh sách chứng chỉ sẽ hiển thị khi frontend được kết nối với API quản trị CA.",
  },
  audit: {
    title: "Chưa có nhật ký CA",
    message: "Nhật ký cấp, tra cứu và thu hồi chứng chỉ sẽ được tải từ CA Service.",
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

export default function AdminCA() {
  const [view, setView] = useState<View>("certificates")
  const copy = VIEW_COPY[view]

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
        <main className="flex-1 p-5 flex items-center justify-center">
          <div className="w-full max-w-lg bg-card border border-border rounded-xl p-8 text-center">
            <div className="w-12 h-12 bg-purple-500/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <Database className="w-6 h-6 text-purple-400" />
            </div>
            <h1 className="text-base font-semibold text-foreground">{copy.title}</h1>
            <p className="text-sm text-muted-foreground mt-2">{copy.message}</p>
          </div>
        </main>
      </div>
    </div>
  )
}
