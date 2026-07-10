import { useState, useRef, useEffect, FormEvent } from "react"
import { Link, useNavigate } from "react-router"
import { Lock, Shield, Key, CheckCircle, ArrowLeft, RefreshCw, Mail, UserRound, AlertTriangle } from "lucide-react"
import { requestOtp, verifyOtp, enrollAndRegister } from "../services/pki-registration"
import { newOperationId } from "../services/operation-id"
import { getUserErrorMessage } from "../services/user-error-message"

// Lấy message thân thiện từ lỗi bất kỳ
const errMessage = (e: unknown) =>
  getUserErrorMessage(e, "Có lỗi xảy ra, vui lòng thử lại")

// ─── Auth background ──────────────────────────────────────────────────────────
const BG_STYLE = {
  background: "radial-gradient(ellipse 80% 60% at 50% -10%, rgba(59,130,246,0.12) 0%, transparent 70%)",
}

function AuthShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4" style={BG_STYLE}>
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <div className="w-12 h-12 bg-blue-600 rounded-2xl flex items-center justify-center mx-auto mb-3 shadow-lg shadow-blue-600/20">
            <Lock className="w-6 h-6 text-white" />
          </div>
          <p className="text-xs font-mono text-muted-foreground tracking-widest uppercase">Mini Banking System</p>
        </div>
        <div className="bg-card border border-border rounded-2xl p-7 shadow-xl shadow-black/40">
          {children}
        </div>
      </div>
    </div>
  )
}

// ─── Step indicators ──────────────────────────────────────────────────────────
function StepDots({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center justify-center gap-2 mb-6">
      {Array(total).fill(null).map((_, i) => (
        <div key={i} className={`rounded-full transition-all duration-300 ${
          i + 1 === current
            ? "w-6 h-1.5 bg-blue-500"
            : i + 1 < current
              ? "w-1.5 h-1.5 bg-blue-500/40"
              : "w-1.5 h-1.5 bg-muted-foreground/20"
        }`} />
      ))}
    </div>
  )
}

// ─── PIN dot display ──────────────────────────────────────────────────────────
function PinDots({ filled }: { filled: number }) {
  return (
    <div className="flex items-center justify-center gap-3 my-6">
      {Array(6).fill(null).map((_, i) => (
        <div key={i} className={`w-3.5 h-3.5 rounded-full border-2 transition-all duration-150 ${
          i < filled
            ? "bg-blue-500 border-blue-500 scale-110"
            : "border-muted-foreground/25"
        }`} />
      ))}
    </div>
  )
}

// ─── PIN keypad ───────────────────────────────────────────────────────────────
function PinKeypad({ onKey, disabled }: { onKey: (key: string) => void; disabled?: boolean }) {
  const keys = ["1","2","3","4","5","6","7","8","9","","0","del"]
  return (
    <div className="grid grid-cols-3 gap-2.5 mt-4">
      {keys.map((k, i) => {
        if (k === "") return <div key={i} />
        return (
          <button
            key={i}
            disabled={disabled}
            onClick={() => onKey(k)}
            className="h-14 text-lg font-semibold text-foreground rounded-xl bg-background hover:bg-secondary border border-border transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center select-none"
          >
            {k === "del" ? <span className="text-base text-muted-foreground">⌫</span> : k}
          </button>
        )
      })}
    </div>
  )
}

// ─── Inline error banner ──────────────────────────────────────────────────────
function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4">
      <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
      <p className="text-xs text-red-400">{message}</p>
    </div>
  )
}

// ─── Step 1: Email ────────────────────────────────────────────────────────────
function StepEmail({ operationId, onNext }: { operationId: string; onNext: (email: string) => void }) {
  const [email, setEmail] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!email.includes("@")) return
    setLoading(true)
    setError("")
    try {
      await requestOtp(email, operationId)
      onNext(email)
    } catch (err) {
      setError(errMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      <div className="text-center mb-6">
        <div className="w-10 h-10 bg-blue-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
          <Mail className="w-5 h-5 text-blue-400" />
        </div>
        <h1 className="text-lg font-semibold text-foreground">Đăng ký tài khoản</h1>
        <p className="text-xs text-muted-foreground mt-1">Nhập email để nhận mã OTP xác thực</p>
      </div>
      {error && <ErrorBanner message={error} />}
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">Email</label>
          <input
            type="email"
            value={email}
            onChange={e => { setEmail(e.target.value); setError("") }}
            placeholder="ten@example.com"
            autoFocus
            className="w-full bg-background border border-border rounded-xl px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground/30 focus:border-blue-500 focus:outline-none transition-colors"
          />
        </div>
        <button
          type="submit"
          disabled={!email.includes("@") || loading}
          className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-xl py-3 text-sm font-semibold transition-colors flex items-center justify-center gap-2"
        >
          {loading
            ? <RefreshCw className="w-4 h-4 animate-spin" />
            : "Gửi mã OTP"
          }
        </button>
      </form>
      <p className="text-center text-xs text-muted-foreground mt-5">
        Đã có tài khoản?{" "}
        <Link to="/login" className="text-blue-400 hover:text-blue-300 transition-colors">Đăng nhập</Link>
      </p>
    </>
  )
}

