-- +goose Up
DELETE FROM maintenance_schedules WHERE action = 'trash.purge';

-- +goose Down
SELECT 1;
