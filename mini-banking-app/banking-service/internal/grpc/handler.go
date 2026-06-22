package grpc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

type replayStore interface {
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
}

type redisReplayStore struct {
	client *redis.Client
}

func (s redisReplayStore) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, value, ttl).Result()
}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Handler struct {
	pb.UnimplementedBankServiceServer
	db     *sql.DB
	replay replayStore
	ca     capb.CAServiceClient
	bankKV []byte
	clock  clock
	random io.Reader
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
	return &Handler{db: db, replay: replay, ca: caClient, bankKV: bankKVKey, clock: clk, random: random}
}

func (h *Handler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	if h.db == nil {
		return nil, status.Error(codes.FailedPrecondition, "database is not configured")
	}
	if strings.TrimSpace(req.GetEmail()) == "" || strings.TrimSpace(req.GetFullName()) == "" {
		return nil, status.Error(codes.InvalidArgument, "email and full_name are required")
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "begin create user transaction")
	}
	defer rollbackUnlessCommitted(tx)

	userID := newUUID()
	now := h.clock.Now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users(id, email, full_name, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'active', $4, $4)`,
		userID, strings.TrimSpace(req.GetEmail()), strings.TrimSpace(req.GetFullName()), now); err != nil {
		return nil, status.Error(codes.AlreadyExists, "user already exists")
	}

	accountID := newUUID()
	accountNumber := accountNumberFromUser(userID)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO accounts(id, user_id, account_number, balance, daily_transfer_limit, currency, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 50000000, 'VND', 'active', $5, $5)`,
		accountID, userID, accountNumber, int64(50000000), now); err != nil {
		return nil, status.Error(codes.Internal, "create default account")
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "commit create user")
	}
	return &pb.CreateUserResponse{UserId: userID, Status: "active", CreatedAtUnix: now.Unix()}, nil
}

