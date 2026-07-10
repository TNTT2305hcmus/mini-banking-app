-- Allow CA Admin certificates to be distinguished from customer and Bank Admin certs.
ALTER TABLE certificates
    DROP CONSTRAINT IF EXISTS certificates_role_check;

ALTER TABLE certificates
    ADD CONSTRAINT certificates_role_check
    CHECK (role IN ('customer', 'bank_admin', 'ca_admin'));
