import { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router"
import { KeyRound, Lock, RefreshCw, UserRound, XCircle } from "lucide-react"
import {
  getStoredClientProfile,
  isEnrolled,
} from "../services/pki-registration"
import { performAsExchange } from "../services/as-exchange"
import { ApiError } from "../services/api.service"

const BG_STYLE = {
  background: "radial-gradient(ellipse 80% 60% at 50% -10%, rgba(59,130,246,0.12) 0%, transparent 70%)",
}

function PinDots({ filled, error }: { filled: number; error: boolean }) {
  return (
    <div className="flex items-center justify-center gap-3 my-6">
      {Array(6).fill(null).map((_, i) => (
        <div key={i} className={`w-3.5 h-3.5 rounded-full border-2 transition-all duration-150 ${
          error
            ? "bg-red-500 border-red-500"
            : i < filled
              ? "bg-blue-500 border-blue-500 scale-110"
              : "border-muted-foreground/25"
        }`} />
      ))}
    </div>
  )
}

function PinKeypad({ onKey, disabled }: { onKey: (key: string) => void; disabled: boolean }) {
  const keys = ["1","2","3","4","5","6","7","8","9","","0","del"]
  return (
    <div className="grid grid-cols-3 gap-2.5">
      {keys.map((key, index) => {
        if (key === "") return <div key={index} />
        return (
          <button
            key={index}
            type="button"
            disabled={disabled}
            onClick={() => onKey(key)}
            className="h-14 text-lg font-semibold text-foreground rounded-xl bg-background hover:bg-secondary border border-border transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center select-none"
          >
            {key === "del" ? <span className="text-base text-muted-foreground">⌫</span> : key}
          </button>
        )
      })}
    </div>
  )
}

export default function Login() {
  const navigate = useNavigate()
  const [fullName, setFullName] = useState("")
  const [ready, setReady] = useState(false)
  const [enrolled, setEnrolled] = useState(false)
  const [pin, setPin] = useState("")
  const [verifying, setVerifying] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    Promise.all([getStoredClientProfile(), isEnrolled()])
      .then(([profile, hasEnrollment]) => {
        if (!active) return
        setFullName(profile?.fullName ?? "")
        setEnrolled(hasEnrollment)
      })
      .catch(() => {
        if (active) setError("Không thể đọc dữ liệu đăng ký trên thiết bị này")
      })
      .finally(() => {
        if (active) setReady(true)
      })
    return () => { active = false }
  }, [])

  const verifyPin = async (candidate: string) => {
    setVerifying(true)
    setError("")
    try {
      // PIN đúng → unwrap private key, ký AS_REQ và lấy TGT + K_{c,tgs} (lưu trong RAM).
      // PIN sai sẽ khiến bước unwrap thất bại và ném lỗi (không phải ApiError).
      await performAsExchange(candidate)
      navigate("/home")
    } catch (err) {
      // Lỗi từ KDC/gateway (cert bị thu hồi, replay, mạng…) trả thông điệp cụ thể;
      // còn lại coi như PIN sai.
      setError(err instanceof ApiError ? err.message : "Mã PIN không chính xác")
      setPin("")
    } finally {
      setVerifying(false)
    }
  }

  const handleKey = (key: string) => {
    if (!ready || !enrolled || verifying) return
    if (error) setError("")
    if (key === "del") {
      setPin(current => current.slice(0, -1))
      return
    }
    if (pin.length >= 6) return

    const next = pin + key
    setPin(next)
    if (next.length === 6) void verifyPin(next)
  }

  const initial = fullName.trim().charAt(0).toLocaleUpperCase("vi-VN")

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
          {!ready ? (
            <div className="py-10 text-center">
              <RefreshCw className="w-7 h-7 text-blue-400 animate-spin mx-auto mb-3" />
              <p className="text-sm text-muted-foreground">Đang đọc thông tin đăng ký…</p>
            </div>
          ) : !enrolled ? (
            <div className="py-6 text-center">
              <div className="w-14 h-14 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center mx-auto mb-4">
                <KeyRound className="w-7 h-7 text-amber-400" />
              </div>
              <h1 className="text-base font-semibold text-foreground">Thiết bị chưa được đăng ký</h1>
              <p className="text-sm text-muted-foreground mt-2 mb-5">Không tìm thấy khóa và chứng chỉ cục bộ để xác minh mã PIN.</p>
              <Link to="/register" className="inline-flex bg-blue-600 hover:bg-blue-500 text-white rounded-xl px-5 py-2.5 text-sm font-semibold transition-colors">
                Đăng ký tài khoản
              </Link>
            </div>
          ) : (
            <>
              <div className="flex flex-col items-center mb-4 text-center">
                <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-blue-600 to-blue-800 flex items-center justify-center mb-3 shadow-lg shadow-blue-600/20">
                  {initial
                    ? <span className="text-2xl font-bold text-white">{initial}</span>
                    : <UserRound className="w-7 h-7 text-white" />
                  }
                </div>
                <p className="text-base font-semibold text-foreground">
                  {fullName ? `Xin chào, ${fullName}` : "Xin chào"}
                </p>
                <p className="text-sm text-muted-foreground mt-1">Nhập mã PIN để đăng nhập</p>
              </div>

              <PinDots filled={pin.length} error={Boolean(error)} />

              {verifying && (
                <p className="text-center text-xs text-muted-foreground flex items-center justify-center gap-1.5 mb-3 -mt-2">
                  <RefreshCw className="w-3.5 h-3.5 animate-spin" /> Đang xác minh…
                </p>
              )}

              {error && !verifying && (
                <div className="flex items-center justify-center gap-1.5 mb-3 -mt-2">
                  <XCircle className="w-3.5 h-3.5 text-red-400" />
                  <p className="text-xs text-red-400">{error}</p>
                </div>
              )}

              <PinKeypad onKey={handleKey} disabled={verifying} />
            </>
          )}

          <div className="mt-5 pt-4 border-t border-border text-center">
            <p className="text-xs text-muted-foreground">
              Chưa có tài khoản?{" "}
              <Link to="/register" className="text-blue-400 hover:text-blue-300 transition-colors">Đăng ký</Link>
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
