-- +goose Up
ALTER TABLE media_metadata
    ADD COLUMN overridden_by TEXT;

UPDATE media_metadata
SET overridden_by = 'user:' || override_updated_by::text
WHERE override_updated_by IS NOT NULL;

ALTER TABLE media_metadata
    DROP COLUMN override_updated_by;

-- +goose Down
ALTER TABLE media_metadata
    ADD COLUMN override_updated_by UUID REFERENCES users (id);

UPDATE media_metadata
SET override_updated_by = NULLIF(substring(overridden_by FROM '^user:(.+)$'), '')::uuid
WHERE overridden_by LIKE 'user:%';

ALTER TABLE media_metadata
    DROP COLUMN overridden_by;
