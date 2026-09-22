-- +goose Up
CREATE TABLE trash_items (
    id                UUID        PRIMARY KEY,
    original_rel_path TEXT        NOT NULL,
    trash_rel_path    TEXT        NOT NULL UNIQUE,
    library_id        UUID        REFERENCES libraries (id) ON DELETE SET NULL,
    deleted_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_by        UUID        REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX trash_items_deleted_at_idx ON trash_items (deleted_at);
CREATE INDEX trash_items_original_rel_path_idx ON trash_items (original_rel_path);

-- +goose Down
DROP TABLE IF EXISTS trash_items;
