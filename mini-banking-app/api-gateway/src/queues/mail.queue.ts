import { Queue } from "bullmq";
import connection from "../config/ioredis";

export const mailQueue = new Queue("mail-queue", {
  connection,
  defaultJobOptions: {
    attempts: 3,
    backoff: {
      type: "exponential",
      delay: 1000,
    },
    removeOnComplete: true,
    removeOnFail: true,
  },
});
