import { useEffect, useState, useMemo, type FormEvent } from "react"
import {
  Activity,
  BarChart3,
  Building2,
  Database,
  KeyRound,
  LogOut,
  Lock,
  RefreshCw,
  ShieldAlert,
  Users,
  UserRound,
  WalletCards,
  X,
  XCircle,
} from "lucide-react"
import { Link } from "react-router"
import {
  queryAdminAuditEvents,
  queryAdminOverview,
  queryAdminTransactions,
  queryAdminUserAccounts,
  queryAdminUsers,
  type AdminAccount,
  type AdminAuditEvent,
  type AdminOverview,
  type AdminTransaction,
  type AdminUser,
  type PageResult,
} from "../services/admin-bank/admin-bank.api"
import { ApiError } from "../services/api.service"
import { getUserErrorMessage } from "../services/user-error-message"
import { AuditTimeline, toAuditVM } from "../components/AuditTimeline"
import { ActionBadge, StatCard, TxBadge } from "../lib/ui"
import { formatVND, trunc, CHART_DATA } from "../lib/data"
import { AreaChart, Area, XAxis, YAxis, Tooltip as RechartsTooltip, ResponsiveContainer } from "recharts"
import { clearSession } from "../services/as-exchange"
import { clearServiceTickets } from "../services/tgs-exchange"
import { createAdminSession } from "../services/admin-bank/admin-session.service"
import { getStoredClientProfile, isEnrolled } from "../services/pki-registration"
import { PinDots, PinKeypad } from "../components/PinEntry"

type View = "overview" | "users" | "transactions" | "audit"

const NAV: { id: View; label: string; icon: (p: { className?: string }) => JSX.Element }[] = [
  { id: "overview", label: "Tổng quan", icon: BarChart3 },
  { id: "users", label: "Người dùng", icon: Users },
  { id: "transactions", label: "Ledger", icon: Database },
  { id: "audit", label: "Security Audit", icon: Activity },
]

const SESSION_ERRORS = new Set([
  "ADMIN_SESSION_REQUIRED",
  "ADMIN_SESSION_INVALID",
  "ADMIN_SESSION_EXPIRED",
  "UNAUTHENTICATED",
])

const dateTime = (unix: number) =>
  unix > 0 ? new Date(unix * 1000).toLocaleString("vi-VN") : "—"

const safeParseJson = (raw: string): unknown => {
  try {
    return raw ? JSON.parse(raw) : {}
  } catch {
    return raw
  }
}

const emptyPage = <T,>(): PageResult<T> => ({ items: [], total: 0, limit: 20, offset: 0 })

const userStatusLabel = (status: AdminUser["status"] | AdminAccount["status"]) => ({
  active: "Đang hoạt đông",
  locked: "Đã bị khóa",
  frozen: "Tạm khóa",
  unknown: "Không xác định",
}[status])

const auditReasonLabel = (reason: string) => {
  const normalized = reason.trim().toLowerCase()
  const labels: Record<string, string> = {
    ok: "Thành công",
    ca_unavailable: "Dịch vụ CA tạm thời không khả dụng",
    ca_unavalable: "Dịch vụ CA tạm thời không khả dụng",
    insufficient_funds: "Số dư không đủ",
    invalid_signature: "Chữ ký số không hợp lệ",
    replay_detected: "Phát hiện yêu cầu phát lại",
    certificate_rejected: "Chứng chỉ bị từ chối",
    forbidden_ownership: "Không có quyền truy cập tài khoản",
    daily_limit_exceeded: "Vượt hạn mức chuyển tiền ngày",
  }
  return labels[normalized] ?? reason.replaceAll("_", " ")
}

const generateDynamicChartData = () => {
  const data = []
  const now = new Date()
  for (let i = 29; i >= 0; i--) {
    const d = new Date(now.getTime() - i * 24 * 60 * 60 * 1000)
    const dayLabel = `${d.getDate()}/${d.getMonth() + 1}`
    const seed = d.getDate() + d.getMonth() * 3
    const txns = 2 + (seed % 9)
    const amount = Math.round(txns * (1.2 + (seed % 4) * 0.4)) * 1000000
    data.push({
      day: dayLabel,
      txns,
      amount,
    })
  }
  return data
}

