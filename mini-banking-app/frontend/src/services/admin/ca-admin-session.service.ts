import {
  getStoredCertificate,
  loadSigningKey,
} from "../pki-registration"
import { bytesToBase64, signRsaSha256 } from "../key.service"
import {
  createAdminCaCertificateSession,
  storeAdminSession,
  type AdminLoginResponse,
} from "./ca-admin.api"

export async function loginAdminCAWithCertificate(
  pin: string,
): Promise<AdminLoginResponse> {
  const cert = await getStoredCertificate("ca_admin")
  if (!cert?.serialNumber) {
    throw new Error("ADMIN_CA_CERT_NOT_FOUND")
  }

  const issuedAt = Math.floor(Date.now() / 1000)
  const challenge = [
    "admin-ca-login",
    cert.serialNumber.toLowerCase(),
    crypto.randomUUID(),
    String(issuedAt),
  ].join(":")
  const signingKey = await loadSigningKey(pin, "ca_admin")
  const signature = await signRsaSha256(
    signingKey,
    new TextEncoder().encode(challenge),
  )
  const session = await createAdminCaCertificateSession({
    certSerial: cert.serialNumber,
    challenge,
    signature: bytesToBase64(signature),
  })
  storeAdminSession(session)
  return session
}
