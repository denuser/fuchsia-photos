-- name: CreateLibraryFolder :one
INSERT INTO library_folders (path, owner_id, scan_interval)
VALUES (@path, @owner_id, @scan_interval)
RETURNING *;

-- name: GetLibraryFolder :one
SELECT * FROM library_folders WHERE id = @id;

-- name: ListLibraryFolders :many
SELECT * FROM library_folders ORDER BY path;

-- name: FinishLibraryScan :exec
UPDATE library_folders
SET last_scan_at = now(), last_result = sqlc.narg(last_result), last_error = sqlc.narg(last_error)
WHERE id = @id;

-- Folders whose interval since the last scan has passed.
-- name: ListLibraryFoldersDue :many
SELECT * FROM library_folders
WHERE last_scan_at IS NULL OR last_scan_at + make_interval(secs => scan_interval) <= now()
ORDER BY id;

-- name: SetLibraryFolderInterval :one
UPDATE library_folders SET scan_interval = @scan_interval WHERE id = @id RETURNING *;

-- name: DeleteLibraryFolder :exec
DELETE FROM library_folders WHERE id = @id;

-- Photos that are read from the folder and not deleted permanently.
-- name: CountLibraryFolderPhotos :one
SELECT count(*)
FROM photo_files f
JOIN photos p ON p.id = f.photo_id
WHERE f.folder_id = @folder_id AND f.state = 'source' AND p.purged_at IS NULL;

-- What each folder holds, for the administration page: photos read from it, files
-- that are away, and files skipped as duplicates.
-- name: SummarizeLibraryFolders :many
SELECT f.folder_id,
       count(*) FILTER (WHERE f.state = 'source')::bigint AS photos,
       count(*) FILTER (WHERE f.state = 'source' AND f.missing_since IS NOT NULL)::bigint AS missing,
       count(*) FILTER (WHERE f.state = 'duplicate')::bigint AS duplicates
FROM photo_files f
GROUP BY f.folder_id;

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

-- The file of a photo changed in place: the photo keeps its id, its albums, tags and
-- shares, and takes the new hash. Processing makes the rest again, so the photo
-- keeps what it shows until then.
-- name: ReplacePhotoFile :one
UPDATE photos
SET sha256 = @sha256,
    mime = @mime,
    size = @size,
    error = NULL,
    has_motion = false,
    video_copy = NULL,
    updated_at = now()
WHERE id = @id AND purged_at IS NULL
RETURNING *;

-- name: ForgetPhotoEmbedding :exec
WITH embedding_removed AS (DELETE FROM photo_embeddings e WHERE e.photo_id = @id)
DELETE FROM photo_embedding_failures f WHERE f.photo_id = @id;

-- name: UpdatePhotoFileStat :exec
UPDATE photo_files SET size = @size, mtime = @mtime WHERE id = @id;
