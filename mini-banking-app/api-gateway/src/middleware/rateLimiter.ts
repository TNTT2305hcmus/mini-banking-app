import { Request, Response, NextFunction } from "express";
import redis, { RedisKeys } from "../config/ioredis";

type LimitConfig = { window: number; max: number };

const check = async (
  key: string,
  cfg: LimitConfig,
  res: Response,
  next: NextFunction,
  code: string,
) => {
  try {
    const count = await redis.incr(key);
    if (count === 1) await redis.expire(key, cfg.window);
    if (count > cfg.max) {
      return res.status(429).json({
        success: false,
        error_code: code,
        message: "Too many requests. Try again later.",
      });
    }
    next();
  } catch {
    next();
  }
};

// POST /v1/auth/as-req  — 10 req / IP / 5 min
export const rateLimitByIP = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  return check(
    RedisKeys.IP_RATE_LIMIT + ip,
    { window: 300, max: 10 },
    res,
    next,
    "AUTH_RATE_LIMITED",
  );
};

// POST /v1/auth/tgs-req — 10 req / cert_sn / 5 min
export const rateLimitByCertSn = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const certSn: string | undefined = req.body?.cert_sn;
  if (!certSn) return next();
  return check(
    RedisKeys.CERT_RATE_LIMIT + certSn,
    { window: 300, max: 10 },
    res,
    next,
    "AUTH_RATE_LIMITED",
  );
};

// POST /v1/otp/request — 3 req / email / 10 min
export const rateLimitOtpByEmail = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const email: string | undefined = req.body?.email?.toLowerCase().trim();
  if (!email) return next();
  return check(
    RedisKeys.EMAIL_RATE_LIMIT + email,
    { window: 600, max: 3 },
    res,
    next,
    "OTP_RATE_LIMITED",
  );
};
