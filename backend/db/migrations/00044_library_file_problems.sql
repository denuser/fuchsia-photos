-- +goose Up

-- What a scan leaves out is kept, so that a rescan reads directories and not files, and the page
-- can list what needs a look. 'unsupported' files are not photos or videos: they are read again
-- when they change or when the application learns a format (support_version is lower than the
-- code's). 'failed' files could not be read and are tried again by every scan.
ALTER TABLE photo_files DROP CONSTRAINT photo_files_state_check;
ALTER TABLE photo_files ADD CONSTRAINT photo_files_state_check
    CHECK (state IN ('source', 'duplicate', 'unsupported', 'failed'));

-- A source file with a problem, which the page lists: 'conflict' holds the content of another photo of
-- the owner, 'failed' could not be read after it changed. detail says why.
ALTER TABLE photo_files
    ADD COLUMN problem         text CHECK (problem IN ('conflict', 'failed')),
    ADD COLUMN detail          text,
    ADD COLUMN support_version integer NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE photo_files
    DROP COLUMN support_version,
    DROP COLUMN detail,
    DROP COLUMN problem;
DELETE FROM photo_files WHERE state IN ('unsupported', 'failed');
ALTER TABLE photo_files DROP CONSTRAINT photo_files_state_check;
ALTER TABLE photo_files ADD CONSTRAINT photo_files_state_check CHECK (state IN ('source', 'duplicate'));
