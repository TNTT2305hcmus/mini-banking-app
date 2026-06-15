// Middleware rate limit dùng Redis counter để hạn chế spam OTP/Auth/Bank.
import { Request, Response, NextFunction } from "express";
import redis, { RedisKeys } from "../config/ioredis";

// Cấu hình cửa sổ thời gian và số request tối đa.
type LimitConfig = { window: number; max: number };

// Helper dùng chung: tăng counter Redis, set TTL và reject khi vượt giới hạn.
const check = async (
  key: string,
  cfg: LimitConfig,
  res: Response,
  next: NextFunction,
  code: string,
) => {
  try {
    // Gọi Redis INCR để đếm request trong window hiện tại.
    const count = await redis.incr(key);
    // Request đầu tiên tạo TTL cho counter để Redis tự reset window.
    if (count === 1) await redis.expire(key, cfg.window);
    // Vượt ngưỡng thì trả 429, không gọi handler phía sau.
    if (count > cfg.max) {
      return res.status(429).json({
        success: false,
        error_code: code,
        message: "Too many requests. Try again later.",
      });
    }
    // Chưa vượt giới hạn thì cho request đi tiếp.
    next();
  } catch {
    // Redis lỗi thì fail-open để không làm sập API Gateway.
    next();
  }
};

// Rate limit AS_REQ theo IP: 10 request trong 5 phút.
export const rateLimitByIP = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  // Lấy IP từ Express hoặc socket để tạo Redis key.
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  // Gọi helper Redis rate limit dùng chung.
  return check(
    RedisKeys.IP_RATE_LIMIT + ip,
    { window: 300, max: 10 },
    res,
    next,
    "AUTH_RATE_LIMITED",
  );
};

// Rate limit nhóm Bank API theo IP: hiện cấu hình 20 request trong 1 phút.
export const rateLimitBankByIP = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  // Dùng IP caller để giới hạn toàn bộ /v1/bank/*.
  const ip = req.ip ?? req.socket.remoteAddress ?? "unknown";
  // Gọi Redis counter riêng cho Bank để tách khỏi Auth/OTP.
  return check(
    RedisKeys.BANK_RATE_LIMIT + ip,
    { window: 60, max: 20 },
    res,
    next,
    "BANK_RATE_LIMITED",
  );
};

// Rate limit TGS_REQ theo cert serial để giảm lạm dụng theo danh tính.
export const rateLimitByCertSn = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  // Nếu request chưa có cert_sn thì bỏ qua, phần validate/auth sẽ xử lý sau.
  const certSn: string | undefined = req.body?.cert_sn;
  if (!certSn) return next();
  // Gọi helper Redis với key theo cert serial.
  return check(
    RedisKeys.CERT_RATE_LIMIT + certSn,
    { window: 300, max: 10 },
    res,
    next,
    "AUTH_RATE_LIMITED",
  );
};

// Rate limit OTP request theo email: 3 request trong 10 phút.
export const rateLimitOtpByEmail = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  // Chuẩn hóa email để tránh bypass bằng chữ hoa/thừa khoảng trắng.
  const email: string | undefined = req.body?.email?.toLowerCase().trim();
  if (!email) return next();
  // Gọi helper Redis với key riêng cho OTP/email.
  return check(
    RedisKeys.EMAIL_RATE_LIMIT + email,
    { window: 600, max: 3 },
    res,
    next,
    "OTP_RATE_LIMITED",
  );
};
