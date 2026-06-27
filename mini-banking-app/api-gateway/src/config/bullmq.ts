import Redis from "ioredis";
import ENV from "./env";

export const createBullMQConnection = (name: string) =>
  new Redis(ENV.GATEWAY_REDIS_URL, {
    maxRetriesPerRequest: null,
  })
    .on("connect", () => console.log(`[BullMQ:${name}] Redis connected.`))
    .on("error", (err) => console.error(`[BullMQ:${name}] Redis error:`, err));
