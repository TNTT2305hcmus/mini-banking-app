import {
  prepareEnrollment,
  storeCertificate,
  storeClientProfile,
  type StoredCertificate,
} from "../pki-registration"
import { postAdminActivation } from "./admin-enrollment.api"

export interface ActivateBankAdminParams {
  activationToken: string
  email: string
  fullName: string
  pin: string
}

export interface ActivatedBankAdmin {
  adminId: string
  email: string
  fullName: string
  certificate: StoredCertificate
}

export async function activateBankAdmin(
  params: ActivateBankAdminParams,
): Promise<ActivatedBankAdmin> {
  const email = params.email.trim().toLowerCase()
  const fullName = params.fullName.trim()
  const { csrPem } = await prepareEnrollment({
    email,
    fullName,
    pin: params.pin,
  })

  const response = await postAdminActivation({
    activationToken: params.activationToken.trim(),
    csrPem,
  })
  if (response.role !== "bank_admin") {
    throw new Error("ADMIN_ROLE_REQUIRED")
  }

  const certificate: StoredCertificate = {
    certificatePem: response.cert_pem,
    serialNumber: response.cert_serial,
    notAfter: new Date(response.expires_at * 1000).toISOString(),
  }
  await storeCertificate(certificate)
  await storeClientProfile({ fullName: response.full_name.trim() })

  return {
    adminId: response.admin_id,
    email: response.email,
    fullName: response.full_name,
    certificate,
  }
}
