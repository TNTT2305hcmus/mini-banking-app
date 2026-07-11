import redis from "../config/ioredis";
import { sendBankAdminActivationEmail } from "../config/mail";
import ENV from "../config/env";
import {
  createPendingBankAdmin,
  discardPendingBankAdminProvision,
} from "../services/admin-activation.service";

const ACTIVATION_PATH = "/admin-bank/activate";

const activationUrl = (token: string): string => {
  const url = new URL(ACTIVATION_PATH, `${ENV.FRONTEND_BASE_URL.replace(/\/$/, "")}/`);
  url.hash = new URLSearchParams({ token }).toString();
  return url.toString();
};

const readArg = (name: string): string => {
  const index = process.argv.indexOf(name);
  return index >= 0 ? (process.argv[index + 1] ?? "") : "";
};

const hasFlag = (name: string): boolean => process.argv.includes(name);

async function main() {
  const email = readArg("--email");
  const fullName = readArg("--full-name");
  const printOnly = hasFlag("--print-only") || hasFlag("--no-email");
  if (!email || !fullName) {
    throw new Error(
      'Usage: npm run provision:bank-admin -- --email bank.admin@example.com --full-name "Bank Administrator" [--print-only]',
    );
  }

  const provision = await createPendingBankAdmin({ email, fullName });
  const expiresAt = new Date(
    provision.identity.activation_expires_at * 1000,
  ).toISOString();
  const url = activationUrl(provision.activationToken);
  if (!printOnly) {
    try {
      await sendBankAdminActivationEmail(provision.identity.email, {
        adminId: provision.identity.admin_id,
        fullName: provision.identity.full_name,
        expiresAt,
        activationUrl: url,
      });
    } catch (error) {
      await discardPendingBankAdminProvision(provision);
      throw error;
    }
  }

  console.log(
    printOnly
      ? "Bank Admin provisioned. Activation email was skipped (--print-only):"
      : "Bank Admin provisioned and activation email sent:",
  );
  console.log(`  admin_id: ${provision.identity.admin_id}`);
  console.log(`  email: ${provision.identity.email}`);
  console.log(`  full_name: ${provision.identity.full_name}`);
  console.log(`  expires_at: ${expiresAt}`);
  console.log(`  activation_path: ${ACTIVATION_PATH}`);
  console.log(`  activation_url: ${url}`);
}

main()
  .catch((err) => {
    console.error(`Provision failed: ${err instanceof Error ? err.message : String(err)}`);
    process.exitCode = 1;
  })
  .finally(async () => {
    await redis.quit();
  });
