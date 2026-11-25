-- name: InsertPhoto :one
INSERT INTO photos (owner_id, sha256, mime, size, original_name)
VALUES (@owner_id, @sha256, @mime, @size, @original_name)
RETURNING *;

-- name: GetPhotoByHash :one
SELECT * FROM photos WHERE owner_id = @owner_id AND sha256 = @sha256;

-- name: GetPhoto :one
SELECT * FROM photos WHERE id = @id;

-- name: GetVisiblePhoto :one
SELECT p.*
FROM photos p
WHERE p.id = @id
  AND p.purged_at IS NULL
  AND (p.deleted_at IS NULL OR p.owner_id = @viewer_id::bigint)
  AND photo_visible_to(p, @viewer_id::bigint, @viewer_is_admin::boolean)
  -- A version hidden under another photo is opened only by its owner: the
  -- original of an edited copy may show what the edit took out. Others play
  -- the video of a Live Photo through its photo.
  AND (p.owner_id = @viewer_id::bigint OR @viewer_is_admin::boolean
       OR NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id));

-- The photos of one owner: the current user, or the owner of an album the
-- user can see. With with_shared, which the API allows only without an album,
-- the list also takes other users' processed photos that the current user can
-- see, as photo_visible_to does. Tags are personal, so a tag filter keeps only
-- the owner's photos: another user's tag must not reveal how that user grouped
-- the photos it shared. favorites keeps the current user's favorites, which
-- the API allows only without an album. Photos still being processed are
-- left out until processing has read their date and size, so that they do not
-- appear in the current month and then move. The queries below take the same
-- filters.
-- name: ListTimeline :many
SELECT p.*
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  -- A photo not processed yet has no date or size to place it by.
  AND p.status <> 'pending'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (NOT @favorites::boolean
       OR EXISTS (SELECT 1 FROM photo_favorites f WHERE f.photo_id = p.id AND f.user_id = @owner_id))
  -- The user's archived photos leave the timeline and stay in albums and
  -- favorites; the archived filter lists only them.
  AND (CASE WHEN @archived::boolean
            THEN EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            WHEN sqlc.narg(album_id)::bigint IS NULL AND NOT @favorites::boolean
            THEN NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            ELSE true END)
  AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
       OR (p.sort_at, p.id) < (sqlc.narg(after_sort_at)::timestamptz, sqlc.narg(after_id)::bigint))
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR photo_kinds(p.mime, p.has_motion, p.has_raw) && sqlc.narg(kinds)::text[])
  -- The photos taken inside a box of coordinates, which may cross the date
  -- line (west east of east). The API allows it only where it shows locations.
  AND (sqlc.narg(south)::double precision IS NULL
       OR (p.latitude BETWEEN sqlc.narg(south)::double precision AND sqlc.narg(north)::double precision
           AND (CASE WHEN sqlc.narg(west)::double precision <= sqlc.narg(east)::double precision
                     THEN p.longitude BETWEEN sqlc.narg(west)::double precision AND sqlc.narg(east)::double precision
                     ELSE p.longitude >= sqlc.narg(west)::double precision OR p.longitude <= sqlc.narg(east)::double precision END)))
ORDER BY p.sort_at DESC, p.id DESC
LIMIT @max_rows;

