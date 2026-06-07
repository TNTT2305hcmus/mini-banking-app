import { RequestHandler } from 'express'
import { AppError } from './error-envelope'

interface Entry {
  count: number
  resetAt: number
}

export function rateLimit(opts: { windowMs: number; max: number }): RequestHandler {
  const store = new Map<string, Entry>()

  const cleanup = setInterval(() => {
    const now = Date.now()
    for (const [key, entry] of store) {
      if (entry.resetAt <= now) store.delete(key)
    }
  }, opts.windowMs)

  cleanup.unref()

  return (req, _res, next) => {
    const forwarded = req.headers['x-forwarded-for']
    const ip =
      (typeof forwarded === 'string' ? forwarded.split(',')[0].trim() : undefined) ??
      req.ip ??
      'unknown'

    const now = Date.now()
    const entry = store.get(ip)

    if (!entry || entry.resetAt <= now) {
      store.set(ip, { count: 1, resetAt: now + opts.windowMs })
      return next()
    }

    if (entry.count >= opts.max) {
      return next(new AppError(429, 'RATE_LIMITED', 'Too many requests, please try again later'))
    }

    entry.count++
    next()
  }
}
