-- +goose Up

-- Each user's own archive, on any photo the user can see. An archived photo
-- leaves that user's timeline and stays in albums, favorites, search and
-- "Shared with me".
CREATE TABLE photo_archive (
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    photo_id   bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, photo_id)
);

CREATE INDEX photo_archive_photo_id_idx ON photo_archive (photo_id);

-- Photos imported before the archive was kept take their place in it from
-- the JSON the import saved.
INSERT INTO photo_archive (user_id, photo_id)
SELECT p.owner_id, t.id
FROM imported_metadata m
JOIN photos p ON p.id = m.photo_id
JOIN photos t ON t.id = photo_top(m.photo_id)
WHERE m.sidecar -> 'archived' = 'true'::jsonb
  AND t.purged_at IS NULL
ON CONFLICT DO NOTHING;

-- +goose Down

DROP TABLE photo_archive;
