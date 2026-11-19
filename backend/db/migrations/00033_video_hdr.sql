-- +goose Up

-- Whether a video uses an HDR transfer function, which browsers such as
-- Firefox play grey without tone mapping: the viewer then waits for the H.264
-- copy instead of playing the original. NULL for photos, and for videos until
-- processing or the backfill_video_hdr job has probed them.
ALTER TABLE photos ADD COLUMN hdr boolean;

-- +goose Down

ALTER TABLE photos DROP COLUMN hdr;
