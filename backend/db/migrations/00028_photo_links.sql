-- +goose Up

-- Files hidden under another photo: the video of a Live Photo under its photo,
-- and an original under its edited copy. A hidden photo is left out of every
-- list and count and follows the photo above it into the trash, out of it and
-- away for good. Links may chain: the video of a Live Photo stays under its
-- photo when that photo is hidden under its edited copy. A file is hidden under
-- one photo only, and a photo has at most one Live Photo video.
CREATE TABLE photo_links (
    hidden_id  bigint PRIMARY KEY REFERENCES photos (id) ON DELETE CASCADE,
    shown_id   bigint NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('live', 'edited')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (hidden_id <> shown_id)
);

CREATE INDEX photo_links_shown_id_idx ON photo_links (shown_id);
CREATE UNIQUE INDEX photo_links_live_key ON photo_links (shown_id) WHERE kind = 'live';

INSERT INTO photo_links (hidden_id, shown_id, kind)
SELECT id, live_photo_id, 'live' FROM photos WHERE live_photo_id IS NOT NULL;

ALTER TABLE photos DROP COLUMN live_photo_id;

-- Whether a photo plays motion through a link: the video of its Live Photo,
-- or the motion of the original under an edited copy.
-- +goose StatementBegin
CREATE FUNCTION photo_has_linked_motion(photo_id bigint)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT EXISTS (SELECT 1 FROM photo_links l JOIN photos v ON v.id = l.hidden_id
                   WHERE l.shown_id = photo_id AND l.kind = 'live' AND v.purged_at IS NULL)
        OR EXISTS (SELECT 1 FROM photo_links e JOIN photos o ON o.id = e.hidden_id
                   WHERE e.shown_id = photo_id AND e.kind = 'edited' AND o.has_motion AND o.purged_at IS NULL)
$$;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION photo_has_linked_motion(bigint);

ALTER TABLE photos
    ADD COLUMN live_photo_id bigint REFERENCES photos (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX photos_live_photo_id_key ON photos (live_photo_id);
UPDATE photos p SET live_photo_id = l.shown_id
FROM photo_links l
WHERE l.hidden_id = p.id AND l.kind = 'live';
DROP TABLE photo_links;