// ─── Step 2: OTP ──────────────────────────────────────────────────────────────
function StepOTP({
  email,
  operationId,
  onNext,
  onBack,
}: {
  email: string
  operationId: string
  onNext: (regToken: string) => void
  onBack: () => void
}) {
  const [digits, setDigits] = useState(Array(6).fill(""))
  const [error, setError] = useState("")
  const [verifying, setVerifying] = useState(false)
  const [countdown, setCountdown] = useState(300)
  const refs = useRef<(HTMLInputElement | null)[]>([])

  useEffect(() => {
    const timer = setInterval(() => setCountdown(c => c > 0 ? c - 1 : 0), 1000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    refs.current[0]?.focus()
  }, [])

  const fmt = (s: number) => `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`

  const resetDigits = () => {
    setDigits(Array(6).fill(""))
    refs.current[0]?.focus()
  }

  const verify = async (code: string) => {
    setVerifying(true)
    setError("")
    try {
      const res = await verifyOtp(email, code, operationId)
      onNext(res.reg_token)
    } catch (err) {
      setError(errMessage(err))
      resetDigits()
    } finally {
      setVerifying(false)
    }
  }

  const resend = async () => {
    setError("")
    try {
      await requestOtp(email, operationId)
      setCountdown(300)
      resetDigits()
    } catch (err) {
      setError(errMessage(err))
    }
  }

  const handleInput = (idx: number, val: string) => {
    if (!/^\d*$/.test(val) || verifying) return
    const next = [...digits]
    next[idx] = val.slice(-1)
    setDigits(next)
    setError("")
    if (val && idx < 5) refs.current[idx + 1]?.focus()
    if (idx === 5 && val) {
      const code = next.join("")
      if (code.length === 6) verify(code)
    }
  }

  const handleKeyDown = (idx: number, e: React.KeyboardEvent) => {
    if (e.key === "Backspace" && !digits[idx] && idx > 0) {
      refs.current[idx - 1]?.focus()
    }
    if (e.key === "ArrowLeft" && idx > 0) refs.current[idx - 1]?.focus()
    if (e.key === "ArrowRight" && idx < 5) refs.current[idx + 1]?.focus()
  }

  const handlePaste = (e: React.ClipboardEvent) => {
    const text = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, 6)
    if (!text) return
    e.preventDefault()
    const next = Array(6).fill("")
    text.split("").forEach((c, i) => { next[i] = c })
    setDigits(next)
    refs.current[Math.min(text.length, 5)]?.focus()
    if (text.length === 6) verify(text)
  }

  return (
    <>
      <button onClick={onBack} className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors mb-5">
        <ArrowLeft className="w-3.5 h-3.5" /> Quay lại
      </button>
      <div className="text-center mb-6">
        <div className="w-10 h-10 bg-cyan-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
          <Shield className="w-5 h-5 text-cyan-400" />
        </div>
        <h1 className="text-lg font-semibold text-foreground">Xác thực email</h1>
        <p className="text-xs text-muted-foreground mt-1">
          Mã OTP đã gửi đến{" "}
          <span className="text-foreground font-mono">{email}</span>
        </p>
      </div>

      <div className="flex gap-2 justify-center mb-2" onPaste={handlePaste}>
        {digits.map((d, i) => (
          <input
            key={i}
            ref={el => { refs.current[i] = el }}
            type="text"
            inputMode="numeric"
            maxLength={1}
            value={d}
            disabled={verifying}
            onChange={e => handleInput(i, e.target.value)}
            onKeyDown={e => handleKeyDown(i, e)}
            className={`w-11 h-13 text-center text-xl font-mono bg-background border-2 rounded-xl text-foreground focus:outline-none transition-all disabled:opacity-50 ${
              error
                ? "border-red-500 text-red-400"
                : d ? "border-blue-500" : "border-border focus:border-blue-500"
            }`}
            style={{ height: "52px" }}
          />
        ))}
      </div>

      {verifying && (
        <p className="text-center text-xs text-muted-foreground flex items-center justify-center gap-1.5 mb-2">
          <RefreshCw className="w-3.5 h-3.5 animate-spin" /> Đang xác thực…
        </p>
      )}

      {error && !verifying && (
        <p className="text-center text-xs text-red-400 mb-2">{error}</p>
      )}

      <div className="flex items-center justify-between mt-4 text-xs text-muted-foreground">
        <span className="font-mono">{fmt(countdown)}</span>
        {countdown === 0
          ? <button onClick={resend} className="text-blue-400 hover:text-blue-300 transition-colors">Gửi lại mã</button>
          : <span className="text-muted-foreground/50">Hết hạn sau {fmt(countdown)}</span>
        }
      </div>
    </>
  )
}

