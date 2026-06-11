import { ErrorRequestHandler } from 'express'
import { status as GrpcStatus } from '@grpc/grpc-js'

export class AppError extends Error {
  constructor(
    public readonly httpStatus: number,
    public readonly code: string,
    message: string,
  ) {
    super(message)
    this.name = 'AppError'
  }
}

const grpcToHttp: Record<number, number> = {
  [GrpcStatus.INVALID_ARGUMENT]: 400,
  [GrpcStatus.UNAUTHENTICATED]: 401,
  [GrpcStatus.PERMISSION_DENIED]: 403,
  [GrpcStatus.NOT_FOUND]: 404,
  [GrpcStatus.ALREADY_EXISTS]: 409,
  [GrpcStatus.RESOURCE_EXHAUSTED]: 429,
  [GrpcStatus.UNIMPLEMENTED]: 501,
  [GrpcStatus.UNAVAILABLE]: 503,
}

export const errorEnvelope: ErrorRequestHandler = (err, _req, res, _next) => {
  if (err instanceof AppError) {
    res.status(err.httpStatus).json({ error: { code: err.code, message: err.message } })
    return
  }

  const grpcCode = (err as any).code
  if (typeof grpcCode === 'number' && grpcCode in grpcToHttp) {
    res.status(grpcToHttp[grpcCode]).json({
      error: { code: GrpcStatus[grpcCode] ?? 'GRPC_ERROR', message: 'Service error' },
    })
    return
  }

  res.status(500).json({ error: { code: 'INTERNAL_ERROR', message: 'Internal server error' } })
}

export function certRevokedError(): AppError {
  return new AppError(403, 'CERT_REVOKED', 'Certificate has been revoked')
}

export function certExpiredError(): AppError {
  return new AppError(403, 'CERT_EXPIRED', 'Certificate has expired')
}

export function ticketExpiredError(): AppError {
  return new AppError(401, 'TICKET_EXPIRED', 'Ticket has expired, please re-authenticate')
}

export function replayDetectedError(): AppError {
  return new AppError(409, 'REPLAY_DETECTED', 'Request has already been processed')
}

export function scopeDeniedError(scope: string): AppError {
  return new AppError(403, 'SCOPE_DENIED', `Ticket scope does not permit: ${scope}`)
}

export function invalidSignatureError(): AppError {
  return new AppError(400, 'INVALID_SIGNATURE', 'Signature verification failed')
}
