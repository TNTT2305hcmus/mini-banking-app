import type { NextFunction, Request, Response } from "express";
import { status as grpcStatus } from "@grpc/grpc-js";
import {
  AccountStatus,
  BankAuditAction,
  TransactionStatus,
  UserStatus,
} from "../proto/bank";
import {
  AdminActivationError,
  getPendingAdminByToken,
  markAdminActivated,
} from "../services/admin-activation.service";
import {
  createBankAdminSession,
  getBankAdminOverview,
  issueBankAdminCertificate,
  listBankAdminAuditEvents,
  listBankAdminTransactions,
  listBankAdminUserAccounts,
  listBankAdminUsers,
} from "../services/admin-bank.service";
import { BANK_ADMIN_SESSION_COOKIE } from "../middleware/admin-bank.middleware";
import { bankGrpcError } from "../middleware/errorHandler";

const meta = (req: Request) => ({
  request_id: req.headers["x-request-id"] as string,
  timestamp: new Date().toISOString(),
});

const caActivationError = (err: any) => {
  const message = err?.details || err?.message || "CA service unavailable";
  switch (err?.code) {
    case grpcStatus.ALREADY_EXISTS:
      return { status: 409, code: "ADMIN_ALREADY_ACTIVE", message };
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, code: "INVALID_CSR_FORMAT", message };
    case grpcStatus.UNAVAILABLE:
    case grpcStatus.DEADLINE_EXCEEDED:
      return { status: 503, code: "CA_SERVICE_UNAVAILABLE", message };
    default:
      return { status: 502, code: "CA_SERVICE_UNAVAILABLE", message };
  }
};

export const handleAdminActivate = async (req: Request, res: Response) => {
  const m = meta(req);
  try {
    const identity = await getPendingAdminByToken(req.body.activation_token);
    const issued = await issueBankAdminCertificate({
      csrPem: req.body.csr_pem,
      adminId: identity.admin_id,
      subjectEmail: identity.email,
      fullName: identity.full_name,
    });
    const active = await markAdminActivated({
      identity,
      certSerial: issued.serialNumber,
    });

    return res.status(201).json({
      success: true,
      message: "Bank Admin certificate issued",
      data: {
        cert_pem: issued.certificatePem,
        cert_serial: issued.serialNumber,
        issued_at: issued.notBeforeUnix,
        expires_at: issued.notAfterUnix,
        admin_id: active.admin_id,
        email: active.email,
        full_name: active.full_name,
        role: active.role,
      },
      ...m,
    });
  } catch (err: any) {
    if (err instanceof AdminActivationError) {
      return res.status(err.status).json({
        success: false,
        error_code: err.code,
        message: err.message,
        ...m,
      });
    }
    const mapped = caActivationError(err);
    return res.status(mapped.status).json({
      success: false,
      error_code: mapped.code,
      message: mapped.message,
      ...m,
    });
  }
};

const adminSessionToken = (res: Response): string =>
  String(res.locals.bankAdminSessionToken ?? "");

const userStatus = (status: string | undefined): UserStatus => {
  if (status === "active") return UserStatus.USER_STATUS_ACTIVE;
  if (status === "locked") return UserStatus.USER_STATUS_LOCKED;
  return UserStatus.USER_STATUS_UNKNOWN;
};

const transactionStatus = (status: string | undefined): TransactionStatus => {
  if (status === "pending") return TransactionStatus.TRANSACTION_STATUS_PENDING;
  if (status === "completed") return TransactionStatus.TRANSACTION_STATUS_COMPLETED;
  if (status === "failed") return TransactionStatus.TRANSACTION_STATUS_FAILED;
  return TransactionStatus.TRANSACTION_STATUS_UNKNOWN;
};

const auditAction = (action: string | undefined): BankAuditAction => {
  const values: Record<string, BankAuditAction> = {
    transfer_completed: BankAuditAction.BANK_AUDIT_ACTION_TRANSFER_COMPLETED,
    transfer_rejected: BankAuditAction.BANK_AUDIT_ACTION_TRANSFER_REJECTED,
    replay_detected: BankAuditAction.BANK_AUDIT_ACTION_REPLAY_DETECTED,
    invalid_signature: BankAuditAction.BANK_AUDIT_ACTION_INVALID_SIGNATURE,
    certificate_rejected: BankAuditAction.BANK_AUDIT_ACTION_CERTIFICATE_REJECTED,
    forbidden_ownership: BankAuditAction.BANK_AUDIT_ACTION_FORBIDDEN_OWNERSHIP,
    insufficient_funds: BankAuditAction.BANK_AUDIT_ACTION_INSUFFICIENT_FUNDS,
  };
  return action ? values[action] : BankAuditAction.BANK_AUDIT_ACTION_UNKNOWN;
};

