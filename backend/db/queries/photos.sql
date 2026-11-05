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

-- name: ListTimeline :many
SELECT p.*
FROM photos p
WHERE p.owner_id = @owner_id
  AND p.deleted_at IS NULL
  AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
       OR (p.sort_at, p.id) < (sqlc.narg(after_sort_at)::timestamptz, sqlc.narg(after_id)::bigint))
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[])))
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
WHERE p.owner_id = @owner_id
  AND p.deleted_at IS NULL
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(album_id)::bigint IS NULL
       OR EXISTS (SELECT 1 FROM album_photos ap WHERE ap.photo_id = p.id AND ap.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(tag_ids)::bigint[] IS NULL
       OR EXISTS (SELECT 1 FROM photo_tags pt WHERE pt.photo_id = p.id AND pt.tag_id = ANY (sqlc.narg(tag_ids)::bigint[])))
  AND (sqlc.narg(cameras)::text[] IS NULL OR p.camera = ANY (sqlc.narg(cameras)::text[]))
  AND (sqlc.narg(lenses)::text[] IS NULL OR p.lens = ANY (sqlc.narg(lenses)::text[]))
GROUP BY 1
ORDER BY 1 DESC;

-- The cameras and lenses of the current user's photos, with how many photos
-- each took, for the filters of the timeline.
-- name: ListCameras :many
SELECT camera::text AS name, count(*) AS count
FROM photos
WHERE owner_id = @owner_id AND deleted_at IS NULL AND camera IS NOT NULL
GROUP BY camera
ORDER BY camera;

-- name: ListLenses :many
SELECT lens::text AS name, count(*) AS count
FROM photos
WHERE owner_id = @owner_id AND deleted_at IS NULL AND lens IS NOT NULL
GROUP BY lens
ORDER BY lens;

-- name: ListTrash :many
SELECT *
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL
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
UPDATE photos
SET status = 'ready',
    error = NULL,
    width = @width,
    height = @height,
    taken_at = sqlc.narg(taken_at),
    sort_at = COALESCE(sqlc.narg(taken_at), sort_at),
    exif = @exif,
    preview_format = @preview_format,
    updated_at = now()
WHERE id = @id AND purged_at IS NULL;

-- name: MarkPhotoFailed :exec
UPDATE photos SET status = 'failed', error = @error, updated_at = now() WHERE id = @id AND purged_at IS NULL;

-- name: TrashPhotos :exec
UPDATE photos
SET deleted_at = now(), updated_at = now()
WHERE id = ANY (@ids::bigint[])
  AND deleted_at IS NULL
  AND (owner_id = @actor_id::bigint OR @actor_is_admin::boolean);

-- name: RestorePhotos :exec
UPDATE photos
SET deleted_at = NULL, updated_at = now()
WHERE owner_id = @owner_id AND id = ANY (@ids::bigint[]) AND deleted_at IS NOT NULL AND purged_at IS NULL;

-- name: RestorePhoto :one
UPDATE photos
SET deleted_at = NULL, updated_at = now()
WHERE id = @id AND purged_at IS NULL
RETURNING *;

-- name: ListTrashedPhotoIDs :many
SELECT id
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND purged_at IS NULL
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
    preview_format = NULL
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL
RETURNING *;

-- name: DeletePhotoLinks :exec
WITH albums_removed AS (DELETE FROM album_photos ap WHERE ap.photo_id = @id),
     tags_removed AS (DELETE FROM photo_tags pt WHERE pt.photo_id = @id),
     covers_cleared AS (UPDATE albums a SET cover_photo_id = NULL WHERE a.cover_photo_id = @id)
DELETE FROM photo_shares s WHERE s.photo_id = @id;

-- An upload brings a purged photo back as a new one, as photo ids are never reused.
-- name: DeletePurgedPhoto :exec
DELETE FROM photos WHERE id = @id AND purged_at IS NOT NULL;

-- Serializes changes to one owner's copy of a file, whose path is derived from
-- its hash: an upload must not move a file into place while a purge removes it.
-- name: LockPhotoFile :exec
SELECT pg_advisory_xact_lock(hashtextextended(format('%s/%s', @owner_id::bigint, encode(@sha256::bytea, 'hex')), 0));

-- name: GetTrashedPhoto :one
SELECT * FROM photos WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL AND purged_at IS NULL;
