-- +goose Up

-- Whether the timeline includes other users' photos the user can see, next to
-- the user's own. On by default, so that a user with nothing uploaded yet sees
-- what was shared with them.
ALTER TABLE users ADD COLUMN timeline_includes_shared boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE users DROP COLUMN timeline_includes_shared;
