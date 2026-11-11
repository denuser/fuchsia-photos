-- +goose Up

-- Videos are kept in photos, as they go through the same ingest, permissions,
-- albums and sharing. The MIME type tells a video, and duration is its length
-- in seconds. has_motion marks a motion photo whose embedded video processing
-- extracted next to the previews.
ALTER TABLE photos
    ADD COLUMN duration double precision CHECK (duration >= 0),
    ADD COLUMN has_motion boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE photos
    DROP COLUMN has_motion,
    DROP COLUMN duration;
