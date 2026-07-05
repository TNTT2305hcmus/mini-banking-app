package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/ca-service/internal/ca"
	pb "mini_banking/pkg/pb/ca"
)

func TestAdminRevokeRejectsNonClientCertificate(t *testing.T) {
	ctx := context.Background()
	store := ca.NewStore()
	now := time.Now().UTC()
	record := ca.CertificateRecord{
		SerialNumber:      "service-tls-serial",
		CertType:          ca.CertTypeServiceTLS,
		IssuerID:          ca.ClientCAID,
		IssuerCommonName:  "Mini_App_Banking Test Client CA",
		IssuerSerial:      "02",
		SubjectCN:         "banking-service",
		CertificatePEM:    "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n",
		FingerprintSHA256: strings.Repeat("b", 64),
		NotBefore:         now.Add(-time.Minute),
		NotAfter:          now.Add(time.Hour),
		Status:            ca.CertStatusActive,
		IssuedAt:          now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.CreateCertificate(ctx, record); err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	handler := NewHandler(ca.NewService(nil, store, t.TempDir(), 365))
	_, err := handler.RevokeCertificate(ctx, &pb.RevokeCertificateRequest{
		SerialNumber: record.SerialNumber,
		Reason:       "operator_request",
		PerformedBy:  "admin-ca:test",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition for non-client revoke, got err=%v code=%s", err, status.Code(err))
	}

	after, err := store.GetCertificate(ctx, record.SerialNumber)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if after.Status != ca.CertStatusActive || after.RevokedAt != nil {
		t.Fatalf("non-client certificate must remain active, got %+v", after)
	}
}
