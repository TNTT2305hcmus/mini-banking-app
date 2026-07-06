package grpc

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/banking-service/internal/bank"
	pb "mini-banking/pkg/pb/bank"
	capb "mini-banking/pkg/pb/ca"
)

const (
	aes256KeySize   = 32
	freshnessWindow = 5 * time.Minute
	replayTTL       = 5 * time.Minute

	scopeTransfer = "transfer:create"
	scopeBalance  = "balance:read"
	scopeHistory  = "history:read"
)

type Handler struct {
	pb.UnimplementedBankServiceServer
	db     *sql.DB
	replay replayStore
	ca     capb.CAServiceClient
	bankKV []byte
	clock  clock
	random io.Reader
	bank   *bank.Service
}

func NewHandler(db *sql.DB, redisClient *redis.Client, caClient capb.CAServiceClient, bankKVKey []byte) *Handler {
	var replay replayStore
	if redisClient != nil {
		replay = redisReplayStore{client: redisClient}
	}
	return NewHandlerWithDeps(db, replay, caClient, bankKVKey, realClock{}, rand.Reader)
}

func NewHandlerWithDeps(db *sql.DB, replay replayStore, caClient capb.CAServiceClient, bankKVKey []byte, clk clock, random io.Reader) *Handler {
	if clk == nil {
		clk = realClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Handler{db: db, replay: replay, ca: caClient, bankKV: bankKVKey, clock: clk, random: random, bank: bank.NewService(db, clk)}
}

func (h *Handler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	result, err := h.bank.CreateUser(ctx, bank.CreateUserInput{
		UserID:       req.GetUserId(),
		SubjectEmail: req.GetSubjectEmail(),
		FullName:     req.GetFullName(),
	})
	if err != nil {
		return nil, toStatusError("create user", err)
	}
	return &pb.CreateUserResponse{
		UserId:        result.UserID,
		Status:        result.Status,
		CreatedAtUnix: result.CreatedAt.Unix(),
	}, nil
}

// ListAuditEvents serves the admin dashboard read path. It bypasses the AP
// exchange on purpose: the RPC is only reachable on the internal TLS network
// and admin authn/authz is enforced by the API Gateway before forwarding.
func (h *Handler) ListAuditEvents(ctx context.Context, req *pb.ListAuditEventsRequest) (*pb.ListAuditEventsResponse, error) {
	filter := bank.AuditListFilter{
		Action:     req.GetAction(),
		UserID:     req.GetUserId(),
		CertSerial: req.GetCertSerial(),
		RequestID:  req.GetRequestId(),
		Limit:      int(req.GetLimit()),
		Offset:     int(req.GetOffset()),
	}
	if req.GetFromUnix() > 0 {
		filter.From = time.Unix(req.GetFromUnix(), 0).UTC()
	}
	if req.GetToUnix() > 0 {
		filter.To = time.Unix(req.GetToUnix(), 0).UTC()
	}
	records, total, err := h.bank.ListAuditEvents(ctx, filter)
	if err != nil {
		return nil, toStatusError("list audit events", err)
	}
	events := make([]*pb.BankAuditRecord, 0, len(records))
	for _, rec := range records {
		events = append(events, &pb.BankAuditRecord{
			Id:            rec.ID,
			Action:        rec.Action,
			UserId:        rec.UserID,
			AccountId:     rec.AccountID,
			TransactionId: rec.TransactionID,
			CertSerial:    rec.CertSerial,
			RequestId:     rec.RequestID,
			Reason:        rec.Reason,
			MetadataJson:  rec.MetadataJSON,
			CreatedAtUnix: rec.CreatedAt.Unix(),
		})
	}
	limit := int32(filter.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := int32(filter.Offset)
	if offset < 0 {
		offset = 0
	}
	return &pb.ListAuditEventsResponse{Events: events, Total: int32(total), Limit: limit, Offset: offset}, nil
}

func toStatusError(operation string, err error) error {
	switch {
	case errors.Is(err, bank.ErrNotConfigured):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, bank.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, bank.ErrUserExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, bank.ErrAccountNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, bank.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, bank.ErrAccountNotActive),
		errors.Is(err, bank.ErrInsufficientFunds),
		errors.Is(err, bank.ErrDailyLimitExceeded),
		errors.Is(err, bank.ErrIdempotency):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, bank.ErrBadRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}

func (h *Handler) TransferMoney(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	auth, err := h.authorize(ctx, req.GetTicketV(), req.GetAuthenticator(), scopeTransfer)
	if err != nil {
		return nil, err
	}

	transfer, canonical, signature, err := h.decryptTransferPayload(req.GetCipherPayload(), req.GetIv(), auth.sessionKey)
	if err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "invalid_payload"})
		return nil, status.Error(codes.InvalidArgument, "invalid transfer payload")
	}
	if transfer.Scope != "" && transfer.Scope != scopeTransfer {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "wrong_payload_scope"})
		return nil, status.Error(codes.PermissionDenied, "WRONG_SCOPE")
	}
	if transfer.RequestID != "" && transfer.RequestID != auth.requestID {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "request_id_mismatch"})
		return nil, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	if err := verifyRSAPSS(auth.publicKeyPEM, canonical, signature); err != nil {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "invalid_signature", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "payload_signature_invalid"})
		return nil, status.Error(codes.Unauthenticated, "INVALID_SIGNATURE")
	}
	if err := h.markReplay(ctx, auth); err != nil {
		return nil, err
	}

	txID, err := h.bank.Transfer(ctx, callerFrom(auth), bank.TransferInput{
		FromAccountNumber: transfer.FromAccountNumber,
		ToAccountNumber:   transfer.ToAccountNumber,
		Amount:         transfer.Amount,
		Currency:       transfer.Currency,
		Description:    transfer.Description,
		IdempotencyKey: transfer.IdempotencyKey,
		Canonical:      canonical,
		Signature:      signature,
	})
	if err != nil {
		return nil, toStatusError("transfer", err)
	}

	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{"result": "ok", "tx_id": txID, "nonce": auth.nonce, "request_id": auth.requestID})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.TransferResponse{ApRep: apRep, TransactionId: txID}, nil
}

