-- +goose Up

-- Previews are cached by browsers for a year. The version goes into their
-- address, so a preview made again replaces the cached one.
ALTER TABLE photos ADD COLUMN preview_version integer NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE photos DROP COLUMN preview_version;