-- Counts the photos of ListTimeline by month in the viewer's time zone, so
-- that the timeline can lay out months it has not loaded yet.
-- name: CountTimelineMonths :many
SELECT to_char(date_trunc('month', p.sort_at AT TIME ZONE @tz::text), 'YYYY-MM')::text AS month,
       count(*) AS count
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  -- A photo not processed yet has no date or size to place it by.
  AND p.status <> 'pending'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (NOT @favorites::boolean
       OR EXISTS (SELECT 1 FROM photo_favorites f WHERE f.photo_id = p.id AND f.user_id = @owner_id))
  -- The user's archived photos leave the timeline and stay in albums and
  -- favorites; the archived filter lists only them.
  AND (CASE WHEN @archived::boolean
            THEN EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            WHEN sqlc.narg(album_id)::bigint IS NULL AND NOT @favorites::boolean
            THEN NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            ELSE true END)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR photo_kinds(p.mime, p.has_motion, p.has_raw) && sqlc.narg(kinds)::text[])
  -- The photos taken inside a box of coordinates, which may cross the date
  -- line (west east of east). The API allows it only where it shows locations.
  AND (sqlc.narg(south)::double precision IS NULL
       OR (p.latitude BETWEEN sqlc.narg(south)::double precision AND sqlc.narg(north)::double precision
           AND (CASE WHEN sqlc.narg(west)::double precision <= sqlc.narg(east)::double precision
                     THEN p.longitude BETWEEN sqlc.narg(west)::double precision AND sqlc.narg(east)::double precision
                     ELSE p.longitude >= sqlc.narg(west)::double precision OR p.longitude <= sqlc.narg(east)::double precision END)))
GROUP BY 1
ORDER BY 1 DESC;

-- The cameras, lenses and media kinds of the photos ListTimeline filters, with
-- how many photos each has, for the filters of the timeline. Each list takes
-- every filter but its own: the filter keeps any of its names, so a name not
-- chosen yet counts the photos it would add. A kind is one of photo_kinds'
-- names; a photo may have two, so the kinds may add up to more than the photos.
-- name: ListCameras :many
SELECT p.camera::text AS name, count(*) AS count
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  -- A photo not processed yet has no date or size to place it by.
  AND p.status <> 'pending'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (NOT @favorites::boolean
       OR EXISTS (SELECT 1 FROM photo_favorites f WHERE f.photo_id = p.id AND f.user_id = @owner_id))
  -- The user's archived photos leave the timeline and stay in albums and
  -- favorites; the archived filter lists only them.
  AND (CASE WHEN @archived::boolean
            THEN EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            WHEN sqlc.narg(album_id)::bigint IS NULL AND NOT @favorites::boolean
            THEN NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            ELSE true END)
  AND p.camera IS NOT NULL
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR photo_kinds(p.mime, p.has_motion, p.has_raw) && sqlc.narg(kinds)::text[])
GROUP BY p.camera
ORDER BY p.camera;

-- name: ListLenses :many
SELECT p.lens::text AS name, count(*) AS count
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  -- A photo not processed yet has no date or size to place it by.
  AND p.status <> 'pending'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (NOT @favorites::boolean
       OR EXISTS (SELECT 1 FROM photo_favorites f WHERE f.photo_id = p.id AND f.user_id = @owner_id))
  -- The user's archived photos leave the timeline and stay in albums and
  -- favorites; the archived filter lists only them.
  AND (CASE WHEN @archived::boolean
            THEN EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            WHEN sqlc.narg(album_id)::bigint IS NULL AND NOT @favorites::boolean
            THEN NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            ELSE true END)
  AND p.lens IS NOT NULL
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR photo_kinds(p.mime, p.has_motion, p.has_raw) && sqlc.narg(kinds)::text[])
GROUP BY p.lens
ORDER BY p.lens;

-- name: ListKinds :many
SELECT k.name::text AS name, count(*) AS count
FROM photos p
CROSS JOIN LATERAL unnest(photo_kinds(p.mime, p.has_motion, p.has_raw)) AS k(name)
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  -- A photo not processed yet has no date or size to place it by.
  AND p.status <> 'pending'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND (NOT @favorites::boolean
       OR EXISTS (SELECT 1 FROM photo_favorites f WHERE f.photo_id = p.id AND f.user_id = @owner_id))
  -- The user's archived photos leave the timeline and stay in albums and
  -- favorites; the archived filter lists only them.
  AND (CASE WHEN @archived::boolean
            THEN EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            WHEN sqlc.narg(album_id)::bigint IS NULL AND NOT @favorites::boolean
            THEN NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
            ELSE true END)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
GROUP BY 1
ORDER BY 1;

