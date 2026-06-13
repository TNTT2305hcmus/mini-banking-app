import { Request, Response, NextFunction } from "express";
import { status as grpcStatus } from "@grpc/grpc-js";

export const errorHandler = (
  err: Error,
  _req: Request,
  res: Response,
  _next: NextFunction,
) => {
  console.error("[UNHANDLED]", err);
  res.status(500).json({
    success: false,
    error_code: "INTERNAL_ERROR",
    message: "An unexpected error occurred",
  });
};

type HttpError = { status: number; error_code: string; message: string };

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
