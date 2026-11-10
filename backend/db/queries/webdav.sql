-- name: CreateAppPassword :one
INSERT INTO app_passwords (user_id, name, token_hash) VALUES (@user_id, @name, @token_hash) RETURNING *;

-- name: ListAppPasswords :many
SELECT * FROM app_passwords WHERE user_id = @user_id ORDER BY created_at, id;

-- name: DeleteAppPassword :execrows
DELETE FROM app_passwords WHERE id = @id AND user_id = @user_id;

-- name: CountAppPasswords :one
SELECT count(*) FROM app_passwords WHERE user_id = @user_id;

-- name: GetAppPasswordUser :one
SELECT sqlc.embed(u), p.id AS app_password_id, p.last_used_at
FROM app_passwords p
JOIN users u ON u.id = p.user_id
WHERE p.token_hash = @token_hash AND NOT u.disabled;

-- name: TouchAppPassword :exec
UPDATE app_passwords SET last_used_at = now() WHERE id = @id;

-- name: GetWebDAVEntry :one
SELECT * FROM webdav_entries WHERE user_id = @user_id AND path = @path;

-- name: ListWebDAVChildren :many
SELECT * FROM webdav_entries WHERE user_id = @user_id AND parent = @parent ORDER BY path;

-- name: CreateWebDAVDir :exec
INSERT INTO webdav_entries (user_id, path, parent, is_dir) VALUES (@user_id, @path, @parent, true);

-- name: PutWebDAVFile :exec
-- Replaces a file at the same path, as a PUT does.
INSERT INTO webdav_entries (user_id, path, parent, is_dir, size, modified_at, photo_id)
VALUES (@user_id, @path, @parent, false, @size, @modified_at, @photo_id)
ON CONFLICT (user_id, path) DO UPDATE
SET size = excluded.size, modified_at = excluded.modified_at, photo_id = excluded.photo_id
WHERE NOT webdav_entries.is_dir;

-- name: DeleteWebDAVTree :execrows
-- Deletes an entry and, for a folder, everything in it. Photos stay.
DELETE FROM webdav_entries
WHERE user_id = @user_id AND (path = @path OR starts_with(path, @path::text || '/'));

-- name: MoveWebDAVTree :execrows
-- Moves an entry and everything in it from old_path to new_path.
UPDATE webdav_entries
SET path = @new_path::text || substr(path, length(@old_path::text) + 1),
    parent = CASE WHEN path = @old_path::text THEN @new_parent::text
                  ELSE @new_path::text || substr(parent, length(@old_path::text) + 1) END
WHERE user_id = @user_id AND (path = @old_path::text OR starts_with(path, @old_path::text || '/'));

-- name: GetWebDAVPhoto :one
-- The photo behind a file, if its original is still there.
SELECT * FROM photos WHERE id = @id AND owner_id = @owner_id AND purged_at IS NULL;

-- name: ListWebDAVFolders :many
-- Folders to offer when linking one to an album.
SELECT path FROM webdav_entries WHERE user_id = @user_id AND is_dir ORDER BY path LIMIT 1000;

-- name: ListWebDAVAlbums :many
SELECT l.id, l.path, l.album_id, a.name AS album_name
FROM webdav_albums l
JOIN albums a ON a.id = l.album_id
WHERE l.user_id = @user_id
ORDER BY l.path;

-- name: CountWebDAVAlbums :one
SELECT count(*) FROM webdav_albums WHERE user_id = @user_id;

-- name: CreateWebDAVAlbum :one
-- Links a folder to an album of the same user. Returns no row if the album is
-- not the user's, and fails on a unique violation if the folder is linked.
WITH album AS (
    SELECT a.id, a.owner_id, a.name FROM albums a WHERE a.id = @album_id AND a.owner_id = @user_id
), link AS (
    INSERT INTO webdav_albums (user_id, path, album_id)
    SELECT album.owner_id, @path, album.id FROM album
    RETURNING webdav_albums.id, webdav_albums.path, webdav_albums.album_id
)
SELECT link.id, link.path, link.album_id, album.name AS album_name
FROM link JOIN album ON album.id = link.album_id;

-- name: DeleteWebDAVAlbum :execrows
DELETE FROM webdav_albums WHERE id = @id AND user_id = @user_id;

-- name: FindWebDAVAlbum :one
-- The album of the nearest linked folder that holds a file.
SELECT album_id
FROM webdav_albums
WHERE user_id = @user_id
  AND (path = '/' OR starts_with(@file_path::text, path || '/'))
ORDER BY length(path) DESC
LIMIT 1;
