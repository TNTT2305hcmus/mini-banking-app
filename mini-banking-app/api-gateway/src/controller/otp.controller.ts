import { Request, Response } from "express";
import crypto from "crypto";
import jwt from "jsonwebtoken";
import redis, { RedisKeys } from "../config/ioredis";
import ENV from "../config/env";
import { mailQueue } from "../queues/mail.queue";

const hmacOtp = (otp: string): string =>
  crypto.createHmac("sha256", ENV.GATEWAY_OTP_SECRET).update(otp).digest("hex");

const safeEqual = (a: string, b: string): boolean => {
  const ba = Buffer.from(a, "utf8");
  const bb = Buffer.from(b, "utf8");
  if (ba.length !== bb.length) {
    crypto.timingSafeEqual(Buffer.alloc(1), Buffer.alloc(1));
    return false;
  }
  return crypto.timingSafeEqual(ba, bb);
};

const maskEmail = (email: string): string => {
  const [local, domain] = email.split("@");
  return `${local[0]}***@${domain}`;
};

const meta = (req: Request) => ({
  request_id: req.headers["x-request-id"] as string,
  timestamp: new Date().toISOString(),
});

// ── POST /v1/otp/request ──────────────────────────────────────────────────────

export const handleOtpRequest = async (req: Request, res: Response) => {
  const m = meta(req);
  const email: string = req.body.email;

  // 1. Generate 6-digit OTP
  const otp = crypto.randomInt(100_000, 1_000_000).toString();

  // 2. HMAC before storing — raw OTP never touches Redis
  const hmac = hmacOtp(otp);

  // 3. Persist (re-request resets TTL)
  await redis.set(RedisKeys.OTP + email, hmac, "EX", ENV.OTP_EXPIRES_IN);

  // 4. Dispatch via BullMQ worker (async — response is 200 regardless)
  mailQueue
    .add("send-otp", { to: email, otp })
    .catch((err) =>
      console.error(
        `[OTP] Failed to enqueue email for ${maskEmail(email)}:`,
        err,
      ),
    );

  return res.status(200).json({
    success: true,
    message: "OTP dispatched",
    data: { email_masked: maskEmail(email), expires_in: ENV.OTP_EXPIRES_IN },
    ...m,
  });
};

// ── POST /v1/otp/verify ───────────────────────────────────────────────────────

export const handleOtpVerify = async (req: Request, res: Response) => {
  const m = meta(req);
  const email: string = req.body.email.toLowerCase();
  const otp: string = req.body.otp;
  const failKey = RedisKeys.OTP_FAILED + email;

  // 1. OTP must exist in Redis
  const stored = await redis.get(RedisKeys.OTP + email);
  if (!stored) {
    return res.status(404).json({
      success: false,
      error_code: "OTP_NOT_FOUND",
      message: "No pending OTP for this email",
      ...m,
    });
  }

  // 2. Block brute-force before comparing
  const failRaw = await redis.get(failKey);
  if (failRaw && parseInt(failRaw, 10) >= ENV.OTP_MAX_ATTEMPTS) {
    return res.status(429).json({
      success: false,
      error_code: "VERIFY_RATE_LIMITED",
      message: "Too many failed attempts. Try again later.",
      ...m,
    });
  }

  // 3. Constant-time HMAC comparison
  const match = safeEqual(hmacOtp(otp), stored);

  if (!match) {
    const count = await redis.incr(failKey);
    if (count === 1) await redis.expire(failKey, ENV.OTP_COOLDOWN);
    return res.status(401).json({
      success: false,
      error_code: "OTP_MISMATCH",
      message: "Invalid OTP",
      ...m,
    });
  }

  // 4. Single-use: delete OTP immediately after success
  await redis.del(RedisKeys.OTP + email);

  // 5. Issue registration JWT (10 minute lifetime)
  const jti = crypto.randomUUID();
  const iat = Math.floor(Date.now() / 1000);

  // 6. Nếu user xác thực OTP thành công thì api-gateway sinh UUID làm owner_id
  // idC = owner_id thay vì idC = email
  const ownerId = crypto.randomUUID();

  const reg_token = jwt.sign(
    { sub: email, owner_id: ownerId, purpose: "pki_registration", jti, iat, exp: iat + 600 },
    ENV.GATEWAY_JWT_SECRET,
  );

  // 6. Pre-register jti as unused (single-use sentinel)
  await redis.set(RedisKeys.REG_TOKEN + jti, "0", "EX", 600);

  return res.status(200).json({
    success: true,
    message: "OTP verified",
    data: { reg_token, owner_id: ownerId, expires_in: 600 },
    ...m,
  });
};
