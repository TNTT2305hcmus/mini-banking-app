package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/banking-service/internal/bank"
	pb "mini-banking/pkg/pb/bank"
)

const adminSessionTokenBytes = 32

func (h *Handler) CreateAdminSession(ctx context.Context, req *pb.CreateAdminSessionRequest) (*pb.CreateAdminSessionResponse, error) {
	if h.adminSessions == nil {
		return nil, status.Error(codes.FailedPrecondition, "admin session store is not configured")
	}
	auth, err := h.authorize(ctx, req.GetTicketV(), req.GetAuthenticator(), scopeAdminRead)
	if err != nil {
		return nil, err
	}
	if auth.identityRole != bank.IdentityRoleBankAdmin {
		return nil, status.Error(codes.PermissionDenied, "ADMIN_ROLE_REQUIRED")
	}
	if err := h.markReplay(ctx, auth); err != nil {
		return nil, err
	}

	tokenBytes := make([]byte, adminSessionTokenBytes)
	if _, err := io.ReadFull(h.random, tokenBytes); err != nil {
		return nil, status.Error(codes.Internal, "generate admin session token")
	}
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	ttl := auth.ticket.expires.Sub(h.clock.Now())
	if ttl <= 0 {
		return nil, status.Error(codes.Unauthenticated, "INVALID_TICKET")
	}
	session := bank.AdminSession{
		AdminID:   auth.ticket.clientID,
		CertSN:    auth.ticket.certSN,
		Role:      bank.IdentityRoleBankAdmin,
		ExpiresAt: auth.ticket.expires.Unix(),
	}
	if err := h.adminSessions.Create(ctx, hashAdminSessionToken(rawToken), session, ttl); err != nil {
		return nil, status.Error(codes.Unavailable, "ADMIN_SESSION_STORE_UNAVAILABLE")
	}

	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{
		"result":             "ok",
		"nonce":              auth.nonce,
		"request_id":         auth.requestID,
		"role":               string(session.Role),
		"session_expires_at": session.ExpiresAt,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.CreateAdminSessionResponse{
		ApRep:         apRep,
		SessionToken:  rawToken,
		ExpiresAtUnix: session.ExpiresAt,
		AdminId:       session.AdminID,
		Role:          string(session.Role),
	}, nil
}

func (h *Handler) requireAdminSession(ctx context.Context, rawToken string) (bank.AdminSession, error) {
	if rawToken == "" || h.adminSessions == nil {
		return bank.AdminSession{}, status.Error(codes.Unauthenticated, "ADMIN_SESSION_REQUIRED")
	}
	session, err := h.adminSessions.Get(ctx, hashAdminSessionToken(rawToken))
	if errors.Is(err, errAdminSessionNotFound) {
		return bank.AdminSession{}, status.Error(codes.Unauthenticated, "ADMIN_SESSION_INVALID")
	}
	if err != nil {
		return bank.AdminSession{}, status.Error(codes.Unavailable, "ADMIN_SESSION_STORE_UNAVAILABLE")
	}
	if session.ExpiresAt <= h.clock.Now().Unix() {
		return bank.AdminSession{}, status.Error(codes.Unauthenticated, "ADMIN_SESSION_EXPIRED")
	}
	if session.Role != bank.IdentityRoleBankAdmin {
		return bank.AdminSession{}, status.Error(codes.PermissionDenied, "ADMIN_ROLE_REQUIRED")
	}
	return session, nil
}

func (h *Handler) GetAdminOverview(ctx context.Context, req *pb.AdminOverviewRequest) (*pb.AdminOverviewResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	result, err := h.bank.GetAdminOverview(ctx)
	if err != nil {
		return nil, toStatusError("admin overview", err)
	}
	return &pb.AdminOverviewResponse{
		TotalUsers:            result.TotalUsers,
		ActiveUsers:           result.ActiveUsers,
		TotalAccounts:         result.TotalAccounts,
		TotalBalance:          result.TotalBalance,
		TotalTransactions:     result.TotalTransactions,
		CompletedTransactions: result.CompletedTransactions,
		FailedTransactions:    result.FailedTransactions,
		AuditEvents_24H:       result.AuditEvents24h,
	}, nil
}

func (h *Handler) ListAdminUsers(ctx context.Context, req *pb.ListAdminUsersRequest) (*pb.ListAdminUsersResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	statusFilter := userStatusFilter(req.GetStatus())
	if req.GetStatus() != pb.UserStatus_USER_STATUS_UNKNOWN && statusFilter == "" {
		return nil, status.Error(codes.InvalidArgument, "INVALID_FILTER")
	}
	result, err := h.bank.ListAdminUsers(ctx, bank.AdminUsersFilter{
		Email: req.GetEmail(), Status: statusFilter,
		Limit: int(req.GetLimit()), Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, toStatusError("admin users", err)
	}
	users := make([]*pb.AdminUser, 0, len(result.Users))
	for _, user := range result.Users {
		users = append(users, &pb.AdminUser{
			UserId: user.UserID, Email: user.Email, FullName: user.FullName,
			Status: userStatus(user.Status), AccountCount: user.AccountCount,
			TotalBalance: user.TotalBalance, CreatedAtUnix: user.CreatedAt.Unix(),
		})
	}
	return &pb.ListAdminUsersResponse{Users: users, Total: result.Total, Limit: int32(result.Limit), Offset: int32(result.Offset)}, nil
}

func (h *Handler) ListAdminUserAccounts(ctx context.Context, req *pb.ListAdminUserAccountsRequest) (*pb.ListAdminUserAccountsResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	result, err := h.bank.ListAdminUserAccounts(ctx, req.GetUserId())
	if err != nil {
		return nil, toStatusError("admin user accounts", err)
	}
	accounts := make([]*pb.AdminAccount, 0, len(result))
	for _, account := range result {
		accounts = append(accounts, &pb.AdminAccount{
			AccountId: account.AccountID, AccountNumber: account.AccountNumber,
			Balance: account.Balance, Currency: account.Currency,
			Status: accountStatus(account.Status), CreatedAtUnix: account.CreatedAt.Unix(),
		})
	}
	return &pb.ListAdminUserAccountsResponse{Accounts: accounts}, nil
}

func (h *Handler) ListAdminTransactions(ctx context.Context, req *pb.ListAdminTransactionsRequest) (*pb.ListAdminTransactionsResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	statusFilter := transactionStatusFilter(req.GetStatus())
	if req.GetStatus() != pb.TransactionStatus_TRANSACTION_STATUS_UNKNOWN && statusFilter == "" {
		return nil, status.Error(codes.InvalidArgument, "INVALID_FILTER")
	}
	result, err := h.bank.ListAdminTransactions(ctx, bank.AdminTransactionsFilter{
		AccountID: req.GetAccountId(), Status: statusFilter,
		FromUnix: req.GetFromUnix(), ToUnix: req.GetToUnix(),
		Limit: int(req.GetLimit()), Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, toStatusError("admin transactions", err)
	}
	transactions := make([]*pb.AdminTransaction, 0, len(result.Transactions))
	for _, transaction := range result.Transactions {
		transactions = append(transactions, &pb.AdminTransaction{
			TransactionId:     transaction.TransactionID,
			FromAccountNumber: transaction.FromAccountNumber,
			ToAccountNumber:   transaction.ToAccountNumber,
			Amount:            transaction.Amount, Currency: transaction.Currency,
			Status: transactionStatus(transaction.Status), Description: transaction.Description,
			CertSerial: transaction.CertSerial, CurrentHash: transaction.CurrentHash,
			CreatedAtUnix: transaction.CreatedAt.Unix(),
		})
	}
	return &pb.ListAdminTransactionsResponse{Transactions: transactions, Total: result.Total, Limit: int32(result.Limit), Offset: int32(result.Offset)}, nil
}

func (h *Handler) VerifyAuditChain(ctx context.Context, req *pb.VerifyAuditChainRequest) (*pb.VerifyAuditChainResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	result, err := h.bank.VerifyAuditChain(ctx)
	if err != nil {
		return nil, toStatusError("verify audit chain", err)
	}
	return &pb.VerifyAuditChainResponse{
		Ok:        result.OK,
		Checked:   int32(result.Checked),
		BrokenSeq: result.BrokenSeq,
		BrokenId:  result.BrokenID,
		Detail:    result.Detail,
	}, nil
}

func (h *Handler) ListAdminAuditEvents(ctx context.Context, req *pb.ListAdminAuditEventsRequest) (*pb.ListAdminAuditEventsResponse, error) {
	if _, err := h.requireAdminSession(ctx, req.GetAdminSessionToken()); err != nil {
		return nil, err
	}
	actionFilter := auditActionFilter(req.GetAction())
	if req.GetAction() != pb.BankAuditAction_BANK_AUDIT_ACTION_UNKNOWN && actionFilter == "" {
		return nil, status.Error(codes.InvalidArgument, "INVALID_FILTER")
	}
	result, err := h.bank.ListAdminAuditEvents(ctx, bank.AdminAuditFilter{
		Action: actionFilter, UserID: req.GetUserId(),
		CertSerial: req.GetCertSerial(), RequestID: req.GetRequestId(),
		FromUnix: req.GetFromUnix(), ToUnix: req.GetToUnix(),
		Limit: int(req.GetLimit()), Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, toStatusError("admin audit", err)
	}
	events := make([]*pb.AdminAuditEvent, 0, len(result.Events))
	for _, event := range result.Events {
		events = append(events, &pb.AdminAuditEvent{
			EventId: event.EventID, Action: auditAction(event.Action), UserId: event.UserID,
			AccountId: event.AccountID, TransactionId: event.TransactionID,
			CertSerial: event.CertSerial, RequestId: event.RequestID, Reason: event.Reason,
			MetadataJson: event.MetadataJSON, CreatedAtUnix: event.CreatedAt.Unix(),
		})
	}
	return &pb.ListAdminAuditEventsResponse{Events: events, Total: result.Total, Limit: int32(result.Limit), Offset: int32(result.Offset)}, nil
}

func userStatusFilter(value pb.UserStatus) string {
	switch value {
	case pb.UserStatus_USER_STATUS_ACTIVE:
		return "active"
	case pb.UserStatus_USER_STATUS_LOCKED:
		return "locked"
	default:
		return ""
	}
}

func userStatus(value string) pb.UserStatus {
	if value == "active" {
		return pb.UserStatus_USER_STATUS_ACTIVE
	}
	if value == "locked" {
		return pb.UserStatus_USER_STATUS_LOCKED
	}
	return pb.UserStatus_USER_STATUS_UNKNOWN
}

func transactionStatusFilter(value pb.TransactionStatus) string {
	switch value {
	case pb.TransactionStatus_TRANSACTION_STATUS_PENDING:
		return "pending"
	case pb.TransactionStatus_TRANSACTION_STATUS_COMPLETED:
		return "completed"
	case pb.TransactionStatus_TRANSACTION_STATUS_FAILED:
		return "failed"
	default:
		return ""
	}
}

func auditActionFilter(value pb.BankAuditAction) string {
	switch value {
	case pb.BankAuditAction_BANK_AUDIT_ACTION_TRANSFER_COMPLETED:
		return "transfer_completed"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_TRANSFER_REJECTED:
		return "transfer_rejected"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_REPLAY_DETECTED:
		return "replay_detected"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_INVALID_SIGNATURE:
		return "invalid_signature"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_CERTIFICATE_REJECTED:
		return "certificate_rejected"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_FORBIDDEN_OWNERSHIP:
		return "forbidden_ownership"
	case pb.BankAuditAction_BANK_AUDIT_ACTION_INSUFFICIENT_FUNDS:
		return "insufficient_funds"
	default:
		return ""
	}
}

func auditAction(value string) pb.BankAuditAction {
	switch value {
	case "transfer_completed":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_TRANSFER_COMPLETED
	case "transfer_rejected":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_TRANSFER_REJECTED
	case "replay_detected":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_REPLAY_DETECTED
	case "invalid_signature":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_INVALID_SIGNATURE
	case "certificate_rejected":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_CERTIFICATE_REJECTED
	case "forbidden_ownership":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_FORBIDDEN_OWNERSHIP
	case "insufficient_funds":
		return pb.BankAuditAction_BANK_AUDIT_ACTION_INSUFFICIENT_FUNDS
	default:
		return pb.BankAuditAction_BANK_AUDIT_ACTION_UNKNOWN
	}
}
