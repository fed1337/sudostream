-- +goose Up
-- Non-admin accounts that were enabled without email verification should start disabled
-- when email confirmation is the activation path (conservative backfill).
UPDATE users
SET enabled = FALSE, updated_at = NOW()
WHERE enabled = TRUE
  AND email_verified_at IS NULL
  AND role <> 'admin';

-- +goose Down
-- Irreversible data fix; no down migration.
