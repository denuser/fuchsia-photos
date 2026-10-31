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
ORDER BY p.sort_at DESC, p.id DESC
LIMIT @max_rows;

-- name: ListTrash :many
SELECT *
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
  AND (sqlc.narg(after_deleted_at)::timestamptz IS NULL
       OR (deleted_at, id) < (sqlc.narg(after_deleted_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY deleted_at DESC, id DESC
LIMIT @max_rows;

-- Includes photos moved to the trash, so that other open pages can remove them.
-- name: ListChangedPhotos :many
SELECT *
FROM photos
WHERE owner_id = @owner_id AND updated_at > @since
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
WHERE id = @id;

-- name: MarkPhotoFailed :exec
UPDATE photos SET status = 'failed', error = @error, updated_at = now() WHERE id = @id;

-- name: TrashPhotos :exec
UPDATE photos
SET deleted_at = now(), updated_at = now()
WHERE owner_id = @owner_id AND id = ANY (@ids::bigint[]) AND deleted_at IS NULL;

-- name: RestorePhotos :exec
UPDATE photos
SET deleted_at = NULL, updated_at = now()
WHERE owner_id = @owner_id AND id = ANY (@ids::bigint[]) AND deleted_at IS NOT NULL;

-- name: RestorePhoto :one
UPDATE photos
SET deleted_at = NULL, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListTrashedPhotoIDs :many
SELECT id
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NOT NULL
ORDER BY id
LIMIT @max_rows;

-- name: PurgePhoto :one
DELETE FROM photos
WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL
RETURNING *;

-- Serializes changes to one owner's copy of a file, whose path is derived from
-- its hash: an upload must not move a file into place while a purge removes it.
-- name: LockPhotoFile :exec
SELECT pg_advisory_xact_lock(hashtextextended(format('%s/%s', @owner_id::bigint, encode(@sha256::bytea, 'hex')), 0));

-- name: GetTrashedPhotoHash :one
SELECT sha256 FROM photos WHERE id = @id AND owner_id = @owner_id AND deleted_at IS NOT NULL;
