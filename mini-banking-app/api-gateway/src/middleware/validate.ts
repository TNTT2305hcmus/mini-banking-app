import { RequestHandler } from 'express'
import { AppError } from './error-envelope'

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

function isNonEmpty(value: unknown): boolean {
  return typeof value === 'string' && value.trim().length > 0
}

function isUuidV4(value: unknown): boolean {
  return typeof value === 'string' && UUID_V4.test(value)
}

function checkField(field: string, value: unknown): AppError | null {
  if (field === 'request_id') {
    if (!isUuidV4(value)) {
      return new AppError(400, 'INVALID_REQUEST', `${field} must be a valid UUID v4`)
    }
  } else {
    if (!isNonEmpty(value)) {
      return new AppError(400, 'INVALID_REQUEST', `${field} is required`)
    }
  }
  return null
}

export function requireBodyFields(...fields: string[]): RequestHandler {
  return (req, _res, next) => {
    for (const field of fields) {
      const err = checkField(field, req.body[field])
      if (err) return next(err)
    }
    next()
  }
}

export function requireQueryFields(...fields: string[]): RequestHandler {
  return (req, _res, next) => {
    for (const field of fields) {
      const err = checkField(field, req.query[field])
      if (err) return next(err)
    }
    next()
  }
}

const BASE64 = /^[A-Za-z0-9+/]*={0,2}$/

function isBase64(value: unknown): boolean {
  return typeof value === 'string' && value.trim().length > 0 && BASE64.test(value.trim())
}

export function requireBase64Fields(...fields: string[]): RequestHandler {
  return (req, _res, next) => {
    for (const field of fields) {
      if (!isBase64(req.body[field])) {
        return next(new AppError(400, 'INVALID_REQUEST', `${field} must be a non-empty base64 string`))
      }
    }
    next()
  }
}

export function validateScope(allowed: string[]): RequestHandler {
  return (req, _res, next) => {
    const scope = req.body.scope
    if (typeof scope !== 'string' || !allowed.includes(scope)) {
      return next(new AppError(400, 'INVALID_SCOPE', `scope must be one of: ${allowed.join(', ')}`))
    }
    next()
  }
}

const TIMESTAMP_WINDOW_MS = 5 * 60 * 1000

export const validateTimestamp: RequestHandler = (req, _res, next) => {
  const ts = req.body.timestamp
  if (typeof ts !== 'number' || !Number.isInteger(ts)) {
    return next(new AppError(400, 'INVALID_REQUEST', 'timestamp must be an integer Unix seconds'))
  }
  if (Math.abs(Date.now() - ts * 1000) > TIMESTAMP_WINDOW_MS) {
    return next(new AppError(400, 'STALE_REQUEST', 'timestamp is too old or too far in the future'))
  }
  next()
}
