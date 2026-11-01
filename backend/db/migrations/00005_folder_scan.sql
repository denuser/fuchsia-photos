-- +goose Up

-- A permanently deleted photo keeps its row, without anything that describes the
-- photo, so that a folder scan can recognize its hash and leave it deleted. Such a
-- row stays in the trash as far as deleted_at goes, which keeps it out of every
-- list of photos. Queries that deal with the trash must also check purged_at.
ALTER TABLE photos
    ADD COLUMN purged_at timestamptz,
    ADD CONSTRAINT photos_purged_from_trash CHECK (purged_at IS NULL OR deleted_at IS NOT NULL);

DROP INDEX photos_trash_idx;
CREATE INDEX photos_trash_idx ON photos (owner_id, deleted_at DESC, id DESC)
    WHERE deleted_at IS NOT NULL AND purged_at IS NULL;

CREATE TABLE import_runs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id    bigint NOT NULL REFERENCES users (id),
    started_by  bigint NOT NULL REFERENCES users (id),
    -- Relative to the import root. Empty means the root itself.
    folder      text NOT NULL,
    status      text NOT NULL DEFAULT 'listing'
                CHECK (status IN ('listing', 'importing', 'finished', 'stopped', 'failed')),
    error       text,
    found       bigint NOT NULL DEFAULT 0,
    added       bigint NOT NULL DEFAULT 0,
    duplicates  bigint NOT NULL DEFAULT 0,
    in_trash    bigint NOT NULL DEFAULT 0,
    purged      bigint NOT NULL DEFAULT 0,
    unsupported bigint NOT NULL DEFAULT 0,
    changing    bigint NOT NULL DEFAULT 0,
    failed      bigint NOT NULL DEFAULT 0,
    -- Files listed before this time have had long enough to show a change.
    settle_at   timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

-- One scan at a time: scans of overlapping folders would only compete for the disk.
CREATE UNIQUE INDEX import_runs_one_active_key ON import_runs ((true))
    WHERE status IN ('listing', 'importing');

-- Files still to process, and the ones to report. A file that was handled without
-- a problem leaves this table and is only counted in import_runs.
CREATE TABLE import_files (
    run_id bigint NOT NULL REFERENCES import_runs (id) ON DELETE CASCADE,
    path   text NOT NULL,
    size   bigint NOT NULL,
    mtime  timestamptz NOT NULL,
    state  text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'changing', 'failed')),
    error  text,
    PRIMARY KEY (run_id, path)
);

-- +goose Down
DROP TABLE import_files;
DROP TABLE import_runs;

DROP INDEX photos_trash_idx;
CREATE INDEX photos_trash_idx ON photos (owner_id, deleted_at DESC, id DESC) WHERE deleted_at IS NOT NULL;

DELETE FROM photos WHERE purged_at IS NOT NULL;
ALTER TABLE photos
    DROP CONSTRAINT photos_purged_from_trash,
    DROP COLUMN purged_at;