func (h *Handler) GetBalance(ctx context.Context, req *pb.BalanceRequest) (*pb.BalanceResponse, error) {
	auth, err := h.authorize(ctx, req.GetTicketV(), req.GetAuthenticator(), scopeBalance)
	if err != nil {
		return nil, err
	}
	if err := h.markReplay(ctx, auth); err != nil {
		return nil, err
	}

	res, err := h.bank.GetBalance(ctx, callerFrom(auth), req.GetAccountId())
	if err != nil {
		return nil, toStatusError("balance", err)
	}

	// ap_rep là JSON tự do mã hóa bằng K_{c,v}: nhồi đầy đủ profile user + tài khoản
	// (gồm full_name/email/daily_transfer_limit không có trong BalanceResponse proto)
	// để luồng /auth/me lấy thông tin cá nhân mà không cần đổi proto.
	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{
		"result":               "ok",
		"account_id":           res.AccountID,
		"nonce":                auth.nonce,
		"request_id":           auth.requestID,
		"full_name":            res.FullName,
		"email":                res.Email,
		"user_status":          res.UserStatus,
		"account_number":       res.AccountNumber,
		"balance":              res.Balance,
		"currency":             res.Currency,
		"account_status":       res.Status,
		"daily_transfer_limit": res.Limit,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.BalanceResponse{
		ApRep:         apRep,
		AccountId:     res.AccountID,
		AccountNumber: res.AccountNumber,
		Balance:       res.Balance,
		Currency:      res.Currency,
		Status:        accountStatus(res.Status),
	}, nil
}

func (h *Handler) GetHistory(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	auth, err := h.authorize(ctx, req.GetTicketV(), req.GetAuthenticator(), scopeHistory)
	if err != nil {
		return nil, err
	}
	if err := h.markReplay(ctx, auth); err != nil {
		return nil, err
	}

	res, err := h.bank.GetHistory(ctx, callerFrom(auth), req.GetAccountId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, toStatusError("history", err)
	}

	records := make([]*pb.TransactionRecord, 0, len(res.Records))
	for _, rec := range res.Records {
		records = append(records, &pb.TransactionRecord{
			TransactionId:     rec.TransactionID,
			FromAccountNumber: rec.FromAccountNumber,
			ToAccountNumber:   rec.ToAccountNumber,
			Amount:            rec.Amount,
			Currency:          rec.Currency,
			Status:            transactionStatus(rec.Status),
			Description:       rec.Description,
			Scope:             rec.Scope,
			CreatedAtUnix:     rec.CreatedAtUnix,
			CompletedAtUnix:   rec.CompletedAtUnix,
		})
	}

	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{"result": "ok", "account_id": req.GetAccountId(), "nonce": auth.nonce, "request_id": auth.requestID})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.HistoryResponse{ApRep: apRep, Transactions: records, Total: res.Total, Limit: int32(res.Limit), Offset: int32(res.Offset)}, nil
}

// callerFrom adapts the authenticated AP-exchange result into the identity the
// business layer needs for ownership checks and audit.
func callerFrom(auth authInfo) bank.Caller {
	return bank.Caller{
		ClientID:  auth.ticket.clientID,
		CertSN:    auth.ticket.certSN,
		Scope:     auth.ticket.scope,
		Nonce:     auth.nonce,
		RequestID: auth.requestID,
	}
}

func accountStatus(statusText string) pb.AccountStatus {
	switch statusText {
	case "active":
		return pb.AccountStatus_ACCOUNT_STATUS_ACTIVE
	case "locked":
		return pb.AccountStatus_ACCOUNT_STATUS_LOCKED
	case "frozen":
		return pb.AccountStatus_ACCOUNT_STATUS_FROZEN
	default:
		return pb.AccountStatus_ACCOUNT_STATUS_UNKNOWN
	}
}

func transactionStatus(statusText string) pb.TransactionStatus {
	switch statusText {
	case "pending":
		return pb.TransactionStatus_TRANSACTION_STATUS_PENDING
	case "completed":
		return pb.TransactionStatus_TRANSACTION_STATUS_COMPLETED
	case "failed":
		return pb.TransactionStatus_TRANSACTION_STATUS_FAILED
	default:
		return pb.TransactionStatus_TRANSACTION_STATUS_UNKNOWN
	}
}
