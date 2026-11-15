-- +goose Up

-- What an import knew about a photo besides its file: the JSON Google Takeout
-- kept next to it, from the newest export that had the photo. Later steps fill
-- descriptions, favorites, the archive and versions from it, after the export
-- is deleted. An edited copy has no JSON of its own and takes its original's,
-- which applies only when both files have the same EXIF date. export_time is
-- the time in the name of the export's parts, and the start of time for an
-- unpacked folder, which counts as older than any export.
CREATE TABLE imported_metadata (
    photo_id    bigint PRIMARY KEY REFERENCES photos (id) ON DELETE CASCADE,
    original_id bigint REFERENCES photos (id) ON DELETE CASCADE,
    run_id      bigint REFERENCES import_runs (id) ON DELETE SET NULL,
    export      text NOT NULL,
    export_time timestamptz NOT NULL,
    path        text NOT NULL,
    sidecar     jsonb NOT NULL,
    -- photoTakenTime and geoData, read out of the JSON; geoData of 0,0 means none.
    taken_at    timestamptz,
    latitude    double precision,
    longitude   double precision,
    CONSTRAINT imported_metadata_location CHECK ((latitude IS NULL) = (longitude IS NULL))
);

CREATE INDEX imported_metadata_original_idx ON imported_metadata (original_id) WHERE original_id IS NOT NULL;
CREATE INDEX imported_metadata_run_idx ON imported_metadata (run_id);

-- A location from an import, for a photo whose file has none. The file's own
-- location, in the EXIF, always comes first.
ALTER TABLE photos
    ADD COLUMN imported_latitude double precision,
    ADD COLUMN imported_longitude double precision,
    ADD CONSTRAINT photos_imported_location CHECK ((imported_latitude IS NULL) = (imported_longitude IS NULL));

-- +goose Down
ALTER TABLE photos
    DROP CONSTRAINT photos_imported_location,
    DROP COLUMN imported_longitude,
    DROP COLUMN imported_latitude;
DROP TABLE imported_metadata;
