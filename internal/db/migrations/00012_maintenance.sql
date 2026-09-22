-- +goose Up
CREATE TABLE maintenance_schedules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action      TEXT        NOT NULL,
    library_id  UUID        REFERENCES libraries (id) ON DELETE CASCADE,
    cron        TEXT        NOT NULL DEFAULT '',
    enabled     BOOLEAN     NOT NULL DEFAULT FALSE,
    config      JSONB       NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX maintenance_schedules_global_action_uidx
    ON maintenance_schedules (action)
    WHERE library_id IS NULL;

CREATE UNIQUE INDEX maintenance_schedules_library_action_uidx
    ON maintenance_schedules (action, library_id)
    WHERE library_id IS NOT NULL;

CREATE TABLE maintenance_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action      TEXT        NOT NULL,
    library_id  UUID        REFERENCES libraries (id) ON DELETE SET NULL,
    trigger     TEXT        NOT NULL,
    status      TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    summary     JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX maintenance_runs_action_started_idx
    ON maintenance_runs (action, library_id, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS maintenance_runs;
DROP TABLE IF EXISTS maintenance_schedules;
