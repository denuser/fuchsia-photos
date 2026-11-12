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
  AND photo_visible_to(p, @viewer_id::bigint, @viewer_is_admin::boolean);

-- The photos of one owner: the current user, or the owner of an album the
-- user can see. With with_shared, which the API allows only without an album,
-- the list also takes other users' processed photos that the current user can
-- see, as photo_visible_to does. Tags are personal, so a tag filter keeps only
-- the owner's photos: another user's tag must not reveal how that user grouped
-- the photos it shared. The queries below take the same filters.
-- name: ListTimeline :many
SELECT p.*
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
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
  AND p.live_photo_id IS NULL
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
ORDER BY 1 DESC;

-- The cameras and lenses of the photos ListTimeline filters, with how many
-- photos each took, for the filters of the timeline. Each list takes every
-- filter but its own: the filter keeps any of its names, so a name not chosen
-- yet counts the photos it would add.
-- name: ListCameras :many
SELECT p.camera::text AS name, count(*) AS count
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND p.camera IS NOT NULL
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
GROUP BY p.camera
ORDER BY p.camera;

-- name: ListLenses :many
SELECT p.lens::text AS name, count(*) AS count
FROM photos p
WHERE (p.owner_id = @owner_id
       OR (@with_shared::boolean AND p.status = 'ready'
           AND (photo_shared_directly(p, @owner_id::bigint) OR photo_in_visible_album(p, @owner_id::bigint))))
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND p.lens IS NOT NULL
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR (p.owner_id = @owner_id
           AND EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[]))))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
GROUP BY p.lens
ORDER BY p.lens;

-- name: ListTrash :many
SELECT *
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL
  AND live_photo_id IS NULL
  AND (sqlc.narg(after_deleted_at)::timestamptz IS NULL
       OR (deleted_at, id) < (sqlc.narg(after_deleted_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY deleted_at DESC, id DESC
LIMIT @max_rows;

-- Includes photos moved to the trash, so that other open pages can remove them.
-- name: ListChangedPhotos :many
SELECT *
FROM photos
WHERE owner_id = @owner_id AND updated_at > @since AND purged_at IS NULL
ORDER BY updated_at
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
    -- The video of a Live Photo is its motion too.
    has_motion = @has_motion OR EXISTS (SELECT 1 FROM photos v WHERE v.live_photo_id = p.id),
    updated_at = now()
WHERE p.id = @id AND p.purged_at IS NULL;

-- name: MarkPhotoFailed :exec
UPDATE photos SET status = 'failed', error = @error, updated_at = now() WHERE id = @id AND purged_at IS NULL;

-- The video of a Live Photo goes into the trash and out of it with its photo,
-- and not on its own.
-- name: TrashPhotos :exec
UPDATE photos
SET deleted_at = now(), updated_at = now()
WHERE (id = ANY (@ids::bigint[]) AND live_photo_id IS NULL OR live_photo_id = ANY (@ids::bigint[]))
  AND deleted_at IS NULL
  AND (owner_id = @actor_id::bigint OR @actor_is_admin::boolean);

-- name: RestorePhotos :exec
UPDATE photos
SET deleted_at = NULL, updated_at = now()
WHERE owner_id = @owner_id
  AND (id = ANY (@ids::bigint[]) AND live_photo_id IS NULL OR live_photo_id = ANY (@ids::bigint[]))
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL;

-- name: RestorePhoto :one
WITH video_restored AS (
    UPDATE photos v
    SET deleted_at = NULL, updated_at = now()
    WHERE v.live_photo_id = @id AND v.deleted_at IS NOT NULL AND v.purged_at IS NULL
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
  AND live_photo_id IS NULL
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
    has_motion = false
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL
RETURNING *;

-- name: DeletePhotoLinks :exec
WITH albums_removed AS (DELETE FROM album_photos ap WHERE ap.photo_id = @id),
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

-- The video of a Live Photo is purged with its photo, and not on its own.
-- name: GetTrashedPhoto :one
SELECT * FROM photos
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL AND live_photo_id IS NULL;

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
SELECT id, owner_id, sha256, mime, exif
FROM photos
WHERE status = 'ready'
  AND purged_at IS NULL
  AND NOT has_motion
  AND mime NOT LIKE 'video/%'
  AND (exif -> 'MotionPhotoVideo' IS NOT NULL OR exif -> 'EmbeddedVideoFile' IS NOT NULL)
  AND id > @after_id
ORDER BY id
LIMIT @max_rows;

-- Moves updated_at too, so that open pages learn of the motion.
-- name: SetPhotoMotion :exec
UPDATE photos
SET has_motion = true,
    updated_at = now()
WHERE id = @id AND status = 'ready' AND purged_at IS NULL;

-- The video of a Live Photo, which plays as the photo's motion.
-- name: GetLivePhotoVideo :one
SELECT * FROM photos WHERE live_photo_id = @photo_id AND purged_at IS NULL;

-- Purges the video of a Live Photo with its photo, like PurgePhoto. It does not
-- ask for the video to be in the trash: a video paired while its photo was
-- being moved there may have stayed out of it.
-- name: PurgeLivePhotoVideo :one
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
    has_motion = false
WHERE id = @id AND live_photo_id = @photo_id AND purged_at IS NULL
RETURNING *;

-- Serializes pairing, so that two workers that finish the two files of one
-- Live Photo at once cannot pair either of them twice.
-- name: LockLivePhotoPairing :exec
SELECT pg_advisory_xact_lock(hashtextextended('live_photo_pairing', 0));

-- Pairs the photos and videos of Live Photos that share a ContentIdentifier
-- and have no pair yet, optionally only those of one owner and identifier.
-- Each run pairs the oldest photo with the oldest video of each identifier, so
-- it is run until it pairs nothing. Neither file may be in the trash: a video
-- pairs with its photo when the photo is restored. The photo takes the video
-- as its motion, and both move updated_at, so that open pages drop the video.
-- name: PairLivePhotos :many
WITH stills AS (
    SELECT DISTINCT ON (s.owner_id, s.content_identifier) s.id, s.owner_id, s.content_identifier
    FROM photos s
    WHERE s.content_identifier IS NOT NULL
      AND s.mime NOT LIKE 'video/%'
      AND s.status = 'ready'
      AND s.deleted_at IS NULL
      AND s.live_photo_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM photos o WHERE o.live_photo_id = s.id)
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
      AND v.live_photo_id IS NULL
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
    FROM pairs
    WHERE p.id = pairs.still_id
)
UPDATE photos p
SET live_photo_id = pairs.still_id, updated_at = now()
FROM pairs
WHERE p.id = pairs.video_id
RETURNING p.id AS video_id, p.live_photo_id::bigint AS photo_id;

-- Gives back the motion to paired photos that lost it: processing that waited
-- for a pairing to commit writes the flag from what it saw before.
-- name: RepairLivePhotoMotion :exec
UPDATE photos p
SET has_motion = true, updated_at = now()
WHERE NOT p.has_motion
  AND p.purged_at IS NULL
  AND (sqlc.narg(owner_id)::bigint IS NULL OR p.owner_id = sqlc.narg(owner_id)::bigint)
  AND (sqlc.narg(content_identifier)::text IS NULL OR p.content_identifier = sqlc.narg(content_identifier)::text)
  AND EXISTS (SELECT 1 FROM photos v WHERE v.live_photo_id = p.id AND v.purged_at IS NULL);
