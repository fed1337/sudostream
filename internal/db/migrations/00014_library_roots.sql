-- +goose Up
CREATE TABLE library_roots (
    library_id UUID NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path   TEXT NOT NULL,
    PRIMARY KEY (library_id, rel_path)
);

CREATE UNIQUE INDEX library_roots_rel_path_uidx ON library_roots (rel_path);

INSERT INTO library_roots (library_id, rel_path)
SELECT id, rel_path FROM libraries;

ALTER TABLE libraries DROP COLUMN rel_path;

-- +goose Down
ALTER TABLE libraries ADD COLUMN rel_path TEXT;

UPDATE libraries l
SET rel_path = r.rel_path
FROM (
    SELECT DISTINCT ON (library_id) library_id, rel_path
    FROM library_roots
    ORDER BY library_id, rel_path
) r
WHERE l.id = r.library_id;

DELETE FROM libraries WHERE rel_path IS NULL OR rel_path = '';

ALTER TABLE libraries ALTER COLUMN rel_path SET NOT NULL;
CREATE UNIQUE INDEX libraries_rel_path_key ON libraries (rel_path);

DROP TABLE IF EXISTS library_roots;
