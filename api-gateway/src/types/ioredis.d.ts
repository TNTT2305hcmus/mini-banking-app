import "ioredis";

declare module "ioredis" {
  interface RedisCommander<Context> {
    rateLimit(key: string, windowSeconds: number): Promise<number>;
  }
}
