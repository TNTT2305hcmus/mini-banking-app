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
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mini-banking/pkg/pb/bank"
	capb "mini-banking/pkg/pb/ca"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type memoryReplay struct {
	mu   sync.Mutex
	keys map[string]bool
	fail bool
}

func (r *memoryReplay) SetNX(_ context.Context, key string, _ string, _ time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return false, fmt.Errorf("redis down")
	}
	if r.keys == nil {
		r.keys = map[string]bool{}
	}
	if r.keys[key] {
		return false, nil
	}
	r.keys[key] = true
	return true, nil
}

type mockCA struct {
	resp *capb.VerifyCertificateResponse
	err  error
}

func (m mockCA) VerifyCertificate(context.Context, *capb.VerifyCertificateRequest, ...grpc.CallOption) (*capb.VerifyCertificateResponse, error) {
	return m.resp, m.err
}
func (m mockCA) RegisterUser(context.Context, *capb.RegisterUserRequest, ...grpc.CallOption) (*capb.RegisterUserResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) GetCertificate(context.Context, *capb.GetCertificateRequest, ...grpc.CallOption) (*capb.GetCertificateResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) CheckRevocation(context.Context, *capb.CheckRevocationRequest, ...grpc.CallOption) (*capb.CheckRevocationResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) ListCertificates(context.Context, *capb.ListCertificatesRequest, ...grpc.CallOption) (*capb.ListCertificatesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) GetCertificateDetail(context.Context, *capb.GetCertificateDetailRequest, ...grpc.CallOption) (*capb.GetCertificateDetailResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) RevokeCertificate(context.Context, *capb.RevokeCertificateRequest, ...grpc.CallOption) (*capb.RevokeCertificateResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) ListAuditEvents(context.Context, *capb.ListAuditEventsRequest, ...grpc.CallOption) (*capb.ListAuditEventsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) AppendAuditEvent(context.Context, *capb.AppendAuditEventRequest, ...grpc.CallOption) (*capb.AppendAuditEventResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m mockCA) VerifyAuditChain(context.Context, *capb.VerifyAuditChainRequest, ...grpc.CallOption) (*capb.VerifyAuditChainResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

type hex64Arg struct{}

func (hex64Arg) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok || len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

type bankHarness struct {
	handler   *Handler
	mock      sqlmock.Sqlmock
	replay    *memoryReplay
	now       time.Time
	bankKey   []byte
	session   []byte
	userID    string
	certSN    string
	private   *rsa.PrivateKey
	publicPEM string
}

func newBankHarness(t *testing.T, certStatus capb.CertStatus) *bankHarness {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	now := time.Unix(1715500700, 0).UTC()
	userID := "11111111-1111-4111-8111-111111111111"
	certSN := "cert-serial-001"
	replay := &memoryReplay{keys: map[string]bool{}}
	resp := &capb.VerifyCertificateResponse{
		Status:             certStatus,
		OwnerId:            userID,
		PublicKeyPem:       pubPEM,
		CertType:           "client",
		IssuerId:           "client-ca",
		IssuerCommonName:   "Mini_App_Banking Client CA",
		IssuerSerialNumber: "02",
		ChainFingerprints:  []string{"client-ca-fingerprint", "root-ca-fingerprint"},
		NotBeforeUnix:      now.Add(-time.Hour).Unix(),
		NotAfterUnix:       now.Add(time.Hour).Unix(),
	}
	if certStatus == capb.CertStatus_CERT_STATUS_REVOKED {
		resp.RevokedAtUnix = now.Add(-time.Minute).Unix()
	}
	h := &bankHarness{
		mock:      mock,
		replay:    replay,
		now:       now,
		bankKey:   fixtureKey("bank-kv"),
		session:   fixtureKey("client-bank-session"),
		userID:    userID,
		certSN:    certSN,
		private:   priv,
		publicPEM: pubPEM,
	}
	h.handler = NewHandlerWithDeps(db, replay, mockCA{resp: resp}, h.bankKey, fixedClock{now: now}, bytes.NewReader(bytes.Repeat([]byte{0x55}, 4096)))
	return h
}

func TestTransferHappyPathWritesLedgerHashChain(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-happy", "request-happy", "idem-happy", scopeTransfer, 1000)

	h.expectNoIdempotent("idem-happy")
	h.mock.ExpectBegin()
	h.expectAccountQuery("from-acc", h.userID, "active", "1000000001", 5000, 10000, "VND", "active")
	h.expectAccountQuery("to-acc", "22222222-2222-4222-8222-222222222222", "active", "1000000002", 1000, 10000, "VND", "active")
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM(amount), 0)`)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(0)))
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE`)).
		WillReturnRows(sqlmock.NewRows([]string{"last_hash"}).AddRow("genesis"))
	h.mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET balance = balance - $1 WHERE id = $2`)).
		WithArgs(int64(1000), "from-acc").
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET balance = balance + $1 WHERE id = $2`)).
		WithArgs(int64(1000), "to-acc").
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO transactions(`)).
		WithArgs(sqlmock.AnyArg(), "from-acc", "to-acc", "1000000001", "1000000002",
			int64(1000), "VND", "", hex64Arg{}, sqlmock.AnyArg(), h.certSN, scopeTransfer,
			"nonce-happy", "idem-happy", "genesis", hex64Arg{}, h.now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectExec(regexp.QuoteMeta(`UPDATE ledger_state SET last_hash = $1, last_transaction_id = $2, updated_at = $3 WHERE id = 'main'`)).
		WithArgs(hex64Arg{}, sqlmock.AnyArg(), h.now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectCommit()
	h.expectAudit()

	resp, err := h.handler.TransferMoney(context.Background(), req)
	if err != nil {
		t.Fatalf("TransferMoney() error = %v", err)
	}
	if resp.GetTransactionId() == "" || len(resp.GetApRep()) == 0 {
		t.Fatalf("missing transaction id or ap_rep")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestReplayAttackRejectsDuplicateAuthenticator(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-replay", "request-replay", "idem-replay", scopeTransfer, 1000)
	auth := h.authInfoFor("nonce-replay", "request-replay")
	_, _ = h.replay.SetNX(context.Background(), replayKey(auth), "1", replayTTL)
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if err == nil {
		t.Fatalf("TransferMoney() error = nil, want replay error")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestIdempotencyReturnsPreviousCompletedTransaction(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-idem", "request-idem", "idem-dup", scopeTransfer, 1000)
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, status FROM transactions WHERE idempotency_key = $1`)).
		WithArgs("idem-dup").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("tx-existing", "completed"))

	resp, err := h.handler.TransferMoney(context.Background(), req)
	if err != nil {
		t.Fatalf("TransferMoney() error = %v", err)
	}
	if resp.GetTransactionId() != "tx-existing" {
		t.Fatalf("transaction id = %q, want tx-existing", resp.GetTransactionId())
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestInsufficientFundsRejectsAndAudits(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-funds", "request-funds", "idem-funds", scopeTransfer, 9000)
	h.expectNoIdempotent("idem-funds")
	h.mock.ExpectBegin()
	h.expectAccountQuery("from-acc", h.userID, "active", "1000000001", 1000, 10000, "VND", "active")
	h.expectAccountQuery("to-acc", "22222222-2222-4222-8222-222222222222", "active", "1000000002", 1000, 10000, "VND", "active")
	h.expectFailedTransaction("from-acc", "to-acc", 9000, "nonce-funds", "idem-funds")
	h.mock.ExpectCommit()
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if err == nil {
		t.Fatalf("TransferMoney() error = nil, want insufficient funds")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDailyLimitExceededRejectsAndAudits(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-limit", "request-limit", "idem-limit", scopeTransfer, 2000)
	h.expectNoIdempotent("idem-limit")
	h.mock.ExpectBegin()
	h.expectAccountQuery("from-acc", h.userID, "active", "1000000001", 5000, 2500, "VND", "active")
	h.expectAccountQuery("to-acc", "22222222-2222-4222-8222-222222222222", "active", "1000000002", 1000, 10000, "VND", "active")
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM(amount), 0)`)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(1000)))
	h.expectFailedTransaction("from-acc", "to-acc", 2000, "nonce-limit", "idem-limit")
	h.mock.ExpectCommit()
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if err == nil {
		t.Fatalf("TransferMoney() error = nil, want daily limit error")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestMissingSourceAccountPersistsFailedTransaction(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-source", "request-source", "idem-source", scopeTransfer, 1000)
	h.expectNoIdempotent("idem-source")
	h.mock.ExpectBegin()
	h.expectMissingAccountQuery("from-acc")
	h.expectFailedTransaction("", "", 1000, "nonce-source", "idem-source")
	h.mock.ExpectCommit()
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("TransferMoney() code = %v, want NotFound", status.Code(err))
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestMissingDestinationAccountPersistsFailedTransaction(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-destination", "request-destination", "idem-destination", scopeTransfer, 1000)
	h.expectNoIdempotent("idem-destination")
	h.mock.ExpectBegin()
	h.expectAccountQuery("from-acc", h.userID, "active", "1000000001", 5000, 10000, "VND", "active")
	h.expectMissingAccountQuery("to-acc")
	h.expectFailedTransaction("from-acc", "", 1000, "nonce-destination", "idem-destination")
	h.mock.ExpectCommit()
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("TransferMoney() code = %v, want NotFound", status.Code(err))
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestIdempotencyRejectsPreviousFailedTransaction(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-idem-failed", "request-idem-failed", "idem-failed", scopeTransfer, 1000)
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, status FROM transactions WHERE idempotency_key = $1`)).
		WithArgs("idem-failed").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("tx-failed", "failed"))
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "IDEMPOTENCY_FAILED" {
		t.Fatalf("TransferMoney() error = %v, want IDEMPOTENCY_FAILED", err)
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRevokedCertificateRejects(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_REVOKED)
	req := h.transferRequest(t, "nonce-cert", "request-cert", "idem-cert", scopeTransfer, 1000)
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if err == nil {
		t.Fatalf("TransferMoney() error = nil, want cert rejection")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestTicketCertificateMetadataMismatchRejects(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	req := h.transferRequest(t, "nonce-chain", "request-chain", "idem-chain", scopeTransfer, 1000)
	req.TicketV = h.ticketWithIssuer(t, scopeTransfer, "grpc-ca")
	h.expectAudit()

	_, err := h.handler.TransferMoney(context.Background(), req)
	if err == nil {
		t.Fatalf("TransferMoney() error = nil, want certificate metadata mismatch")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestWrongScopeBalanceRejects(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	ticket := h.ticket(t, scopeHistory)
	auth := h.authenticator(t, "nonce-scope", "request-scope")
	h.expectAudit()

	_, err := h.handler.GetBalance(context.Background(), &pb.BalanceRequest{TicketV: ticket, Authenticator: auth, AccountId: "from-acc"})
	if err == nil {
		t.Fatalf("GetBalance() error = nil, want wrong scope")
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func (h *bankHarness) expectNoIdempotent(key string) {
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text, status FROM transactions WHERE idempotency_key = $1`)).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
}

func (h *bankHarness) expectAccountQuery(accountID, userID, userStatus, number string, balance, limit int64, currency, status string) {
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id::text, a.user_id::text, u.status, a.account_number, a.balance,`)).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "user_status", "account_number", "balance", "daily_transfer_limit", "currency", "status"}).
			AddRow(accountID, userID, userStatus, number, balance, limit, currency, status))
}

func (h *bankHarness) expectMissingAccountQuery(accountNumber string) {
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id::text, a.user_id::text, u.status, a.account_number, a.balance,`)).
		WithArgs(accountNumber).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "user_status", "account_number", "balance", "daily_transfer_limit", "currency", "status"}))
}

func (h *bankHarness) expectFailedTransaction(fromAccountID, toAccountID string, amount int64, nonce, idempotencyKey string) {
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_hash FROM ledger_state WHERE id = 'main' FOR UPDATE`)).
		WillReturnRows(sqlmock.NewRows([]string{"last_hash"}).AddRow("genesis"))
	h.mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO transactions(`)).
		WithArgs(sqlmock.AnyArg(), fromAccountID, toAccountID, "from-acc", "to-acc",
			amount, "VND", "", hex64Arg{}, sqlmock.AnyArg(), h.certSN,
			scopeTransfer, nonce, idempotencyKey, "genesis", hex64Arg{}, h.now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectExec(regexp.QuoteMeta(`UPDATE ledger_state SET last_hash = $1, last_transaction_id = $2, updated_at = $3 WHERE id = 'main'`)).
		WithArgs(hex64Arg{}, sqlmock.AnyArg(), h.now).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func (h *bankHarness) expectAudit() {
	// Audit inserts now run in their own transaction with a hash-chain: lock the
	// chain, read the previous hash, insert the linked row, commit.
	h.mock.ExpectBegin()
	h.mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT hash FROM bank_audit_log ORDER BY seq DESC LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"hash"}))
	h.mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO bank_audit_log(action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata, prev_hash, hash)`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	h.mock.ExpectCommit()
}