-- name: ListTrash :many
SELECT *
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = photos.id)
  AND (sqlc.narg(after_deleted_at)::timestamptz IS NULL
       OR (deleted_at, id) < (sqlc.narg(after_deleted_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY deleted_at DESC, id DESC
LIMIT @max_rows;

-- Includes photos moved to the trash, and photos hidden under another one with
-- the kind of their link, so that other open pages can remove them.
-- name: ListChangedPhotos :many
SELECT sqlc.embed(p), l.kind AS hidden_kind
FROM photos p
LEFT JOIN photo_links l ON l.hidden_id = p.id
WHERE p.owner_id = @owner_id AND p.updated_at > @since AND p.purged_at IS NULL
ORDER BY p.updated_at
LIMIT @max_rows;

-- name: CountPhotosByStatus :one
SELECT count(*) FILTER (WHERE status = 'pending') AS pending,
       count(*) FILTER (WHERE status = 'failed') AS failed
FROM photos
WHERE owner_id = @owner_id AND deleted_at IS NULL;

-- name: MarkPhotoReady :exec
UPDATE photos p
SET status = 'ready',
    error = NULL,
    width = @width,
    height = @height,
    taken_at = sqlc.narg(taken_at),
    sort_at = COALESCE(sqlc.narg(taken_at), sort_at),
    exif = @exif,
    preview_format = @preview_format,
    duration = sqlc.narg(duration),
    hdr = sqlc.narg(hdr),
    -- The video of a Live Photo is its motion too, and an edited copy plays
    -- the motion of its original.
    has_motion = @has_motion OR photo_has_linked_motion(p.id),
    updated_at = now()
WHERE p.id = @id AND p.purged_at IS NULL;

-- name: MarkPhotoFailed :exec
UPDATE photos SET status = 'failed', error = @error, updated_at = now() WHERE id = @id AND purged_at IS NULL;

-- Photos hidden under another one, such as the video of a Live Photo or the
-- original of an edited copy, go into the trash and out of it with the photo
-- shown above them, and not on their own.
-- name: TrashPhotos :exec
WITH RECURSIVE chain AS (
    SELECT p.id
    FROM photos p
    WHERE p.id = ANY (@ids::bigint[])
      AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
      AND (p.owner_id = @actor_id::bigint OR @actor_is_admin::boolean)
    UNION
    SELECT l.hidden_id FROM photo_links l JOIN chain c ON l.shown_id = c.id
)
UPDATE photos
SET deleted_at = now(), updated_at = now()
WHERE id IN (SELECT id FROM chain) AND deleted_at IS NULL;

-- name: RestorePhotos :exec
WITH RECURSIVE chain AS (
    SELECT p.id
    FROM photos p
    WHERE p.id = ANY (@ids::bigint[])
      AND p.owner_id = @owner_id
      AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
    UNION
    SELECT l.hidden_id FROM photo_links l JOIN chain c ON l.shown_id = c.id
)
UPDATE photos u
SET deleted_at = NULL, updated_at = now()
WHERE u.id IN (SELECT id FROM chain)
  AND u.owner_id = @owner_id
  AND u.deleted_at IS NOT NULL
  AND u.purged_at IS NULL;

-- name: RestorePhoto :one
WITH RECURSIVE chain AS (
    SELECT l.hidden_id AS id FROM photo_links l WHERE l.shown_id = @id
    UNION
    SELECT l.hidden_id FROM photo_links l JOIN chain c ON l.shown_id = c.id
),
versions_restored AS (
    UPDATE photos v
    SET deleted_at = NULL, updated_at = now()
    WHERE v.id IN (SELECT id FROM chain) AND v.deleted_at IS NOT NULL AND v.purged_at IS NULL
)
UPDATE photos p
SET deleted_at = NULL, updated_at = now()
WHERE p.id = @id AND p.purged_at IS NULL
RETURNING *;

-- name: ListTrashedPhotoIDs :many
SELECT id
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = photos.id)
ORDER BY id
LIMIT @max_rows;

-- Keeps only what a folder scan needs to recognize the file.
-- name: PurgePhoto :one
UPDATE photos
SET purged_at = now(),
    updated_at = now(),
    visibility = 'private',
    original_name = '',
    width = NULL,
    height = NULL,
    taken_at = NULL,
    exif = NULL,
    error = NULL,
    preview_format = NULL,
    duration = NULL,
    has_motion = false,
    video_copy = NULL
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL
RETURNING *;

-- name: DeletePhotoMemberships :exec
WITH albums_removed AS (DELETE FROM album_photos ap WHERE ap.photo_id = @id),
     favorites_removed AS (DELETE FROM photo_favorites f WHERE f.photo_id = @id),
     archive_removed AS (DELETE FROM photo_archive a WHERE a.photo_id = @id),
     tags_removed AS (DELETE FROM photo_tags pt WHERE pt.photo_id = @id),
     covers_cleared AS (UPDATE albums a SET cover_photo_id = NULL WHERE a.cover_photo_id = @id),
     embeddings_removed AS (DELETE FROM photo_embeddings e WHERE e.photo_id = @id),
     failures_removed AS (DELETE FROM photo_embedding_failures f WHERE f.photo_id = @id)
DELETE FROM photo_shares s WHERE s.photo_id = @id;

-- An upload brings a purged photo back as a new one, as photo ids are never reused.
-- name: DeletePurgedPhoto :exec
DELETE FROM photos WHERE id = @id AND purged_at IS NOT NULL;

-- Serializes changes to one owner's copy of a file, whose path is derived from
-- its hash: an upload must not move a file into place while a purge removes it.
-- name: LockPhotoFile :exec
SELECT pg_advisory_xact_lock(hashtextextended(format('%s/%s', @owner_id::bigint, encode(@sha256::bytea, 'hex')), 0));

-- A photo hidden under another one is purged with it, and not on its own.
-- name: GetTrashedPhoto :one
SELECT * FROM photos
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = photos.id);

-- name: ListReadyPhotoIDs :many
SELECT id FROM photos WHERE status = 'ready' AND purged_at IS NULL ORDER BY id;

-- Moves updated_at too, so that open pages load the new preview.
-- name: BumpPreviewVersion :exec
UPDATE photos
SET preview_version = preview_version + 1,
    updated_at = now()
WHERE id = @id AND status = 'ready' AND purged_at IS NULL;

-- Photos processed before motion photos were supported, whose stored tags
-- name an embedded video that was never extracted.
-- name: ListPhotosMissingMotion :many
SELECT id, owner_id, sha256, mime, exif, source_path
FROM photos
WHERE status = 'ready'
  AND purged_at IS NULL
  AND NOT has_motion
  AND mime NOT LIKE 'video/%'
  AND (exif -> 'MotionPhotoVideo' IS NOT NULL OR exif -> 'EmbeddedVideoFile' IS NOT NULL
       OR exif -> 'MicroVideoOffset' IS NOT NULL)
  AND id > @after_id
ORDER BY id
LIMIT @max_rows;

-- Videos processed before HDR was recorded, for the backfill_video_hdr job.
-- name: ListVideosMissingHDR :many
SELECT id, owner_id, sha256, mime, source_path
FROM photos
WHERE status = 'ready'
  AND purged_at IS NULL
  AND mime LIKE 'video/%'
  AND hdr IS NULL
  AND id > @after_id
ORDER BY id
LIMIT @max_rows;

-- Moves updated_at too, so that open pages learn whether to wait for the copy.
-- name: SetVideoHDR :exec
UPDATE photos
SET hdr = @hdr,
    updated_at = now()
WHERE id = @id AND status = 'ready' AND purged_at IS NULL;

-- Moves updated_at too, so that open pages learn of the motion. An edited
-- copy shown above the photo plays the same motion.
-- name: SetPhotoMotion :exec
UPDATE photos
SET has_motion = true,
    updated_at = now()
WHERE (id = @id OR id IN (SELECT l.shown_id FROM photo_links l WHERE l.hidden_id = @id AND l.kind = 'edited'))
  AND status = 'ready' AND purged_at IS NULL;

-- The video of a Live Photo, which plays as the photo's motion.
-- name: GetLivePhotoVideo :one
SELECT v.*
FROM photo_links l
JOIN photos v ON v.id = l.hidden_id
WHERE l.shown_id = @photo_id AND l.kind = 'live' AND v.purged_at IS NULL;

-- Where the motion of a photo may come from, in order: the photo itself and
-- the original of an edited copy or the RAW under a JPEG, each with the video
-- of its Live Photo if it has one. A photo without such a video plays the
-- motion extracted from it.
-- name: ListMotionSources :many
SELECT c.id AS photo_id, v.owner_id AS video_owner_id, v.sha256 AS video_sha256, v.mime AS video_mime,
       v.source_path AS video_source_path
FROM (
    SELECT @photo_id::bigint AS id, 0 AS rank
    UNION ALL
    SELECT e.hidden_id, 1
    FROM photo_links e
    JOIN photos o ON o.id = e.hidden_id
    WHERE e.shown_id = @photo_id AND e.kind IN ('edited', 'raw') AND o.has_motion AND o.purged_at IS NULL
) c
LEFT JOIN photo_links l ON l.shown_id = c.id AND l.kind = 'live'
LEFT JOIN photos v ON v.id = l.hidden_id AND v.purged_at IS NULL
ORDER BY c.rank;

-- The photos hidden under a photo, the whole chain down, that are not purged.
-- name: ListHiddenVersions :many
WITH RECURSIVE chain AS (
    SELECT l.hidden_id AS id FROM photo_links l WHERE l.shown_id = @photo_id
    UNION
    SELECT l.hidden_id FROM photo_links l JOIN chain c ON l.shown_id = c.id
)
SELECT p.* FROM photos p WHERE p.id IN (SELECT id FROM chain) AND p.purged_at IS NULL ORDER BY p.id;

-- The photo shown at the top of the chain a photo is hidden in, or the photo
-- itself when it is not hidden.
-- name: GetShownPhoto :one
WITH RECURSIVE up AS (
    SELECT @id::bigint AS id, 0 AS depth
    UNION ALL
    SELECT l.shown_id, up.depth + 1 FROM photo_links l JOIN up ON l.hidden_id = up.id WHERE up.depth < 16
)
SELECT p.* FROM photos p JOIN up ON up.id = p.id ORDER BY up.depth DESC LIMIT 1;

-- The link a photo is hidden under, if any.
-- name: GetLinkAbove :one
SELECT shown_id, kind FROM photo_links WHERE hidden_id = @id;

-- The versions right under a photo, for its owner's viewer.
-- name: ListPhotoVersions :many
SELECT l.hidden_id AS id, l.kind
FROM photo_links l
JOIN photos v ON v.id = l.hidden_id
WHERE l.shown_id = @photo_id AND v.purged_at IS NULL
ORDER BY l.kind, l.hidden_id;

-- Purges a photo hidden under another one with the photo above it, like
-- PurgePhoto. It does not ask for the photo to be in the trash: a video paired
-- while its photo was being moved there may have stayed out of it.
-- name: PurgeHiddenPhoto :one
UPDATE photos
SET purged_at = now(),
    deleted_at = COALESCE(deleted_at, now()),
    updated_at = now(),
    visibility = 'private',
    original_name = '',
    width = NULL,
    height = NULL,
    taken_at = NULL,
    exif = NULL,
    error = NULL,
    preview_format = NULL,
    duration = NULL,
    has_motion = false,
    video_copy = NULL
WHERE id = @id AND purged_at IS NULL
  AND EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = photos.id)
