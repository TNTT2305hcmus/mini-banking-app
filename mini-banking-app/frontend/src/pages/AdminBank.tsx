import { useEffect, useState, type FormEvent } from "react"
import {
  Activity,
  BarChart3,
  Building2,
  Database,
  LogOut,
  Lock,
  RefreshCw,
  ShieldAlert,
  Users,
  WalletCards,
  X,
} from "lucide-react"
import { useNavigate } from "react-router"
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
import { formatVND, trunc } from "../lib/data"
import { clearSession } from "../services/as-exchange"
import { clearServiceTickets } from "../services/tgs-exchange"

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
  active: "Hoạt động",
  locked: "Đã khóa",
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
  }
  return labels[normalized] ?? reason.replaceAll("_", " ")
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
  const navigate = useNavigate()
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
  const [userStatus, setUserStatus] = useState<"" | "active" | "locked">("")
  const [transactionStatus, setTransactionStatus] = useState<"" | "pending" | "completed" | "failed">("")
  const [auditAction, setAuditAction] = useState("")

  const fail = (err: unknown) => {
    if (err instanceof ApiError && SESSION_ERRORS.has(err.code)) {
      navigate("/admin-bank/login", { replace: true })
      return
    }
    setError(getUserErrorMessage(err, "Không thể tải dữ liệu quản trị Bank"))
  }

  const loadOverview = async () => setOverview(await queryAdminOverview())
  const loadUsers = async (offset = 0) => {
    setUsers(await queryAdminUsers({
      email: userEmail.trim() || undefined,
      status: userStatus || undefined,
      limit: 20,
      offset,
    }))
  }
  const loadTransactions = async (offset = 0) => {
    setTransactions(await queryAdminTransactions({
      status: transactionStatus || undefined,
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
    void loadView(view)
    // Filter changes are applied explicitly through the form buttons.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view])

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
    navigate("/admin-bank/login", { replace: true })
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
                  <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
                    <StatCard label="Tổng người dùng" value={String(overview.total_users)} sub={`${overview.active_users} đang hoạt động`} icon={Users} color="blue" />
                    <StatCard label="Tổng tài khoản" value={String(overview.total_accounts)} icon={WalletCards} color="cyan" />
                    <StatCard label="Tổng số dư" value={formatVND(overview.total_balance)} icon={Building2} color="emerald" />
                    <StatCard label="Tổng giao dịch" value={String(overview.total_transactions)} sub={`${overview.completed_transactions} hoàn tất`} icon={Database} color="purple" />
                    <StatCard label="Giao dịch lỗi" value={String(overview.failed_transactions)} icon={ShieldAlert} color="red" />
                    <StatCard label="Audit 24 giờ" value={String(overview.audit_events_24h)} icon={Activity} color="amber" />
                  </div>
                )}

                {view === "users" && (
                  <div className="space-y-4">
                    <form onSubmit={submitFilter} className="flex flex-wrap gap-2 bg-card border border-border rounded-xl p-3">
                      <input value={userEmail} onChange={(event) => setUserEmail(event.target.value)} placeholder="Lọc theo email" className="min-w-56 bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground focus:border-cyan-500 focus:outline-none" />
                      <select value={userStatus} onChange={(event) => setUserStatus(event.target.value as typeof userStatus)} className="bg-background border border-border rounded-lg px-3 py-2 text-xs text-foreground">
                        <option value="">Tất cả trạng thái</option><option value="active">active</option><option value="locked">locked</option>
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
                        <option value="">Tất cả trạng thái</option><option value="pending">pending</option><option value="completed">completed</option><option value="failed">failed</option>
                      </select>
                      <button className="bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg px-4 py-2 text-xs font-medium">Áp dụng</button>
                    </form>
                    <div className="bg-card border border-border rounded-xl overflow-hidden">
                      {transactions.items.length === 0 ? <Empty message="Không có giao dịch phù hợp." /> : (
                        <div className="overflow-x-auto"><table className="w-full text-xs">
                          <thead className="bg-background/60 text-muted-foreground"><tr><th className="text-left p-3">Thời gian</th><th className="text-left p-3">Từ / Đến</th><th className="text-right p-3">Số tiền</th><th className="text-left p-3">Trạng thái</th><th className="text-left p-3">Chain hash</th></tr></thead>
                          <tbody>{transactions.items.map((transaction) => (
                            <tr key={transaction.transaction_id} className="border-t border-border">
                              <td className="p-3 text-muted-foreground whitespace-nowrap">{dateTime(transaction.created_at_unix)}</td>
                              <td className="p-3 font-mono"><p>{transaction.from_account_number}</p><p className="text-muted-foreground">→ {transaction.to_account_number}</p></td>
                              <td className="p-3 text-right font-mono text-foreground">{formatVND(transaction.amount)}</td>
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
                      <p className="text-sm font-mono text-foreground">—</p>
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
