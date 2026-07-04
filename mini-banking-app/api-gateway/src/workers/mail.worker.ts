import { Job, Worker } from "bullmq";
import { sendOTPEmail } from "../config/mail";
import { createBullMQConnection } from "../config/bullmq";

const connection = createBullMQConnection("mail-worker");

export const mailWorker = new Worker(
  "mail-queue",
  async (job: Job) => {
    const { to, otp } = job.data;
    await sendOTPEmail(to, otp);
  },
  {
    connection,
    concurrency: 5,
    lockDuration: 120_000,
  },
)
  .on("completed", (job: Job) =>
    console.log(`Mail job ${job.data.to} completed.`),
  )
  .on("failed", (job: Job | undefined, err: Error, prev: string) =>
    console.error(`Mail job ${job!.data.to} failed:`, err),
  );
