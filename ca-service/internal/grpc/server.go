package grpc

/**
 * @title CA Service - gRPC Server
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Wrapping the standard grpc.Server, setting up health checks, reflection, and handling TCP connections and graceful stops.
 */

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "mini-banking/pkg/pb/ca"
)

/**
 * @description SecurityConfig contains the CA gRPC transport security and authorization settings.
 *
 * @typedef {Object} SecurityConfig
 * @property {string} ServerCertPath - The server certificate path.
 * @property {string} ServerKeyPath - The server private key path.
 * @property {string} ClientCACertPath - The CA certificate used to verify client certificates.
 * @property {[]string} RevokeAllowedClientCNs - Client certificate CNs allowed to call RevokeCertificate.
 */
type SecurityConfig struct {
	ServerCertPath         string
	ServerKeyPath          string
	ClientCACertPath       string
	RevokeAllowedClientCNs []string
}

/**
 * @description Server wraps grpc.Server with configuration for the CA Service.
 *
 * @typedef {Object} Server
 * @property {*grpc.Server} grpcServer - The underlying gRPC server instance.
 * @property {string} port - The port on which the server listens.
 */
type Server struct {
	grpcServer *grpc.Server
	port       string
}

/**
 * @descriptionNewServer creates a new gRPC server equipped with:
 * @note CAService handler for business logic.
 * @note Health check (grpc.health.v1) — used for Docker healthchecks and service mesh routing.
 * @note Reflection — allows using tools like grpcurl to debug without needing the proto file.
 *
 * @function NewServer
 * @param {*Handler} handler - The gRPC handler containing the CA endpoints.
 * @param {string} port - The port string (e.g., "50051").
 * @param {SecurityConfig} security - The gRPC mTLS and authorization config.
 * @returns {(*Server, error)} A pointer to the configured Server, or an error if secure setup fails.
 */
func NewServer(handler *Handler, port string, security SecurityConfig) (*Server, error) {
	transportCreds, err := newMTLSTransportCredentials(security)
	if err != nil {
		return nil, err
	}

	grpcSrv := grpc.NewServer(
		grpc.Creds(transportCreds),
		grpc.UnaryInterceptor(authorizeRevokeInterceptor(security.RevokeAllowedClientCNs)),
	)

	// @note Register CA Service
	pb.RegisterCAServiceServer(grpcSrv, handler)

	// @note Health check — other services (KDC, Bank) use this to verify if the CA is ready.
	// @note Returns SERVING when the CA is operating normally.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("ca.CAService", grpc_health_v1.HealthCheckResponse_SERVING)

	// @note "" represents the overall health of the entire server
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// @note Reflection allows dynamic client interactions:
	// @note grpcurl must use client cert/key and trusted CA options.
	reflection.Register(grpcSrv)

	return &Server{
		grpcServer: grpcSrv,
		port:       port,
	}, nil
}

func newMTLSTransportCredentials(security SecurityConfig) (credentials.TransportCredentials, error) {
	if security.ServerCertPath == "" {
		return nil, fmt.Errorf("GRPC_SERVER_CERT_PATH is required")
	}
	if security.ServerKeyPath == "" {
		return nil, fmt.Errorf("GRPC_SERVER_KEY_PATH is required")
	}
	if security.ClientCACertPath == "" {
		return nil, fmt.Errorf("GRPC_CLIENT_CA_CERT_PATH is required")
	}

	serverCert, err := tls.LoadX509KeyPair(security.ServerCertPath, security.ServerKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load CA gRPC server certificate/key: %w", err)
	}

	clientCAPEM, err := os.ReadFile(security.ClientCACertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA gRPC client CA certificate: %w", err)
	}
	clientCAPool := x509.NewCertPool()
	if !clientCAPool.AppendCertsFromPEM(clientCAPEM) {
		return nil, fmt.Errorf("parse CA gRPC client CA certificate: no certificates found")
	}

	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}), nil
}

func authorizeRevokeInterceptor(allowedClientCNs []string) grpc.UnaryServerInterceptor {
	allowed := make(map[string]struct{}, len(allowedClientCNs))
	for _, cn := range allowedClientCNs {
		if cn != "" {
			allowed[cn] = struct{}{}
		}
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod != pb.CAService_RevokeCertificate_FullMethodName {
			return handler(ctx, req)
		}
		if len(allowed) == 0 {
			return nil, status.Error(codes.PermissionDenied, "revoke authorization is not configured")
		}

		clientCN, err := authenticatedClientCommonName(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "client certificate required: %v", err)
		}
		if _, ok := allowed[clientCN]; !ok {
			return nil, status.Errorf(codes.PermissionDenied, "client %q is not allowed to revoke certificates", clientCN)
		}
		return handler(ctx, req)
	}
}

func authenticatedClientCommonName(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing peer info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("connection is not authenticated with TLS")
	}
	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", fmt.Errorf("client certificate chain was not verified")
	}
	commonName := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	if commonName == "" {
		return "", fmt.Errorf("client certificate common name is empty")
	}
	return commonName, nil
}

/**
 * @description Start begins listening on the configured TCP port and serves gRPC requests.
 * @note It blocks the current goroutine until an error occurs or the server stops.
 *
 * @function Start
 * @memberof Server
 * @returns {error} An error if the server fails to listen or serve.
 */
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", s.port, err)
	}

	fmt.Printf("[CA] gRPC server listening on :%s\n", s.port)
	return s.grpcServer.Serve(lis)
}

/**
 * @description Stop gracefully shuts down the gRPC server.
 * @note It stops accepting new connections and waits for pending RPCs to finish.
 *
 * @function Stop
 * @memberof Server
 */
func (s *Server) Stop() {
	fmt.Println("[CA] Shutting down gRPC server...")
	s.grpcServer.GracefulStop()
}
