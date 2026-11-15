-- +goose Up

-- A Takeout scan makes an album of every album of the export, found by its
-- title among the owner's albums. album_trash says what happens to a photo of
-- such an album that is in the owner's trash: it is left out, added to the
-- album, or added and restored. albums counts the albums the run filled,
-- created or found, and restored the photos it took out of the trash.
ALTER TABLE import_runs
    ADD COLUMN album_trash text NOT NULL DEFAULT 'skip' CHECK (album_trash IN ('skip', 'add', 'restore')),
    ADD COLUMN albums bigint NOT NULL DEFAULT 0,
    ADD COLUMN restored bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE import_runs DROP COLUMN restored, DROP COLUMN albums, DROP COLUMN album_trash;
