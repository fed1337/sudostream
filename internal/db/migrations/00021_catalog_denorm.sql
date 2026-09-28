-- +goose Up
ALTER TABLE media_metadata
    ADD COLUMN IF NOT EXISTS catalog_show_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS catalog_season INTEGER,
    ADD COLUMN IF NOT EXISTS catalog_episode INTEGER,
    ADD COLUMN IF NOT EXISTS catalog_sort_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS catalog_year INTEGER,
    ADD COLUMN IF NOT EXISTS catalog_episode_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS catalog_display_name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS media_metadata_catalog_film_sort_idx
    ON media_metadata (library_id, catalog_sort_title);

CREATE INDEX IF NOT EXISTS media_metadata_catalog_show_ep_idx
    ON media_metadata (library_id, catalog_show_key, catalog_season, catalog_episode);

CREATE INDEX IF NOT EXISTS media_metadata_catalog_show_key_idx
    ON media_metadata (library_id, catalog_show_key)
    WHERE catalog_show_key <> '';

-- +goose Down
DROP INDEX IF EXISTS media_metadata_catalog_show_key_idx;
DROP INDEX IF EXISTS media_metadata_catalog_show_ep_idx;
DROP INDEX IF EXISTS media_metadata_catalog_film_sort_idx;
ALTER TABLE media_metadata
    DROP COLUMN IF EXISTS catalog_display_name,
    DROP COLUMN IF EXISTS catalog_episode_title,
    DROP COLUMN IF EXISTS catalog_year,
    DROP COLUMN IF EXISTS catalog_sort_title,
    DROP COLUMN IF EXISTS catalog_episode,
    DROP COLUMN IF EXISTS catalog_season,
    DROP COLUMN IF EXISTS catalog_show_key;