func (h *bankHarness) transferRequest(t *testing.T, nonce, requestID, idemKey, scope string, amount int64) *pb.TransferRequest {
	t.Helper()
	payload := map[string]any{
		"amount":              amount,
		"currency":            "VND",
		"from_account_number": "from-acc",
		"idempotency_key":     idemKey,
		"request_id":          requestID,
		"scope":               scope,
		"timestamp":           h.now.Unix(),
		"to_account_number":   "to-acc",
	}
	canonical, err := canonicalJSON(mustJSON(t, payload))
	if err != nil {
		t.Fatalf("canonicalJSON() error = %v", err)
	}
	sum := sha256.Sum256(canonical)
	signature, err := rsa.SignPSS(rand.Reader, h.private, cryptoHash(), sum[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	if err != nil {
		t.Fatalf("SignPSS() error = %v", err)
	}
	body := map[string]any{
		"payload":          payload,
		"client_signature": base64.StdEncoding.EncodeToString(signature),
	}
	iv := bytes.Repeat([]byte{0x22}, 12)
	cipherPayload := mustEncryptDetached(t, h.session, iv, mustJSON(t, body))
	return &pb.TransferRequest{
		TicketV:       h.ticket(t, scopeTransfer),
		Authenticator: h.authenticator(t, nonce, requestID),
		CipherPayload: cipherPayload,
		Iv:            iv,
	}
}

func (h *bankHarness) ticket(t *testing.T, scope string) []byte {
	return h.ticketWithIssuer(t, scope, "client-ca")
}

func (h *bankHarness) ticketWithIssuer(t *testing.T, scope, issuerID string) []byte {
	t.Helper()
	return mustEncryptEmbedded(t, h.bankKey, map[string]any{
		"client_id":            h.userID,
		"k_c_v":                base64.StdEncoding.EncodeToString(h.session),
		"cert_sn":              h.certSN,
		"cert_type":            "client",
		"issuer_id":            issuerID,
		"issuer_common_name":   "Mini_App_Banking Client CA",
		"issuer_serial_number": "02",
		"chain_fingerprints":   []string{"client-ca-fingerprint", "root-ca-fingerprint"},
		"scope":                scope,
		"expires_at":           h.now.Add(time.Minute).Unix(),
	})
}

func (h *bankHarness) authenticator(t *testing.T, nonce, requestID string) []byte {
	t.Helper()
	return mustEncryptEmbedded(t, h.session, map[string]any{
		"client_id":  h.userID,
		"nonce":      nonce,
		"request_id": requestID,
		"ts_3":       h.now.Unix(),
	})
}

func (h *bankHarness) authInfoFor(nonce, requestID string) authInfo {
	return authInfo{
		ticket:     ticketInfo{clientID: h.userID},
		nonce:      nonce,
		requestID:  requestID,
		timestamp:  h.now,
		sessionKey: h.session,
	}
}

func replayKey(auth authInfo) string {
	hash := sha256.Sum256([]byte(auth.ticket.clientID + "|" + auth.nonce + "|" + fmt.Sprint(auth.timestamp.Unix()) + "|" + auth.requestID))
	return "replay:bank:" + fmt.Sprintf("%x", hash[:])
}

func fixtureKey(label string) []byte {
	sum := sha256.Sum256([]byte("bank-test:" + label))
	return sum[:]
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsonMarshal(v)
	if err != nil {
		t.Fatalf("json marshal error = %v", err)
	}
	return b
}

func jsonMarshal(v any) ([]byte, error) {
	return canonicalJSONFromValue(v)
}

func canonicalJSONFromValue(v any) ([]byte, error) {
	return json.Marshal(v)
}

func cryptoHash() crypto.Hash {
	return crypto.SHA256
}

func mustEncryptEmbedded(t *testing.T, key []byte, v any) []byte {
	t.Helper()
	out, err := encryptAESGCMEmbedded(key, mustJSON(t, v), bytes.NewReader(bytes.Repeat([]byte{0x11}, 1024)))
	if err != nil {
		t.Fatalf("encryptAESGCMEmbedded() error = %v", err)
	}
	return out
}

func mustEncryptDetached(t *testing.T, key, nonce, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM() error = %v", err)
	}
	return gcm.Seal(nil, nonce, plaintext, nil)
}
