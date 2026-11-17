-- +goose Up

-- Photos a Takeout scan added that Google had in the trash, which the scan
-- moved to the trash too.
ALTER TABLE import_runs ADD COLUMN trashed bigint NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE import_runs DROP COLUMN trashed;
