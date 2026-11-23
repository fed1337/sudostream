-- +goose Up
ALTER TABLE media_metadata
    ADD COLUMN IF NOT EXISTS trashed_at TIMESTAMPTZ;

-- Backfill from existing recycle-bin rows (exact path or descendant under a trashed folder).
UPDATE media_metadata AS m
SET trashed_at = sub.deleted_at
FROM (
    SELECT DISTINCT ON (m2.library_id, m2.rel_path)
        m2.library_id,
        m2.rel_path,
        t.deleted_at
    FROM media_metadata AS m2
    INNER JOIN trash_items AS t
        ON m2.rel_path = t.original_rel_path
        OR m2.rel_path LIKE t.original_rel_path || '/%'
    ORDER BY m2.library_id, m2.rel_path, t.deleted_at DESC
) AS sub
WHERE m.library_id = sub.library_id
  AND m.rel_path = sub.rel_path
  AND m.trashed_at IS NULL;

CREATE INDEX IF NOT EXISTS media_metadata_active_by_library_idx
    ON media_metadata (library_id)
    WHERE trashed_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS media_metadata_active_by_library_idx;
ALTER TABLE media_metadata
    DROP COLUMN IF EXISTS trashed_at;
