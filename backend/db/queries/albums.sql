-- These three queries return the same columns, so the handlers can treat their rows alike.
-- name: ListAlbums :many
SELECT a.id, a.owner_id, a.name, a.created_at, a.cover_photo_id AS chosen_cover_id,
       (a.visibility = 'all' OR EXISTS (SELECT 1 FROM album_shares s WHERE s.album_id = a.id))::boolean AS shared,
       (SELECT count(*)
        FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
        WHERE ap.album_id = a.id AND p.deleted_at IS NULL)::bigint AS photo_count,
       COALESCE((SELECT p.id
                 FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
                 WHERE ap.album_id = a.id AND p.deleted_at IS NULL AND p.status = 'ready'
                 ORDER BY p.id = a.cover_photo_id DESC NULLS LAST, p.sort_at DESC, p.id DESC
                 LIMIT 1), 0)::bigint AS cover_id
FROM albums a
WHERE a.owner_id = @owner_id
  AND (sqlc.narg(id)::bigint IS NULL OR a.id = sqlc.narg(id)::bigint)
ORDER BY lower(a.name), a.id;

-- name: ListSharedAlbums :many
SELECT a.id, a.owner_id, a.name, a.created_at, a.cover_photo_id AS chosen_cover_id,
       (a.visibility = 'all' OR EXISTS (SELECT 1 FROM album_shares s WHERE s.album_id = a.id))::boolean AS shared,
       (SELECT count(*)
        FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
        WHERE ap.album_id = a.id AND p.deleted_at IS NULL)::bigint AS photo_count,
       COALESCE((SELECT p.id
                 FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
                 WHERE ap.album_id = a.id AND p.deleted_at IS NULL AND p.status = 'ready'
                 ORDER BY p.id = a.cover_photo_id DESC NULLS LAST, p.sort_at DESC, p.id DESC
                 LIMIT 1), 0)::bigint AS cover_id
FROM albums a
WHERE a.owner_id <> @viewer_id::bigint
  AND album_visible_to(a, @viewer_id::bigint, false)
ORDER BY lower(a.name), a.id;

-- name: GetVisibleAlbum :one
SELECT a.id, a.owner_id, a.name, a.created_at, a.cover_photo_id AS chosen_cover_id,
       (a.visibility = 'all' OR EXISTS (SELECT 1 FROM album_shares s WHERE s.album_id = a.id))::boolean AS shared,
       (SELECT count(*)
        FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
        WHERE ap.album_id = a.id AND p.deleted_at IS NULL)::bigint AS photo_count,
       COALESCE((SELECT p.id
                 FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
                 WHERE ap.album_id = a.id AND p.deleted_at IS NULL AND p.status = 'ready'
                 ORDER BY p.id = a.cover_photo_id DESC NULLS LAST, p.sort_at DESC, p.id DESC
                 LIMIT 1), 0)::bigint AS cover_id
FROM albums a
WHERE a.id = @id
  AND album_visible_to(a, @viewer_id::bigint, @viewer_is_admin::boolean);

-- name: CreateAlbum :one
INSERT INTO albums (owner_id, name) VALUES (@owner_id, @name) RETURNING id;

-- name: RenameAlbum :execrows
UPDATE albums SET name = @name WHERE id = @id AND owner_id = @owner_id;

-- name: DeleteAlbum :execrows
DELETE FROM albums WHERE id = @id AND owner_id = @owner_id;

-- A cover must be a photo of the album that is not in the trash.
-- name: SetAlbumCover :execrows
UPDATE albums a
SET cover_photo_id = @photo_id
WHERE a.id = @id
  AND a.owner_id = @owner_id
  AND EXISTS (SELECT 1
              FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
              WHERE ap.album_id = a.id AND ap.photo_id = @photo_id AND p.deleted_at IS NULL);

-- name: ClearAlbumCover :execrows
UPDATE albums SET cover_photo_id = NULL WHERE id = @id AND owner_id = @owner_id;

-- name: AddPhotosToAlbum :exec
INSERT INTO album_photos (album_id, photo_id)
SELECT a.id, p.id
FROM albums a
JOIN photos p ON p.owner_id = a.owner_id
WHERE a.id = @album_id
  AND a.owner_id = @owner_id
  AND p.id = ANY (@ids::bigint[])
  AND p.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- name: RemovePhotosFromAlbum :exec
DELETE FROM album_photos ap
USING albums a
WHERE ap.album_id = a.id
  AND a.id = @album_id
  AND a.owner_id = @owner_id
  AND ap.photo_id = ANY (@ids::bigint[]);
