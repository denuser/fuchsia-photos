-- +goose Up

-- Each user's own favorite marks, on any photo the user can see. Others do
-- not see them.
CREATE TABLE photo_favorites (
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    photo_id   bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, photo_id)
);

CREATE INDEX photo_favorites_photo_id_idx ON photo_favorites (photo_id);

-- The photo shown at the top of the chain a photo is hidden in, or the photo
-- itself when it is not hidden.
-- +goose StatementBegin
CREATE FUNCTION photo_top(photo_id bigint)
RETURNS bigint
LANGUAGE sql STABLE
AS $$
    WITH RECURSIVE up AS (
        SELECT photo_id AS id, 0 AS depth
        UNION ALL
        SELECT l.shown_id, up.depth + 1 FROM photo_links l JOIN up ON l.hidden_id = up.id WHERE up.depth < 16
    )
    SELECT id FROM up ORDER BY depth DESC LIMIT 1
$$;
-- +goose StatementEnd

-- Photos imported before favorites were kept take their marks from the JSON
-- the import saved.
INSERT INTO photo_favorites (user_id, photo_id)
SELECT p.owner_id, t.id
FROM imported_metadata m
JOIN photos p ON p.id = m.photo_id
JOIN photos t ON t.id = photo_top(m.photo_id)
WHERE m.sidecar -> 'favorited' = 'true'::jsonb
  AND t.purged_at IS NULL
ON CONFLICT DO NOTHING;

-- +goose Down

DROP FUNCTION photo_top(bigint);
DROP TABLE photo_favorites;