func (h *Handler) TransferMoney(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	auth, err := h.authorize(ctx, req.GetTicketV(), req.GetAuthenticator(), scopeTransfer)
	if err != nil {
		return nil, err
	}

	transfer, canonical, signature, err := h.decryptTransferPayload(req.GetCipherPayload(), req.GetIv(), auth.sessionKey)
	if err != nil {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "invalid_payload"})
		return nil, status.Error(codes.InvalidArgument, "invalid transfer payload")
	}
	if transfer.Scope != "" && transfer.Scope != scopeTransfer {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "wrong_payload_scope"})
		return nil, status.Error(codes.PermissionDenied, "WRONG_SCOPE")
	}
	if transfer.RequestID != "" && transfer.RequestID != auth.requestID {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "request_id_mismatch"})
		return nil, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	if err := verifyRSAPSS(auth.publicKeyPEM, canonical, signature); err != nil {
		h.audit(ctx, auditEvent{Action: "invalid_signature", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "payload_signature_invalid"})
		return nil, status.Error(codes.Unauthenticated, "INVALID_SIGNATURE")
	}
	if err := h.markReplay(ctx, auth); err != nil {
		return nil, err
	}

	txID, err := h.findIdempotentTransaction(ctx, transfer.IdempotencyKey)
	if err != nil {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: err.Error()})
		return nil, status.Error(codes.FailedPrecondition, "IDEMPOTENCY_FAILED")
	}
	if txID != "" {
		apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{"result": "ok", "tx_id": txID, "nonce": auth.nonce, "request_id": auth.requestID})
		if err != nil {
			return nil, status.Error(codes.Internal, "encrypt ap_rep")
		}
		return &pb.TransferResponse{ApRep: apRep, TransactionId: txID}, nil
	}

	txID, err = h.executeTransfer(ctx, auth, transfer, canonical, signature)
	if err != nil {
		return nil, err
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

	var ownerID, accountNumber, currency, statusText string
	var balance int64
	err = h.db.QueryRowContext(ctx,
		`SELECT user_id::text, account_number, balance, currency, status
		 FROM accounts WHERE id = $1`, req.GetAccountId()).
		Scan(&ownerID, &accountNumber, &balance, &currency, &statusText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "NOT_FOUND")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "query balance")
	}
	if ownerID != auth.ticket.clientID {
		h.audit(ctx, auditEvent{Action: "forbidden_ownership", UserID: auth.ticket.clientID, AccountID: req.GetAccountId(), CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "balance_account_owner_mismatch"})
		return nil, status.Error(codes.PermissionDenied, "FORBIDDEN")
	}

	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{"result": "ok", "account_id": req.GetAccountId(), "nonce": auth.nonce, "request_id": auth.requestID})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.BalanceResponse{
		ApRep:         apRep,
		AccountId:     req.GetAccountId(),
		AccountNumber: accountNumber,
		Balance:       balance,
		Currency:      currency,
		Status:        accountStatus(statusText),
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

	var ownerID string
	if err := h.db.QueryRowContext(ctx, `SELECT user_id::text FROM accounts WHERE id = $1`, req.GetAccountId()).Scan(&ownerID); errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "NOT_FOUND")
	} else if err != nil {
		return nil, status.Error(codes.Internal, "query account owner")
	}
	if ownerID != auth.ticket.clientID {
		h.audit(ctx, auditEvent{Action: "forbidden_ownership", UserID: auth.ticket.clientID, AccountID: req.GetAccountId(), CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "history_account_owner_mismatch"})
		return nil, status.Error(codes.PermissionDenied, "FORBIDDEN")
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}

	var total int32
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions WHERE from_account_id = $1 OR to_account_id = $1`,
		req.GetAccountId()).Scan(&total); err != nil {
		return nil, status.Error(codes.Internal, "count history")
	}

	rows, err := h.db.QueryContext(ctx,
		`SELECT id::text, COALESCE(from_account_number, ''), COALESCE(to_account_number, ''),
		        amount, currency, status, COALESCE(description, ''), scope,
		        EXTRACT(EPOCH FROM created_at)::bigint,
		        COALESCE(EXTRACT(EPOCH FROM completed_at)::bigint, 0)
		 FROM transactions
		 WHERE from_account_id = $1 OR to_account_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, req.GetAccountId(), limit, offset)
	if err != nil {
		return nil, status.Error(codes.Internal, "query history")
	}
	defer rows.Close()

	var records []*pb.TransactionRecord
	for rows.Next() {
		var r pb.TransactionRecord
		var statusText string
		if err := rows.Scan(&r.TransactionId, &r.FromAccountNumber, &r.ToAccountNumber, &r.Amount, &r.Currency, &statusText, &r.Description, &r.Scope, &r.CreatedAtUnix, &r.CompletedAtUnix); err != nil {
			return nil, status.Error(codes.Internal, "scan history")
		}
		r.Status = transactionStatus(statusText)
		records = append(records, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Internal, "read history")
	}

	apRep, err := h.encryptAPRep(auth.sessionKey, map[string]any{"result": "ok", "account_id": req.GetAccountId(), "nonce": auth.nonce, "request_id": auth.requestID})
	if err != nil {
		return nil, status.Error(codes.Internal, "encrypt ap_rep")
	}
	return &pb.HistoryResponse{ApRep: apRep, Transactions: records, Total: total, Limit: int32(limit), Offset: int32(offset)}, nil
}

type ticketInfo struct {
	clientID     string
	session      []byte
	certSN       string
	scope        string
	expires      time.Time
	publicKeyPEM string
}

type authInfo struct {
	ticket       ticketInfo
	sessionKey   []byte
	nonce        string
	requestID    string
	timestamp    time.Time
	publicKeyPEM string
}

