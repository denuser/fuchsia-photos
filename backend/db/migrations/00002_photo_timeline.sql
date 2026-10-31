-- +goose Up
ALTER TABLE photos
    ADD COLUMN original_name  text NOT NULL DEFAULT '',
    ADD COLUMN preview_format text,
    ADD COLUMN sort_at        timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN updated_at     timestamptz NOT NULL DEFAULT now();

DROP INDEX photos_owner_taken_at_idx;
CREATE INDEX photos_timeline_idx ON photos (owner_id, sort_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX photos_owner_updated_at_idx ON photos (owner_id, updated_at);

-- The only definition of who may see a photo. Every query that returns a photo
-- to a user who is not its owner, including file delivery, goes through it.
-- +goose StatementBegin
CREATE FUNCTION photo_visible_to(photo photos, viewer_id bigint, viewer_is_admin boolean)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT viewer_is_admin OR photo.owner_id = viewer_id OR photo.visibility = 'all'
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION photo_visible_to(photos, bigint, boolean);
DROP INDEX photos_owner_updated_at_idx;
DROP INDEX photos_timeline_idx;
CREATE INDEX photos_owner_taken_at_idx ON photos (owner_id, taken_at DESC, id DESC);
ALTER TABLE photos
    DROP COLUMN original_name,
    DROP COLUMN preview_format,
    DROP COLUMN sort_at,
    DROP COLUMN updated_at;
