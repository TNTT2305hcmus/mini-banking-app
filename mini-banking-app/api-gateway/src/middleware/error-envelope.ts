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