RETURNING *;

-- Serializes changes to links, so that two workers that finish the two files
-- of one Live Photo at once cannot pair either of them twice, and a version
-- is not linked while its photo is being paired.
-- name: LockPhotoLinks :exec
SELECT pg_advisory_xact_lock(hashtextextended('live_photo_pairing', 0));

-- Pairs the photos and videos of Live Photos that share a ContentIdentifier
-- and have no pair yet, optionally only those of one owner and identifier.
-- Each run pairs the oldest photo with the oldest video of each identifier, so
-- it is run until it pairs nothing. Neither file may be in the trash: a video
-- pairs with its photo when the photo is restored. An edited copy that an
-- import recorded is not the photo of a video while its original, with the
-- same identifier, is there: the video belongs to the original. The photo
-- takes the video as its motion, and so does an edited copy shown above it;
-- all move updated_at, so that open pages drop the video.
-- name: PairLivePhotos :many
WITH stills AS (
    SELECT DISTINCT ON (s.owner_id, s.content_identifier) s.id, s.owner_id, s.content_identifier
    FROM photos s
    WHERE s.content_identifier IS NOT NULL
      AND s.mime NOT LIKE 'video/%'
      AND s.status = 'ready'
      AND s.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM photo_links o WHERE o.shown_id = s.id AND o.kind = 'live')
      -- A RAW hidden under its JPEG leaves the video to the JPEG.
      AND NOT EXISTS (SELECT 1 FROM photo_links h WHERE h.hidden_id = s.id AND h.kind = 'raw')
      AND NOT EXISTS (SELECT 1 FROM imported_metadata m JOIN photos o ON o.id = m.original_id
                      WHERE m.photo_id = s.id AND o.owner_id = s.owner_id AND o.purged_at IS NULL
                        AND o.content_identifier = s.content_identifier)
      AND (sqlc.narg(owner_id)::bigint IS NULL OR s.owner_id = sqlc.narg(owner_id)::bigint)
      AND (sqlc.narg(content_identifier)::text IS NULL OR s.content_identifier = sqlc.narg(content_identifier)::text)
    ORDER BY s.owner_id, s.content_identifier, s.id
),
videos AS (
    SELECT DISTINCT ON (v.owner_id, v.content_identifier) v.id, v.owner_id, v.content_identifier
    FROM photos v
    WHERE v.content_identifier IS NOT NULL
      AND v.mime LIKE 'video/%'
      AND v.status = 'ready'
      AND v.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = v.id)
      AND (sqlc.narg(owner_id)::bigint IS NULL OR v.owner_id = sqlc.narg(owner_id)::bigint)
      AND (sqlc.narg(content_identifier)::text IS NULL OR v.content_identifier = sqlc.narg(content_identifier)::text)
    ORDER BY v.owner_id, v.content_identifier, v.id
),
pairs AS (
    SELECT s.id AS still_id, v.id AS video_id
    FROM stills s
    JOIN videos v ON v.owner_id = s.owner_id AND v.content_identifier = s.content_identifier
),
stills_marked AS (
    UPDATE photos p
    SET has_motion = true, updated_at = now()
    WHERE p.id IN (SELECT still_id FROM pairs
                   UNION
                   SELECT e.shown_id FROM photo_links e JOIN pairs ON e.hidden_id = pairs.still_id
                   WHERE e.kind = 'edited')
),
videos_marked AS (
    UPDATE photos p
    SET updated_at = now()
    WHERE p.id IN (SELECT video_id FROM pairs)
)
INSERT INTO photo_links (hidden_id, shown_id, kind)
SELECT video_id, still_id, 'live' FROM pairs
RETURNING hidden_id AS video_id, shown_id AS photo_id;

