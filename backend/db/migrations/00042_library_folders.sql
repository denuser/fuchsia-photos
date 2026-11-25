-- +goose Up

-- A library folder is read where it is, never copied: its path is relative to the folders mount
-- (/data/folders), and its photos belong to one owner. The scan interval is per folder.
CREATE TABLE library_folders (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path          text NOT NULL UNIQUE,
    owner_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    scan_interval integer NOT NULL DEFAULT 600 CHECK (scan_interval >= 60),
    last_scan_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- What a scan knows about each file of a library folder. A row is keyed by folder and path and outlives
-- a missing file and a permanent delete, so a path is always the same file whatever its hash.
-- 'source' is the file a photo is read from; 'duplicate' has the hash of another photo and never
-- becomes a source.
CREATE TABLE photo_files (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    folder_id        bigint NOT NULL REFERENCES library_folders (id) ON DELETE CASCADE,
    path             text NOT NULL,
    photo_id         bigint REFERENCES photos (id) ON DELETE SET NULL,
    state            text NOT NULL DEFAULT 'source' CHECK (state IN ('source', 'duplicate')),
    size             bigint NOT NULL,
    mtime            timestamptz NOT NULL,
    missed_scans     integer NOT NULL DEFAULT 0,
    missing_since    timestamptz,
    trashed_by_scan  boolean NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (folder_id, path)
);

-- One photo has one source file.
CREATE UNIQUE INDEX photo_files_source_idx ON photo_files (photo_id) WHERE state = 'source';

-- Where a photo's original is when it is not in the application's library: a path under the folders
-- mount, copied here by the scan from photo_files so that every reader of an original needs only
-- the photo.
ALTER TABLE photos ADD COLUMN source_path text;

-- +goose Down

ALTER TABLE photos DROP COLUMN source_path;
DROP TABLE photo_files;
DROP TABLE library_folders;
