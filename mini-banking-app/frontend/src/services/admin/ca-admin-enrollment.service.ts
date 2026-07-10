import {
  prepareEnrollment,
  storeCertificate,
  storeClientProfile,
  type StoredCertificate,
} from "../pki-registration"
import { activateAdminCA } from "./ca-admin.api"

export interface ActivateCaAdminParams {
  activationToken: string
  email: string
  fullName: string
  pin: string
}

export interface ActivatedCaAdmin {
  adminId: string
  email: string
  fullName: string
  certificate: StoredCertificate
}

export async function activateCaAdmin(
  params: ActivateCaAdminParams,
): Promise<ActivatedCaAdmin> {
  const email = params.email.trim().toLowerCase()
  const fullName = params.fullName.trim()
  const { csrPem } = await prepareEnrollment({
    email,
    fullName,
    pin: params.pin,
    scope: "ca_admin",
  })

  const response = await activateAdminCA({
    activationToken: params.activationToken.trim(),
    csrPem,
  })
  if (response.role !== "ca_admin") {
    throw new Error("ADMIN_ROLE_REQUIRED")
  }

  const certificate: StoredCertificate = {
    certificatePem: response.cert_pem,
    serialNumber: response.cert_serial,
    notAfter: new Date(response.expires_at * 1000).toISOString(),
    role: "ca_admin",
  }
  await storeCertificate(certificate, "ca_admin")
  await storeClientProfile({ fullName: response.full_name.trim(), role: "ca_admin" }, "ca_admin")

  return {
    adminId: response.admin_id,
    email: response.email,
    fullName: response.full_name,
    certificate,
  }
}
