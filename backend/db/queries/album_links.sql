-- name: CreateAlbumLink :one
INSERT INTO album_links (album_id, token, password_hash, expires_at)
SELECT a.id, @token, @password_hash, sqlc.narg(expires_at)::timestamptz
FROM albums a WHERE a.id = @album_id AND a.owner_id = @owner_id
RETURNING *;

-- name: ListAlbumLinks :many
SELECT l.* FROM album_links l JOIN albums a ON a.id = l.album_id
WHERE l.album_id = @album_id AND a.owner_id = @owner_id
ORDER BY l.created_at, l.id;

-- name: CountAlbumLinks :one
SELECT count(*) FROM album_links WHERE album_id = @album_id;

-- name: DeleteAlbumLink :execrows
DELETE FROM album_links l USING albums a
WHERE l.id = @id AND l.album_id = @album_id AND a.id = l.album_id AND a.owner_id = @owner_id;

-- A link works while it has not expired and its owner is active. Anything else looks like no link.
-- name: GetPublicLink :one
SELECT l.id, l.album_id, l.password_hash, l.expires_at, a.name AS album_name
FROM album_links l
JOIN albums a ON a.id = l.album_id
JOIN users u ON u.id = a.owner_id
WHERE l.token = @token
  AND (l.expires_at IS NULL OR l.expires_at > now())
  AND NOT u.disabled;

-- What a visitor may see of each photo: no name, no camera, no place. Photos in the trash, still being
-- processed, or hidden under another one (a RAW under its JPEG) are left out.
-- name: ListPublicPhotos :many
SELECT p.id, p.mime, p.width, p.height, p.taken_at, p.duration, p.preview_version
FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
WHERE ap.album_id = @album_id
  AND p.deleted_at IS NULL AND p.status = 'ready'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id)
ORDER BY p.sort_at DESC, p.id DESC;

-- name: GetPublicPhoto :one
SELECT p.* FROM album_photos ap JOIN photos p ON p.id = ap.photo_id
WHERE ap.album_id = @album_id AND p.id = @photo_id
  AND p.deleted_at IS NULL AND p.status = 'ready'
  AND NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id);
