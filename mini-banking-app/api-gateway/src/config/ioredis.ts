import ENV from "./env";
import Redis from "ioredis";

const client = new Redis(ENV.REDIS_URI, { maxRetriesPerRequest: null })
  .on("connect", () => console.log("Redis connected!"))
  .on("error", (err) => console.error("Redis Error:", err))
  .on("ready", () => console.log("Redis is ready to use!"));

client.defineCommand("rateLimit", {
  numberOfKeys: 1,
  lua: `
    local current = redis.call("INCR", KEYS[1])

    if current == 1 then
        redis.call("EXPIRE", KEYS[1], ARGV[1])
    end

    return current
  `,
});

export const enum RedisKeys {
  OTP = "otp:val:",
  IP_RATE_LIMIT = "rate:as:",
  CERT_RATE_LIMIT = "rate:tgs:",
  EMAIL_RATE_LIMIT = "rate:otp:",
  OTP_FAILED = "rate:verify:",
  REG_TOKEN = "`jwt_used:",
}

export default client;
