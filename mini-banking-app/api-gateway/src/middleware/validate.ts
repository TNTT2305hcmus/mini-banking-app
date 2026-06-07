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
