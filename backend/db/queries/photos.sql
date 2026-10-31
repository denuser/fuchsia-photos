-- name: InsertPhoto :one
INSERT INTO photos (owner_id, sha256, mime, size, original_name)
VALUES (@owner_id, @sha256, @mime, @size, @original_name)
ON CONFLICT (owner_id, sha256) DO NOTHING
RETURNING *;

-- name: GetPhotoByHash :one
SELECT * FROM photos WHERE owner_id = @owner_id AND sha256 = @sha256;

-- name: GetPhoto :one
SELECT * FROM photos WHERE id = @id AND deleted_at IS NULL;

-- name: GetVisiblePhoto :one
SELECT p.*
FROM photos p
WHERE p.id = @id
  AND p.deleted_at IS NULL
  AND photo_visible_to(p, @viewer_id::bigint, @viewer_is_admin::boolean);

-- name: ListTimeline :many
SELECT *
FROM photos
WHERE owner_id = @owner_id
  AND deleted_at IS NULL
  AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
       OR (sort_at, id) < (sqlc.narg(after_sort_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY sort_at DESC, id DESC
LIMIT @max_rows;

-- name: ListChangedPhotos :many
SELECT *
FROM photos
WHERE owner_id = @owner_id AND updated_at > @since AND deleted_at IS NULL
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
