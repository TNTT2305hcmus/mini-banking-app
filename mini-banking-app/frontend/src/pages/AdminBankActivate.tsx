import { useEffect, useState, type FormEvent } from "react"
import { CheckCircle, KeyRound, Lock, RefreshCw, ShieldCheck } from "lucide-react"
import { Link, useNavigate } from "react-router"
import { activateBankAdmin } from "../services/admin-bank/admin-enrollment.service"
import { getUserErrorMessage } from "../services/user-error-message"

const BG_STYLE = {
  background: "radial-gradient(ellipse 80% 60% at 50% -10%, rgba(6,182,212,0.14) 0%, transparent 70%)",
}

export default function AdminBankActivate() {
  const navigate = useNavigate()
  const [token, setToken] = useState("")
  const [email, setEmail] = useState("")
  const [fullName, setFullName] = useState("")
  const [pin, setPin] = useState("")
  const [confirmPin, setConfirmPin] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    const hash = window.location.hash.startsWith("#")
      ? window.location.hash.slice(1)
      : window.location.hash
    const activationToken = new URLSearchParams(hash).get("token")?.trim() ?? ""

    if (activationToken) {
      setToken(activationToken)
      // Không giữ bearer token trong thanh địa chỉ hoặc browser history sau khi SPA đã đọc.
      window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`)
    }
  }, [])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError("")
    if (!token.trim() || !email.includes("@") || fullName.trim().length < 2) {
      setError("Vui lòng nhập đầy đủ token, email và họ tên đã được cấp")
      return
    }
    if (!/^\d{6}$/.test(pin)) {
      setError("Mã PIN phải gồm đúng 6 chữ số")
      return
    }
    if (pin !== confirmPin) {
      setError("Mã PIN xác nhận không khớp")
      return
    }

    setSubmitting(true)
    try {
      await activateBankAdmin({
        activationToken: token,
        email,
        fullName,
        pin,
      })
      setSuccess(true)
      window.setTimeout(() => navigate("/admin-bank/login", { replace: true }), 1200)
    } catch (err) {
      setError(getUserErrorMessage(err, "Không thể kích hoạt Bank Admin. Vui lòng kiểm tra lại thông tin."))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4" style={BG_STYLE}>
      <div className="w-full max-w-md">
        <div className="text-center mb-7">
          <div className="w-12 h-12 bg-cyan-600 rounded-2xl flex items-center justify-center mx-auto mb-3 shadow-lg shadow-cyan-600/20">
            <ShieldCheck className="w-6 h-6 text-white" />
          </div>
          <p className="text-xs font-mono text-muted-foreground tracking-widest uppercase">Bank Admin Enrollment</p>
        </div>

        <div className="bg-card border border-border rounded-2xl p-7 shadow-xl shadow-black/40">
          {success ? (
            <div className="py-10 text-center">
              <CheckCircle className="w-12 h-12 text-emerald-400 mx-auto mb-4" />
              <h1 className="text-lg font-semibold text-foreground">Kích hoạt thành công</h1>
              <p className="text-sm text-muted-foreground mt-2">Chứng chỉ đã được lưu trên thiết bị. Đang chuyển đến trang đăng nhập…</p>
            </div>
          ) : (
            <>
              <div className="mb-6">
                <h1 className="text-lg font-semibold text-foreground">Kích hoạt Bank Admin</h1>
                <p className="text-xs text-muted-foreground mt-1">Token được điền từ liên kết email. Vui lòng nhập lại email và họ tên đã provision.</p>
              </div>

              {error && (
                <div className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2.5 text-xs text-red-400">
                  {error}
                </div>
              )}

              <form onSubmit={submit} className="space-y-4">
                <label className="block">
                  <span className="block text-xs text-muted-foreground mb-1.5">One-time token</span>
                  <div className="relative">
                    <KeyRound className="absolute left-3.5 top-3.5 w-4 h-4 text-muted-foreground" />
                    <input
                      value={token}
                      onChange={(event) => setToken(event.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                      className="w-full bg-background border border-border rounded-xl py-3 pl-10 pr-4 text-sm font-mono text-foreground focus:border-cyan-500 focus:outline-none"
                      placeholder="Token từ liên kết email"
                    />
                  </div>
                </label>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <label className="block">
                    <span className="block text-xs text-muted-foreground mb-1.5">Email Admin</span>
                    <input
                      type="email"
                      value={email}
                      onChange={(event) => setEmail(event.target.value)}
                      autoComplete="email"
                      className="w-full bg-background border border-border rounded-xl px-3.5 py-3 text-sm text-foreground focus:border-cyan-500 focus:outline-none"
                    />
                  </label>
                  <label className="block">
                    <span className="block text-xs text-muted-foreground mb-1.5">Họ và tên</span>
                    <input
                      value={fullName}
                      onChange={(event) => setFullName(event.target.value)}
                      autoComplete="name"
                      className="w-full bg-background border border-border rounded-xl px-3.5 py-3 text-sm text-foreground focus:border-cyan-500 focus:outline-none"
                    />
                  </label>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <label className="block">
                    <span className="block text-xs text-muted-foreground mb-1.5">Mã PIN</span>
                    <input
                      type="password"
                      inputMode="numeric"
                      maxLength={6}
                      value={pin}
                      onChange={(event) => setPin(event.target.value.replace(/\D/g, ""))}
                      autoComplete="new-password"
                      className="w-full bg-background border border-border rounded-xl px-3.5 py-3 text-sm font-mono tracking-[0.3em] text-foreground focus:border-cyan-500 focus:outline-none"
                    />
                  </label>
                  <label className="block">
                    <span className="block text-xs text-muted-foreground mb-1.5">Xác nhận PIN</span>
                    <input
                      type="password"
                      inputMode="numeric"
                      maxLength={6}
                      value={confirmPin}
                      onChange={(event) => setConfirmPin(event.target.value.replace(/\D/g, ""))}
                      autoComplete="new-password"
                      className="w-full bg-background border border-border rounded-xl px-3.5 py-3 text-sm font-mono tracking-[0.3em] text-foreground focus:border-cyan-500 focus:outline-none"
                    />
                  </label>
                </div>

                <div className="flex items-start gap-2 rounded-lg border border-border bg-background p-3">
                  <Lock className="w-4 h-4 text-cyan-400 shrink-0 mt-0.5" />
                  <p className="text-xs text-muted-foreground">PIN và private key chỉ được xử lý trong trình duyệt. Gateway chỉ nhận token và CSR.</p>
                </div>

                <button
                  type="submit"
                  disabled={submitting}
                  className="w-full bg-cyan-600 hover:bg-cyan-500 disabled:opacity-50 text-white rounded-xl py-3 text-sm font-semibold flex items-center justify-center gap-2 transition-colors"
                >
                  {submitting ? <><RefreshCw className="w-4 h-4 animate-spin" /> Đang kích hoạt…</> : "Kích hoạt"}
                </button>
              </form>

              <p className="text-center text-xs text-muted-foreground mt-5">
                Đã kích hoạt? <Link to="/admin-bank/login" className="text-cyan-400 hover:text-cyan-300">Đăng nhập Bank Admin</Link>
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
