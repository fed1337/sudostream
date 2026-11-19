-- +goose Up
ALTER TABLE users
    ADD COLUMN auto_skip_intro BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN auto_next_episode BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users
    DROP COLUMN IF EXISTS auto_skip_intro,
    DROP COLUMN IF EXISTS auto_next_episode;
