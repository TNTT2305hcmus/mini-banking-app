package grpc

import (
	"context"
	"encoding/json"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/banking-service/internal/bank"
	pb "mini-banking/pkg/pb/bank"
	capb "mini-banking/pkg/pb/ca"
)

type memoryAdminSessions struct {
	mu       sync.Mutex
	sessions map[string]bank.AdminSession
}

func (s *memoryAdminSessions) Create(_ context.Context, tokenHash string, session bank.AdminSession, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]bank.AdminSession)
	}
	s.sessions[tokenHash] = session
	return nil
}

func (s *memoryAdminSessions) Get(_ context.Context, tokenHash string) (bank.AdminSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[tokenHash]
	if !ok {
		return bank.AdminSession{}, errAdminSessionNotFound
	}
	return session, nil
}

func setHarnessRole(h *bankHarness, role capb.IdentityRole) {
	h.handler.ca = mockCA{resp: &capb.VerifyCertificateResponse{
		Status:        capb.CertStatus_CERT_STATUS_ACTIVE,
		OwnerId:       h.userID,
		PublicKeyPem:  h.publicPEM,
		NotBeforeUnix: h.now.Add(-time.Hour).Unix(),
		NotAfterUnix:  h.now.Add(time.Hour).Unix(),
		Role:          role,
	}}
}

func TestCreateAdminSession(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	setHarnessRole(h, capb.IdentityRole_IDENTITY_ROLE_BANK_ADMIN)
	sessions := &memoryAdminSessions{}
	h.handler.adminSessions = sessions

	response, err := h.handler.CreateAdminSession(context.Background(), &pb.CreateAdminSessionRequest{
		TicketV:       h.ticket(t, scopeAdminRead),
		Authenticator: h.authenticator(t, "admin-nonce", "admin-request"),
	})
	if err != nil {
		t.Fatalf("CreateAdminSession() error = %v", err)
	}
	if response.GetSessionToken() == "" || response.GetRole() != string(bank.IdentityRoleBankAdmin) {
		t.Fatalf("invalid session response: %+v", response)
	}
	stored, err := sessions.Get(context.Background(), hashAdminSessionToken(response.GetSessionToken()))
	if err != nil || stored.AdminID != h.userID || stored.CertSN != h.certSN {
		t.Fatalf("stored session = %+v, err = %v", stored, err)
	}

	plain, err := decryptAESGCMEmbedded(h.session, response.GetApRep())
	if err != nil {
		t.Fatalf("decrypt ap_rep: %v", err)
	}
	var apRep map[string]any
	if err := json.Unmarshal(plain, &apRep); err != nil || apRep["nonce"] != "admin-nonce" {
		t.Fatalf("invalid ap_rep: %s, err = %v", plain, err)
	}
}

func TestCreateAdminSessionRejectsCustomerRole(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	h.handler.adminSessions = &memoryAdminSessions{}

	_, err := h.handler.CreateAdminSession(context.Background(), &pb.CreateAdminSessionRequest{
		TicketV:       h.ticket(t, scopeAdminRead),
		Authenticator: h.authenticator(t, "customer-nonce", "customer-request"),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateAdminSession() code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestGetAdminOverviewRequiresValidSession(t *testing.T) {
	h := newBankHarness(t, capb.CertStatus_CERT_STATUS_ACTIVE)
	sessions := &memoryAdminSessions{sessions: map[string]bank.AdminSession{}}
	h.handler.adminSessions = sessions

	if _, err := h.handler.GetAdminOverview(context.Background(), &pb.AdminOverviewRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing session code = %v, want Unauthenticated", status.Code(err))
	}

	rawToken := "valid-admin-session"
	sessions.sessions[hashAdminSessionToken(rawToken)] = bank.AdminSession{
		AdminID: h.userID, CertSN: h.certSN, Role: bank.IdentityRoleBankAdmin,
		ExpiresAt: h.now.Add(time.Minute).Unix(),
	}
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT
			(SELECT COUNT(*) FROM users),`)).
		WillReturnRows(h.adminOverviewRows())

	response, err := h.handler.GetAdminOverview(context.Background(), &pb.AdminOverviewRequest{AdminSessionToken: rawToken})
	if err != nil {
		t.Fatalf("GetAdminOverview() error = %v", err)
	}
	if response.GetTotalUsers() != 10 || response.GetAuditEvents_24H() != 8 {
		t.Fatalf("unexpected overview: %+v", response)
	}
	if err := h.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func (h *bankHarness) adminOverviewRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"total_users", "active_users", "total_accounts", "total_balance",
		"total_transactions", "completed_transactions", "failed_transactions", "audit_events_24h",
	}).AddRow(int64(10), int64(9), int64(12), int64(100000), int64(20), int64(18), int64(2), int64(8))
}