-- Gives back the motion to photos that lost it: processing that waited for a
-- pairing or a link to commit writes the flag from what it saw before. The
-- daily pairing looks at every photo, and restoring at the owner's.
-- name: RepairLinkedMotion :exec
UPDATE photos p
SET has_motion = true, updated_at = now()
WHERE NOT p.has_motion
  AND p.purged_at IS NULL
  AND (sqlc.narg(owner_id)::bigint IS NULL OR p.owner_id = sqlc.narg(owner_id)::bigint)
  AND photo_has_linked_motion(p.id);

-- RepairLinkedMotion for the photos of one Live Photo, which processing
-- pairs: those with its identifier, and the edited copies shown above them.
-- name: RepairLinkedMotionOf :exec
UPDATE photos p
SET has_motion = true, updated_at = now()
WHERE p.id IN (SELECT s.id FROM photos s
               WHERE s.owner_id = @owner_id AND s.content_identifier = @content_identifier::text
               UNION
               SELECT e.shown_id FROM photos s JOIN photo_links e ON e.hidden_id = s.id AND e.kind = 'edited'
               WHERE s.owner_id = @owner_id AND s.content_identifier = @content_identifier::text)
  AND NOT p.has_motion
  AND p.purged_at IS NULL
  AND photo_has_linked_motion(p.id);

