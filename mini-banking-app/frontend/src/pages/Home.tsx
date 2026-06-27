import { useState, FormEvent } from "react"
import { Link } from "react-router"
import {
  Shield,
  FileText, BarChart3, Send, CheckCircle, XCircle,
  ArrowUpRight, ArrowDownLeft, Lock, X, AlertTriangle,
  KeyRound, ArrowRight,
} from "lucide-react"
import { CertBadge, TxBadge } from "../lib/ui"
import { CERTS, TXS, formatVND, fmtDate, fmtDateTime, trunc, type TxStatus } from "../lib/data"

const ALICE_CERT = CERTS[0]
const ALICE_TXS = TXS.filter(t => t.from_name === "Alice Nguyen" || t.to_name === "Alice Nguyen")

type View = "overview" | "transfer" | "history" | "certificate"

// ─── Portal header ────────────────────────────────────────────────────────────
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

// ─── Views ────────────────────────────────────────────────────────────────────
function Overview() {
  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-card border border-border rounded-xl p-5">
          <div className="flex items-center gap-4">
            <div className="w-14 h-14 rounded-2xl bg-blue-600/20 border border-blue-500/30 flex items-center justify-center shrink-0">
              <span className="text-2xl font-bold text-blue-400">A</span>
            </div>
            <div>
              <p className="text-3xl font-bold text-foreground">Alice Nguyen</p>
              <p className="text-sm text-muted-foreground mt-1">alice@minibank.vn</p>
            </div>
          </div>
        </div>

        <div className="bg-gradient-to-br from-blue-600/20 via-blue-600/10 to-transparent border border-blue-500/20 rounded-xl p-5">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs text-blue-300/60 font-mono uppercase tracking-wider">Tài khoản chính</span>
            <span className="text-xs font-mono text-muted-foreground">VN0001001</span>
          </div>
          <p className="text-3xl font-bold text-white font-mono mb-1">{formatVND(98_500_000)}</p>
          <p className="text-xs text-muted-foreground">Hạn mức ngày: {formatVND(50_000_000)}</p>
          <div className="mt-4 flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse inline-block" />
            <span className="text-xs text-emerald-400 font-mono">active</span>
          </div>
        </div>
      </div>

      <div className="bg-card border border-border rounded-xl p-5">
        <h3 className="text-sm font-semibold text-foreground mb-3 flex items-center gap-2">
          <Shield className="w-4 h-4 text-cyan-400" />
          Trạng thái phiên bảo mật
        </h3>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {[
            { label: "X.509 Certificate", value: "Active" },
            { label: "TGT", value: "Hợp lệ · 12m" },
            { label: "Ticket_v", value: "Hợp lệ · 4m" },
            { label: "Phiên giao dịch", value: "Đã xác thực" },
          ].map(item => (
            <div key={item.label} className="bg-background border border-border rounded-lg p-3">
              <p className="text-xs text-muted-foreground mb-1.5">{item.label}</p>
              <div className="flex items-center gap-1.5">
                <CheckCircle className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span className="text-xs font-mono text-foreground">{item.value}</span>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="bg-card border border-border rounded-xl">
        <div className="px-5 py-3.5 border-b border-border flex items-center justify-between">
          <h3 className="text-sm font-semibold text-foreground">Giao dịch gần đây</h3>
          <span className="text-xs text-muted-foreground">{ALICE_TXS.length} giao dịch</span>
        </div>
        <div className="divide-y divide-border">
          {ALICE_TXS.slice(0, 4).map(tx => {
            const isOut = tx.from_name === "Alice Nguyen"
            return (
              <div key={tx.id} className="px-5 py-3.5 flex items-center justify-between hover:bg-accent/40 transition-colors">
                <div className="flex items-center gap-3">
                  <div className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 ${
                    tx.status === "failed" ? "bg-red-500/10" : isOut ? "bg-blue-500/10" : "bg-emerald-500/10"
                  }`}>
                    {tx.status === "failed"
                      ? <XCircle className="w-4 h-4 text-red-400" />
                      : isOut ? <ArrowUpRight className="w-4 h-4 text-blue-400" />
                        : <ArrowDownLeft className="w-4 h-4 text-emerald-400" />}
                  </div>
                  <div>
                    <p className="text-sm text-foreground">{isOut ? tx.to_name : tx.from_name}</p>
                    <p className="text-xs text-muted-foreground">{trunc(tx.description, 42)}</p>
                  </div>
                </div>
                <div className="text-right shrink-0">
                  <p className={`text-sm font-mono font-semibold ${
                    tx.status === "failed" ? "text-muted-foreground line-through"
                    : isOut ? "text-red-400" : "text-emerald-400"
                  }`}>
                    {isOut ? "−" : "+"}{formatVND(tx.amount)}
                  </p>
                  <p className="text-xs text-muted-foreground">{fmtDate(tx.created_at)}</p>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

// ─── Transfer modal step type ─────────────────────────────────────────────────
type TransferModal = "confirm" | "pin" | "processing" | "success" | "error-pin" | "error-cert" | "error-balance" | null

const DEMO_ERRORS = [
  { id: "error-pin", label: "Sai mã PIN" },
  { id: "error-cert", label: "Certificate lỗi" },
  { id: "error-balance", label: "Số dư không đủ" },
] as const

// PIN keypad (shared inside Transfer)
function TransferPinDots({ filled, error }: { filled: number; error: boolean }) {
  return (
    <div className="flex items-center justify-center gap-3 my-5">
      {Array(6).fill(null).map((_, i) => (
        <div key={i} className={`w-3 h-3 rounded-full border-2 transition-all duration-150 ${
          error ? "bg-red-500 border-red-500"
            : i < filled ? "bg-blue-500 border-blue-500 scale-110"
              : "border-muted-foreground/25"
        }`} />
      ))}
    </div>
  )
}

function TransferPinKeypad({ onKey }: { onKey: (k: string) => void }) {
  const keys = ["1","2","3","4","5","6","7","8","9","","0","del"]
  return (
    <div className="grid grid-cols-3 gap-2">
      {keys.map((k, i) => {
        if (k === "") return <div key={i} />
        return (
          <button key={i} onClick={() => onKey(k)}
            className="h-12 text-base font-semibold text-foreground rounded-xl bg-background hover:bg-secondary border border-border transition-all active:scale-95 flex items-center justify-center select-none">
            {k === "del" ? <span className="text-sm text-muted-foreground">⌫</span> : k}
          </button>
        )
      })}
    </div>
  )
}

function Transfer() {
  const [form, setForm] = useState({ to_account: "", amount: "", description: "" })
  const [modal, setModal] = useState<TransferModal>(null)
  const [pin, setPin] = useState("")
  const [pinError, setPinError] = useState(false)
  // demo error simulation
  const [demoError, setDemoError] = useState<"error-pin" | "error-cert" | "error-balance" | "">("")

  const reset = () => { setModal(null); setPin(""); setPinError(false) }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!form.to_account.trim() || !form.amount) return
    setModal("confirm")
  }

  const handleConfirm = () => { setPin(""); setPinError(false); setModal("pin") }

  const handlePinKey = (key: string) => {
    if (pinError) { setPinError(false); setPin("") }
    if (key === "del") { setPin(p => p.slice(0, -1)); return }
    if (pin.length >= 6) return
    const next = pin + key
    setPin(next)
    if (next.length === 6) {
      // short pause so user sees all 6 dots filled, then go to processing
      setTimeout(() => {
        if (demoError === "error-pin") {
          setPinError(true)
          setTimeout(() => setPin(""), 700)
          return
        }
        // PIN ok → show processing spinner while "waiting for server"
        setModal("processing")
        const delay = 1400 + Math.random() * 800
        setTimeout(() => {
          if (demoError === "error-cert") {
            setModal("error-cert")
          } else if (demoError === "error-balance") {
            setModal("error-balance")
          } else {
            setModal("success")
            setTimeout(() => { reset(); setForm({ to_account: "", amount: "", description: "" }) }, 2200)
          }
        }, delay)
      }, 300)
    }
  }

  const amountNum = Number(form.amount)

  return (
    <>
      <div className="max-w-lg">
        <div className="bg-card border border-border rounded-xl p-6">
          <h3 className="text-base font-semibold text-foreground mb-5 flex items-center gap-2">
            <Send className="w-4 h-4 text-blue-400" /> Chuyển khoản
          </h3>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-xs text-muted-foreground mb-1.5">Tài khoản nguồn</label>
              <div className="w-full bg-background border border-border rounded-lg px-3 py-2.5 flex items-center justify-between">
                <span className="text-sm font-mono text-foreground">VN0001001</span>
                <span className="text-sm font-mono text-muted-foreground">{formatVND(98_500_000)}</span>
              </div>
            </div>
            <div>
              <label className="block text-xs text-muted-foreground mb-1.5">Số tài khoản nhận</label>
              <input type="text" value={form.to_account} onChange={e => setForm({ ...form, to_account: e.target.value })} placeholder="VN0002001" className="w-full bg-background border border-border rounded-lg px-3 py-2.5 text-sm font-mono text-foreground placeholder:text-muted-foreground/30" />
            </div>
            <div>
              <label className="block text-xs text-muted-foreground mb-1.5">Số tiền (VND)</label>
              <input type="number" value={form.amount} onChange={e => setForm({ ...form, amount: e.target.value })} placeholder="1000000" className="w-full bg-background border border-border rounded-lg px-3 py-2.5 text-sm font-mono text-foreground placeholder:text-muted-foreground/30" />
            </div>
            <div>
              <label className="block text-xs text-muted-foreground mb-1.5">Nội dung</label>
              <input type="text" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} placeholder="Thanh toán..." className="w-full bg-background border border-border rounded-lg px-3 py-2.5 text-sm text-foreground placeholder:text-muted-foreground/30" />
            </div>

            {/* Demo error simulator */}
            <div className="bg-background border border-border rounded-lg p-3">
              <p className="text-xs text-muted-foreground/60 font-mono mb-2">Demo: Mô phỏng lỗi (tuỳ chọn)</p>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={() => setDemoError("")}
                  className={`px-2.5 py-1 rounded text-xs font-mono transition-colors ${!demoError ? "bg-emerald-500/15 text-emerald-400 border border-emerald-500/20" : "bg-background border border-border text-muted-foreground hover:text-foreground"}`}>
                  Thành công
                </button>
                {DEMO_ERRORS.map(e => (
                  <button key={e.id} type="button" onClick={() => setDemoError(e.id)}
                    className={`px-2.5 py-1 rounded text-xs font-mono transition-colors ${demoError === e.id ? "bg-red-500/15 text-red-400 border border-red-500/20" : "bg-background border border-border text-muted-foreground hover:text-foreground"}`}>
                    {e.label}
                  </button>
                ))}
              </div>
            </div>

            <button type="submit" disabled={!form.to_account.trim() || !form.amount}
              className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-lg py-2.5 text-sm font-semibold transition-colors flex items-center justify-center gap-2">
              <ArrowRight className="w-4 h-4" /> Tiếp tục
            </button>
          </form>
        </div>
      </div>

      {/* ── Modal overlay ───────────────────────────────────────────────────── */}
      {modal && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">

          {/* Step 1: Confirm summary */}
          {modal === "confirm" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-6 shadow-2xl">
              <div className="flex items-center justify-between mb-5">
                <h3 className="text-base font-semibold text-foreground">Xác nhận giao dịch</h3>
                <button onClick={reset} className="p-1 hover:bg-accent rounded transition-colors">
                  <X className="w-4 h-4 text-muted-foreground" />
                </button>
              </div>

              <div className="space-y-2 mb-5">
                {[
                  { label: "Tài khoản nguồn", value: "VN0001001", mono: true },
                  { label: "Số tài khoản nhận", value: form.to_account || "—", mono: true },
                  { label: "Nội dung", value: form.description || "—", mono: false },
                ].map(f => (
                  <div key={f.label} className="flex items-start justify-between gap-4 py-2.5 border-b border-border last:border-0">
                    <span className="text-xs text-muted-foreground shrink-0">{f.label}</span>
                    <span className={`text-sm text-foreground text-right break-all ${f.mono ? "font-mono" : ""}`}>{f.value}</span>
                  </div>
                ))}
                <div className="flex items-center justify-between py-2.5">
                  <span className="text-xs text-muted-foreground">Số tiền</span>
                  <span className="text-lg font-bold font-mono text-blue-400">{formatVND(amountNum)}</span>
                </div>
              </div>

              <div className="bg-amber-500/5 border border-amber-500/15 rounded-lg p-3 mb-5">
                <p className="text-xs text-amber-400/80">Vui lòng kiểm tra kỹ thông tin trước khi xác nhận. Giao dịch sau khi hoàn tất không thể hoàn tác.</p>
              </div>

              <div className="flex gap-2">
                <button onClick={reset} className="flex-1 bg-background border border-border text-muted-foreground hover:text-foreground rounded-xl py-2.5 text-sm transition-colors">Huỷ</button>
                <button onClick={handleConfirm} className="flex-1 bg-blue-600 hover:bg-blue-500 text-white rounded-xl py-2.5 text-sm font-semibold transition-colors flex items-center justify-center gap-2">
                  <KeyRound className="w-3.5 h-3.5" /> Nhập mã PIN
                </button>
              </div>
            </div>
          )}

          {/* Step 2: PIN entry */}
          {modal === "pin" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-6 shadow-2xl">
              <div className="flex items-center justify-between mb-4">
                <button onClick={() => { setPin(""); setPinError(false); setModal("confirm") }}
                  className="text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1">
                  ← Quay lại
                </button>
                <button onClick={reset} className="p-1 hover:bg-accent rounded transition-colors">
                  <X className="w-4 h-4 text-muted-foreground" />
                </button>
              </div>

              <div className="text-center mb-1">
                <div className="w-10 h-10 bg-blue-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
                  <KeyRound className="w-5 h-5 text-blue-400" />
                </div>
                <p className="text-base font-semibold text-foreground">Nhập mã PIN</p>
                <p className="text-xs text-muted-foreground mt-1">Xác thực giao dịch <span className="font-mono text-blue-400">{formatVND(amountNum)}</span></p>
              </div>

              <TransferPinDots filled={pin.length} error={pinError} />

              {pinError && (
                <div className="flex items-center justify-center gap-1.5 mb-3 -mt-2">
                  <XCircle className="w-3.5 h-3.5 text-red-400" />
                  <p className="text-xs text-red-400">Mã PIN không chính xác. Thử lại.</p>
                </div>
              )}

              <TransferPinKeypad onKey={handlePinKey} />
            </div>
          )}

          {/* Processing */}
          {modal === "processing" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="relative w-16 h-16 mx-auto mb-5">
                {/* outer ring */}
                <svg className="absolute inset-0 w-full h-full animate-spin" viewBox="0 0 64 64" fill="none">
                  <circle cx="32" cy="32" r="28" stroke="rgba(59,130,246,0.15)" strokeWidth="4" />
                  <path d="M32 4 a28 28 0 0 1 28 28" stroke="#3b82f6" strokeWidth="4" strokeLinecap="round" />
                </svg>
                {/* inner icon */}
                <div className="absolute inset-0 flex items-center justify-center">
                  <Send className="w-6 h-6 text-blue-400" />
                </div>
              </div>
              <h3 className="text-base font-semibold text-foreground mb-2">Đang xử lý giao dịch</h3>
              <p className="text-sm text-muted-foreground mb-5">Đang chờ phản hồi từ Bank Server…</p>
              <div className="bg-background border border-border rounded-lg p-3 text-left space-y-1.5">
                {[
                  { label: "Verify Ticket_v + chữ ký số", done: true },
                  { label: "Kiểm tra revocation certificate", done: true },
                  { label: "Kiểm tra số dư & hạn mức ngày", done: true },
                  { label: "Ghi ledger & hash chain…", done: false },
                ].map((step, i) => (
                  <div key={i} className="flex items-center gap-2">
                    {step.done
                      ? <CheckCircle className="w-3 h-3 text-emerald-400 shrink-0" />
                      : <svg className="w-3 h-3 text-blue-400 shrink-0 animate-spin" viewBox="0 0 12 12" fill="none">
                          <circle cx="6" cy="6" r="4.5" stroke="rgba(59,130,246,0.2)" strokeWidth="1.5"/>
                          <path d="M6 1.5 a4.5 4.5 0 0 1 4.5 4.5" stroke="#3b82f6" strokeWidth="1.5" strokeLinecap="round"/>
                        </svg>
                    }
                    <span className={`text-xs font-mono ${step.done ? "text-muted-foreground" : "text-blue-400"}`}>{step.label}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Success */}
          {modal === "success" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-emerald-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <CheckCircle className="w-8 h-8 text-emerald-400" />
              </div>
              <h3 className="text-lg font-semibold text-foreground mb-1">Giao dịch thành công</h3>
              <p className="text-sm text-muted-foreground mb-4">
                Đã chuyển <span className="font-mono font-semibold text-emerald-400">{formatVND(amountNum)}</span> đến <span className="font-mono text-foreground">{form.to_account}</span>
              </p>
              <p className="text-xs text-muted-foreground/50 font-mono">Payload đã ký số · Hash chain đã ghi</p>
            </div>
          )}

          {/* Error: wrong PIN */}
          {modal === "error-pin" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <XCircle className="w-8 h-8 text-red-400" />
              </div>
              <h3 className="text-lg font-semibold text-foreground mb-1">Sai mã PIN</h3>
              <p className="text-sm text-muted-foreground mb-5">Mã PIN không chính xác. Giao dịch bị từ chối.</p>
              <button onClick={() => { setPin(""); setPinError(false); setModal("pin") }}
                className="w-full bg-background border border-border text-foreground hover:bg-accent rounded-xl py-2.5 text-sm transition-colors">
                Thử lại
              </button>
            </div>
          )}

          {/* Error: certificate */}
          {modal === "error-cert" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <AlertTriangle className="w-8 h-8 text-red-400" />
              </div>
              <h3 className="text-lg font-semibold text-foreground mb-1">Lỗi xác thực Certificate</h3>
              <p className="text-sm text-muted-foreground mb-2">Giao dịch bị từ chối bởi Bank Service.</p>
              <div className="bg-background border border-border rounded-lg p-3 mb-5 text-left">
                <p className="text-xs text-muted-foreground">Certificate X.509 của bạn đã bị thu hồi hoặc hết hạn. Vui lòng liên hệ Admin CA để cấp lại chứng chỉ.</p>
                <p className="text-xs font-mono text-muted-foreground/50 mt-1.5">serial: {ALICE_CERT.serial.slice(0, 16)}</p>
              </div>
              <button onClick={reset} className="w-full bg-background border border-border text-foreground hover:bg-accent rounded-xl py-2.5 text-sm transition-colors">Đóng</button>
            </div>
          )}

          {/* Error: insufficient balance */}
          {modal === "error-balance" && (
            <div className="bg-card border border-border rounded-2xl w-full max-w-sm p-8 shadow-2xl text-center">
              <div className="w-16 h-16 bg-amber-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
                <AlertTriangle className="w-8 h-8 text-amber-400" />
              </div>
              <h3 className="text-lg font-semibold text-foreground mb-1">Số dư không đủ</h3>
              <p className="text-sm text-muted-foreground mb-2">Giao dịch bị từ chối bởi Bank Service.</p>
              <div className="bg-background border border-border rounded-lg p-3 mb-5 text-left">
                <p className="text-xs text-muted-foreground">Số dư tài khoản không đủ để thực hiện giao dịch này.</p>
                <p className="text-xs font-mono text-muted-foreground/50 mt-1.5">balance: {formatVND(98_500_000)} &lt; requested: {formatVND(amountNum)}</p>
              </div>
              <button onClick={reset} className="w-full bg-background border border-border text-foreground hover:bg-accent rounded-xl py-2.5 text-sm transition-colors">Đóng</button>
            </div>
          )}

        </div>
      )}
    </>
  )
}

function History() {
  const [filter, setFilter] = useState<TxStatus | "all">("all")
  const filtered = filter === "all" ? ALICE_TXS : ALICE_TXS.filter(t => t.status === filter)
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        {(["all", "completed", "failed"] as const).map(f => (
          <button key={f} onClick={() => setFilter(f)} className={`px-3 py-1.5 rounded-lg text-xs font-mono transition-colors ${filter === f ? "bg-blue-600 text-white" : "bg-card border border-border text-muted-foreground hover:text-foreground"}`}>
            {f === "all" ? "Tất cả" : f}
          </button>
        ))}
      </div>
      <div className="bg-card border border-border rounded-xl overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border">
              {["Đối tác","Nội dung","Số tiền","Trạng thái","Ngày"].map(h => (
                <th key={h} className="text-left text-xs text-muted-foreground font-medium px-4 py-3">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {filtered.map(tx => {
              const isOut = tx.from_name === "Alice Nguyen"
              return (
                <tr key={tx.id} className="hover:bg-accent/30 transition-colors">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      {isOut ? <ArrowUpRight className="w-3.5 h-3.5 text-blue-400 shrink-0" /> : <ArrowDownLeft className="w-3.5 h-3.5 text-emerald-400 shrink-0" />}
                      <span className="text-sm text-foreground">{isOut ? tx.to_name : tx.from_name}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">{trunc(tx.description, 36)}</td>
                  <td className="px-4 py-3">
                    <span className={`text-sm font-mono font-semibold ${tx.status === "failed" ? "text-muted-foreground line-through" : isOut ? "text-red-400" : "text-emerald-400"}`}>
                      {isOut ? "−" : "+"}{formatVND(tx.amount)}
                    </span>
                  </td>
                  <td className="px-4 py-3"><TxBadge status={tx.status} /></td>
                  <td className="px-4 py-3 text-xs font-mono text-muted-foreground">{fmtDate(tx.created_at)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function Certificate() {
  return (
    <div className="max-w-2xl space-y-4">
      <div className="bg-card border border-border rounded-xl p-5">
        <div className="flex items-start justify-between mb-5">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-cyan-500/10 rounded-xl"><Shield className="w-5 h-5 text-cyan-400" /></div>
            <div>
              <p className="text-base font-semibold text-foreground">{ALICE_CERT.cn}</p>
              <p className="text-xs text-muted-foreground">{ALICE_CERT.email}</p>
            </div>
          </div>
          <CertBadge status={ALICE_CERT.status} />
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {[
            { label: "Serial Number", value: ALICE_CERT.serial, mono: true },
            { label: "Owner ID", value: ALICE_CERT.owner_id, mono: true },
            { label: "Subject CN", value: ALICE_CERT.cn, mono: false },
            { label: "Subject Email", value: ALICE_CERT.email, mono: false },
            { label: "Not Before", value: fmtDate(ALICE_CERT.not_before), mono: true },
            { label: "Not After", value: fmtDate(ALICE_CERT.not_after), mono: true },
            { label: "Issued At", value: fmtDateTime(ALICE_CERT.issued_at), mono: true },
          ].map(f => (
            <div key={f.label} className="bg-background border border-border rounded-lg p-3">
              <p className="text-xs text-muted-foreground mb-1">{f.label}</p>
              <p className={`text-sm text-foreground break-all ${f.mono ? "font-mono" : ""}`}>{f.value}</p>
            </div>
          ))}
        </div>
        <div className="mt-3 bg-background border border-border rounded-lg p-3">
          <p className="text-xs text-muted-foreground mb-1">SHA-256 Fingerprint</p>
          <p className="text-xs font-mono text-foreground break-all leading-relaxed">{ALICE_CERT.fingerprint}</p>
        </div>
        <div className="mt-3 bg-amber-500/5 border border-amber-500/15 rounded-lg p-3">
          <p className="text-xs text-amber-400/70 font-mono">Private key được sinh tại trình duyệt (WebCrypto API) và lưu dạng wrapped key trong IndexedDB. Server không giữ private key.</p>
        </div>
      </div>
    </div>
  )
}

// ─── Home page ────────────────────────────────────────────────────────────────
const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "overview", label: "Tổng quan", icon: BarChart3 },
  { id: "transfer", label: "Chuyển khoản", icon: Send },
  { id: "history", label: "Lịch sử GD", icon: FileText },
  { id: "certificate", label: "Chứng chỉ X.509", icon: Shield },
]

export default function Home() {
  const [view, setView] = useState<View>("overview")
  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header />
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
          {view === "overview" && <Overview />}
          {view === "transfer" && <Transfer />}
          {view === "history" && <History />}
          {view === "certificate" && <Certificate />}
        </main>
      </div>
    </div>
  )
}
