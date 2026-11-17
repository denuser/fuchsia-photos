-- Edited copies an import recorded, with their originals, that may be linked:
-- both processed, of one owner, out of the trash and not purged, and neither
-- hidden under anything yet. One copy per original, the oldest. The caller
-- checks that both have the same EXIF date. With ids, only pairs that one of
-- them is in.
-- name: ListVersionCandidates :many
SELECT DISTINCT ON (m.original_id)
       m.photo_id AS edited_id, e.mime AS edited_mime, e.exif AS edited_exif,
       o.id AS original_id, o.mime AS original_mime, o.exif AS original_exif
FROM imported_metadata m
JOIN photos e ON e.id = m.photo_id
JOIN photos o ON o.id = m.original_id
WHERE e.owner_id = o.owner_id
  AND e.status = 'ready' AND o.status = 'ready'
  AND e.deleted_at IS NULL AND o.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id IN (e.id, o.id))
  AND (sqlc.narg(owner_id)::bigint IS NULL OR e.owner_id = sqlc.narg(owner_id)::bigint)
  AND (sqlc.narg(ids)::bigint[] IS NULL
       OR m.photo_id = ANY (sqlc.narg(ids)::bigint[]) OR m.original_id = ANY (sqlc.narg(ids)::bigint[]))
ORDER BY m.original_id, m.photo_id;

-- Hides an original under its edited copy, if both are still as
-- ListVersionCandidates found them. The copy takes over what the original
-- had, so that nobody loses the photo: its albums, the album covers it was,
-- its direct shares, visibility to all users, its tags, its motion and its
-- description, if the copy has none. The
-- video of a Live Photo paired with the copy moves to the original, which it
-- belongs to, when the original has none. Both move updated_at, so that open
-- pages drop the original. Run under LockPhotoLinks.
-- name: LinkEditedVersion :execrows
WITH link AS (
    INSERT INTO photo_links (hidden_id, shown_id, kind)
    SELECT o.id, e.id, 'edited'
    FROM photos e
    JOIN photos o ON o.id = @original_id AND o.owner_id = e.owner_id
    WHERE e.id = @edited_id
      AND e.status = 'ready' AND o.status = 'ready'
      AND e.deleted_at IS NULL AND o.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id IN (e.id, o.id))
    RETURNING hidden_id, shown_id
),
albums_added AS (
    INSERT INTO album_photos (album_id, photo_id)
    SELECT ap.album_id, link.shown_id FROM album_photos ap JOIN link ON ap.photo_id = link.hidden_id
    ON CONFLICT DO NOTHING
),
covers_moved AS (
    UPDATE albums a SET cover_photo_id = link.shown_id FROM link WHERE a.cover_photo_id = link.hidden_id
),
shares_added AS (
    INSERT INTO photo_shares (photo_id, user_id)
    SELECT link.shown_id, s.user_id FROM photo_shares s JOIN link ON s.photo_id = link.hidden_id
    ON CONFLICT DO NOTHING
),
tags_added AS (
    INSERT INTO photo_tags (tag_id, photo_id)
    SELECT t.tag_id, link.shown_id FROM photo_tags t JOIN link ON t.photo_id = link.hidden_id
    ON CONFLICT DO NOTHING
),
video_moved AS (
    UPDATE photo_links v SET shown_id = link.hidden_id
    FROM link
    WHERE v.shown_id = link.shown_id AND v.kind = 'live'
      AND NOT EXISTS (SELECT 1 FROM photo_links w WHERE w.shown_id = link.hidden_id AND w.kind = 'live')
    RETURNING v.hidden_id
),
original_marked AS (
    UPDATE photos o
    SET has_motion = o.has_motion OR EXISTS (SELECT 1 FROM video_moved), updated_at = now()
    FROM link
    WHERE o.id = link.hidden_id
)
UPDATE photos e
SET visibility = CASE WHEN o.visibility = 'all' THEN 'all' ELSE e.visibility END,
    has_motion = e.has_motion OR o.has_motion,
    description = CASE WHEN e.description = '' THEN o.description ELSE e.description END,
    updated_at = now()
FROM link
JOIN photos o ON o.id = link.hidden_id
WHERE e.id = link.shown_id;
