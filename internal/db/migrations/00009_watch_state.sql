-- +goose Up
CREATE TABLE watch_state (
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    library_id  UUID        NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path    TEXT        NOT NULL,
    watched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, library_id, rel_path)
);

CREATE INDEX watch_state_user_library_idx ON watch_state (user_id, library_id);

-- +goose Down
DROP TABLE IF EXISTS watch_state;
