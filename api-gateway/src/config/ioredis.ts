import ENV from "./env";
import Redis from "ioredis";

const client = new Redis(ENV.REDIS_URI, { maxRetriesPerRequest: null })
  .on("connect", () => console.log("Redis connected!"))
  .on("error", (err) => console.error("Redis Error:", err))
  .on("ready", () => console.log("Redis is ready to use!"));

export default client;
