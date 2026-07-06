import crypto from "crypto";
import redis from "../config/ioredis";
import {
  ADMIN_BANK_ACTIVATION_KEY_PREFIX,
  ADMIN_BANK_IDENTITY_KEY_PREFIX,
  BANK_ADMIN_STATUSES,
  IDENTITY_ROLES,
  type BankAdminIdentity,
} from "../models/bank-admin";

const DEFAULT_ACTIVATION_TTL_SECONDS = 900;

const activationTtlSeconds = (): number => {
  const configured = Number(process.env.ADMIN_ACTIVATION_TTL_SECONDS);
  if (!Number.isInteger(configured) || configured < 60 || configured > 86_400) {
    return DEFAULT_ACTIVATION_TTL_SECONDS;
  }
  return configured;
};

const identityKey = (adminId: string) =>
  ADMIN_BANK_IDENTITY_KEY_PREFIX + adminId;

const activationKey = (tokenHash: string) =>
  ADMIN_BANK_ACTIVATION_KEY_PREFIX + tokenHash;

export class AdminActivationError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "AdminActivationError";
  }
}

export const hashActivationToken = (rawToken: string): string =>
  crypto.createHash("sha256").update(rawToken, "utf8").digest("hex");

export interface CreatePendingBankAdminInput {
  email: string;
  fullName: string;
}

export interface PendingBankAdminProvision {
  identity: BankAdminIdentity;
  activationToken: string;
  expiresIn: number;
}

export async function createPendingBankAdmin(
  input: CreatePendingBankAdminInput,
): Promise<PendingBankAdminProvision> {
  const email = input.email.trim().toLowerCase();
  const fullName = input.fullName.trim();
  if (!email || !email.includes("@")) {
    throw new AdminActivationError(
      "INVALID_ADMIN_EMAIL",
      "A valid Admin email is required",
      400,
    );
  }
  if (!fullName) {
    throw new AdminActivationError(
      "INVALID_ADMIN_NAME",
      "Admin full name is required",
      400,
    );
  }

  const now = Math.floor(Date.now() / 1000);
  const expiresIn = activationTtlSeconds();
  const activationToken = crypto.randomBytes(32).toString("base64url");
  const tokenHash = hashActivationToken(activationToken);
  const identity: BankAdminIdentity = {
    admin_id: crypto.randomUUID(),
    email,
    full_name: fullName,
    role: IDENTITY_ROLES.BANK_ADMIN,
    status: BANK_ADMIN_STATUSES.PENDING_ACTIVATION,
    activation_token_hash: tokenHash,
    activation_expires_at: now + expiresIn,
    created_at: now,
  };

  await redis
    .multi()
    .set(identityKey(identity.admin_id), JSON.stringify(identity))
    .set(activationKey(tokenHash), identity.admin_id, "EX", expiresIn)
    .exec();

  return { identity, activationToken, expiresIn };
}

export async function getPendingAdminByToken(
  rawToken: string,
): Promise<BankAdminIdentity> {
  const tokenHash = hashActivationToken(rawToken);
  const adminId = await redis.get(activationKey(tokenHash));
  if (!adminId) {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Activation token is invalid or expired",
      401,
    );
  }

  const encoded = await redis.get(identityKey(adminId));
  if (!encoded) {
    await redis.del(activationKey(tokenHash));
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Pending Admin identity was not found",
      401,
    );
  }

  let identity: BankAdminIdentity;
  try {
    identity = JSON.parse(encoded) as BankAdminIdentity;
  } catch {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Pending Admin identity is corrupted",
      401,
    );
  }

  if (identity.activation_token_hash !== tokenHash) {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Activation token does not match the Admin identity",
      401,
    );
  }
  if (identity.status === BANK_ADMIN_STATUSES.ACTIVE) {
    throw new AdminActivationError(
      "ADMIN_ALREADY_ACTIVE",
      "Bank Admin is already active",
      409,
    );
  }
  if (identity.activation_expires_at <= Math.floor(Date.now() / 1000)) {
    await redis.del(activationKey(tokenHash));
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_EXPIRED",
      "Activation token has expired",
      401,
    );
  }
  if (identity.role !== IDENTITY_ROLES.BANK_ADMIN) {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Pending identity is not a Bank Admin",
      403,
    );
  }

  return identity;
}

export async function markAdminActivated(input: {
  identity: BankAdminIdentity;
  certSerial: string;
}): Promise<BankAdminIdentity> {
  const now = Math.floor(Date.now() / 1000);
  const active: BankAdminIdentity = {
    ...input.identity,
    status: BANK_ADMIN_STATUSES.ACTIVE,
    cert_serial: input.certSerial,
    activated_at: now,
  };

  await redis
    .multi()
    .set(identityKey(active.admin_id), JSON.stringify(active))
    .del(activationKey(active.activation_token_hash))
    .exec();

  return active;
}

