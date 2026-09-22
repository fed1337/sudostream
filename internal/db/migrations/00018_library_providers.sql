-- +goose Up
CREATE TABLE library_provider_settings (
    library_id                  UUID PRIMARY KEY REFERENCES libraries (id) ON DELETE CASCADE,
    metadata_provider           TEXT,
    poster_provider             TEXT,
    subtitle_provider           TEXT,
    subtitle_languages          JSONB       NOT NULL DEFAULT '[]',
    allow_override_user_metadata BOOLEAN    NOT NULL DEFAULT FALSE,
    metadata_apply_mode         TEXT        NOT NULL DEFAULT 'fill_missing',
    metadata_write_target       TEXT        NOT NULL DEFAULT 'db',
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE provider_artifacts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    library_id    UUID        NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path      TEXT        NOT NULL,
    kind          TEXT        NOT NULL,
    lang          TEXT,
    provider_key  TEXT        NOT NULL,
    cache_path    TEXT        NOT NULL,
    external_id   TEXT,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX provider_artifacts_lookup_uidx
    ON provider_artifacts (library_id, rel_path, kind, COALESCE(lang, ''));

CREATE INDEX provider_artifacts_library_idx ON provider_artifacts (library_id);

-- +goose Down
DROP TABLE IF EXISTS provider_artifacts;
DROP TABLE IF EXISTS library_provider_settings;
