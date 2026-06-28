import { useState } from "react"
import { Link, useNavigate } from "react-router"
import { Lock, XCircle } from "lucide-react"

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

function PinKeypad({ onKey }: { onKey: (key: string) => void }) {
  const keys = ["1","2","3","4","5","6","7","8","9","","0","del"]
  return (
    <div className="grid grid-cols-3 gap-2.5">
      {keys.map((k, i) => {
        if (k === "") return <div key={i} />
        return (
          <button
            key={i}
            onClick={() => onKey(k)}
            className="h-14 text-lg font-semibold text-foreground rounded-xl bg-background hover:bg-secondary border border-border transition-all active:scale-95 flex items-center justify-center select-none"
          >
            {k === "del" ? <span className="text-base text-muted-foreground">⌫</span> : k}
          </button>
        )
      })}
    </div>
  )
}

export default function Login() {
  const navigate = useNavigate()
  const [pin, setPin] = useState("")
  const [error, setError] = useState(false)
  const [shake, setShake] = useState(false)

  const handleKey = (key: string) => {
    if (error) { setError(false); setShake(false) }
    if (key === "del") { setPin(p => p.slice(0, -1)); return }
    if (pin.length >= 6) return

    const next = pin + key
    setPin(next)

    if (next.length === 6) {
      // demo: any 6-digit PIN works
      setTimeout(() => navigate("/home"), 200)
    }
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4" style={BG_STYLE}>
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <div className="w-12 h-12 bg-blue-600 rounded-2xl flex items-center justify-center mx-auto mb-3 shadow-lg shadow-blue-600/20">
            <Lock className="w-6 h-6 text-white" />
          </div>
          <p className="text-xs font-mono text-muted-foreground tracking-widest uppercase">Mini Banking System</p>
        </div>

        <div className={`bg-card border border-border rounded-2xl p-7 shadow-xl shadow-black/40 ${shake ? "animate-bounce" : ""}`}>
          {/* User avatar */}
          <div className="flex flex-col items-center mb-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-blue-600 to-blue-800 flex items-center justify-center mb-3 shadow-lg shadow-blue-600/20">
              <span className="text-2xl font-bold text-white">A</span>
            </div>
            <p className="text-base font-semibold text-foreground">Xin chào, Alice Nguyen</p>
            <p className="text-xs text-muted-foreground mt-0.5">alice@minibank.vn</p>
          </div>

          <p className="text-center text-sm text-muted-foreground mb-0">Nhập mã PIN để đăng nhập</p>

          <PinDots filled={pin.length} error={error} />

          {error && (
            <div className="flex items-center justify-center gap-1.5 mb-3 -mt-2">
              <XCircle className="w-3.5 h-3.5 text-red-400" />
              <p className="text-xs text-red-400">Mã PIN không chính xác</p>
            </div>
          )}

          <PinKeypad onKey={handleKey} />

          <div className="mt-5 pt-4 border-t border-border text-center">
            <p className="text-xs text-muted-foreground">
              Chưa có tài khoản?{" "}
              <Link to="/register" className="text-blue-400 hover:text-blue-300 transition-colors">Đăng ký</Link>
            </p>
          </div>

          <div className="mt-3 bg-background border border-border rounded-lg p-2.5">
            <p className="text-xs text-muted-foreground/50 font-mono text-center">Demo: nhập bất kỳ 6 chữ số để đăng nhập</p>
          </div>
        </div>
      </div>
    </div>
  )
}
