-- Photos of other users that the viewer can see. A shared album can hold
-- thousands of photos, so its photos join the list only if the viewer asks.
-- name: ListSharedPhotos :many
SELECT p.*
FROM photos p
WHERE p.owner_id <> @viewer_id::bigint
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND p.status = 'ready'
  AND (photo_shared_directly(p, @viewer_id::bigint)
       OR (@include_albums::boolean AND photo_in_visible_album(p, @viewer_id::bigint)))
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
       OR (p.sort_at, p.id) < (sqlc.narg(after_sort_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY p.sort_at DESC, p.id DESC
LIMIT @max_rows;

-- Counts the photos of ListSharedPhotos by month, like CountTimelineMonths.
-- name: CountSharedMonths :many
SELECT to_char(date_trunc('month', p.sort_at AT TIME ZONE @tz::text), 'YYYY-MM')::text AS month,
       count(*) AS count
FROM photos p
WHERE p.owner_id <> @viewer_id::bigint
  AND p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND p.status = 'ready'
  AND (photo_shared_directly(p, @viewer_id::bigint)
       OR (@include_albums::boolean AND photo_in_visible_album(p, @viewer_id::bigint)))
GROUP BY 1
ORDER BY 1 DESC;

-- The administrator's view of all photos.
-- name: ListAllPhotos :many
SELECT p.*
FROM photos p
WHERE p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND (sqlc.narg(owner_id)::bigint IS NULL OR p.owner_id = sqlc.narg(owner_id)::bigint)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR p.sort_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR p.sort_at < sqlc.narg(to_time)::timestamptz)
  AND (sqlc.narg(after_sort_at)::timestamptz IS NULL
       OR (p.sort_at, p.id) < (sqlc.narg(after_sort_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY p.sort_at DESC, p.id DESC
LIMIT @max_rows;

-- Counts the photos of ListAllPhotos by month, like CountTimelineMonths.
-- name: CountAllMonths :many
SELECT to_char(date_trunc('month', p.sort_at AT TIME ZONE @tz::text), 'YYYY-MM')::text AS month,
       count(*) AS count
FROM photos p
WHERE p.deleted_at IS NULL
  AND p.live_photo_id IS NULL
  AND (sqlc.narg(owner_id)::bigint IS NULL OR p.owner_id = sqlc.narg(owner_id)::bigint)
GROUP BY 1
ORDER BY 1 DESC;

-- name: GetManagedPhotoVisibility :one
SELECT visibility
FROM photos
WHERE id = @id
  AND deleted_at IS NULL
  AND (owner_id = @actor_id::bigint OR @actor_is_admin::boolean);

-- name: SetPhotosVisibility :exec
UPDATE photos
SET visibility = @visibility, updated_at = now()
WHERE id = ANY (@ids::bigint[])
  AND deleted_at IS NULL
  AND visibility <> @visibility
  AND (owner_id = @actor_id::bigint OR @actor_is_admin::boolean);

-- name: SharePhotos :exec
INSERT INTO photo_shares (photo_id, user_id)
SELECT p.id, u.id
FROM photos p
CROSS JOIN users u
WHERE p.id = ANY (@ids::bigint[])
  AND p.deleted_at IS NULL
  AND (p.owner_id = @actor_id::bigint OR @actor_is_admin::boolean)
  AND u.id = ANY (@user_ids::bigint[])
  AND u.id <> p.owner_id
  AND NOT u.disabled
ON CONFLICT DO NOTHING;

-- name: UnsharePhotos :exec
DELETE FROM photo_shares s
USING photos p
WHERE s.photo_id = p.id
  AND p.id = ANY (@ids::bigint[])
  AND (p.owner_id = @actor_id::bigint OR @actor_is_admin::boolean)
  AND s.user_id = ANY (@user_ids::bigint[]);

-- name: ListPhotoShareUsers :many
SELECT u.id, u.login
FROM photo_shares s JOIN users u ON u.id = s.user_id
WHERE s.photo_id = @photo_id
ORDER BY lower(u.login);

-- name: GetOwnAlbumVisibility :one
SELECT visibility FROM albums WHERE id = @id AND owner_id = @owner_id;

-- name: SetAlbumVisibility :exec
UPDATE albums SET visibility = @visibility WHERE id = @id AND owner_id = @owner_id;

-- name: ShareAlbum :exec
INSERT INTO album_shares (album_id, user_id)
SELECT a.id, u.id
FROM albums a
CROSS JOIN users u
WHERE a.id = @album_id
  AND a.owner_id = @owner_id
  AND u.id = ANY (@user_ids::bigint[])
  AND u.id <> a.owner_id
  AND NOT u.disabled
ON CONFLICT DO NOTHING;

-- name: UnshareAlbum :exec
DELETE FROM album_shares s
USING albums a
WHERE s.album_id = a.id
  AND a.id = @album_id
  AND a.owner_id = @owner_id
  AND s.user_id = ANY (@user_ids::bigint[]);

-- name: ListAlbumShareUsers :many
SELECT u.id, u.login
FROM album_shares s JOIN users u ON u.id = s.user_id
WHERE s.album_id = @album_id
ORDER BY lower(u.login);
