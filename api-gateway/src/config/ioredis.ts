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

export default client;
