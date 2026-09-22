-- +goose Up
ALTER TABLE library_grants
    ADD COLUMN can_create BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN can_read   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN can_update BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN can_delete BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE library_grants SET can_read = TRUE WHERE access = 'ro';

UPDATE library_grants
SET can_create = TRUE, can_read = TRUE, can_update = TRUE, can_delete = TRUE
WHERE access = 'rw';

ALTER TABLE library_grants DROP COLUMN access;

ALTER TABLE library_grants
    ADD CONSTRAINT library_grants_any_perm_chk
    CHECK (can_create OR can_read OR can_update OR can_delete);

-- +goose Down
ALTER TABLE library_grants DROP CONSTRAINT IF EXISTS library_grants_any_perm_chk;

ALTER TABLE library_grants ADD COLUMN access TEXT;

UPDATE library_grants SET access = 'rw'
WHERE can_create AND can_read AND can_update AND can_delete;

UPDATE library_grants SET access = 'ro'
WHERE access IS NULL AND can_read;

UPDATE library_grants SET access = 'none'
WHERE access IS NULL;

ALTER TABLE library_grants
    ALTER COLUMN access SET NOT NULL;

ALTER TABLE library_grants
    ADD CONSTRAINT library_grants_access_check CHECK (access IN ('none', 'ro', 'rw'));

ALTER TABLE library_grants
    DROP COLUMN can_create,
    DROP COLUMN can_read,
    DROP COLUMN can_update,
    DROP COLUMN can_delete;
