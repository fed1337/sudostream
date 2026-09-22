-- +goose Up
CREATE TABLE libraries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    rel_path TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE library_grants (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id UUID NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    access TEXT NOT NULL CHECK (access IN ('none', 'ro', 'rw')),
    PRIMARY KEY (user_id, library_id)
);

CREATE INDEX library_grants_library_id_idx ON library_grants (library_id);

-- +goose Down
DROP TABLE IF EXISTS library_grants;
DROP TABLE IF EXISTS libraries;
