import { useEffect, useState } from "react"
import { useNavigate } from "react-router"
import { BarChart3, Database, FileText, Lock, LogOut, Send, Shield } from "lucide-react"
import { getStoredClientProfile } from "../services/pki-registration"
import { getSession, hasValidTgt, clearSession, type AsSession } from "../services/as-exchange"

type View = "overview" | "transfer" | "history" | "certificate"

const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "overview", label: "Tổng quan", icon: BarChart3 },
  { id: "transfer", label: "Chuyển khoản", icon: Send },
  { id: "history", label: "Lịch sử GD", icon: FileText },
  { id: "certificate", label: "Chứng chỉ X.509", icon: Shield },
]

const VIEW_COPY: Record<View, { title: string; message: string }> = {
  overview: {
    title: "Chưa có dữ liệu tài khoản",
    message: "Thông tin khách hàng, tài khoản và phiên bảo mật sẽ hiển thị khi API tương ứng được kết nối.",
  },
  transfer: {
    title: "Chưa thể chuyển khoản",
    message: "Frontend chưa nhận được tài khoản nguồn và số dư thật từ Bank Service.",
  },
  history: {
    title: "Chưa có lịch sử giao dịch",
    message: "Danh sách giao dịch sẽ hiển thị từ dữ liệu do Bank Service trả về.",
  },
  certificate: {
    title: "Chưa có dữ liệu chứng chỉ",
    message: "Thông tin chứng chỉ sẽ hiển thị sau khi được đọc từ kho cục bộ hoặc API PKI.",
  },
}

function Header({ onLogout }: { onLogout: () => void }) {
  return (
    <header className="shrink-0 border-b border-border bg-card/60 backdrop-blur-sm z-10" style={{ height: "52px" }}>
      <div className="flex items-center px-5 h-full gap-3">
        <div className="w-7 h-7 bg-blue-600 rounded-lg flex items-center justify-center">
          <Lock className="w-3.5 h-3.5 text-white" />
        </div>
        <span className="text-sm font-semibold text-foreground font-mono">Mini Banking</span>
        <button
          type="button"
          onClick={onLogout}
          className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground hover:text-red-400 transition-colors px-2.5 py-1.5 rounded-lg hover:bg-accent/50"
        >
          <LogOut className="w-3.5 h-3.5" />
          Đăng xuất
        </button>
      </div>
    </header>
  )
}

function TgtExpiry({ session }: { session: AsSession }) {
  const expires = new Date(session.tgtExpiresAt * 1000)
  return (
    <div className="mt-3 rounded-lg border border-border bg-card px-3 py-2 text-center">
      <p className="text-xs text-muted-foreground">
        Ticket Grant Ticket expired at{" "}
        <time dateTime={expires.toISOString()} className="font-mono text-foreground">
          {expires.toLocaleString("vi-VN")}
        </time>
      </p>
    </div>
  )
}

function EmptyView({ view, fullName, session }: { view: View; fullName: string; session: AsSession }) {
  const copy = VIEW_COPY[view]
  return (
    <div className="h-full min-h-80 flex items-center justify-center">
      <div className="w-full max-w-lg">
        <div className="bg-card border border-border rounded-xl p-8 text-center">
          <div className="w-12 h-12 bg-blue-500/10 rounded-xl flex items-center justify-center mx-auto mb-4">
            <Database className="w-6 h-6 text-blue-400" />
          </div>
          {view === "overview" && fullName && (
            <p className="text-lg font-semibold text-foreground mb-4">Xin chào, {fullName}</p>
          )}
          <h1 className="text-base font-semibold text-foreground">{copy.title}</h1>
          <p className="text-sm text-muted-foreground mt-2">{copy.message}</p>
        </div>
        {view === "overview" && <TgtExpiry session={session} />}
      </div>
    </div>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [view, setView] = useState<View>("overview")
  const [fullName, setFullName] = useState("")
  const [session, setSession] = useState<AsSession | null>(null)

  useEffect(() => {
    // Guard: không có TGT hợp lệ (chưa AS Exchange / TGT hết hạn / vừa reload) → quay lại đăng nhập.
    if (!hasValidTgt()) {
      navigate("/login", { replace: true })
      return
    }
    setSession(getSession())

    let active = true
    getStoredClientProfile().then(profile => {
      if (active) setFullName(profile?.fullName ?? "")
    }).catch(() => undefined)
    return () => { active = false }
  }, [navigate])

  const handleLogout = () => {
    clearSession() // zero K_{c,tgs} + xóa TGT khỏi RAM
    navigate("/login", { replace: true })
  }

  // Trong lúc guard điều hướng, tránh nháy nội dung khi chưa có phiên.
  if (!session) return null

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header onLogout={handleLogout} />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-44 shrink-0 border-r border-border bg-card/20 flex flex-col py-3 px-2 gap-1">
          {NAV.map(n => {
            const Icon = n.icon
            return (
              <button key={n.id} onClick={() => setView(n.id)} className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-left transition-colors ${view === n.id ? "bg-blue-600/15 text-blue-400 border border-blue-500/20" : "text-muted-foreground hover:text-foreground hover:bg-accent/50"}`}>
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{n.label}</span>
              </button>
            )
          })}
          <div className="mt-auto px-3 pt-4 border-t border-border">
            <p className="text-xs text-muted-foreground/40 font-mono">customer</p>
          </div>
        </aside>
        <main className="flex-1 overflow-y-auto p-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <EmptyView view={view} fullName={fullName} session={session} />
        </main>
      </div>
    </div>
  )
}
