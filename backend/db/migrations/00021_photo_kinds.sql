-- +goose Up

-- What an item of the library is, for the media kind filter of the timeline:
-- a video, a photo with motion (an Android motion photo or a paired Live
-- Photo), or a still photo. Each item is exactly one of them.
-- +goose StatementBegin
CREATE FUNCTION photo_kind(mime text, has_motion boolean)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
               WHEN starts_with(mime, 'video/') THEN 'video'
               WHEN has_motion THEN 'motion'
               ELSE 'photo'
           END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION photo_kind(text, boolean);