func (h *Handler) authorize(ctx context.Context, ticketCipher []byte, authenticatorCipher []byte, requiredScope string) (authInfo, error) {
	var out authInfo
	if h.db == nil || h.ca == nil || len(h.bankKV) != aes256KeySize {
		return out, status.Error(codes.FailedPrecondition, "bank handler dependencies are not configured")
	}
	ticket, err := h.decryptTicket(ticketCipher)
	if err != nil {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", Reason: "invalid_ticket"})
		return out, status.Error(codes.Unauthenticated, "INVALID_TICKET")
	}
	out.ticket = ticket
	out.sessionKey = ticket.session
	if h.clock.Now().After(ticket.expires) || h.clock.Now().Equal(ticket.expires) {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "ticket_expired"})
		return out, status.Error(codes.Unauthenticated, "INVALID_TICKET")
	}
	if ticket.scope != requiredScope {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "wrong_scope"})
		return out, status.Error(codes.PermissionDenied, "WRONG_SCOPE")
	}

	authMap, err := decryptJSONMap(ticket.session, authenticatorCipher)
	if err != nil {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "invalid_authenticator"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	clientID := firstString(authMap, "client_id", "id_c", "ID_c")
	if clientID != ticket.clientID {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "authenticator_client_mismatch"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	out.nonce = firstString(authMap, "nonce", "nonce_req")
	out.requestID = firstString(authMap, "request_id", "requestId")
	ts, ok := firstUnix(authMap, "ts_3", "timestamp", "timestamp_unix", "ts")
	if !ok || out.nonce == "" || out.requestID == "" {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "authenticator_missing_fields"})
		return out, status.Error(codes.InvalidArgument, "BAD_REQUEST")
	}
	out.timestamp = time.Unix(ts, 0).UTC()
	if absDuration(h.clock.Now().Sub(out.timestamp)) > freshnessWindow {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "stale_request"})
		return out, status.Error(codes.Unauthenticated, "STALE_REQUEST")
	}

	cert, err := h.ca.VerifyCertificate(ctx, &capb.VerifyCertificateRequest{
		SerialNumber:          ticket.certSN,
		Caller:                "bank-service",
		IncludePublicKeyPem:   true,
		IncludeCertificatePem: false,
	}, grpc.WaitForReady(false))
	if err != nil {
		h.audit(ctx, auditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "ca_unavailable"})
		return out, status.Error(codes.Unavailable, "SERVICE_UNAVAILABLE")
	}
	nowUnix := h.clock.Now().Unix()
	if cert.GetStatus() != capb.CertStatus_CERT_STATUS_ACTIVE ||
		cert.GetRevokedAtUnix() > 0 ||
		cert.GetNotBeforeUnix() > nowUnix ||
		(cert.GetNotAfterUnix() > 0 && nowUnix >= cert.GetNotAfterUnix()) {
		h.audit(ctx, auditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "certificate_not_active"})
		return out, status.Error(codes.Unauthenticated, "CERT_REJECTED")
	}
	if cert.GetOwnerId() != "" && cert.GetOwnerId() != ticket.clientID {
		h.audit(ctx, auditEvent{Action: "certificate_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "certificate_owner_mismatch"})
		return out, status.Error(codes.Unauthenticated, "CERT_REJECTED")
	}
	out.publicKeyPEM = cert.GetPublicKeyPem()
	if out.publicKeyPEM == "" {
		out.publicKeyPEM = ticket.publicKeyPEM
	}
	return out, nil
}

func (h *Handler) decryptTicket(ciphertext []byte) (ticketInfo, error) {
	m, err := decryptJSONMap(h.bankKV, ciphertext)
	if err != nil {
		return ticketInfo{}, err
	}
	clientID := firstString(m, "client_id", "id_c", "ID_c")
	certSN := firstString(m, "cert_sn", "cert_serial", "serial_number")
	scope := firstString(m, "scope")
	publicKeyPEM := firstString(m, "pub_c_pem", "pub_c", "public_key_pem")
	kcv, err := firstBytes(m, "k_c_v", "K_c_v", "kcv", "KCV")
	if err != nil {
		return ticketInfo{}, err
	}
	expiresUnix, ok := firstUnix(m, "expires_at", "expiry", "exp")
	if clientID == "" || certSN == "" || scope == "" || len(kcv) != aes256KeySize || !ok {
		return ticketInfo{}, errors.New("ticket missing required fields")
	}
	return ticketInfo{clientID: clientID, certSN: certSN, scope: scope, session: kcv, expires: time.Unix(expiresUnix, 0).UTC(), publicKeyPEM: publicKeyPEM}, nil
}

type transferPayload struct {
	FromAccountID  string `json:"from_account_id"`
	ToAccountID    string `json:"to_account_id"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Description    string `json:"description,omitempty"`
	Nonce          string `json:"nonce,omitempty"`
	Timestamp      int64  `json:"timestamp,omitempty"`
	RequestID      string `json:"request_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Scope          string `json:"scope"`
}

func (h *Handler) decryptTransferPayload(cipherPayload []byte, iv []byte, sessionKey []byte) (transferPayload, []byte, []byte, error) {
	var raw []byte
	var err error
	if len(iv) > 0 {
		raw, err = decryptAESGCMDetached(sessionKey, iv, cipherPayload)
	} else {
		raw, err = decryptAESGCMEmbedded(sessionKey, cipherPayload)
	}
	if err != nil {
		return transferPayload{}, nil, nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transferPayload{}, nil, nil, err
	}

	var payloadRaw json.RawMessage
	var sigText string
	if v, ok := envelope["payload"]; ok {
		payloadRaw = v
		if s, ok := envelope["client_signature"]; ok {
			_ = json.Unmarshal(s, &sigText)
		}
	} else {
		payloadMap := map[string]json.RawMessage{}
		for k, v := range envelope {
			if k == "client_signature" || k == "signature" {
				_ = json.Unmarshal(v, &sigText)
				continue
			}
			payloadMap[k] = v
		}
		payloadRaw, err = json.Marshal(payloadMap)
		if err != nil {
			return transferPayload{}, nil, nil, err
		}
	}
	if sigText == "" {
		if s, ok := envelope["signature"]; ok {
			_ = json.Unmarshal(s, &sigText)
		}
	}
	signature, err := base64.StdEncoding.DecodeString(sigText)
	if err != nil {
		signature, err = hex.DecodeString(sigText)
	}
	if err != nil || len(signature) == 0 {
		return transferPayload{}, nil, nil, errors.New("missing signature")
	}

	canonical, err := canonicalJSON(payloadRaw)
	if err != nil {
		return transferPayload{}, nil, nil, err
	}
	var payload transferPayload
	if err := json.Unmarshal(canonical, &payload); err != nil {
		return transferPayload{}, nil, nil, err
	}
	if payload.Currency == "" {
		payload.Currency = "VND"
	}
	if payload.Amount <= 0 || payload.FromAccountID == "" || payload.ToAccountID == "" || payload.IdempotencyKey == "" {
		return transferPayload{}, nil, nil, errors.New("invalid transfer fields")
	}
	return payload, canonical, signature, nil
}

func (h *Handler) markReplay(ctx context.Context, auth authInfo) error {
	hash := sha256.Sum256([]byte(auth.ticket.clientID + "|" + auth.nonce + "|" + strconv.FormatInt(auth.timestamp.Unix(), 10) + "|" + auth.requestID))
	key := "replay:bank:" + hex.EncodeToString(hash[:])
	if h.replay != nil {
		ok, err := h.replay.SetNX(ctx, key, "1", replayTTL)
		if err == nil {
			if !ok {
				h.audit(ctx, auditEvent{Action: "replay_detected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "redis_replay"})
				return status.Error(codes.Unauthenticated, "REPLAY_DETECTED")
			}
			return nil
		}
	}
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO used_nonces(nonce, used_at, expires_at) VALUES ($1, $2, $3)`,
		key, h.clock.Now(), h.clock.Now().Add(replayTTL))
	if err != nil {
		h.audit(ctx, auditEvent{Action: "replay_detected", UserID: auth.ticket.clientID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "db_replay"})
		return status.Error(codes.Unauthenticated, "REPLAY_DETECTED")
	}
	return nil
}

func (h *Handler) findIdempotentTransaction(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	var txID, statusText string
	err := h.db.QueryRowContext(ctx, `SELECT id::text, status FROM transactions WHERE idempotency_key = $1`, key).Scan(&txID, &statusText)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if statusText == "completed" {
		return txID, nil
	}
	return "", fmt.Errorf("previous transaction status=%s", statusText)
}

func (h *Handler) executeTransfer(ctx context.Context, auth authInfo, payload transferPayload, canonical []byte, signature []byte) (string, error) {
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", status.Error(codes.Internal, "begin transfer")
	}
	defer rollbackUnlessCommitted(tx)

	type accountRow struct {
		id         string
		userID     string
		userStatus string
		number     string
		balance    int64
		limit      int64
		currency   string
		status     string
	}
	load := func(accountID string) (accountRow, error) {
		var a accountRow
		err := tx.QueryRowContext(ctx,
			`SELECT a.id::text, a.user_id::text, u.status, a.account_number, a.balance,
			        a.daily_transfer_limit, a.currency, a.status
			 FROM accounts a JOIN users u ON u.id = a.user_id
			 WHERE a.id = $1
			 FOR UPDATE`, accountID).
			Scan(&a.id, &a.userID, &a.userStatus, &a.number, &a.balance, &a.limit, &a.currency, &a.status)
		return a, err
	}
	from, err := load(payload.FromAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "from_account_not_found"})
		return "", status.Error(codes.NotFound, "NOT_FOUND")
	}
	if err != nil {
		return "", status.Error(codes.Internal, "query from account")
	}
	to, err := load(payload.ToAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "to_account_not_found"})
		return "", status.Error(codes.NotFound, "NOT_FOUND")
	}
	if err != nil {
		return "", status.Error(codes.Internal, "query to account")
	}
	if from.userID != auth.ticket.clientID {
		h.audit(ctx, auditEvent{Action: "forbidden_ownership", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "from_account_owner_mismatch"})
		return "", status.Error(codes.PermissionDenied, "FORBIDDEN")
	}
	if from.userStatus != "active" || from.status != "active" || to.status != "active" {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "account_not_active"})
		return "", status.Error(codes.FailedPrecondition, "ACCOUNT_NOT_ACTIVE")
	}
	if from.currency != payload.Currency || to.currency != payload.Currency {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "currency_mismatch"})
		return "", status.Error(codes.InvalidArgument, "BAD_REQUEST")
	}
	if from.balance < payload.Amount {
		h.audit(ctx, auditEvent{Action: "insufficient_funds", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "insufficient_funds"})
		return "", status.Error(codes.FailedPrecondition, "INSUFFICIENT_FUNDS")
	}

	startOfDay := h.clock.Now().UTC().Truncate(24 * time.Hour)
	var spentToday int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM transactions
		 WHERE from_account_id = $1 AND status = 'completed' AND created_at >= $2 AND created_at <= $3`,
		payload.FromAccountID, startOfDay, h.clock.Now()).Scan(&spentToday); err != nil {
		return "", status.Error(codes.Internal, "query daily transfer total")
	}
	if spentToday+payload.Amount > from.limit {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "daily_limit_exceeded"})
		return "", status.Error(codes.FailedPrecondition, "DAILY_LIMIT_EXCEEDED")
	}

	var previousHash string
	if err := tx.QueryRowContext(ctx, `SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE`).Scan(&previousHash); err != nil {
		return "", status.Error(codes.Internal, "lock ledger")
	}
	if previousHash == "" {
		previousHash = "genesis"
	}
	txID := newUUID()
	createdAt := h.clock.Now()
	payloadHashBytes := sha256.Sum256(canonical)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])
	signatureText := base64.StdEncoding.EncodeToString(signature)
	currentHashBytes := sha256.Sum256([]byte(previousHash + payloadHash + signatureText + txID + createdAt.Format(time.RFC3339Nano)))
	currentHash := hex.EncodeToString(currentHashBytes[:])

	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET balance = balance - $1 WHERE id = $2`, payload.Amount, payload.FromAccountID); err != nil {
		return "", status.Error(codes.Internal, "debit account")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET balance = balance + $1 WHERE id = $2`, payload.Amount, payload.ToAccountID); err != nil {
		return "", status.Error(codes.Internal, "credit account")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO transactions(
		    id, from_account_id, to_account_id, from_account_number, to_account_number,
		    amount, currency, status, description, payload_hash, client_signature,
		    cert_serial, scope, nonce, idempotency_key, previous_hash, current_hash,
		    created_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $17)`,
		txID, payload.FromAccountID, payload.ToAccountID, from.number, to.number,
		payload.Amount, payload.Currency, payload.Description, payloadHash, signatureText,
		auth.ticket.certSN, auth.ticket.scope, auth.nonce, payload.IdempotencyKey,
		previousHash, currentHash, createdAt); err != nil {
		h.audit(ctx, auditEvent{Action: "transfer_rejected", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "transaction_insert_failed"})
		return "", status.Error(codes.Internal, "insert transaction")
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE ledger_state SET last_hash = $1, last_transaction_id = $2, updated_at = $3 WHERE id = 'main'`,
		currentHash, txID, createdAt); err != nil {
		return "", status.Error(codes.Internal, "update ledger")
	}
	if err := tx.Commit(); err != nil {
		return "", status.Error(codes.Internal, "commit transfer")
	}
	h.audit(ctx, auditEvent{Action: "transfer_completed", UserID: auth.ticket.clientID, AccountID: payload.FromAccountID, TransactionID: txID, CertSerial: auth.ticket.certSN, RequestID: auth.requestID, Reason: "ok"})
	return txID, nil
}

