import crypto from "crypto";
import redis from "../config/ioredis";
import {
  ADMIN_CA_ACTIVATION_KEY_PREFIX,
  ADMIN_CA_IDENTITY_KEY_PREFIX,
  ADMIN_BANK_ACTIVATION_KEY_PREFIX,
  ADMIN_BANK_IDENTITY_KEY_PREFIX,
  BANK_ADMIN_STATUSES,
  IDENTITY_ROLES,
  type AdminIdentity,
  type BankAdminIdentity,
  type CaAdminIdentity,
} from "../models/bank-admin";

const DEFAULT_ACTIVATION_TTL_SECONDS = 900;

const activationTtlSeconds = (): number => {
  const configured = Number(process.env.ADMIN_ACTIVATION_TTL_SECONDS);
  if (!Number.isInteger(configured) || configured < 60 || configured > 86_400) {
    return DEFAULT_ACTIVATION_TTL_SECONDS;
  }
  return configured;
};

type AdminRole = typeof IDENTITY_ROLES.BANK_ADMIN | typeof IDENTITY_ROLES.CA_ADMIN;

const scopeForRole = (role: AdminRole) =>
  role === IDENTITY_ROLES.CA_ADMIN
    ? {
        role,
        label: "CA Admin",
        identityPrefix: ADMIN_CA_IDENTITY_KEY_PREFIX,
        activationPrefix: ADMIN_CA_ACTIVATION_KEY_PREFIX,
      }
    : {
        role,
        label: "Bank Admin",
        identityPrefix: ADMIN_BANK_IDENTITY_KEY_PREFIX,
        activationPrefix: ADMIN_BANK_ACTIVATION_KEY_PREFIX,
      };

const identityKey = (role: AdminRole, adminId: string) =>
  scopeForRole(role).identityPrefix + adminId;

const activationKey = (role: AdminRole, tokenHash: string) =>
  scopeForRole(role).activationPrefix + tokenHash;

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

export interface PendingCaAdminProvision {
  identity: CaAdminIdentity;
  activationToken: string;
  expiresIn: number;
}

async function createPendingAdmin<T extends AdminIdentity>(
  input: CreatePendingBankAdminInput,
  role: AdminRole,
): Promise<{ identity: T; activationToken: string; expiresIn: number }> {
  const email = input.email.trim().toLowerCase();
  const fullName = input.fullName.trim();
  const scope = scopeForRole(role);
  if (!email || !email.includes("@")) {
    throw new AdminActivationError(
      "INVALID_ADMIN_EMAIL",
      `A valid ${scope.label} email is required`,
      400,
    );
  }
  if (!fullName) {
    throw new AdminActivationError(
      "INVALID_ADMIN_NAME",
      `${scope.label} full name is required`,
      400,
    );
  }

  const now = Math.floor(Date.now() / 1000);
  const expiresIn = activationTtlSeconds();
  const activationToken = crypto.randomBytes(32).toString("base64url");
  const tokenHash = hashActivationToken(activationToken);
  const identity = {
    admin_id: crypto.randomUUID(),
    email,
    full_name: fullName,
    role,
    status: BANK_ADMIN_STATUSES.PENDING_ACTIVATION,
    activation_token_hash: tokenHash,
    activation_expires_at: now + expiresIn,
    created_at: now,
  } as T;

  await redis
    .multi()
    .set(identityKey(role, identity.admin_id), JSON.stringify(identity))
    .set(activationKey(role, tokenHash), identity.admin_id, "EX", expiresIn)
    .exec();

  return { identity, activationToken, expiresIn };
}

export function createPendingBankAdmin(
  input: CreatePendingBankAdminInput,
): Promise<PendingBankAdminProvision> {
  return createPendingAdmin<BankAdminIdentity>(input, IDENTITY_ROLES.BANK_ADMIN);
}

export function createPendingCaAdmin(
  input: CreatePendingBankAdminInput,
): Promise<PendingCaAdminProvision> {
  return createPendingAdmin<CaAdminIdentity>(input, IDENTITY_ROLES.CA_ADMIN);
}

export async function discardPendingBankAdminProvision(
  provision: PendingBankAdminProvision,
): Promise<void> {
  await discardPendingAdminProvision(provision);
}

export async function discardPendingCaAdminProvision(
  provision: PendingCaAdminProvision,
): Promise<void> {
  await discardPendingAdminProvision(provision);
}

async function discardPendingAdminProvision(provision: {
  identity: AdminIdentity;
  activationToken: string;
}): Promise<void> {
  const tokenHash = hashActivationToken(provision.activationToken);
  await redis
    .multi()
    .del(identityKey(provision.identity.role, provision.identity.admin_id))
    .del(activationKey(provision.identity.role, tokenHash))
    .exec();
}

export async function getPendingAdminByToken(
  rawToken: string,
  role: AdminRole = IDENTITY_ROLES.BANK_ADMIN,
): Promise<AdminIdentity> {
  const tokenHash = hashActivationToken(rawToken);
  const scope = scopeForRole(role);
  const adminId = await redis.get(activationKey(role, tokenHash));
  if (!adminId) {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Activation token is invalid or expired",
      401,
    );
  }

  const encoded = await redis.get(identityKey(role, adminId));
  if (!encoded) {
    await redis.del(activationKey(role, tokenHash));
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      "Pending Admin identity was not found",
      401,
    );
  }

  let identity: AdminIdentity;
  try {
    identity = JSON.parse(encoded) as AdminIdentity;
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
      `${scope.label} is already active`,
      409,
    );
  }
  if (identity.activation_expires_at <= Math.floor(Date.now() / 1000)) {
    await redis.del(activationKey(role, tokenHash));
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_EXPIRED",
      "Activation token has expired",
      401,
    );
  }
  if (identity.role !== role) {
    throw new AdminActivationError(
      "ADMIN_ACTIVATION_INVALID",
      `Pending identity is not a ${scope.label}`,
      403,
    );
  }

  return identity;
}

export async function markAdminActivated(input: {
  identity: AdminIdentity;
  certSerial: string;
}): Promise<AdminIdentity> {
  const now = Math.floor(Date.now() / 1000);
  const active: AdminIdentity = {
    ...input.identity,
    status: BANK_ADMIN_STATUSES.ACTIVE,
    cert_serial: input.certSerial,
    activated_at: now,
  };

  await redis
    .multi()
    .set(identityKey(active.role, active.admin_id), JSON.stringify(active))
    .del(activationKey(active.role, active.activation_token_hash))
    .exec();

  return active;
}
