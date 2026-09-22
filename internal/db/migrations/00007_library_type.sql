-- +goose Up
ALTER TABLE libraries
    ADD COLUMN type TEXT NOT NULL DEFAULT 'other'
        CHECK (type IN ('film', 'series', 'music', 'photos', 'other'));

-- +goose Down
ALTER TABLE libraries DROP COLUMN IF EXISTS type;