type auditEvent struct {
	Action        string
	UserID        string
	AccountID     string
	TransactionID string
	CertSerial    string
	RequestID     string
	Reason        string
	Metadata      map[string]any
}

func (h *Handler) audit(ctx context.Context, e auditEvent) {
	if h.db == nil || e.Action == "" {
		return
	}
	metadata := []byte("{}")
	if len(e.Metadata) > 0 {
		if b, err := json.Marshal(e.Metadata); err == nil {
			metadata = b
		}
	}
	_, _ = h.db.ExecContext(ctx,
		`INSERT INTO bank_audit_log(action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata)
		 VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8)`,
		e.Action, e.UserID, e.AccountID, e.TransactionID, e.CertSerial, e.RequestID, e.Reason, string(metadata))
}

func (h *Handler) encryptAPRep(key []byte, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encryptAESGCMEmbedded(key, body, h.random)
}

func decryptJSONMap(key []byte, ciphertext []byte) (map[string]any, error) {
	plain, err := decryptAESGCMEmbedded(key, ciphertext)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encryptAESGCMEmbedded(key []byte, plaintext []byte, random io.Reader) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...), nil
}

func decryptAESGCMEmbedded(key []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func decryptAESGCMDetached(key []byte, nonce []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != aes256KeySize {
		return nil, errors.New("AES-256-GCM requires 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce length")
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func verifyRSAPSS(publicKeyPEM string, payload []byte, signature []byte) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return errors.New("invalid public key pem")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if cert, certErr := x509.ParseCertificate(block.Bytes); certErr == nil {
			pubAny = cert.PublicKey
		} else {
			return err
		}
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return errors.New("unsupported public key type")
	}
	sum := sha256.Sum256(payload)
	return rsa.VerifyPSS(pub, crypto.SHA256, sum[:], signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case string:
				return t
			case json.Number:
				return t.String()
			case float64:
				return strconv.FormatInt(int64(t), 10)
			}
		}
	}
	return ""
}

func firstUnix(m map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			n, err := strconv.ParseInt(t, 10, 64)
			return n, err == nil
		case json.Number:
			n, err := t.Int64()
			return n, err == nil
		case float64:
			if math.Trunc(t) == t {
				return int64(t), true
			}
		}
	}
	return 0, false
}

func firstBytes(m map[string]any, keys ...string) ([]byte, error) {
	value := firstString(m, keys...)
	if value == "" {
		return nil, errors.New("missing bytes")
	}
	if b, err := base64.StdEncoding.DecodeString(value); err == nil {
		return b, nil
	}
	if b, err := hex.DecodeString(value); err == nil {
		return b, nil
	}
	return []byte(value), nil
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

func rollbackUnlessCommitted(tx *sql.Tx) {
	_ = tx.Rollback()
}

func newUUID() string {
	var b [16]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func accountNumberFromUser(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	n := int64(sum[0])<<40 | int64(sum[1])<<32 | int64(sum[2])<<24 | int64(sum[3])<<16 | int64(sum[4])<<8 | int64(sum[5])
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("10%010d", n%10000000000)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
