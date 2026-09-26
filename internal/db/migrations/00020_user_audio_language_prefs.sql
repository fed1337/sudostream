-- +goose Up
ALTER TABLE users
    ADD COLUMN audio_language_prefs JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS audio_language_prefs;
