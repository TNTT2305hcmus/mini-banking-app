import { useEffect, useState } from "react"
import { useNavigate } from "react-router"
import { ArrowRight, BarChart3, CheckCircle2, Database, FileText, KeyRound, Lock, LogOut, RefreshCw, Send, Shield, X, XCircle } from "lucide-react"
import { getStoredClientProfile } from "../services/pki-registration"
import { getSession, hasValidTgt, clearSession, type AsSession } from "../services/as-exchange"
import { clearServiceTickets } from "../services/tgs-exchange"
import { performTransfer } from "../services/bank/transfer"
import { ApiError } from "../services/api.service"

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

type TransferModal = "confirm" | "pin" | "processing" | "success" | "error" | null

function TransferPinDots({ filled }: { filled: number }) {
  return (
    <div className="flex items-center justify-center gap-3 my-5">
      {Array(6).fill(null).map((_, index) => (
        <div key={index} className={`w-3 h-3 rounded-full border-2 transition-all duration-150 ${
          index < filled
            ? "bg-blue-500 border-blue-500 scale-110"
            : "border-muted-foreground/25"
        }`} />
      ))}
    </div>
  )
}

function TransferPinKeypad({ onKey, disabled }: { onKey: (key: string) => void; disabled: boolean }) {
  const keys = ["1","2","3","4","5","6","7","8","9","","0","del"]
  return (
    <div className="grid grid-cols-3 gap-2">
      {keys.map((key, index) => {
        if (key === "") return <div key={index} />
        return (
          <button
            key={index}
            type="button"
            disabled={disabled}
            onClick={() => onKey(key)}
            className="h-12 text-base font-semibold text-foreground rounded-xl bg-background hover:bg-secondary border border-border transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center select-none"
          >
            {key === "del" ? <span className="text-sm text-muted-foreground">⌫</span> : key}
          </button>
        )
      })}
    </div>
  )
}