-- Items whose copy the transcode job has not looked at yet: videos that are
-- not the video of a Live Photo, and photos with motion. Items in the trash
-- wait until they are restored.
-- name: ListPhotosMissingCopy :many
SELECT id
FROM photos
WHERE status = 'ready'
  AND purged_at IS NULL
  AND deleted_at IS NULL
  AND video_copy IS NULL
  AND ((mime LIKE 'video/%' AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = photos.id)) OR has_motion)
  AND id > @after_id
ORDER BY id
LIMIT @max_rows;

-- Moves updated_at too, so that open pages learn of the copy.
-- name: SetPhotoVideoCopy :exec
UPDATE photos
SET video_copy = @video_copy,
    updated_at = now()
WHERE id = @id AND purged_at IS NULL;

-- The processed photos among the given ones, or edited copies of them, that
-- have metadata from an import, with what the date and location are worked
-- out from.
-- name: ListImportedForApply :many
SELECT m.photo_id, p.mime, p.exif, m.taken_at, m.latitude, m.longitude,
       (m.original_id IS NOT NULL)::boolean AS edited, o.mime AS original_mime, o.exif AS original_exif
FROM imported_metadata m
JOIN photos p ON p.id = m.photo_id
LEFT JOIN photos o ON o.id = m.original_id
WHERE (m.photo_id = ANY(@ids::bigint[]) OR m.original_id = ANY(@ids::bigint[])) AND p.status = 'ready';

