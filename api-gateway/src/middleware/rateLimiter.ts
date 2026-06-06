import { Request, Response, NextFunction } from "express";
import redis from "../config/ioredis";

const WINDOW_SECONDS = 300;
const MAX_REQUESTS = 10;

const checkRateLimit = async (
  key: string,
  res: Response,
  next: NextFunction,
) => {
  try {
    const count = await redis.rateLimit(key, WINDOW_SECONDS);
    if (count > MAX_REQUESTS) {
      return res.status(429).json({
        success: false,
        error_code: "AUTH_RATE_LIMITED",
        message: "Too many requests. Try again later.",
      });
    }
    next();
  } catch {
    next();
  }
};

export const rateLimitByIP = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  return checkRateLimit(`rate:as:${ip}`, res, next);
};

export const rateLimitByCertSn = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const certSn: string | undefined = req.body?.cert_sn;
  if (!certSn) return next();
  return checkRateLimit(`rate:tgs:${certSn}`, res, next);
};