function TransferForm() {
  const [fromAccountId, setFromAccountId] = useState("")
  const [toAccountId, setToAccountId] = useState("")
  const [amount, setAmount] = useState("")
  const [description, setDescription] = useState("")
  const [pin, setPin] = useState("")
  const [modal, setModal] = useState<TransferModal>(null)
  const [resultMessage, setResultMessage] = useState("")

  const canContinue =
    fromAccountId.trim() !== "" &&
    toAccountId.trim() !== "" &&
    /^\d+$/.test(amount.trim()) &&
    Number(amount) > 0

  const amountNumber = Number(amount)
  const amountDisplay = new Intl.NumberFormat("vi-VN").format(amountNumber) + " VND"

  const openConfirmation = (e: React.FormEvent) => {
    e.preventDefault()
    if (!canContinue) return
    setResultMessage("")
    setModal("confirm")
  }

  const closeModal = () => {
    if (modal === "processing") return
    if (modal === "success") {
      setToAccountId("")
      setAmount("")
      setDescription("")
    }
    setModal(null)
    setPin("")
  }

  const perform = async (confirmedPin: string) => {
    setModal("processing")
    setResultMessage("")
    try {
      // Tự xin Ticket_v transfer:create (TGS) rồi ký + mã hóa + gửi AP_REQ.
      const r = await performTransfer({
        fromAccountId: fromAccountId.trim(),
        toAccountId: toAccountId.trim(),
        amount: amountNumber,
        description: description.trim() || undefined,
        pin: confirmedPin,
      })
      setResultMessage(`Mã giao dịch: ${r.transactionId}`)
      setModal("success")
      setPin("")
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : (err as Error).message
      setResultMessage(msg)
      setModal("error")
      setPin("")
    }
  }

  const handlePinKey = (key: string) => {
    if (modal !== "pin") return
    if (key === "del") {
      setPin(current => current.slice(0, -1))
      return
    }
    if (pin.length >= 6) return

    const next = pin + key
    setPin(next)
    if (next.length === 6) void perform(next)
  }

  const inputCls =
    "mt-1.5 w-full bg-background border border-border rounded-lg px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:border-blue-500/50 transition-colors"

  return (
    <>
      <form onSubmit={openConfirmation} className="w-full bg-card border border-border rounded-xl p-6">
        <div className="flex items-center gap-2 mb-5">
          <Send className="w-4 h-4 text-blue-400" />
          <h1 className="text-base font-semibold text-foreground">Chuyển khoản</h1>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div>
            <label className="block text-xs text-muted-foreground">Tài khoản nguồn (UUID)</label>
            <input className={`${inputCls} font-mono`} value={fromAccountId} onChange={e => setFromAccountId(e.target.value)} placeholder="Nhập UUID tài khoản nguồn" />
          </div>
          <div>
            <label className="block text-xs text-muted-foreground">Tài khoản nhận (UUID)</label>
            <input className={`${inputCls} font-mono`} value={toAccountId} onChange={e => setToAccountId(e.target.value)} placeholder="Nhập UUID tài khoản nhận" />
          </div>
          <div>
            <label className="block text-xs text-muted-foreground">Số tiền (VND)</label>
            <input className={`${inputCls} font-mono`} value={amount} onChange={e => setAmount(e.target.value.replace(/[^\d]/g, ""))} inputMode="numeric" placeholder="Nhập số tiền" />
          </div>
          <div>
            <label className="block text-xs text-muted-foreground">Nội dung</label>
            <input className={inputCls} value={description} onChange={e => setDescription(e.target.value)} placeholder="Nhập nội dung chuyển khoản" />
          </div>
        </div>

        <button
          type="submit"
          disabled={!canContinue}
          className="mt-5 w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors flex items-center justify-center gap-2"
        >
          <ArrowRight className="w-4 h-4" /> Tiếp tục
        </button>
      </form>

      <p className="text-[11px] text-muted-foreground/60 mt-3 text-center">
        Client tự xin Ticket_v (scope transfer:create), ký RSA-PSS và mã hóa payload bằng K(c,v) trước khi gửi.
      </p>

      {modal && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          {modal === "confirm" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-6 shadow-2xl">
              <div className="flex items-center justify-between mb-5">
                <h2 className="text-base font-semibold text-foreground">Xác nhận giao dịch</h2>
                <button type="button" onClick={closeModal} className="p-1 hover:bg-accent rounded transition-colors">
                  <X className="w-4 h-4 text-muted-foreground" />
                </button>
              </div>

              <div className="space-y-2 mb-5">
                {[
                  { label: "Tài khoản nguồn", value: fromAccountId.trim(), mono: true },
                  { label: "Tài khoản nhận", value: toAccountId.trim(), mono: true },
                  { label: "Nội dung", value: description.trim() || "—", mono: false },
                ].map(field => (
                  <div key={field.label} className="flex items-start justify-between gap-4 py-2.5 border-b border-border last:border-0">
                    <span className="text-xs text-muted-foreground shrink-0">{field.label}</span>
                    <span className={`text-sm text-foreground text-right break-all ${field.mono ? "font-mono" : ""}`}>{field.value}</span>
                  </div>
                ))}
                <div className="flex items-center justify-between py-2.5">
                  <span className="text-xs text-muted-foreground">Số tiền</span>
                  <span className="text-lg font-bold font-mono text-blue-400">{amountDisplay}</span>
                </div>
              </div>

              <div className="bg-amber-500/5 border border-amber-500/15 rounded-lg p-3 mb-5">
                <p className="text-xs text-amber-400/80">Vui lòng kiểm tra kỹ thông tin trước khi xác nhận. Giao dịch sau khi hoàn tất không thể hoàn tác.</p>
              </div>

              <div className="flex gap-2">
                <button type="button" onClick={closeModal} className="flex-1 bg-background border border-border text-muted-foreground hover:text-foreground rounded-xl py-2.5 text-sm transition-colors">Huỷ</button>
                <button type="button" onClick={() => { setPin(""); setModal("pin") }} className="flex-1 bg-blue-600 hover:bg-blue-500 text-white rounded-xl py-2.5 text-sm font-semibold transition-colors flex items-center justify-center gap-2">
                  <KeyRound className="w-3.5 h-3.5" /> Nhập mã PIN
                </button>
              </div>
            </div>
          )}

          {modal === "pin" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-6 shadow-2xl">
              <div className="flex items-center justify-between mb-4">
                <button type="button" onClick={() => { setPin(""); setModal("confirm") }} className="text-xs text-muted-foreground hover:text-foreground transition-colors">← Quay lại</button>
                <button type="button" onClick={closeModal} className="p-1 hover:bg-accent rounded transition-colors">
                  <X className="w-4 h-4 text-muted-foreground" />
                </button>
              </div>
              <div className="text-center mb-1">
                <div className="w-10 h-10 bg-blue-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
                  <KeyRound className="w-5 h-5 text-blue-400" />
                </div>
                <p className="text-base font-semibold text-foreground">Nhập mã PIN</p>
                <p className="text-xs text-muted-foreground mt-1">Xác thực giao dịch <span className="font-mono text-blue-400">{amountDisplay}</span></p>
              </div>
              <TransferPinDots filled={pin.length} />
              <TransferPinKeypad onKey={handlePinKey} disabled={false} />
            </div>
          )}

          {modal === "processing" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <RefreshCw className="w-10 h-10 text-blue-400 animate-spin mx-auto mb-4" />
              <h2 className="text-base font-semibold text-foreground mb-2">Đang xử lý giao dịch</h2>
              <p className="text-sm text-muted-foreground">Đang thực hiện TGS và AP Exchange với Bank Service…</p>
            </div>
          )}

          {modal === "success" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-emerald-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <CheckCircle2 className="w-8 h-8 text-emerald-400" />
              </div>
              <h2 className="text-lg font-semibold text-foreground mb-1">Giao dịch thành công</h2>
              <p className="text-sm text-muted-foreground mb-2">Đã chuyển <span className="font-mono font-semibold text-emerald-400">{amountDisplay}</span>.</p>
              <p className="text-xs text-muted-foreground/60 font-mono break-all mb-5">{resultMessage}</p>
              <button type="button" onClick={closeModal} className="w-full bg-blue-600 hover:bg-blue-500 text-white rounded-xl py-2.5 text-sm font-semibold transition-colors">Đóng</button>
            </div>
          )}

          {modal === "error" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <XCircle className="w-8 h-8 text-red-400" />
              </div>
              <h2 className="text-lg font-semibold text-foreground mb-1">Giao dịch thất bại</h2>
              <p className="text-sm text-muted-foreground break-words mb-5">{resultMessage}</p>
              <div className="flex gap-2">
                <button type="button" onClick={closeModal} className="flex-1 bg-background border border-border text-muted-foreground hover:text-foreground rounded-xl py-2.5 text-sm transition-colors">Đóng</button>
                <button type="button" onClick={() => { setPin(""); setModal("pin") }} className="flex-1 bg-blue-600 hover:bg-blue-500 text-white rounded-xl py-2.5 text-sm font-semibold transition-colors">Thử lại</button>
              </div>
            </div>
          )}
        </div>
      )}
    </>
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
    clearServiceTickets() // zero K_{c,v} + xóa Ticket_v khỏi RAM
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
          {view === "transfer"
            ? <TransferForm />
            : <EmptyView view={view} fullName={fullName} session={session} />}
        </main>
      </div>
    </div>
  )
}
