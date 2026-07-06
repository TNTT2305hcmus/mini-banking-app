-- Add CA-authoritative identity role without changing existing certificates.

ALTER TABLE certificates
    ADD COLUMN IF NOT EXISTS role VARCHAR(20);

UPDATE certificates
SET role = 'customer'
WHERE role IS NULL OR BTRIM(role) = '';

ALTER TABLE certificates
    ALTER COLUMN role SET DEFAULT 'customer',
    ALTER COLUMN role SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'certificates_role_check'
          AND conrelid = 'certificates'::regclass
    ) THEN
        ALTER TABLE certificates
            ADD CONSTRAINT certificates_role_check
            CHECK (role IN ('customer', 'bank_admin'));
    END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_certs_role
    ON certificates(role);