// ─── Step 3: Username ─────────────────────────────────────────────────────────
function StepUsername({ onNext, onBack }: { onNext: (fullName: string) => void; onBack: () => void }) {
  const [fullName, setFullName] = useState("")
  const [error, setError] = useState("")

  const validate = (v: string) => {
    if (v.trim().length < 2) return "Họ và tên phải có ít nhất 2 ký tự"
    if (v.trim().split(" ").length < 2) return "Vui lòng nhập đầy đủ họ và tên"
    return ""
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    const err = validate(fullName)
    if (err) { setError(err); return }
    onNext(fullName.trim())
  }

  const words = fullName.trim().split(" ").filter(Boolean)

  return (
    <>
      <button onClick={onBack} className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors mb-5">
        <ArrowLeft className="w-3.5 h-3.5" /> Quay lại
      </button>
      <div className="text-center mb-6">
        <div className="w-10 h-10 bg-blue-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
          <UserRound className="w-5 h-5 text-blue-400" />
        </div>
        <h1 className="text-lg font-semibold text-foreground">Họ và tên</h1>
        <p className="text-xs text-muted-foreground mt-1">Tên đầy đủ của bạn trong hệ thống</p>
      </div>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-xs text-muted-foreground mb-1.5">Họ và tên</label>
          <input
            type="text"
            value={fullName}
            onChange={e => { setFullName(e.target.value); setError("") }}
            placeholder="Nguyễn Văn A"
            autoFocus
            className={`w-full bg-background border-2 rounded-xl px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground/30 focus:outline-none transition-colors ${
              error ? "border-red-500" : fullName.trim() ? "border-blue-500" : "border-border focus:border-blue-500"
            }`}
          />
          {error && <p className="text-xs text-red-400 mt-1.5">{error}</p>}
        </div>
        <div className="bg-background border border-border rounded-lg p-3 space-y-1.5">
          {[
            { rule: "Ít nhất 2 ký tự", ok: fullName.trim().length >= 2 },
            { rule: "Bao gồm cả họ và tên", ok: words.length >= 2 },
          ].map(r => (
            <div key={r.rule} className="flex items-center gap-2">
              <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${r.ok ? "bg-emerald-400" : "bg-muted-foreground/30"}`} />
              <span className={`text-xs ${r.ok ? "text-emerald-400" : "text-muted-foreground/50"}`}>{r.rule}</span>
            </div>
          ))}
        </div>
        <button
          type="submit"
          disabled={words.length < 2}
          className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-xl py-3 text-sm font-semibold transition-colors"
        >
          Tiếp tục
        </button>
      </form>
    </>
  )
}

// ─── Step 4: PIN ──────────────────────────────────────────────────────────────
// Khi xác nhận PIN khớp: sinh key + CSR, wrap private key bằng PIN, gọi gateway
// register rồi lưu certificate và full name vào IndexedDB.
function StepPIN({
  email,
  fullName,
  regToken,
  operationId,
  onDone,
  onRestart,
}: {
  email: string
  fullName: string
  regToken: string
  operationId: string
  onDone: () => void
  onRestart: () => void
}) {
  const [subStep, setSubStep] = useState<"set" | "confirm">("set")
  const [pin, setPin] = useState("")
  const [saved, setSaved] = useState("")
  const [error, setError] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState("")
  const [success, setSuccess] = useState(false)

  // Hoàn tất enrollment với PIN đã xác nhận
  const finish = async (finalPin: string) => {
    setSubmitting(true)
    setApiError("")
    try {
      await enrollAndRegister({ fullName, email, pin: finalPin, regToken, operationId })
      setSuccess(true)
      setTimeout(() => onDone(), 1400)
    } catch (err) {
      setApiError(errMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleKey = (key: string) => {
    if (submitting) return
    if (key === "del") { setPin(p => p.slice(0, -1)); setError(false); return }
    if (pin.length >= 6) return
    const next = pin + key
    setPin(next)
    if (next.length === 6) {
      if (subStep === "set") {
        setTimeout(() => { setSaved(next); setPin(""); setSubStep("confirm") }, 150)
      } else {
        if (next === saved) {
          finish(next)
        } else {
          setError(true)
          setTimeout(() => { setPin(""); setSaved(""); setError(false); setSubStep("set") }, 900)
        }
      }
    }
  }

  if (success) {
    return (
      <div className="text-center py-8">
        <div className="w-14 h-14 bg-emerald-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
          <CheckCircle className="w-7 h-7 text-emerald-400" />
        </div>
        <h2 className="text-base font-semibold text-foreground mb-1">Đăng ký thành công</h2>
        <p className="text-xs text-muted-foreground">Chứng chỉ đã được cấp · Đang chuyển đến trang đăng nhập…</p>
      </div>
    )
  }

  // reg_token dùng 1 lần và đã bị tiêu thụ ở gateway, nên khi lỗi phải đăng ký lại từ đầu
  if (apiError) {
    return (
      <div className="text-center py-6">
        <div className="w-14 h-14 bg-red-500/10 rounded-full flex items-center justify-center mx-auto mb-4">
          <AlertTriangle className="w-7 h-7 text-red-400" />
        </div>
        <h2 className="text-base font-semibold text-foreground mb-1">Cấp chứng chỉ thất bại</h2>
        <p className="text-xs text-muted-foreground mb-5">{apiError}</p>
        <button
          onClick={onRestart}
          className="w-full bg-blue-600 hover:bg-blue-500 text-white rounded-xl py-3 text-sm font-semibold transition-colors"
        >
          Đăng ký lại
        </button>
      </div>
    )
  }

  if (submitting) {
    return (
      <div className="text-center py-10">
        <RefreshCw className="w-8 h-8 text-blue-400 animate-spin mx-auto mb-4" />
        <h2 className="text-base font-semibold text-foreground mb-1">Đang tạo khóa & chứng chỉ</h2>
        <p className="text-xs text-muted-foreground">Sinh cặp khóa RSA, tạo CSR và gửi tới CA…</p>
      </div>
    )
  }

  return (
    <>
      {subStep === "confirm" && (
        <button onClick={() => { setSubStep("set"); setPin(""); setSaved("") }} className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors mb-5">
          <ArrowLeft className="w-3.5 h-3.5" /> Đặt lại
        </button>
      )}
      <div className="text-center mb-2">
        <div className="w-10 h-10 bg-purple-500/10 rounded-xl flex items-center justify-center mx-auto mb-3">
          <Key className="w-5 h-5 text-purple-400" />
        </div>
        <h1 className="text-lg font-semibold text-foreground">
          {subStep === "set" ? "Đặt mã PIN" : "Xác nhận mã PIN"}
        </h1>
        <p className="text-xs text-muted-foreground mt-1">
          {subStep === "set"
            ? "Mã PIN 6 chữ số bảo vệ khóa bí mật của bạn"
            : "Nhập lại mã PIN để xác nhận"
          }
        </p>
      </div>

      <PinDots filled={pin.length} />

      {error && (
        <p className="text-center text-xs text-red-400 -mt-2 mb-2">Mã PIN không khớp. Thử lại.</p>
      )}

      <PinKeypad onKey={handleKey} />

      {subStep === "set" && (
        <p className="text-center text-xs text-muted-foreground/50 mt-4 font-mono">Mã PIN dùng để mã hóa khóa bí mật — không chia sẻ với ai</p>
      )}
    </>
  )
}

// ─── Register page ────────────────────────────────────────────────────────────
export default function Register() {
  const navigate = useNavigate()
  const [step, setStep] = useState<1 | 2 | 3 | 4>(1)
  const [email, setEmail] = useState("")
  const [regToken, setRegToken] = useState("")
  const [fullName, setFullName] = useState("")
  const [operationId, setOperationId] = useState(() => newOperationId())

  // Đưa về bước đầu, xóa state nhạy cảm (reg_token đã tiêu thụ)
  const restart = () => {
    setOperationId(newOperationId())
    setRegToken("")
    setFullName("")
    setStep(1)
  }

  return (
    <AuthShell>
      <StepDots current={step} total={4} />
      {step === 1 && (
        <StepEmail operationId={operationId} onNext={e => { setEmail(e); setStep(2) }} />
      )}
      {step === 2 && (
        <StepOTP
          email={email}
          operationId={operationId}
          onNext={token => { setRegToken(token); setStep(3) }}
          onBack={() => setStep(1)}
        />
      )}
      {step === 3 && (
        <StepUsername onNext={name => { setFullName(name); setStep(4) }} onBack={() => setStep(2)} />
      )}
      {step === 4 && (
        <StepPIN
          email={email}
          fullName={fullName}
          regToken={regToken}
          operationId={operationId}
          onDone={() => navigate("/login")}
          onRestart={restart}
        />
      )}
    </AuthShell>
  )
}
