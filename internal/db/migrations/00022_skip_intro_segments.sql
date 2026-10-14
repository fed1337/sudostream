-- +goose Up
CREATE TABLE skip_intro_segments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    library_id      UUID           NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path        TEXT           NOT NULL,
    kind            TEXT           NOT NULL DEFAULT 'intro',
    start_ms        BIGINT         NOT NULL,
    end_ms          BIGINT         NOT NULL,
    source          TEXT           NOT NULL,
    confidence      DOUBLE PRECISION NOT NULL DEFAULT 0,
    engine_version  INT            NOT NULL DEFAULT 1,
    show_key        TEXT           NOT NULL DEFAULT '',
    season          INT            NOT NULL DEFAULT 0,
    detected_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT skip_intro_segments_range_chk CHECK (end_ms > start_ms)
);

CREATE UNIQUE INDEX skip_intro_segments_library_path_kind_uidx
    ON skip_intro_segments (library_id, rel_path, kind);

CREATE INDEX skip_intro_segments_show_season_idx
    ON skip_intro_segments (library_id, show_key, season);

-- +goose Down
DROP TABLE IF EXISTS skip_intro_segments;
