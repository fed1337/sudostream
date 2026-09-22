-- +goose Up
CREATE TABLE media_metadata (
    library_id            UUID        NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path              TEXT        NOT NULL,
    original_fields       JSONB       NOT NULL DEFAULT '{}',
    override_fields       JSONB       NOT NULL DEFAULT '{}',
    file_mtime            TIMESTAMPTZ,
    file_size             BIGINT,
    probed_at             TIMESTAMPTZ,
    override_updated_at   TIMESTAMPTZ,
    override_updated_by   UUID        REFERENCES users (id),
    PRIMARY KEY (library_id, rel_path)
);

CREATE INDEX media_metadata_library_path_idx ON media_metadata (library_id, rel_path);

-- +goose Down
DROP TABLE IF EXISTS media_metadata;
