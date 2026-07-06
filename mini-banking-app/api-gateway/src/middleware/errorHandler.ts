import { Request, Response, NextFunction } from "express";
import { status as grpcStatus } from "@grpc/grpc-js";

export const errorHandler = (
  err: any,
  req: Request,
  res: Response,
  _next: NextFunction,
) => {
  const meta = {
    request_id: req.headers["x-request-id"] as string,
    timestamp: new Date().toISOString(),
  };

  if (
    typeof err?.status === "number" &&
    typeof err?.error_code === "string" &&
    typeof err?.message === "string"
  ) {
    return res.status(err.status).json({
      success: false,
      error_code: err.error_code,
      message: err.message,
      ...meta,
    });
  }

  // gRPC errors carry a numeric `code`; relay them with the proper HTTP status.
  if (typeof err?.code === "number") {
    const { status, error_code, message } = bankGrpcError(err);
    return res.status(status).json({ success: false, error_code, message, ...meta });
  }

  console.error("[UNHANDLED]", err);
  res.status(500).json({
    success: false,
    error_code: "INTERNAL_ERROR",
    message: "An unexpected error occurred",
    ...meta,
  });
};

export type HttpError = { status: number; error_code: string; message: string };

export const httpError = (
  status: number,
  error_code: string,
  message: string,
): HttpError => ({ status, error_code, message });

export const caGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "CA service unavailable";

  switch (code) {
    case grpcStatus.ALREADY_EXISTS:
      return httpError(409, "IDENTITY_ALREADY_REGISTERED", msg);
    case grpcStatus.INVALID_ARGUMENT:
      return httpError(400, "INVALID_CSR_FORMAT", msg);
    case grpcStatus.NOT_FOUND:
      return httpError(404, "CERT_NOT_FOUND", msg);
    case grpcStatus.PERMISSION_DENIED:
      return httpError(403, "CERT_NOT_OWNED", msg);
    default:
      return httpError(502, "CA_SERVICE_UNAVAILABLE", msg);
  }
};

export const caAdminGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "CA service unavailable";

  switch (code) {
    case grpcStatus.NOT_FOUND:
      return httpError(404, "CERT_NOT_FOUND", msg);
    case grpcStatus.INVALID_ARGUMENT:
      return httpError(400, "INVALID_ADMIN_CA_REQUEST", msg);
    case grpcStatus.ALREADY_EXISTS:
      return httpError(409, "CERT_ALREADY_REVOKED", msg);
    case grpcStatus.FAILED_PRECONDITION:
      return httpError(422, "CERT_TYPE_NOT_REVOKABLE", msg);
    default:
      return httpError(502, "CA_SERVICE_UNAVAILABLE", msg);
  }
};

export const asGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "KDC unavailable";

  switch (code) {
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: "INVALID_REQUEST", message: msg };
    case grpcStatus.UNAUTHENTICATED:
      return { status: 401, error_code: "SIGNATURE_INVALID", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 409, error_code: "REPLAY_DETECTED", message: msg };
    case grpcStatus.NOT_FOUND:
      return { status: 401, error_code: "CERT_NOT_FOUND", message: msg };
    default:
      return { status: 502, error_code: "KDC_UNAVAILABLE", message: msg };
  }
};

export const tgsGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "KDC unavailable";

  switch (code) {
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: "INVALID_TGT_FORMAT", message: msg };
    case grpcStatus.UNAUTHENTICATED:
      return { status: 401, error_code: "TGT_EXPIRED", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 403, error_code: "SCOPE_DENIED", message: msg };
    default:
      return { status: 502, error_code: "KDC_UNAVAILABLE", message: msg };
  }
};

// A domain-specific error code that BankService may expose via trailing
// metadata (e.g. INVALID_TICKET / WRONG_SCOPE / INSUFFICIENT_FUNDS). When
// present it is relayed as-is; otherwise a generic code per status is used so
// the internal failure reason is not leaked.
const grpcMetaCode = (err: any): string | undefined => {
  const value = err?.metadata?.get("error-code")?.[0];
  return typeof value === "string" && value.length > 0 ? value : undefined;
};

const adminBankCode = (message: string): string | undefined => {
  const allowed = new Set([
    "ADMIN_SESSION_REQUIRED",
    "ADMIN_SESSION_INVALID",
    "ADMIN_SESSION_EXPIRED",
    "ADMIN_ROLE_REQUIRED",
    "INVALID_PAGINATION",
    "INVALID_DATE_RANGE",
    "INVALID_FILTER",
  ]);
  return allowed.has(message) ? message : undefined;
};

export const bankGrpcError = (err: any): HttpError => {
  const code: number = err?.code ?? -1;
  const msg: string = err?.details || err?.message || "Bank service error";
  const domainCode = grpcMetaCode(err) ?? adminBankCode(msg);

  switch (code) {
    case grpcStatus.INVALID_ARGUMENT:
      return { status: 400, error_code: domainCode ?? "BAD_REQUEST", message: msg };
    case grpcStatus.UNAUTHENTICATED:
      return { status: 401, error_code: domainCode ?? "UNAUTHENTICATED", message: msg };
    case grpcStatus.PERMISSION_DENIED:
      return { status: 403, error_code: domainCode ?? "FORBIDDEN", message: msg };
    case grpcStatus.NOT_FOUND:
      return { status: 404, error_code: domainCode ?? "NOT_FOUND", message: msg };
    case grpcStatus.ALREADY_EXISTS:
      return { status: 409, error_code: domainCode ?? "ALREADY_EXISTS", message: msg };
    case grpcStatus.FAILED_PRECONDITION:
      return { status: 422, error_code: domainCode ?? "UNPROCESSABLE_ENTITY", message: msg };
    case grpcStatus.RESOURCE_EXHAUSTED:
      return { status: 429, error_code: domainCode ?? "RATE_LIMITED", message: msg };
    case grpcStatus.UNAVAILABLE:
    case grpcStatus.DEADLINE_EXCEEDED:
      // fail-closed: Bank/CA không phản hồi → service unavailable, không phải lỗi nội bộ
      return { status: 503, error_code: domainCode ?? "SERVICE_UNAVAILABLE", message: msg };
    default:
      return { status: 500, error_code: domainCode ?? "INTERNAL_ERROR", message: msg };
  }
};