-- Sets the date and the imported location of photos. A date of the zero time
-- and a location of NaN stand for none.
-- name: SetImportedDetails :exec
UPDATE photos p
SET taken_at = NULLIF(u.taken_at, '0001-01-01 00:00:00+00'),
    sort_at = COALESCE(NULLIF(u.taken_at, '0001-01-01 00:00:00+00'), p.sort_at),
    imported_latitude = NULLIF(u.latitude, 'NaN'),
    imported_longitude = NULLIF(u.longitude, 'NaN')
FROM (
    SELECT unnest(@ids::bigint[]) AS id, unnest(@taken_ats::timestamptz[]) AS taken_at,
           unnest(@latitudes::double precision[]) AS latitude, unnest(@longitudes::double precision[]) AS longitude
) u
WHERE p.id = u.id;

-- Sets the description of one of the owner's photos, cleaned as
-- clean_description does. Moves updated_at, so that open pages get it.
-- name: SetPhotoDescription :one
UPDATE photos
SET description = clean_description(@description::text), updated_at = now()
WHERE id = @id AND owner_id = @owner_id AND purged_at IS NULL
RETURNING *;

-- Marks photos the user can see as the user's favorites. Photos hidden under
-- another one, in the trash or purged are left out.
-- name: AddFavorites :exec
INSERT INTO photo_favorites (user_id, photo_id)
SELECT @user_id::bigint, p.id
FROM photos p
WHERE p.id = ANY (@ids::bigint[])
  AND p.deleted_at IS NULL
  AND p.purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND photo_visible_to(p, @user_id::bigint, false)
ON CONFLICT DO NOTHING;

-- name: RemoveFavorites :exec
DELETE FROM photo_favorites WHERE user_id = @user_id AND photo_id = ANY (@ids::bigint[]);

