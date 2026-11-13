-- +goose Up

-- When the application makes H.264 copies of videos that some browsers cannot
-- play: never, after processing (upload), or when someone opens the video
-- (view). It takes effect only where the graphics can encode H.264. The table
-- holds exactly one row.
CREATE TABLE video_settings (
    id        smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    transcode text NOT NULL DEFAULT 'view' CHECK (transcode IN ('off', 'upload', 'view'))
);

INSERT INTO video_settings DEFAULT VALUES;

-- The H.264 copy of what an item plays: the original of a video, or the
-- motion of a motion photo or a Live Photo. NULL until the transcode job has
-- looked at it; then 'ready' when the copy is next to the previews,
-- 'not_needed' when the video plays as it is or the graphics cannot decode
-- it, and 'failed' when making it failed.
ALTER TABLE photos
    ADD COLUMN video_copy text CHECK (video_copy IN ('ready', 'not_needed', 'failed'));

-- +goose Down

ALTER TABLE photos DROP COLUMN video_copy;
DROP TABLE video_settings;
