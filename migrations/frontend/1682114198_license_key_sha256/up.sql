-- Add column
ALTER TABLE product_licenses
ADD COLUMN IF NOT EXISTS access_token_sha256 BYTEA;

-- Make sure all values are unqiue
ALTER TABLE product_licenses
    DROP CONSTRAINT IF EXISTS product_licenses_access_token_sha256_unique,
    ADD CONSTRAINT product_licenses_access_token_sha256_unique UNIQUE (access_token_sha256);

-- Index for lookups
CREATE INDEX IF NOT EXISTS product_licenses_access_token_sha256_idx
ON product_licenses (access_token_sha256)
WHERE access_token_sha256 IS NOT NULL;

-- In-band migration to create access_token_sha256 for only active license keys
UPDATE product_licenses
SET access_token_sha256 = 'sgs_' + encode(digest(license_key, 'sha256'), 'hex')
WHERE license_expires_at > NOW();