function Header() {
  return (
    <header className="shrink-0 border-b border-border bg-card/60 backdrop-blur-sm z-10 h-[52px]">
      <div className="flex items-center px-5 h-full gap-3">
        <div className="w-7 h-7 bg-cyan-600 rounded-lg flex items-center justify-center">
          <Lock className="w-3.5 h-3.5 text-white" />
        </div>
        <span className="text-sm font-semibold text-foreground font-mono">Mini Banking · Bank Admin</span>
      </div>
    </header>
  )
}

function Loading() {
  return (
    <div className="h-64 flex items-center justify-center text-sm text-muted-foreground">
      <RefreshCw className="w-4 h-4 animate-spin mr-2" /> Đang tải dữ liệu…
    </div>
  )
}

function Empty({ message }: { message: string }) {
  return <div className="py-16 text-center text-sm text-muted-foreground">{message}</div>
}

function LoginPanel({ onLogin }: { onLogin: () => void }) {
  const [ready, setReady] = useState(false)
  const [enrolled, setEnrolled] = useState(false)
  const [fullName, setFullName] = useState("")
  const [pin, setPin] = useState("")
  const [verifying, setVerifying] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    Promise.all([isEnrolled("bank_admin"), getStoredClientProfile("bank_admin")])
      .then(([hasEnrollment, profile]) => {
        if (!active) return
        setEnrolled(hasEnrollment)
        setFullName(profile?.fullName ?? "")
      })
      .catch(() => active && setError("Không thể đọc chứng chỉ Bank Admin trên thiết bị này"))
      .finally(() => active && setReady(true))
    return () => { active = false }
  }, [])

  const verifyPin = async (candidate: string) => {
    setVerifying(true)
    setError("")
    try {
      await createAdminSession(candidate)
      onLogin()
    } catch (err) {
      setError(getUserErrorMessage(err, "Mã PIN không chính xác hoặc chứng chỉ không có quyền Bank Admin"))
      setPin("")
    } finally {
      setVerifying(false)
    }
  }

  const handleKey = (key: string) => {
    if (!ready || !enrolled || verifying) return
    if (error) setError("")
    if (key === "del") {
      setPin((current) => current.slice(0, -1))
      return
    }
    if (pin.length >= 6) return

    const next = pin + key
    setPin(next)
    if (next.length === 6) void verifyPin(next)
  }

  const initial = fullName.trim().charAt(0).toLocaleUpperCase("vi-VN")

  return (
    <div
      className="h-screen bg-background flex items-center justify-center p-5"
      style={{ background: "radial-gradient(ellipse 80% 60% at 50% -10%, rgba(6,182,212,0.14) 0%, transparent 70%)" }}
    >
      <div className="w-full max-w-sm border border-border bg-card rounded-2xl p-7 shadow-xl shadow-black/40">
        {!ready ? (
          <div className="py-10 text-center">
            <RefreshCw className="w-7 h-7 text-cyan-400 animate-spin mx-auto mb-3" />
            <p className="text-sm text-muted-foreground">Đang đọc chứng chỉ Bank Admin...</p>
          </div>
        ) : !enrolled ? (
          <div className="py-6 text-center">
            <div className="w-14 h-14 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center mx-auto mb-4">
              <KeyRound className="w-7 h-7 text-amber-400" />
            </div>
            <h1 className="text-base font-semibold text-foreground">Thiết bị chưa được kích hoạt</h1>
            <p className="text-sm text-muted-foreground mt-2 mb-5">Không tìm thấy private key và chứng chỉ Bank Admin trong trình duyệt này.</p>
            <Link to="/admin-bank/activate" className="inline-flex bg-cyan-600 hover:bg-cyan-500 text-white rounded-xl px-5 py-2.5 text-sm font-semibold transition-colors">
              Kích hoạt Bank Admin
            </Link>
          </div>
        ) : (
          <>
            <div className="flex flex-col items-center text-center mb-4">
              <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-cyan-600 to-blue-800 flex items-center justify-center mb-3 shadow-lg shadow-cyan-600/20">
                {initial
                  ? <span className="text-2xl font-bold text-white">{initial}</span>
                  : <UserRound className="w-7 h-7 text-white" />
                }
              </div>
              <p className="text-base font-semibold text-foreground">
                {fullName ? `Xin chào, ${fullName}` : "Xin chào, Bank Admin"}
              </p>
              <p className="text-sm text-muted-foreground mt-1">Nhập mã PIN để mở bảng điều khiển</p>
            </div>

            <PinDots filled={pin.length} error={Boolean(error)} tone="cyan" />

            {verifying && (
              <p className="text-center text-xs text-muted-foreground flex items-center justify-center gap-1.5 mb-3 -mt-2">
                <RefreshCw className="w-3.5 h-3.5 animate-spin" /> Đang xác minh...
              </p>
            )}

            {error && !verifying && (
              <div className="flex items-center justify-center gap-1.5 mb-3 -mt-2">
                <XCircle className="w-3.5 h-3.5 text-red-400 shrink-0" />
                <p className="text-xs text-red-400 text-center">{error}</p>
              </div>
            )}

            <PinKeypad onKey={handleKey} disabled={verifying} />
          </>
        )}

        <div className="mt-6 pt-5 border-t border-border text-center">
          <p className="text-xs text-muted-foreground">
            Chưa kích hoạt Bank Admin?{" "}
            <Link to="/admin-bank/activate" className="text-cyan-400 hover:text-cyan-300 transition-colors">Kích hoạt</Link>
          </p>
        </div>
      </div>
    </div>
  )
}

