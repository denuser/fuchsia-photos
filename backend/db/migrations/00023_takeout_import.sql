-- +goose Up

-- A scan reads either a folder of photos or Google Takeout exports: their
-- .zip and .tgz parts and unpacked Takeout folders, with the JSON files that
-- describe the photos. sidecars counts those JSON files.
ALTER TABLE import_runs
    ADD COLUMN kind text NOT NULL DEFAULT 'folder' CHECK (kind IN ('folder', 'takeout')),
    ADD COLUMN sidecars bigint NOT NULL DEFAULT 0;

-- Every file read from a Takeout export, whatever came of it. A folder may hold
-- several exports, and the same path can hold different JSON in two of them, so
-- an entry is known by its export and its path in it. An export is named by its
-- parts without their numbers (takeout-20261115T073434Z), or by the path of an
-- unpacked Takeout folder. source is the part the entry came from, relative to
-- the import root, and empty for a file of an unpacked folder. Rows stay after
-- the run, as matching JSON to photos and making albums need every copy.
CREATE TABLE takeout_entries (
    run_id   bigint NOT NULL REFERENCES import_runs (id) ON DELETE CASCADE,
    export   text NOT NULL,
    path     text NOT NULL,
    source   text NOT NULL,
    outcome  text NOT NULL
             CHECK (outcome IN ('added', 'duplicate', 'in_trash', 'purged', 'unsupported', 'sidecar', 'failed')),
    photo_id bigint REFERENCES photos (id) ON DELETE SET NULL,
    sidecar  jsonb,
    error    text,
    PRIMARY KEY (run_id, export, path),
    CONSTRAINT takeout_entries_sidecar CHECK ((outcome = 'sidecar') = (sidecar IS NOT NULL))
);

CREATE INDEX takeout_entries_source_idx ON takeout_entries (run_id, source);
CREATE INDEX takeout_entries_photo_idx ON takeout_entries (photo_id) WHERE photo_id IS NOT NULL;

-- +goose Down
DROP TABLE takeout_entries;
ALTER TABLE import_runs DROP COLUMN sidecars, DROP COLUMN kind;
