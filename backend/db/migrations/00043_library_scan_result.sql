-- +goose Up

-- What the last scan of a library folder found, and why it stopped if it did, for the administration page.
ALTER TABLE library_folders
    ADD COLUMN last_result jsonb,
    ADD COLUMN last_error  text;

-- +goose Down

ALTER TABLE library_folders
    DROP COLUMN last_error,
    DROP COLUMN last_result;
