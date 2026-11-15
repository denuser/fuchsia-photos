-- +goose Up

-- After a Takeout scan has read every part, it matches each photo to its JSON
-- file and each edited copy to its original, within one export.
ALTER TABLE import_runs DROP CONSTRAINT import_runs_status_check;
ALTER TABLE import_runs
    ADD CONSTRAINT import_runs_status_check
        CHECK (status IN ('listing', 'importing', 'matching', 'finished', 'stopped', 'failed')),
    -- Files without a JSON file of their own or an original to take one from.
    ADD COLUMN without_sidecar bigint NOT NULL DEFAULT 0,
    -- JSON files of photos that matched no file.
    ADD COLUMN unmatched_sidecars bigint NOT NULL DEFAULT 0,
    -- Edited copies matched to their originals.
    ADD COLUMN edited bigint NOT NULL DEFAULT 0;

DROP INDEX import_runs_one_active_key;
CREATE UNIQUE INDEX import_runs_one_active_key ON import_runs ((true))
    WHERE status IN ('listing', 'importing', 'matching');

-- The path of the JSON file that describes a file, and of the original an
-- edited copy was made from, both in the same export. They are set together
-- by the matching, which can run again and set them anew.
ALTER TABLE takeout_entries
    ADD COLUMN sidecar_path text,
    ADD COLUMN original_path text;

-- +goose Down
ALTER TABLE takeout_entries DROP COLUMN original_path, DROP COLUMN sidecar_path;

DROP INDEX import_runs_one_active_key;
CREATE UNIQUE INDEX import_runs_one_active_key ON import_runs ((true))
    WHERE status IN ('listing', 'importing');

UPDATE import_runs SET status = 'failed', error = 'interrupted by a downgrade' WHERE status = 'matching';
ALTER TABLE import_runs
    DROP COLUMN edited,
    DROP COLUMN unmatched_sidecars,
    DROP COLUMN without_sidecar,
    DROP CONSTRAINT import_runs_status_check;
ALTER TABLE import_runs
    ADD CONSTRAINT import_runs_status_check
        CHECK (status IN ('listing', 'importing', 'finished', 'stopped', 'failed'));
