-- Extend certificate_audit_log.action with Registration Authority (RA) and
-- admin-authentication events.
--
-- The API Gateway acts as the RA in the PKI: OTP identity vetting and
-- registration approval are part of the certificate lifecycle, so these events
-- belong in the CA audit domain (actor ra:*), on the same trail as the `issued`
-- event of the resulting certificate. admin-ca logins are recorded here too.

ALTER TABLE certificate_audit_log
    DROP CONSTRAINT IF EXISTS certificate_audit_log_action_check;

ALTER TABLE certificate_audit_log
    ADD CONSTRAINT certificate_audit_log_action_check
    CHECK (action IN (
        'issuer_provisioned',
        'issued',
        'revoked',
        'looked_up',
        'verify_certificate',
        'chain_verified',
        'ra_otp_requested',
        'ra_otp_verified',
        'ra_otp_failed',
        'ra_registration_approved',
        'ra_registration_rejected',
        'admin_ca_login_success',
        'admin_ca_login_failed'
    ));
