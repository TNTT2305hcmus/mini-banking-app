import redis from "../config/ioredis";
import { sendCaAdminActivationEmail } from "../config/mail";
import ENV from "../config/env";
import {
  createPendingCaAdmin,
  discardPendingCaAdminProvision,
} from "../services/admin-activation.service";

const ACTIVATION_PATH = "/admin-ca/activate";

const activationUrl = (token: string): string => {
  const url = new URL(ACTIVATION_PATH, `${ENV.FRONTEND_BASE_URL.replace(/\/$/, "")}/`);
  url.hash = new URLSearchParams({ token }).toString();
  return url.toString();
};

const readArg = (name: string): string => {
  const index = process.argv.indexOf(name);
  return index >= 0 ? (process.argv[index + 1] ?? "") : "";
};

async function main() {
  const email = readArg("--email");
  const fullName = readArg("--full-name");
  if (!email || !fullName) {
    throw new Error(
      'Usage: npm run provision:ca-admin -- --email ca.admin@example.com --full-name "CA Administrator"',
    );
  }

  const provision = await createPendingCaAdmin({ email, fullName });
  const expiresAt = new Date(
    provision.identity.activation_expires_at * 1000,
  ).toISOString();
  try {
    await sendCaAdminActivationEmail(provision.identity.email, {
      adminId: provision.identity.admin_id,
      fullName: provision.identity.full_name,
      expiresAt,
      activationUrl: activationUrl(provision.activationToken),
    });
  } catch (error) {
    await discardPendingCaAdminProvision(provision);
    throw error;
  }

  console.log("CA Admin provisioned and activation email sent:");
  console.log(`  admin_id: ${provision.identity.admin_id}`);
  console.log(`  email: ${provision.identity.email}`);
  console.log(`  full_name: ${provision.identity.full_name}`);
  console.log(`  expires_at: ${expiresAt}`);
  console.log(`  activation_path: ${ACTIVATION_PATH}`);
}

main()
  .catch((err) => {
    console.error(`Provision failed: ${err instanceof Error ? err.message : String(err)}`);
    process.exitCode = 1;
  })
  .finally(async () => {
    await redis.quit();
  });
