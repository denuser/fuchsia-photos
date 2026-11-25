-- name: CreateLibraryFolder :one
INSERT INTO library_folders (path, owner_id, scan_interval)
VALUES (@path, @owner_id, @scan_interval)
RETURNING *;

-- name: GetLibraryFolder :one
SELECT * FROM library_folders WHERE id = @id;

-- name: ListLibraryFolders :many
SELECT * FROM library_folders ORDER BY path;

-- name: FinishLibraryScan :exec
UPDATE library_folders SET last_scan_at = now() WHERE id = @id;

-- name: ListPhotoFiles :many
SELECT * FROM photo_files WHERE folder_id = @folder_id ORDER BY path;

-- name: GetPhotoFileOfPhoto :one
SELECT * FROM photo_files WHERE photo_id = @photo_id AND state = 'source';

-- name: InsertPhotoFile :one
INSERT INTO photo_files (folder_id, path, photo_id, state, size, mtime)
VALUES (@folder_id, @path, @photo_id, @state, @size, @mtime)
RETURNING *;

-- A file that was away is back, or one that is still there is seen again.
-- name: SeePhotoFile :exec
UPDATE photo_files
SET missed_scans = 0, missing_since = NULL, trashed_by_scan = false
WHERE id = @id;

-- A moved or renamed file: the row takes the new path.
-- name: MovePhotoFile :exec
UPDATE photo_files
SET path = @path, size = @size, mtime = @mtime,
    missed_scans = 0, missing_since = NULL, trashed_by_scan = false
WHERE id = @id;

-- Counts a scan that did not find the files. The row of a file that is back, and
-- one already marked missing, are left to SeePhotoFile and to MarkPhotoFilesMissing.
-- name: MissPhotoFiles :many
UPDATE photo_files
SET missed_scans = missed_scans + 1
WHERE id = ANY (@ids::bigint[])
RETURNING *;

-- name: MarkPhotoFilesMissing :exec
UPDATE photo_files
SET missing_since = now(), trashed_by_scan = @trashed_by_scan
WHERE id = ANY (@ids::bigint[]) AND missing_since IS NULL;

-- name: DeletePhotoFiles :exec
DELETE FROM photo_files WHERE id = ANY (@ids::bigint[]);

-- name: SetPhotoSourcePath :exec
UPDATE photos SET source_path = @source_path, updated_at = now() WHERE id = @id;
