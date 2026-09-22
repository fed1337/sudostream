-- +goose Up
ALTER TABLE watch_state
    ADD COLUMN watched BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN position_seconds DOUBLE PRECISION,
    ADD COLUMN duration_seconds DOUBLE PRECISION;

CREATE INDEX watch_state_continue_idx ON watch_state (user_id, updated_at DESC)
    WHERE watched = FALSE;

-- +goose Down
DROP INDEX IF EXISTS watch_state_continue_idx;

ALTER TABLE watch_state
    DROP COLUMN IF EXISTS duration_seconds,
    DROP COLUMN IF EXISTS position_seconds,
    DROP COLUMN IF EXISTS watched;