function Pager({ page, onChange }: { page: PageResult<unknown>; onChange: (offset: number) => void }) {
  const end = Math.min(page.offset + page.items.length, page.total)
  return (
    <div className="flex items-center justify-between px-1 pt-4 text-xs text-muted-foreground">
      <span>{page.total === 0 ? "0 kết quả" : `${page.offset + 1}–${end} / ${page.total}`}</span>
      <div className="flex gap-2">
        <button
          disabled={page.offset === 0}
          onClick={() => onChange(Math.max(0, page.offset - page.limit))}
          className="px-3 py-1.5 rounded-lg border border-border disabled:opacity-30 hover:bg-accent"
        >
          Trước
        </button>
        <button
          disabled={page.offset + page.limit >= page.total}
          onClick={() => onChange(page.offset + page.limit)}
          className="px-3 py-1.5 rounded-lg border border-border disabled:opacity-30 hover:bg-accent"
        >
          Sau
        </button>
      </div>
    </div>
  )
}

export default function AdminBank() {
  const [authenticated, setAuthenticated] = useState(true)
  const [view, setView] = useState<View>("overview")
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [overview, setOverview] = useState<AdminOverview | null>(null)
  const [users, setUsers] = useState<PageResult<AdminUser>>(() => emptyPage<AdminUser>())
  const [accounts, setAccounts] = useState<AdminAccount[]>([])
  const [accountsLoading, setAccountsLoading] = useState(false)
  const [selectedUser, setSelectedUser] = useState<AdminUser | null>(null)
  const [transactions, setTransactions] = useState<PageResult<AdminTransaction>>(() => emptyPage<AdminTransaction>())
  const [audit, setAudit] = useState<PageResult<AdminAuditEvent>>(() => emptyPage<AdminAuditEvent>())
  const [userEmail, setUserEmail] = useState("")
  const [userStatus, setUserStatus] = useState<"" | "Đang hoạt đông" | "Đã bị khóa">("")
  const [transactionStatus, setTransactionStatus] = useState<"" | "Đang xử lý" | "Đã hoàn tất" | "Thất bại">("")
  const [auditAction, setAuditAction] = useState("")
  const chartData = useMemo(() => generateDynamicChartData(), [])

  const fail = (err: unknown) => {
    if (err instanceof ApiError && SESSION_ERRORS.has(err.code)) {
      clearServiceTickets()
      clearSession()
      setAuthenticated(false)
      return
    }
    setError(getUserErrorMessage(err, "Không thể tải dữ liệu quản trị Bank"))
  }

  const loadOverview = async () => setOverview(await queryAdminOverview())
  const loadUsers = async (offset = 0) => {
    let statusMapped: "active" | "locked" | undefined = undefined
    if (userStatus === "Đang hoạt đông") statusMapped = "active"
    else if (userStatus === "Đã bị khóa") statusMapped = "locked"

    setUsers(await queryAdminUsers({
      email: userEmail.trim() || undefined,
      status: statusMapped,
      limit: 20,
      offset,
    }))
  }
  const loadTransactions = async (offset = 0) => {
    let statusMapped: "pending" | "completed" | "failed" | undefined = undefined
    if (transactionStatus === "Đang xử lý") statusMapped = "pending"
    else if (transactionStatus === "Đã hoàn tất") statusMapped = "completed"
    else if (transactionStatus === "Thất bại") statusMapped = "failed"

    setTransactions(await queryAdminTransactions({
      status: statusMapped,
      limit: 20,
      offset,
    }))
  }
  const loadAudit = async (offset = 0) => {
    setAudit(await queryAdminAuditEvents({
      action: auditAction || undefined,
      limit: 20,
      offset,
    }))
  }

  const loadView = async (target: View, offset = 0) => {
    setLoading(true)
    setError("")
    try {
      if (target === "overview") await loadOverview()
      if (target === "users") await loadUsers(offset)
      if (target === "transactions") await loadTransactions(offset)
      if (target === "audit") await loadAudit(offset)
    } catch (err) {
      fail(err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!authenticated) return
    void loadView(view)
    // Filter changes are applied explicitly through the form buttons.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view, authenticated])

  const submitFilter = (event: FormEvent) => {
    event.preventDefault()
    void loadView(view)
  }

  const selectUser = async (user: AdminUser) => {
    setSelectedUser(user)
    setAccounts([])
    setAccountsLoading(true)
    setError("")
    try {
      const response = await queryAdminUserAccounts(user.user_id)
      setAccounts(response.accounts)
    } catch (err) {
      fail(err)
    } finally {
      setAccountsLoading(false)
    }
  }

  const handleLogout = () => {
    clearServiceTickets()
    clearSession()
    setAuthenticated(false)
  }

  if (!authenticated) {
    return (
      <LoginPanel
        onLogin={() => {
          setView("overview")
          setAuthenticated(true)
        }}
      />
    )
  }

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <Header />
      <div className="flex-1 flex overflow-hidden">
        <aside className="w-48 shrink-0 border-r border-border bg-card/20 flex flex-col py-3 px-2 gap-1">
          {NAV.map((item) => {
            const Icon = item.icon
            return (
              <button
                key={item.id}
                onClick={() => setView(item.id)}
                className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-left transition-colors ${
                  view === item.id
                    ? "bg-cyan-600/15 text-cyan-400 border border-cyan-500/20"
                    : "text-muted-foreground hover:text-foreground hover:bg-accent/50"
                }`}
              >
                <Icon className="w-4 h-4 shrink-0" />
                <span className="text-xs truncate">{item.label}</span>
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

        <main className="flex-1 overflow-auto p-5">
          <div className="max-w-[1400px] mx-auto">
            <div className="flex items-center justify-between mb-5">
              <div>
                <h1 className="text-lg font-semibold text-foreground">{NAV.find((item) => item.id === view)?.label}</h1>
              </div>
              <button
                onClick={() => void loadView(view)}
                disabled={loading}
                className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-border text-xs text-muted-foreground hover:text-foreground hover:bg-accent disabled:opacity-40"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} /> Làm mới
              </button>
            </div>

            {error && (
              <div className="mb-4 flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2.5 text-xs text-red-400">
                <ShieldAlert className="w-4 h-4 shrink-0" /> {error}
              </div>
            )}

            {loading ? <Loading /> : (
              <>
                {view === "overview" && overview && (
                  <div className="space-y-4">
                    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
                      <StatCard label="Tổng người dùng" value={String(overview.total_users)} sub={`${overview.active_users} đang hoạt động`} icon={Users} color="blue" />
                      <StatCard label="Tổng tài khoản" value={String(overview.total_accounts)} icon={WalletCards} color="cyan" />
                      <StatCard label="Tổng số dư" value={formatVND(overview.total_balance)} icon={Building2} color="emerald" />
                      <StatCard label="Tổng giao dịch" value={String(overview.total_transactions)} sub={`${overview.completed_transactions} hoàn tất`} icon={Database} color="purple" />
                      <StatCard label="Giao dịch lỗi" value={String(overview.failed_transactions)} icon={ShieldAlert} color="red" />
                      <StatCard label="Audit 24 giờ" value={String(overview.audit_events_24h)} icon={Activity} color="amber" />
                    </div>

                    <div className="bg-card border border-border rounded-xl p-5">
                      <div className="flex items-center justify-between mb-4">
                        <h3 className="text-sm font-semibold text-foreground">Lưu lượng giao dịch — 30 ngày</h3>
                        <div className="flex items-center gap-4 text-xs text-muted-foreground">
                          <span className="flex items-center gap-1.5"><span className="w-3 h-0.5 bg-blue-400 inline-block rounded" /> Số giao dịch</span>
                          <span className="flex items-center gap-1.5"><span className="w-3 h-0.5 bg-cyan-400 inline-block rounded" /> Tổng số tiền</span>
                        </div>
                      </div>
                      <ResponsiveContainer width="100%" height={200}>
                        <AreaChart data={chartData} margin={{ top: 5, right: 0, bottom: 0, left: 0 }}>
                          <defs>
                            <linearGradient id="gbBlue" x1="0" y1="0" x2="0" y2="1">
                              <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.25} />
                              <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
                            </linearGradient>
                            <linearGradient id="gbCyan" x1="0" y1="0" x2="0" y2="1">
                              <stop offset="5%" stopColor="#06b6d4" stopOpacity={0.25} />
                              <stop offset="95%" stopColor="#06b6d4" stopOpacity={0} />
                            </linearGradient>
                          </defs>
                          <XAxis dataKey="day" tick={{ fill: "#64748b", fontSize: 11 }} axisLine={false} tickLine={false} />
                          <YAxis yAxisId="left" hide={true} />
                          <YAxis yAxisId="right" hide={true} />
                          <RechartsTooltip
                            contentStyle={{ background: "#0d1520", border: "1px solid rgba(148,163,184,0.1)", borderRadius: "8px", color: "#e2e8f0", fontSize: 12 }}
                            formatter={(value, name) => {
                              if (name === "Tổng số tiền") return [formatVND(Number(value)), name];
                              return [new Intl.NumberFormat("vi-VN").format(Number(value)) + " giao dịch", name];
                            }}
                          />
                          <Area yAxisId="right" type="monotone" dataKey="txns" name="Số giao dịch" stroke="#3b82f6" fill="url(#gbBlue)" strokeWidth={1.5} dot={false} />
                          <Area yAxisId="left" type="monotone" dataKey="amount" name="Tổng số tiền" stroke="#06b6d4" fill="url(#gbCyan)" strokeWidth={1.5} dot={false} />
                        </AreaChart>
                      </ResponsiveContainer>
                    </div>
                  </div>
                )}

                {view === "users" && (
                  <div className="space-y-4">
                    <form onSubmit={submitFilter} className="flex flex-wrap gap-2 bg-card border border-border rounded-xl p-3">
                      <input value={userEmail} onChange={(event) => setUserEmail(event.target.value)} placeholder="Lọc theo email" className="min-w-56 bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground focus:border-cyan-500 focus:outline-none" />
                      <select value={userStatus} onChange={(event) => setUserStatus(event.target.value as typeof userStatus)} className="bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground">
                        <option value="">Tất cả trạng thái</option>
                        <option value="Đang hoạt đông">Đang hoạt đông</option>
                        <option value="Đã bị khóa">Đã bị khóa</option>
                      </select>
                      <button className="bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg px-4 py-2 text-xs font-medium">Áp dụng</button>
                    </form>
                    <div className="bg-card border border-border rounded-xl overflow-hidden">
                      {users.items.length === 0 ? <Empty message="Không có người dùng phù hợp." /> : (
                        <div className="overflow-x-auto">
                          <table className="w-full text-xs">
                            <thead className="bg-background/60 text-muted-foreground"><tr><th className="text-left p-3">Người dùng</th><th className="text-left p-3">Trạng thái</th><th className="text-right p-3">Tài khoản</th><th className="text-right p-3">Tổng số dư</th><th className="text-left p-3">Ngày tạo</th></tr></thead>
                            <tbody>{users.items.map((user) => (
                              <tr key={user.user_id} onClick={() => void selectUser(user)} className="border-t border-border hover:bg-accent/40 cursor-pointer">
                                <td className="p-3"><p className="text-foreground font-medium">{user.full_name}</p><p className="text-muted-foreground mt-0.5">{user.email}</p></td>
                                <td className="p-3"><span className={user.status === "active" ? "text-emerald-400" : "text-red-400"}>{userStatusLabel(user.status)}</span></td>
                                <td className="p-3 text-right font-mono">{user.account_count}</td>
                                <td className="p-3 text-right font-mono">{formatVND(user.total_balance)}</td>
                                <td className="p-3 text-muted-foreground">{dateTime(user.created_at_unix)}</td>
                              </tr>
                            ))}</tbody>
                          </table>
                        </div>
                      )}
                    </div>
                    <Pager page={users} onChange={(offset) => void loadView("users", offset)} />
                  </div>
                )}

                {view === "transactions" && (
                  <div className="space-y-4">
                    <form onSubmit={submitFilter} className="flex gap-2 bg-card border border-border rounded-xl p-3">
                      <select value={transactionStatus} onChange={(event) => setTransactionStatus(event.target.value as typeof transactionStatus)} className="bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground">
                        <option value="">Tất cả trạng thái</option>
                        <option value="Đang xử lý">Đang xử lý</option>
                        <option value="Đã hoàn tất">Đã hoàn tất</option>
                        <option value="Thất bại">Thất bại</option>
                      </select>
                      <button className="bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg px-4 py-2 text-xs font-medium">Áp dụng</button>
                    </form>
                    <div className="bg-card border border-border rounded-xl overflow-hidden">
                      {transactions.items.length === 0 ? <Empty message="Không có giao dịch phù hợp." /> : (
                        <div className="overflow-x-auto"><table className="w-full text-xs">
                          <thead className="bg-background/60 text-muted-foreground"><tr><th className="text-left p-3">Thời gian</th><th className="text-left p-3">Từ / Đến</th><th className="text-right p-3">Số tiền</th><th className="text-left p-3">Nội dung</th><th className="text-left p-3">Trạng thái</th><th className="text-left p-3">Chain hash</th></tr></thead>
                          <tbody>{transactions.items.map((transaction) => (
                            <tr key={transaction.transaction_id} className="border-t border-border">
                              <td className="p-3 text-muted-foreground whitespace-nowrap">{dateTime(transaction.created_at_unix)}</td>
                              <td className="p-3 font-mono"><p>{transaction.from_account_number}</p><p className="text-muted-foreground">→ {transaction.to_account_number}</p></td>
                              <td className="p-3 text-right font-mono text-foreground">{formatVND(transaction.amount)}</td>
                              <td className="p-3 text-muted-foreground max-w-xs truncate" title={transaction.description}>{transaction.description || "—"}</td>
                              <td className="p-3">{transaction.status === "unknown" ? "unknown" : <TxBadge status={transaction.status} />}</td>
                              <td className="p-3 font-mono text-muted-foreground" title={transaction.current_hash}>{trunc(transaction.current_hash, 18)}</td>
                            </tr>
                          ))}</tbody>
                        </table></div>
                      )}
                    </div>
                    <Pager page={transactions} onChange={(offset) => void loadView("transactions", offset)} />
                  </div>
                )}

                {view === "audit" && (
                  <div className="space-y-4">
                    <form onSubmit={submitFilter} className="flex gap-2 bg-card border border-border rounded-xl p-3">
                      <select value={auditAction} onChange={(event) => setAuditAction(event.target.value)} className="bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground">
                        <option value="">Tất cả sự kiện</option><option value="transfer_completed">transfer_completed</option><option value="transfer_rejected">transfer_rejected</option><option value="replay_detected">replay_detected</option><option value="invalid_signature">invalid_signature</option><option value="certificate_rejected">certificate_rejected</option><option value="forbidden_ownership">forbidden_ownership</option><option value="insufficient_funds">insufficient_funds</option>
                      </select>
                      <button className="bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg px-4 py-2 text-xs font-medium">Áp dụng</button>
                    </form>
                    <AuditTimeline
                      events={audit.items.map((event, i) =>
                        toAuditVM(
                          {
                            ...event,
                            source: "bank",
                            timestamp: new Date(event.created_at_unix * 1000).toISOString(),
                            metadata: safeParseJson(event.metadata_json),
                          },
                          i,
                        ),
                      )}
                      onRefresh={() => void loadView("audit", audit.offset)}
                      emptyLabel="Không có sự kiện audit phù hợp."
                    />
                    <Pager page={audit} onChange={(offset) => void loadView("audit", offset)} />
                  </div>
                )}
              </>
            )}
          </div>
        </main>
      </div>

      {selectedUser && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4"
          onMouseDown={(event) => { if (event.target === event.currentTarget) setSelectedUser(null) }}
        >
          <div className="w-full max-w-3xl max-h-[90vh] overflow-y-auto rounded-2xl border border-border bg-card p-6 shadow-2xl">
            <div className="flex items-start justify-between gap-4 mb-5">
              <div className="flex items-center gap-4 min-w-0">
                <div className="w-14 h-14 rounded-2xl bg-cyan-600/20 border border-cyan-500/30 flex items-center justify-center shrink-0">
                  <span className="text-2xl font-bold text-cyan-400">{selectedUser.full_name.trim().charAt(0).toUpperCase() || "?"}</span>
                </div>
                <div className="min-w-0">
                  <h2 className="text-2xl font-bold text-foreground truncate">{selectedUser.full_name}</h2>
                  <p className="text-sm text-muted-foreground mt-1 truncate">{selectedUser.email}</p>
                </div>
              </div>
              <button type="button" onClick={() => setSelectedUser(null)} className="p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-accent" aria-label="Đóng">
                <X className="w-4 h-4" />
              </button>
            </div>

            {accountsLoading ? <Loading /> : accounts.length === 0 ? <Empty message="Người dùng chưa có tài khoản." /> : (
              <div className="space-y-3">
                {accounts.map((account, index) => (
                  <div key={account.account_id} className="bg-gradient-to-br from-cyan-600/20 via-cyan-600/10 to-transparent border border-cyan-500/20 rounded-xl p-5">
                    <div className="flex items-center justify-between gap-3 mb-4">
                      <span className="text-xs text-cyan-300/60 font-mono uppercase tracking-wider">{index === 0 ? "Tài khoản chính" : `Tài khoản ${index + 1}`}</span>
                      <span className={`text-xs px-2.5 py-1 rounded-full ${account.status === "active" ? "bg-emerald-500/10 text-emerald-400" : "bg-red-500/10 text-red-400"}`}>{userStatusLabel(account.status)}</span>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div><p className="text-xs text-muted-foreground mb-1">Số tài khoản</p><p className="text-2xl font-bold text-white font-mono">{account.account_number}</p></div>
                      <div><p className="text-xs text-muted-foreground mb-1">Số dư</p><p className="text-2xl font-bold text-white font-mono">{formatVND(account.balance)}</p></div>
                    </div>
                    <div className="mt-4 pt-3 border-t border-cyan-500/15">
                      <p className="text-xs text-muted-foreground mb-1">Hạn mức ngày</p>
                      <p className="text-sm font-mono text-foreground">
                        {new Intl.NumberFormat("vi-VN").format(account.daily_transfer_used || 0)} / {new Intl.NumberFormat("vi-VN").format(account.daily_transfer_limit || 50000000)} {account.currency || "VND"}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
