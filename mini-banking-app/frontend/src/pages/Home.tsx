import { useEffect, useState } from "react"
import { useNavigate } from "react-router"
import { ArrowRight, BarChart3, CheckCircle2, Database, FileText, KeyRound, Lock, LogOut, RefreshCw, Send, Shield, X, XCircle } from "lucide-react"
import { getStoredCertificate, getStoredClientProfile, type StoredCertificate } from "../services/pki-registration"
import { certificatePemToJson, getSession, hasValidTgt, clearSession, type AsSession, type ParsedCertificateJson } from "../services/as-exchange"
import { clearServiceTickets } from "../services/tgs-exchange"
import { performTransfer } from "../services/bank/transfer"
import { fetchProfile, type Profile } from "../services/bank/profile"
import { fetchHistory, type HistoryItem } from "../services/bank/history"
import { getUserErrorMessage } from "../services/user-error-message"

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

function SecurityStatus({ session, certificateExpiresAt }: { session: AsSession; certificateExpiresAt: string }) {
  const tgtExpires = new Date(session.tgtExpiresAt * 1000)
  const certificateExpires = new Date(certificateExpiresAt)
  const items = [
    { label: "Ticket Grant Ticket expired at", expires: tgtExpires },
    { label: "X509 certificate expired at", expires: certificateExpires },
  ]

  return (
    <div className="bg-card border border-border rounded-xl p-5">
      <h2 className="text-sm font-semibold text-foreground mb-3 flex items-center gap-2">
        <Shield className="w-4 h-4 text-cyan-400" />
        Trạng thái phiên bảo mật
      </h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {items.map(item => {
          const valid = !Number.isNaN(item.expires.getTime())
          return (
            <div key={item.label} className="bg-background border border-border rounded-lg p-3">
              <p className="text-xs text-muted-foreground mb-1.5">{item.label}</p>
              <div className="flex items-center gap-1.5">
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                {valid ? (
                  <time dateTime={item.expires.toISOString()} className="text-xs font-mono text-foreground">
                    {item.expires.toLocaleString("vi-VN")}
                  </time>
                ) : (
                  <span className="text-xs text-muted-foreground">Chưa có thông tin</span>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

type TransferModal = "confirm" | "pin" | "processing" | "success" | "error" | null

const formatAmountInput = (digits: string) => digits.replace(/\B(?=(\d{3})+(?!\d))/g, ".")

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

function TransferForm({ fromAccountNumber, onSuccess }: { fromAccountNumber: string; onSuccess?: () => void }) {
  const [toAccountNumber, setToAccountNumber] = useState("")
  const [amount, setAmount] = useState("")
  const [description, setDescription] = useState("")
  const [pin, setPin] = useState("")
  const [modal, setModal] = useState<TransferModal>(null)
  const [resultMessage, setResultMessage] = useState("")

  const canContinue =
    /^\d+$/.test(fromAccountNumber.trim()) &&
    /^\d+$/.test(toAccountNumber.trim()) &&
    fromAccountNumber.trim() !== toAccountNumber.trim() &&
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
      setToAccountNumber("")
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
        fromAccountNumber: fromAccountNumber.trim(),
        toAccountNumber: toAccountNumber.trim(),
        amount: amountNumber,
        description: description.trim() || undefined,
        pin: confirmedPin,
      })
      setResultMessage(`Mã giao dịch: ${r.transactionId}`)
      setModal("success")
      setPin("")
      onSuccess?.() // làm mới số dư/profile ở tab Tổng quan
    } catch (err) {
      setResultMessage(getUserErrorMessage(err, "Không thể thực hiện giao dịch. Vui lòng thử lại."))
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
            <label className="block text-xs text-muted-foreground">Số tài khoản nguồn</label>
            <input
              className={`${inputCls} font-mono opacity-70 cursor-not-allowed`}
              value={fromAccountNumber || "Đang tải…"}
              readOnly
              title="Tài khoản của bạn (tự điền từ trang Tổng quan)"
            />
          </div>
          <div>
            <label className="block text-xs text-muted-foreground">Số tài khoản nhận</label>
            <input className={`${inputCls} font-mono`} value={toAccountNumber} onChange={e => setToAccountNumber(e.target.value.replace(/[^\d]/g, ""))} inputMode="numeric" placeholder="Nhập số tài khoản nhận" />
          </div>
          <div>
            <label className="block text-xs text-muted-foreground">Số tiền (VND)</label>
            <input className={`${inputCls} font-mono`} value={formatAmountInput(amount)} onChange={e => setAmount(e.target.value.replace(/[^\d]/g, ""))} inputMode="numeric" placeholder="Nhập số tiền" />
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
                  { label: "Số tài khoản nguồn", value: fromAccountNumber.trim(), mono: true },
                  { label: "Số tài khoản nhận", value: toAccountNumber.trim(), mono: true },
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

const ACCOUNT_STATUS_LABEL: Record<string, { label: string; cls: string }> = {
  active: { label: "Hoạt động", cls: "bg-emerald-500/10 text-emerald-400" },
  locked: { label: "Đã khóa", cls: "bg-amber-500/10 text-amber-400" },
  frozen: { label: "Đóng băng", cls: "bg-red-500/10 text-red-400" },
}

const fmtMoney = (amount: number, currency: string) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)} ${currency}`

function ProfileOverview({ session, certificateExpiresAt, fallbackName, profile, loading, error, onReload }: {
  session: AsSession
  certificateExpiresAt: string
  fallbackName: string
  profile: Profile | null
  loading: boolean
  error: string
  onReload: () => void
}) {
  const statusInfo = profile ? (ACCOUNT_STATUS_LABEL[profile.accountStatus] ?? { label: profile.accountStatus, cls: "bg-muted text-muted-foreground" }) : null

  if (loading) {
    return (
      <div className="w-full bg-card border border-border rounded-xl py-16 text-center">
        <RefreshCw className="w-7 h-7 text-blue-400 animate-spin mx-auto mb-3" />
        <p className="text-sm text-muted-foreground">Đang tải thông tin tài khoản…</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="w-full bg-card border border-border rounded-xl py-12 text-center">
        <XCircle className="w-8 h-8 text-red-400 mx-auto mb-3" />
        <p className="text-sm text-red-400 mb-4">{error}</p>
        <button type="button" onClick={onReload} className="bg-blue-600 hover:bg-blue-500 text-white rounded-lg px-4 py-2 text-sm font-semibold transition-colors">Thử lại</button>
      </div>
    )
  }

  if (!profile) return null

  const displayName = profile.fullName || fallbackName || "—"
  const initial = displayName.trim().charAt(0).toLocaleUpperCase("vi-VN")

  return (
    <div className="w-full space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="bg-card border border-border rounded-xl p-5">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-4 min-w-0">
              <div className="w-14 h-14 rounded-2xl bg-blue-600/20 border border-blue-500/30 flex items-center justify-center shrink-0">
                <span className="text-2xl font-bold text-blue-400">{initial}</span>
              </div>
              <div className="min-w-0">
                <p className="text-2xl lg:text-3xl font-bold text-foreground truncate">{displayName}</p>
                <p className="text-sm text-muted-foreground mt-1 truncate">{profile.email || "—"}</p>
              </div>
            </div>
            <button
              type="button"
              onClick={onReload}
              className="shrink-0 p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
              title="Làm mới"
              aria-label="Làm mới thông tin tài khoản"
            >
              <RefreshCw className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>

        <div className="bg-gradient-to-br from-blue-600/20 via-blue-600/10 to-transparent border border-blue-500/20 rounded-xl p-5">
          <div className="flex items-center justify-between gap-3 mb-4">
            <span className="text-xs text-blue-300/60 font-mono uppercase tracking-wider">Tài khoản chính</span>
            <span className={`text-xs px-2.5 py-1 rounded-full ${statusInfo?.cls ?? "bg-muted text-muted-foreground"}`}>
              {statusInfo?.label ?? "—"}
            </span>
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground mb-1">Số tài khoản</p>
              <p className="text-2xl lg:text-3xl font-bold text-white font-mono whitespace-nowrap overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                {profile.accountNumber}
              </p>
            </div>
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground mb-1">Số dư</p>
              <p className="text-2xl lg:text-3xl font-bold text-white font-mono whitespace-nowrap overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                {fmtMoney(profile.balance, profile.currency)}
              </p>
            </div>
          </div>

          <div className="mt-4 pt-3 border-t border-blue-500/15">
            <p className="text-xs text-muted-foreground mb-1">Hạn mức ngày</p>
            <p className="text-sm font-mono text-foreground">{fmtMoney(profile.dailyTransferLimit, profile.currency)}</p>
          </div>
        </div>
      </div>
      <SecurityStatus session={session} certificateExpiresAt={certificateExpiresAt} />
    </div>
  )
}

function CertificateView({ certificate, loading }: { certificate: StoredCertificate | null; loading: boolean }) {
  const [parsed, setParsed] = useState<ParsedCertificateJson | null>(null)
  const [parseError, setParseError] = useState("")
  const [parsing, setParsing] = useState(false)

  useEffect(() => {
    let active = true
    if (!certificate) {
      setParsed(null)
      setParseError("")
      setParsing(false)
      return () => { active = false }
    }

    setParsing(true)
    setParseError("")
    certificatePemToJson(certificate.certificatePem)
      .then(value => { if (active) setParsed(value) })
      .catch(() => {
        if (!active) return
        setParsed(null)
        setParseError("Không thể phân tích chứng chỉ X.509 được lưu trên thiết bị")
      })
      .finally(() => { if (active) setParsing(false) })
    return () => { active = false }
  }, [certificate])

  if (loading || parsing) {
    return (
      <div className="w-full bg-card border border-border rounded-xl py-16 text-center">
        <RefreshCw className="w-7 h-7 text-cyan-400 animate-spin mx-auto mb-3" />
        <p className="text-sm text-muted-foreground">Đang phân tích chứng chỉ X.509…</p>
      </div>
    )
  }

  if (!certificate) {
    return (
      <div className="w-full bg-card border border-border rounded-xl p-8 text-center">
        <Shield className="w-10 h-10 text-muted-foreground/40 mx-auto mb-3" />
        <h1 className="text-base font-semibold text-foreground">Không tìm thấy chứng chỉ X.509</h1>
        <p className="text-sm text-muted-foreground mt-2">Thiết bị này chưa có chứng chỉ được lưu trong IndexedDB.</p>
      </div>
    )
  }

  if (parseError || !parsed) {
    return (
      <div className="w-full bg-card border border-border rounded-xl p-8 text-center">
        <XCircle className="w-9 h-9 text-red-400 mx-auto mb-3" />
        <h1 className="text-base font-semibold text-foreground">Không đọc được chứng chỉ X.509</h1>
        <p className="text-sm text-red-400 mt-2">{parseError}</p>
      </div>
    )
  }

  const notBefore = new Date(parsed.validity.notBefore)
  const notAfter = new Date(parsed.validity.notAfter)
  const expired = notAfter.getTime() <= Date.now()
  const subjectEmail = parsed.subjectAltName.emails[0] || parsed.subject.emailAddress || "—"
  const formatName = (name: ParsedCertificateJson["subject"]) =>
    [name.commonName, name.organization, name.country].filter(Boolean).join(", ") || "—"

  const fields = [
    { label: "Serial Number", value: parsed.serialNumber, mono: true },
    { label: "Owner ID", value: parsed.ownerId || "—", mono: true },
    { label: "Subject CN", value: parsed.subject.commonName || "—", mono: false },
    { label: "Subject Email", value: subjectEmail, mono: false },
    { label: "Issuer", value: formatName(parsed.issuer), mono: false },
    { label: "X.509 Version", value: `v${parsed.version}`, mono: true },
    { label: "Not Before", value: notBefore.toLocaleString("vi-VN"), mono: true },
    { label: "Not After", value: notAfter.toLocaleString("vi-VN"), mono: true },
    { label: "Signature Algorithm", value: parsed.signatureAlgorithm.name, mono: true },
    { label: "Public Key Algorithm", value: parsed.publicKeyAlgorithm.name, mono: true },
  ]

  return (
    <div className="w-full space-y-4">
      <div className="bg-card border border-border rounded-xl p-5">
        <div className="flex items-start justify-between gap-4 mb-5">
          <div className="flex items-center gap-3 min-w-0">
            <div className="p-2.5 bg-cyan-500/10 rounded-xl shrink-0">
              <Shield className="w-5 h-5 text-cyan-400" />
            </div>
            <div className="min-w-0">
              <h1 className="text-base font-semibold text-foreground">{parsed.subject.commonName || "Chứng chỉ X.509"}</h1>
              <p className="text-xs text-muted-foreground mt-0.5">{subjectEmail}</p>
            </div>
          </div>
          <span className={`shrink-0 text-xs px-2.5 py-1 rounded-full border ${
            expired
              ? "bg-red-500/10 text-red-400 border-red-500/20"
              : "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
          }`}>
            {expired ? "Hết hạn" : "Còn hiệu lực"}
          </span>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {fields.map(field => (
            <div key={field.label} className="bg-background border border-border rounded-lg p-3">
              <p className="text-xs text-muted-foreground mb-1">{field.label}</p>
              <p className={`text-sm text-foreground break-all ${field.mono ? "font-mono" : ""}`}>
                {field.value}
              </p>
            </div>
          ))}
        </div>

        <div className="mt-3 bg-background border border-border rounded-lg p-3">
          <p className="text-xs text-muted-foreground mb-1">SHA-256 Fingerprint</p>
          <p className="text-xs font-mono text-foreground break-all leading-relaxed">
            {parsed.fingerprintSha256}
          </p>
        </div>

        {parsed.subjectAltName.uris.length > 0 && (
          <div className="mt-3 bg-background border border-border rounded-lg p-3">
            <p className="text-xs text-muted-foreground mb-1">Subject Alternative Name · URI</p>
            <p className="text-xs font-mono text-foreground break-all leading-relaxed">
              {parsed.subjectAltName.uris.join("\n")}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}

const TXN_STATUS_LABEL: Record<string, { label: string; cls: string }> = {
  completed: { label: "Hoàn tất", cls: "bg-emerald-500/10 text-emerald-400" },
  pending: { label: "Đang xử lý", cls: "bg-amber-500/10 text-amber-400" },
  failed: { label: "Thất bại", cls: "bg-red-500/10 text-red-400" },
}

function HistoryView({ accountId, ownAccountNumber, profileLoading }: {
  accountId: string
  ownAccountNumber: string
  profileLoading: boolean
}) {
  const [items, setItems] = useState<HistoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!accountId) return
    let active = true
    setLoading(true)
    setError("")
    // TGS (history:read) → AP read /transactions/query → giải mã ap_rep bằng K(c,v).
    fetchHistory({ accountId })
      .then(r => { if (active) { setItems(r.items); setTotal(r.total) } })
      .catch(err => { if (active) setError(getUserErrorMessage(err, "Không tải được lịch sử giao dịch")) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [accountId, reloadKey])

  if (!accountId) {
    return (
      <div className="w-full bg-card border border-border rounded-xl py-16 text-center">
        <RefreshCw className={`w-6 h-6 text-blue-400 mx-auto mb-3 ${profileLoading ? "animate-spin" : ""}`} />
        <p className="text-sm text-muted-foreground">
          {profileLoading ? "Đang tải thông tin tài khoản…" : "Chưa xác định được tài khoản để xem lịch sử"}
        </p>
      </div>
    )
  }

  return (
    <div className="w-full">
      <div className="bg-card border border-border rounded-xl">
        <div className="flex items-center justify-between gap-3 p-5 border-b border-border">
          <div className="flex items-center gap-2">
            <FileText className="w-4 h-4 text-blue-400" />
            <h1 className="text-base font-semibold text-foreground">Lịch sử giao dịch</h1>
            {!loading && !error && <span className="text-xs text-muted-foreground">({total})</span>}
          </div>
          <button
            type="button"
            onClick={() => setReloadKey(k => k + 1)}
            disabled={loading}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors disabled:opacity-40"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} /> Làm mới
          </button>
        </div>

        {loading ? (
          <div className="py-16 text-center">
            <RefreshCw className="w-6 h-6 text-blue-400 animate-spin mx-auto mb-3" />
            <p className="text-sm text-muted-foreground">Đang tải lịch sử giao dịch…</p>
          </div>
        ) : error ? (
          <div className="py-12 text-center">
            <XCircle className="w-8 h-8 text-red-400 mx-auto mb-3" />
            <p className="text-sm text-red-400 mb-4">{error}</p>
            <button type="button" onClick={() => setReloadKey(k => k + 1)} className="bg-blue-600 hover:bg-blue-500 text-white rounded-lg px-4 py-2 text-sm font-semibold transition-colors">Thử lại</button>
          </div>
        ) : items.length === 0 ? (
          <div className="py-16 text-center">
            <Database className="w-8 h-8 text-muted-foreground/40 mx-auto mb-3" />
            <p className="text-sm text-muted-foreground">Chưa có giao dịch nào</p>
          </div>
        ) : (
          <ul className="divide-y divide-border">
            {items.map(item => {
              const outgoing = item.fromAccountNumber === ownAccountNumber
              const counterparty = (outgoing ? item.toAccountNumber : item.fromAccountNumber) || "—"
              const status = TXN_STATUS_LABEL[item.status] ?? { label: item.status, cls: "bg-muted text-muted-foreground" }
              const created = new Date(item.createdAtUnix * 1000)
              return (
                <li key={item.transactionId} className="flex items-center gap-3 px-5 py-3.5">
                  <div className={`w-9 h-9 rounded-full flex items-center justify-center shrink-0 ${outgoing ? "bg-red-500/10" : "bg-emerald-500/10"}`}>
                    <ArrowRight className={`w-4 h-4 ${outgoing ? "text-red-400 rotate-45" : "text-emerald-400 rotate-[225deg]"}`} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm text-foreground truncate">
                      {outgoing ? "Chuyển tới" : "Nhận từ"} <span className="font-mono">{counterparty}</span>
                    </p>
                    <p className="text-xs text-muted-foreground truncate">
                      {item.description?.trim() || "—"} · {created.toLocaleString("vi-VN")}
                    </p>
                  </div>
                  <div className="text-right shrink-0">
                    <p className={`text-sm font-semibold font-mono ${outgoing ? "text-red-400" : "text-emerald-400"}`}>
                      {outgoing ? "-" : "+"}{fmtMoney(item.amount, item.currency)}
                    </p>
                    <span className={`inline-block mt-0.5 text-[10px] px-2 py-0.5 rounded-full ${status.cls}`}>{status.label}</span>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}

function EmptyView({ view, fullName }: { view: View; fullName: string }) {
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
      </div>
    </div>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [view, setView] = useState<View>("overview")
  const [fullName, setFullName] = useState("")
  const [storedCertificate, setStoredCertificate] = useState<StoredCertificate | null>(null)
  const [localDataLoading, setLocalDataLoading] = useState(true)
  const [session, setSession] = useState<AsSession | null>(null)

  // Profile (user + tài khoản) fetch 1 lần khi vào Home, là nguồn dữ liệu chung cho
  // tab Tổng quan và để tự điền STK nguồn ở form Chuyển khoản (không cần global store).
  const [profile, setProfile] = useState<Profile | null>(null)
  const [profileLoading, setProfileLoading] = useState(true)
  const [profileError, setProfileError] = useState("")
  const [profileReloadKey, setProfileReloadKey] = useState(0)

  useEffect(() => {
    // Guard: không có TGT hợp lệ (chưa AS Exchange / TGT hết hạn / vừa reload) → quay lại đăng nhập.
    if (!hasValidTgt()) {
      navigate("/login", { replace: true })
      return
    }
    setSession(getSession())

    let active = true
    Promise.all([getStoredClientProfile(), getStoredCertificate()]).then(([clientProfile, certificate]) => {
      if (!active) return
      setFullName(clientProfile?.fullName ?? "")
      setStoredCertificate(certificate ?? null)
    }).catch(() => undefined)
      .finally(() => { if (active) setLocalDataLoading(false) })
    return () => { active = false }
  }, [navigate])

  useEffect(() => {
    if (!session) return
    let active = true
    setProfileLoading(true)
    setProfileError("")
    // TGS (balance:read) → AP read /auth/me → giải mã ap_rep bằng K(c,v).
    fetchProfile()
      .then(p => { if (active) setProfile(p) })
      .catch(err => { if (active) setProfileError(getUserErrorMessage(err, "Không tải được thông tin tài khoản")) })
      .finally(() => { if (active) setProfileLoading(false) })
    return () => { active = false }
  }, [session, profileReloadKey])

  const reloadProfile = () => setProfileReloadKey(k => k + 1)

  const handleLogout = () => {
    clearServiceTickets() // zero K_{c,v} + xóa Ticket_v khỏi RAM
    clearSession() // zero K_{c,tgs} + xóa TGT khỏi RAM
    navigate("/login", { replace: true })
  }

  // Trong lúc guard điều hướng, tránh nháy nội dung khi chưa có phiên.
  if (!session) return null

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
          <div className="mt-auto pt-3 border-t border-border">
            <button
              type="button"
              onClick={handleLogout}
              className="w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-left text-muted-foreground hover:text-red-400 hover:bg-red-500/10 transition-colors"
            >
              <LogOut className="w-4 h-4 shrink-0" />
              <span className="text-xs">Đăng xuất</span>
            </button>
          </div>
        </aside>
        <main className="flex-1 overflow-y-auto p-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {view === "overview"
            ? <ProfileOverview session={session} certificateExpiresAt={storedCertificate?.notAfter ?? ""} fallbackName={fullName} profile={profile} loading={profileLoading} error={profileError} onReload={reloadProfile} />
            : view === "transfer"
              ? <TransferForm fromAccountNumber={profile?.accountNumber ?? ""} onSuccess={reloadProfile} />
              : view === "certificate"
                ? <CertificateView certificate={storedCertificate} loading={localDataLoading} />
                : view === "history"
                  ? <HistoryView accountId={profile?.accountId ?? ""} ownAccountNumber={profile?.accountNumber ?? ""} profileLoading={profileLoading} />
                  : <EmptyView view={view} fullName={fullName} />}
        </main>
      </div>
    </div>
  )
}