-- name: IsFavorite :one
SELECT EXISTS (SELECT 1 FROM photo_favorites WHERE user_id = @user_id AND photo_id = @photo_id);

-- Archives photos the user can see, for the user. Photos hidden under another
-- one, in the trash or purged are left out.
-- name: AddToArchive :exec
INSERT INTO photo_archive (user_id, photo_id)
SELECT @user_id::bigint, p.id
FROM photos p
WHERE p.id = ANY (@ids::bigint[])
  AND p.deleted_at IS NULL
  AND p.purged_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND photo_visible_to(p, @user_id::bigint, false)
ON CONFLICT DO NOTHING;

-- name: RemoveFromArchive :exec
DELETE FROM photo_archive WHERE user_id = @user_id AND photo_id = ANY (@ids::bigint[]);

-- name: IsArchived :one
SELECT EXISTS (SELECT 1 FROM photo_archive WHERE user_id = @user_id AND photo_id = @photo_id);

-- The photos a user sees on the timeline with "include shared" on, sorted
-- inside the windows of days that the memories of one date take from earlier
-- years: from_times[i] up to to_times[i]. The API picks the days to show from
-- them. The windows are worked out outside, in the viewer's time zone, so that
-- the photos_timeline_idx indexes can serve them. The user's archived photos
-- are left out, as on the timeline. A photo that is not ready has no preview
-- for a card.
-- name: ListMemories :many
SELECT p.*
FROM photos p
JOIN (SELECT unnest(@from_times::timestamptz[]) AS from_time, unnest(@to_times::timestamptz[]) AS to_time) w
  ON p.sort_at >= w.from_time AND p.sort_at < w.to_time
WHERE (p.owner_id = @owner_id
       OR (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint)))
  AND p.status = 'ready'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
ORDER BY p.sort_at, p.id
LIMIT 5000;

-- The sort time of the oldest photo, which bounds how many years of memories
-- are looked through.
-- name: OldestPhotoTime :one
SELECT coalesce(min(sort_at), now())::timestamptz AS oldest
FROM photos
WHERE status = 'ready' AND deleted_at IS NULL;

-- The photos of the map in one box, grouped into cells of a grid that follows
-- the zoom: cells is how many columns the world has at it, and a cell is as tall
-- as it is wide on the Mercator projection, so that cells do not stretch near
-- the poles. Each cell is a cluster at the centre of its photos, with the newest
-- photo as its cover and the box that holds them. The same photos as
-- ListMemories: the user's own and shared with them, out of the archive, the
-- trash and hidden versions. The API allows it only where it shows locations.
-- name: ListMapClusters :many
SELECT count(*)::bigint AS count,
       avg(p.latitude)::double precision AS latitude,
       avg(p.longitude)::double precision AS longitude,
       min(p.latitude)::double precision AS south,
       min(p.longitude)::double precision AS west,
       max(p.latitude)::double precision AS north,
       max(p.longitude)::double precision AS east,
       (array_agg(p.id ORDER BY p.sort_at DESC, p.id DESC))[1]::bigint AS cover_id,
       (array_agg(p.preview_version ORDER BY p.sort_at DESC, p.id DESC))[1]::integer AS cover_version
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint)))
  AND p.status = 'ready'
  AND p.deleted_at IS NULL
  AND p.latitude IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM photo_archive a WHERE a.photo_id = p.id AND a.user_id = @owner_id)
  AND p.latitude BETWEEN @south::double precision AND @north::double precision
  AND (CASE WHEN @west::double precision <= @east::double precision
            THEN p.longitude BETWEEN @west::double precision AND @east::double precision
            ELSE p.longitude >= @west::double precision OR p.longitude <= @east::double precision END)
GROUP BY floor((p.longitude + 180) / 360 * @cells::double precision),
         floor((1 - ln(tan(radians(least(greatest(p.latitude, -85.05), 85.05)))
                        + 1 / cos(radians(least(greatest(p.latitude, -85.05), 85.05))))
                    / pi()) / 2 * @cells::double precision)
ORDER BY count DESC;
