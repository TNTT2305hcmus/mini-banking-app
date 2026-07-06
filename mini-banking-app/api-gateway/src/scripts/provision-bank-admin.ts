import redis from "../config/ioredis";
import { createPendingBankAdmin } from "../services/admin-activation.service";

const readArg = (name: string): string => {
  const index = process.argv.indexOf(name);
  return index >= 0 ? (process.argv[index + 1] ?? "") : "";
};

async function main() {
  const email = readArg("--email");
  const fullName = readArg("--full-name");
  if (!email || !fullName) {
    throw new Error(
      'Usage: npm run provision:bank-admin -- --email admin@bank.local --full-name "Bank Administrator"',
    );
  }

  const provision = await createPendingBankAdmin({ email, fullName });
  console.log("Bank Admin provisioned:");
  console.log(`  admin_id: ${provision.identity.admin_id}`);
  console.log(`  email: ${provision.identity.email}`);
  console.log(`  full_name: ${provision.identity.full_name}`);
  console.log(`  activation_token: ${provision.activationToken}`);
  console.log(
    `  expires_at: ${new Date(provision.identity.activation_expires_at * 1000).toISOString()}`,
  );
  console.log("  activation_url: /admin-bank/activate");
}

main()
  .catch((err) => {
    console.error(`Provision failed: ${err instanceof Error ? err.message : String(err)}`);
    process.exitCode = 1;
  })
  .finally(async () => {
    await redis.quit();
  });