const userStatusText: Record<number, string> = {
  [UserStatus.USER_STATUS_ACTIVE]: "active",
  [UserStatus.USER_STATUS_LOCKED]: "locked",
};
const accountStatusText: Record<number, string> = {
  [AccountStatus.ACCOUNT_STATUS_ACTIVE]: "active",
  [AccountStatus.ACCOUNT_STATUS_LOCKED]: "locked",
  [AccountStatus.ACCOUNT_STATUS_FROZEN]: "frozen",
};
const transactionStatusText: Record<number, string> = {
  [TransactionStatus.TRANSACTION_STATUS_PENDING]: "pending",
  [TransactionStatus.TRANSACTION_STATUS_COMPLETED]: "completed",
  [TransactionStatus.TRANSACTION_STATUS_FAILED]: "failed",
};
const auditActionText: Record<number, string> = {
  [BankAuditAction.BANK_AUDIT_ACTION_TRANSFER_COMPLETED]: "transfer_completed",
  [BankAuditAction.BANK_AUDIT_ACTION_TRANSFER_REJECTED]: "transfer_rejected",
  [BankAuditAction.BANK_AUDIT_ACTION_REPLAY_DETECTED]: "replay_detected",
  [BankAuditAction.BANK_AUDIT_ACTION_INVALID_SIGNATURE]: "invalid_signature",
  [BankAuditAction.BANK_AUDIT_ACTION_CERTIFICATE_REJECTED]: "certificate_rejected",
  [BankAuditAction.BANK_AUDIT_ACTION_FORBIDDEN_OWNERSHIP]: "forbidden_ownership",
  [BankAuditAction.BANK_AUDIT_ACTION_INSUFFICIENT_FUNDS]: "insufficient_funds",
};

export const handleAdminSession = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await createBankAdminSession({
      ticketV: Buffer.from(req.body.ticket_v, "base64"),
      authenticator: Buffer.from(req.body.authenticator, "base64"),
    });
    res.cookie(BANK_ADMIN_SESSION_COOKIE, response.sessionToken, {
      httpOnly: true,
      sameSite: "strict",
      secure: process.env.NODE_ENV === "production",
      path: "/v1/admin/bank",
      expires: new Date(response.expiresAtUnix * 1000),
    });
    return res.json({
      success: true,
      data: {
        ap_rep: Buffer.from(response.apRep).toString("base64"),
        expires_at_unix: response.expiresAtUnix,
        admin_id: response.adminId,
        role: response.role,
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminOverviewQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await getBankAdminOverview({
      adminSessionToken: adminSessionToken(res),
    });
    return res.json({
      success: true,
      data: {
        total_users: response.totalUsers,
        active_users: response.activeUsers,
        total_accounts: response.totalAccounts,
        total_balance: response.totalBalance,
        total_transactions: response.totalTransactions,
        completed_transactions: response.completedTransactions,
        failed_transactions: response.failedTransactions,
        audit_events_24h: response.auditEvents24h,
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminUsersQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await listBankAdminUsers({
      email: req.body.email ?? "",
      status: userStatus(req.body.status),
      limit: req.body.limit,
      offset: req.body.offset,
      adminSessionToken: adminSessionToken(res),
    });
    return res.json({
      success: true,
      data: {
        users: response.users.map((user) => ({
          user_id: user.userId,
          email: user.email,
          full_name: user.fullName,
          status: userStatusText[user.status] ?? "unknown",
          account_count: user.accountCount,
          total_balance: user.totalBalance,
          created_at_unix: user.createdAtUnix,
        })),
        total: response.total,
        limit: response.limit,
        offset: response.offset,
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminUserAccountsQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await listBankAdminUserAccounts({
      userId: String(req.params.userId),
      adminSessionToken: adminSessionToken(res),
    });
    return res.json({
      success: true,
      data: {
        accounts: response.accounts.map((account) => ({
          account_id: account.accountId,
          account_number: account.accountNumber,
          balance: account.balance,
          currency: account.currency,
          status: accountStatusText[account.status] ?? "unknown",
          created_at_unix: account.createdAtUnix,
        })),
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminTransactionsQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await listBankAdminTransactions({
      accountId: req.body.account_id ?? "",
      status: transactionStatus(req.body.status),
      fromUnix: req.body.from_unix,
      toUnix: req.body.to_unix,
      limit: req.body.limit,
      offset: req.body.offset,
      adminSessionToken: adminSessionToken(res),
    });
    return res.json({
      success: true,
      data: {
        transactions: response.transactions.map((transaction) => ({
          transaction_id: transaction.transactionId,
          from_account_number: transaction.fromAccountNumber,
          to_account_number: transaction.toAccountNumber,
          amount: transaction.amount,
          currency: transaction.currency,
          status: transactionStatusText[transaction.status] ?? "unknown",
          description: transaction.description,
          cert_serial: transaction.certSerial,
          current_hash: transaction.currentHash,
          created_at_unix: transaction.createdAtUnix,
        })),
        total: response.total,
        limit: response.limit,
        offset: response.offset,
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminAuditQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  res.set("Cache-Control", "no-store");
  try {
    const response = await listBankAdminAuditEvents({
      action: auditAction(req.body.action),
      userId: req.body.user_id ?? "",
      certSerial: req.body.cert_serial ?? "",
      requestId: req.body.request_id ?? "",
      fromUnix: req.body.from_unix,
      toUnix: req.body.to_unix,
      limit: req.body.limit,
      offset: req.body.offset,
      adminSessionToken: adminSessionToken(res),
    });
    return res.json({
      success: true,
      data: {
        events: response.events.map((event) => ({
          event_id: event.eventId,
          action: auditActionText[event.action] ?? "unknown",
          user_id: event.userId,
          account_id: event.accountId,
          transaction_id: event.transactionId,
          cert_serial: event.certSerial,
          request_id: event.requestId,
          reason: event.reason,
          metadata_json: event.metadataJson,
          created_at_unix: event.createdAtUnix,
        })),
        total: response.total,
        limit: response.limit,
        offset: response.offset,
      },
      ...meta(req),
    });
  } catch (err) {
    next(err);
  }
};

export const handleAdminBankError = (
  err: any,
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  if (typeof err?.code !== "number") return next(err);
  const mapped = bankGrpcError(err);
  return res.status(mapped.status).json({
    success: false,
    error_code: mapped.error_code,
    message: mapped.message,
    ...meta(req),
  });
};
