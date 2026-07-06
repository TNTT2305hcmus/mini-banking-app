import { apiPost } from "../api.service"

export interface AdminOverview {
  total_users: number
  active_users: number
  total_accounts: number
  total_balance: number
  total_transactions: number
  completed_transactions: number
  failed_transactions: number
  audit_events_24h: number
}

export interface AdminUser {
  user_id: string
  email: string
  full_name: string
  status: "active" | "locked" | "unknown"
  account_count: number
  total_balance: number
  created_at_unix: number
}

export interface AdminAccount {
  account_id: string
  account_number: string
  balance: number
  currency: string
  status: "active" | "locked" | "frozen" | "unknown"
  created_at_unix: number
}

export interface AdminTransaction {
  transaction_id: string
  from_account_number: string
  to_account_number: string
  amount: number
  currency: string
  status: "pending" | "completed" | "failed" | "unknown"
  description: string
  cert_serial: string
  current_hash: string
  created_at_unix: number
}

export interface AdminAuditEvent {
  event_id: string
  action: string
  user_id: string
  account_id: string
  transaction_id: string
  cert_serial: string
  request_id: string
  reason: string
  metadata_json: string
  created_at_unix: number
}

export interface PageResult<T> {
  total: number
  limit: number
  offset: number
  items: T[]
}

interface AdminUsersResponse {
  users: AdminUser[]
  total: number
  limit: number
  offset: number
}

interface AdminTransactionsResponse {
  transactions: AdminTransaction[]
  total: number
  limit: number
  offset: number
}

interface AdminAuditResponse {
  events: AdminAuditEvent[]
  total: number
  limit: number
  offset: number
}

export function queryAdminOverview(): Promise<AdminOverview> {
  return apiPost<AdminOverview>("/v1/admin/bank/overview/query", {})
}

export async function queryAdminUsers(params: {
  email?: string
  status?: "active" | "locked"
  limit?: number
  offset?: number
} = {}): Promise<PageResult<AdminUser>> {
  const response = await apiPost<AdminUsersResponse>("/v1/admin/bank/users/query", {
    ...(params.email ? { email: params.email } : {}),
    ...(params.status ? { status: params.status } : {}),
    limit: params.limit ?? 20,
    offset: params.offset ?? 0,
  })
  return { ...response, items: response.users }
}

export function queryAdminUserAccounts(userId: string): Promise<{ accounts: AdminAccount[] }> {
  return apiPost<{ accounts: AdminAccount[] }>(
    `/v1/admin/bank/users/${encodeURIComponent(userId)}/accounts/query`,
    {},
  )
}

export async function queryAdminTransactions(params: {
  accountId?: string
  status?: "pending" | "completed" | "failed"
  fromUnix?: number
  toUnix?: number
  limit?: number
  offset?: number
} = {}): Promise<PageResult<AdminTransaction>> {
  const response = await apiPost<AdminTransactionsResponse>(
    "/v1/admin/bank/transactions/query",
    {
      ...(params.accountId ? { account_id: params.accountId } : {}),
      ...(params.status ? { status: params.status } : {}),
      from_unix: params.fromUnix ?? 0,
      to_unix: params.toUnix ?? 0,
      limit: params.limit ?? 20,
      offset: params.offset ?? 0,
    },
  )
  return { ...response, items: response.transactions }
}

export async function queryAdminAuditEvents(params: {
  action?: string
  userId?: string
  certSerial?: string
  requestId?: string
  fromUnix?: number
  toUnix?: number
  limit?: number
  offset?: number
} = {}): Promise<PageResult<AdminAuditEvent>> {
  const response = await apiPost<AdminAuditResponse>("/v1/admin/bank/audit/query", {
    ...(params.action ? { action: params.action } : {}),
    ...(params.userId ? { user_id: params.userId } : {}),
    ...(params.certSerial ? { cert_serial: params.certSerial } : {}),
    ...(params.requestId ? { request_id: params.requestId } : {}),
    from_unix: params.fromUnix ?? 0,
    to_unix: params.toUnix ?? 0,
    limit: params.limit ?? 20,
    offset: params.offset ?? 0,
  })
  return { ...response, items: response.events }
}
