-- +goose Up

-- Whether the viewer shows the original file instead of the viewer preview.
-- Off by default: originals can be many times larger than previews.
ALTER TABLE users ADD COLUMN viewer_shows_originals boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE users DROP COLUMN viewer_shows_originals;
