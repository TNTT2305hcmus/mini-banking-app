package grpc

/**
 * @title CA Service - gRPC Handler
 * @author Tran Nguyen Tri Thanh (tntt)
 * @summary Implementing the CAServiceServer interface, mapping gRPC requests to core logic, and translating errors into gRPC status codes.
 */

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mini-banking/ca-service/internal/ca"
	pb "mini-banking/pkg/pb/ca"
)

/**
 * @description Handler implements the CAServiceServer interface generated from ca.proto.
 * @note Its sole responsibility is mapping gRPC requests -> ca.Service -> gRPC responses.
 * @note It does not contain any business logic.
 *
 * @typedef {Object} Handler
 * @property {pb.UnimplementedCAServiceServer} UnimplementedCAServiceServer - Embedded for forward-compatibility when new RPCs are added.
 * @property {*ca.Service} svc - The injected core CA Service containing the business logic.
 */
type Handler struct {
	pb.UnimplementedCAServiceServer
	svc *ca.Service
}

/**
 * @description NewHandler creates a new gRPC handler with the injected CA Service.
 *
 * @function NewHandler
 * @param {*ca.Service} svc - The core CA service.
 * @returns {*Handler} A pointer to the newly created gRPC Handler.
 */
func NewHandler(svc *ca.Service) *Handler {
	return &Handler{svc: svc}
}

/**
 * @description RegisterUser receives a CSR PEM, signs it, and returns the generated X.509 certificate.
 *
 * @function RegisterUser
 * @memberof Handler
 * @param {context.Context} ctx - The RPC context.
 * @param {*pb.RegisterUserRequest} req - The gRPC request containing the CSR and User ID.
 * @returns {(*pb.RegisterUserResponse, error)} The gRPC response containing the certificate, or an error.
 */
func (h *Handler) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	if req.CsrPem == "" {
		return nil, status.Error(codes.InvalidArgument, "csr_pem is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	certPEM, serialHex, notAfter, err := h.svc.RegisterUser(req.CsrPem, req.UserId)
	if err != nil {
		// @note Categorize errors to return the correct gRPC status code
		switch {
		case errors.Is(err, ca.ErrCSRIdentityMismatch):
			return nil, status.Errorf(codes.InvalidArgument, "CSR identity mismatch: %v", err)
		case errors.Is(err, ca.ErrActiveCertificateExists):
			return nil, status.Errorf(codes.AlreadyExists, "active certificate already exists for user: %s", req.UserId)
		case isCSRSignatureError(err):
			return nil, status.Errorf(codes.InvalidArgument, "CSR verification failed: %v", err)
		default:
			return nil, status.Errorf(codes.Internal, "register user: %v", err)
		}
	}

	return &pb.RegisterUserResponse{
		CertificatePem: certPEM,
		SerialNumber:   serialHex,
		NotAfterUnix:   notAfter,
	}, nil
}

/**
 * @description GetCertificate returns a certificate and its status by its serial number.
 *
 * @function GetCertificate
 * @memberof Handler
 * @param {context.Context} ctx - The RPC context.
 * @param {*pb.GetCertificateRequest} req - The gRPC request containing the serial number.
 * @returns {(*pb.GetCertificateResponse, error)} The gRPC response containing certificate details, or an error.
 */
func (h *Handler) GetCertificate(ctx context.Context, req *pb.GetCertificateRequest) (*pb.GetCertificateResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	certPEM, userID, certStatus, notAfter, err := h.svc.GetCertificate(req.SerialNumber)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		}
		return nil, status.Errorf(codes.Internal, "get certificate: %v", err)
	}

	return &pb.GetCertificateResponse{
		CertificatePem: certPEM,
		UserId:         userID,
		Status:         pb.CertStatus(certStatus),
		NotAfterUnix:   notAfter,
	}, nil
}

/**
 * @description CheckRevocation checks the revocation status of a certificate.
 * @note KDC calls this during TGS Exchange, and Bank calls it before BEGIN TRANSACTION.
 *
 * @function CheckRevocation
 * @memberof Handler
 * @param {context.Context} ctx - The RPC context.
 * @param {*pb.CheckRevocationRequest} req - The gRPC request containing the serial number.
 * @returns {(*pb.CheckRevocationResponse, error)} The gRPC response containing the revocation status, or an error.
 */
func (h *Handler) CheckRevocation(ctx context.Context, req *pb.CheckRevocationRequest) (*pb.CheckRevocationResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	certStatus, reason, revokedAt, err := h.svc.CheckRevocation(req.SerialNumber)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		}
		return nil, status.Errorf(codes.Internal, "check revocation: %v", err)
	}

	return &pb.CheckRevocationResponse{
		Status:    pb.CertStatus(certStatus),
		Reason:    reason,
		RevokedAt: revokedAt,
	}, nil
}

/**
 * @description RevokeCertificate revokes a certificate.
 * @note Returns codes.NotFound if the serial does not exist.
 * @note Returns codes.AlreadyExists if the certificate was already revoked.
 *
 * @function RevokeCertificate
 * @memberof Handler
 * @param {context.Context} ctx - The RPC context.
 * @param {*pb.RevokeCertificateRequest} req - The gRPC request containing the serial number and reason.
 * @returns {(*pb.RevokeCertificateResponse, error)} An empty response on success, or an error.
 */
func (h *Handler) RevokeCertificate(ctx context.Context, req *pb.RevokeCertificateRequest) (*pb.RevokeCertificateResponse, error) {
	if req.SerialNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "serial_number is required")
	}

	if err := h.svc.RevokeCertificate(req.SerialNumber, req.Reason); err != nil {
		switch {
		case strings.HasPrefix(err.Error(), "not_found"):
			return nil, status.Errorf(codes.NotFound, "certificate not found: %s", req.SerialNumber)
		case strings.HasPrefix(err.Error(), "already_revoked"):
			return nil, status.Errorf(codes.AlreadyExists, "certificate already revoked: %s", req.SerialNumber)
		default:
			return nil, status.Errorf(codes.Internal, "revoke certificate: %v", err)
		}
	}

	return &pb.RevokeCertificateResponse{}, nil
}

// ========================================================
// =================== Error helpers ======================
// ========================================================

/**
 * @description isCSRSignatureError checks if the error is related to an invalid CSR signature.
 *
 * @function isCSRSignatureError
 * @param {error} err - The error to check.
 * @returns {bool} True if it is a signature error, false otherwise.
 */
func isCSRSignatureError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "signature") ||
		strings.Contains(msg, "verification failed") ||
		strings.Contains(msg, "invalid CSR")
}

/**
 * @description isNotFoundError checks if the error indicates a missing record.
 *
 * @function isNotFoundError
 * @param {error} err - The error to check.
 * @returns {bool} True if it is a not found error, false otherwise.
 */
func isNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "not found")
}
