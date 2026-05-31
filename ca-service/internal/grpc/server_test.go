package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	pb "mini-banking/pkg/pb/ca"
)

func TestAuthorizeRevokeInterceptorAllowsConfiguredClientCN(t *testing.T) {
	interceptor := authorizeRevokeInterceptor([]string{"api-gateway"})
	called := false

	resp, err := interceptor(
		contextWithVerifiedClientCN("api-gateway"),
		nil,
		&googlegrpc.UnaryServerInfo{FullMethod: pb.CAService_RevokeCertificate_FullMethodName},
		func(ctx context.Context, req any) (any, error) {
			called = true
			return "ok", nil
		},
	)

	if err != nil {
		t.Fatalf("expected revoke to be authorized: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
	if resp != "ok" {
		t.Fatalf("expected handler response, got %v", resp)
	}
}

func TestAuthorizeRevokeInterceptorRejectsUnauthorizedClientCN(t *testing.T) {
	interceptor := authorizeRevokeInterceptor([]string{"api-gateway"})
	called := false

	_, err := interceptor(
		contextWithVerifiedClientCN("kdc-service"),
		nil,
		&googlegrpc.UnaryServerInfo{FullMethod: pb.CAService_RevokeCertificate_FullMethodName},
		func(ctx context.Context, req any) (any, error) {
			called = true
			return nil, nil
		},
	)

	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v (%v)", status.Code(err), err)
	}
	if called {
		t.Fatal("handler must not be called for unauthorized revoke")
	}
}

func TestAuthorizeRevokeInterceptorRejectsMissingClientCertificate(t *testing.T) {
	interceptor := authorizeRevokeInterceptor([]string{"api-gateway"})
	called := false

	_, err := interceptor(
		context.Background(),
		nil,
		&googlegrpc.UnaryServerInfo{FullMethod: pb.CAService_RevokeCertificate_FullMethodName},
		func(ctx context.Context, req any) (any, error) {
			called = true
			return nil, nil
		},
	)

	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v (%v)", status.Code(err), err)
	}
	if called {
		t.Fatal("handler must not be called without a verified client certificate")
	}
}

func TestAuthorizeRevokeInterceptorAllowsNonRevokeMethods(t *testing.T) {
	interceptor := authorizeRevokeInterceptor([]string{"api-gateway"})
	called := false

	_, err := interceptor(
		context.Background(),
		nil,
		&googlegrpc.UnaryServerInfo{FullMethod: pb.CAService_CheckRevocation_FullMethodName},
		func(ctx context.Context, req any) (any, error) {
			called = true
			return nil, nil
		},
	)

	if err != nil {
		t.Fatalf("expected non-revoke method to pass through: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func contextWithVerifiedClientCN(commonName string) context.Context {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: commonName}}
	tlsInfo := credentials.TLSInfo{
		State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{{cert}},
		},
	}
	return peer.NewContext(context.Background(), &peer.Peer{AuthInfo: tlsInfo})
}
