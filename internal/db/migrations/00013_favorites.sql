-- +goose Up
CREATE TABLE favorites (
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    library_id  UUID        NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path    TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, library_id, rel_path)
);

CREATE INDEX favorites_user_library_idx ON favorites (user_id, library_id);

-- +goose Down
DROP TABLE IF EXISTS favorites;
