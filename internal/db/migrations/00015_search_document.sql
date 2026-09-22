-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS unaccent WITH SCHEMA public;

ALTER TABLE media_metadata
    ADD COLUMN search_document TEXT NOT NULL DEFAULT '';

UPDATE media_metadata
SET search_document = lower(concat_ws(
    ' ',
    rel_path,
    coalesce(original_fields->>'title', ''),
    coalesce(original_fields->>'sort_title', ''),
    coalesce(original_fields->>'original_title', ''),
    coalesce(original_fields->>'episode_title', ''),
    coalesce(original_fields->>'show', ''),
    coalesce(original_fields->>'description', ''),
    coalesce(original_fields->>'studio', ''),
    coalesce(original_fields->>'imdb_id', ''),
    coalesce(original_fields->>'tmdb_id', ''),
    coalesce(override_fields->>'title', ''),
    coalesce(override_fields->>'sort_title', ''),
    coalesce(override_fields->>'original_title', ''),
    coalesce(override_fields->>'episode_title', ''),
    coalesce(override_fields->>'show', ''),
    coalesce(override_fields->>'description', ''),
    coalesce(override_fields->>'studio', ''),
    coalesce(override_fields->>'imdb_id', ''),
    coalesce(override_fields->>'tmdb_id', '')
));

CREATE INDEX media_metadata_search_trgm_idx
    ON media_metadata USING gin (search_document public.gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS media_metadata_search_trgm_idx;
ALTER TABLE media_metadata DROP COLUMN IF EXISTS search_document;
