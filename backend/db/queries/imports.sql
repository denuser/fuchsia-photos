-- name: CreateImportRun :one
INSERT INTO import_runs (owner_id, started_by, folder, kind, album_trash)
VALUES (@owner_id, @started_by, @folder, @kind, @album_trash)
RETURNING *;

-- name: GetImportRun :one
SELECT * FROM import_runs WHERE id = @id;

-- name: ListImportRuns :many
SELECT * FROM import_runs ORDER BY id DESC LIMIT @max_rows;

-- Starts a listing over, after an attempt that did not finish.
-- name: ResetImportListing :exec
WITH removed AS (DELETE FROM import_files WHERE run_id = @id)
UPDATE import_runs SET found = 0, failed = 0 WHERE id = @id;

-- name: AddImportFiles :exec
WITH added AS (
    INSERT INTO import_files (run_id, path, size, mtime)
    SELECT @run_id::bigint, unnest(@paths::text[]), unnest(@sizes::bigint[]), unnest(@mtimes::timestamptz[])
    ON CONFLICT DO NOTHING
    RETURNING 1
)
-- A Takeout scan counts the files inside its parts as it reads them instead.
UPDATE import_runs
SET found = found + CASE WHEN kind = 'folder' THEN (SELECT count(*) FROM added) ELSE 0 END
WHERE id = @run_id;

-- A file that could not even be listed, such as one whose name is not valid text.
-- name: AddImportProblem :exec
WITH added AS (
    INSERT INTO import_files (run_id, path, size, mtime, state, error)
    VALUES (@run_id, @path, 0, now(), 'failed', @error)
    ON CONFLICT DO NOTHING
)
UPDATE import_runs SET found = found + 1, failed = failed + 1 WHERE id = @run_id;

-- name: StartImporting :exec
UPDATE import_runs SET status = 'importing', settle_at = @settle_at WHERE id = @id AND status = 'listing';

-- name: ListPendingImportFiles :many
SELECT path, size, mtime
FROM import_files
WHERE run_id = @run_id AND state = 'pending'
ORDER BY path
LIMIT @max_rows;

-- A crash between ingesting a file and this query counts that file as a duplicate
-- when the scan resumes. The photo itself is safe.
-- name: FinishImportFile :exec
WITH removed AS (DELETE FROM import_files f WHERE f.run_id = @run_id AND f.path = @path)
UPDATE import_runs
SET added = added + (@outcome::text = 'added')::int,
    duplicates = duplicates + (@outcome::text = 'duplicate')::int,
    in_trash = in_trash + (@outcome::text = 'in_trash')::int,
    purged = purged + (@outcome::text = 'purged')::int,
    unsupported = unsupported + (@outcome::text = 'unsupported')::int
WHERE id = @run_id;

-- name: MarkImportFileChanging :exec
WITH marked AS (UPDATE import_files f SET state = 'changing' WHERE f.run_id = @run_id AND f.path = @path)
UPDATE import_runs SET changing = changing + 1 WHERE id = @run_id;

-- name: MarkImportFileFailed :exec
WITH marked AS (UPDATE import_files f SET state = 'failed', error = @error WHERE f.run_id = @run_id AND f.path = @path)
UPDATE import_runs SET failed = failed + 1 WHERE id = @run_id;

-- Files of a Takeout export that failed are listed by the part they are in.
-- name: ListImportProblems :many
SELECT p.path, p.state, p.error
FROM (
    SELECT f.path, f.state, f.error
    FROM import_files f
    WHERE f.run_id = @run_id AND f.state <> 'pending'
    UNION ALL
    SELECT ltrim(CASE WHEN e.source = '' THEN e.export || '/' || e.path ELSE e.source || '/' || e.path END, '/'), 'failed', e.error
    FROM takeout_entries e
    WHERE e.run_id = @run_id AND e.outcome = 'failed'
) p
ORDER BY p.path
LIMIT @max_rows;

-- name: DeletePendingImportFiles :exec
DELETE FROM import_files WHERE run_id = @run_id AND state = 'pending';

-- name: FinishImportRun :exec
UPDATE import_runs SET status = 'finished', finished_at = now() WHERE id = @id AND status = 'importing';

-- name: FailImportRun :exec
UPDATE import_runs
SET status = 'failed', error = @error, finished_at = now()
WHERE id = @id AND status IN ('listing', 'importing', 'matching');

-- name: StopImportRun :execrows
UPDATE import_runs
SET status = 'stopped', finished_at = now()
WHERE id = @id AND status IN ('listing', 'importing', 'matching');

-- A part of a Takeout export that was read to its end. Its entries are counted
-- one by one as they are recorded.
-- name: FinishImportPart :exec
DELETE FROM import_files WHERE run_id = @run_id AND path = @path;

-- The paths of the entries of an export already recorded, from any of its
-- parts: a part downloaded twice adds nothing the second time.
-- name: ListTakeoutEntryPaths :many
SELECT path FROM takeout_entries WHERE run_id = @run_id AND export = @export;

-- Records an entry once. Reading it again after a restart counts nothing.
-- name: AddTakeoutEntry :exec
WITH added AS (
    INSERT INTO takeout_entries (run_id, export, path, source, outcome, photo_id, sidecar, error)
    VALUES (@run_id, @export, @path, @source, @outcome, @photo_id, @sidecar, @error)
    ON CONFLICT DO NOTHING
    RETURNING outcome
)
UPDATE import_runs
SET found = found + (SELECT count(*) FROM added),
    added = added + (SELECT count(*) FROM added WHERE outcome = 'added'),
    duplicates = duplicates + (SELECT count(*) FROM added WHERE outcome = 'duplicate'),
    in_trash = in_trash + (SELECT count(*) FROM added WHERE outcome = 'in_trash'),
    purged = purged + (SELECT count(*) FROM added WHERE outcome = 'purged'),
    unsupported = unsupported + (SELECT count(*) FROM added WHERE outcome = 'unsupported'),
    sidecars = sidecars + (SELECT count(*) FROM added WHERE outcome = 'sidecar'),
    failed = failed + (SELECT count(*) FROM added WHERE outcome = 'failed')
WHERE id = @run_id;

-- A Takeout scan matches its entries once every part is read.
-- name: StartMatching :exec
UPDATE import_runs SET status = 'matching' WHERE id = @id AND status = 'importing';

-- The entries the matching looks at. title is the file name a JSON file
-- describes, and photo tells the JSON of a photo from that of an album.
-- name: ListTakeoutEntriesForMatching :many
SELECT export, path, outcome, coalesce(sidecar ->> 'title', '')::text AS title, ((sidecar -> 'photoTakenTime') IS NOT NULL)::boolean AS photo
FROM takeout_entries
WHERE run_id = @run_id AND outcome <> 'failed';

-- name: SetTakeoutMatches :exec
UPDATE takeout_entries e
SET sidecar_path = NULLIF(m.sidecar_path, ''), original_path = NULLIF(m.original_path, '')
FROM (
    SELECT unnest(@exports::text[]) AS export, unnest(@paths::text[]) AS path,
           unnest(@sidecar_paths::text[]) AS sidecar_path, unnest(@original_paths::text[]) AS original_path
) m
WHERE e.run_id = @run_id AND e.export = m.export AND e.path = m.path;

-- name: FinishMatching :exec
UPDATE import_runs
SET status = 'finished', finished_at = now(),
    without_sidecar = @without_sidecar, unmatched_sidecars = @unmatched_sidecars, edited = @edited,
    albums = @albums
WHERE id = @id AND status = 'matching';

-- Keeps the JSON of each photo of a run, from the newest export that has it.
-- A photo's own JSON comes before its original's, which an edited copy takes.
-- name: SaveImportedMetadata :exec
INSERT INTO imported_metadata (photo_id, original_id, run_id, export, export_time, path, sidecar, taken_at, latitude, longitude)
SELECT DISTINCT ON (e.photo_id)
       e.photo_id, o.photo_id, e.run_id, e.export, x.export_time, s.path, s.sidecar,
       CASE WHEN s.sidecar -> 'photoTakenTime' ->> 'timestamp' ~ '^[0-9]{1,12}$'
                 AND (s.sidecar -> 'photoTakenTime' ->> 'timestamp')::bigint > 0
            THEN to_timestamp((s.sidecar -> 'photoTakenTime' ->> 'timestamp')::bigint) END,
       g.latitude, g.longitude
FROM takeout_entries e
JOIN (SELECT unnest(@exports::text[]) AS export, unnest(@export_times::timestamptz[]) AS export_time) x
    ON x.export = e.export
LEFT JOIN takeout_entries o ON o.run_id = e.run_id AND o.export = e.export AND o.path = e.original_path
JOIN takeout_entries s ON s.run_id = e.run_id AND s.export = e.export AND s.path = coalesce(e.sidecar_path, o.sidecar_path)
LEFT JOIN LATERAL (
    SELECT (s.sidecar -> 'geoData' ->> 'latitude')::double precision AS latitude,
           (s.sidecar -> 'geoData' ->> 'longitude')::double precision AS longitude
    WHERE jsonb_typeof(s.sidecar -> 'geoData' -> 'latitude') = 'number'
      AND jsonb_typeof(s.sidecar -> 'geoData' -> 'longitude') = 'number'
      AND ((s.sidecar -> 'geoData' ->> 'latitude')::double precision <> 0
           OR (s.sidecar -> 'geoData' ->> 'longitude')::double precision <> 0)
      AND abs((s.sidecar -> 'geoData' ->> 'latitude')::double precision) <= 90
      AND abs((s.sidecar -> 'geoData' ->> 'longitude')::double precision) <= 180
) g ON true
WHERE e.run_id = @run_id AND e.photo_id IS NOT NULL AND s.sidecar IS NOT NULL
  AND (e.original_path IS NULL OR o.photo_id IS NOT NULL)
ORDER BY e.photo_id, x.export_time DESC, (e.original_path IS NULL) DESC, e.export, e.path
ON CONFLICT (photo_id) DO UPDATE
SET original_id = excluded.original_id, run_id = excluded.run_id, export = excluded.export,
    export_time = excluded.export_time, path = excluded.path, sidecar = excluded.sidecar,
    taken_at = excluded.taken_at, latitude = excluded.latitude, longitude = excluded.longitude
WHERE excluded.export_time >= imported_metadata.export_time;

-- name: ListImportedPhotosOfRun :many
SELECT photo_id FROM imported_metadata WHERE run_id = @run_id ORDER BY photo_id;

-- The JSON files that may describe albums: those of no photo, with a title
-- that is a string. The matching keeps the ones right under the product folder.
-- name: ListTakeoutAlbumFiles :many
SELECT export, path, (sidecar ->> 'title')::text AS title
FROM takeout_entries
WHERE run_id = @run_id AND outcome = 'sidecar'
  AND (sidecar -> 'photoTakenTime') IS NULL
  AND jsonb_typeof(sidecar -> 'title') = 'string'
ORDER BY export, path;

-- The owner's album with this name, the oldest if there are several.
-- name: FindAlbumByName :one
SELECT id FROM albums WHERE owner_id = @owner_id AND name = @name ORDER BY id LIMIT 1;

-- The photos of the owner in the folders of a run's albums, files right in the
-- folder only. Purged photos stay out; trashed tells those in the trash.
-- name: ListTakeoutAlbumPhotos :many
SELECT DISTINCT f.album_id::bigint AS album_id, p.id AS photo_id,
       (p.deleted_at IS NOT NULL)::boolean AS trashed, (NOT EXISTS (SELECT 1 FROM photo_links l WHERE l.hidden_id = p.id))::boolean AS shown
FROM (
    SELECT unnest(@exports::text[]) AS export, unnest(@folders::text[]) AS folder, unnest(@album_ids::bigint[]) AS album_id
) f
JOIN takeout_entries e ON e.run_id = @run_id AND e.export = f.export
    AND starts_with(e.path, f.folder) AND strpos(substr(e.path, length(f.folder) + 1), '/') = 0
JOIN photos p ON p.id = e.photo_id
WHERE e.outcome IN ('added', 'duplicate', 'in_trash')
  AND p.owner_id = @owner_id
  AND p.purged_at IS NULL
ORDER BY 1, 2;

-- name: AddPhotosToAlbums :exec
INSERT INTO album_photos (album_id, photo_id)
SELECT unnest(@album_ids::bigint[]), unnest(@photo_ids::bigint[])
ON CONFLICT DO NOTHING;

-- name: CountRestoredPhotos :exec
UPDATE import_runs SET restored = restored + @restored WHERE id = @id;

-- Gives the photos whose newest JSON came from this run the description in
-- it, if they have none. An import never replaces a description, so the
-- owner's edits stay.
-- name: FillImportedDescriptions :exec
UPDATE photos p
SET description = clean_description(m.sidecar ->> 'description'), updated_at = now()
FROM imported_metadata m
WHERE m.run_id = @run_id
  AND m.photo_id = p.id
  AND p.description = ''
  AND p.purged_at IS NULL
  AND jsonb_typeof(m.sidecar -> 'description') = 'string'
  AND clean_description(m.sidecar ->> 'description') <> '';

-- Marks as the owner's favorites the photos whose newest JSON came from this
-- run with "favorited": true, on the photo shown at the top of their chain.
-- Importing again marks them again, as it adds photos to albums again.
-- name: FillImportedFavorites :exec
INSERT INTO photo_favorites (user_id, photo_id)
SELECT p.owner_id, t.id
FROM imported_metadata m
JOIN photos p ON p.id = m.photo_id
JOIN photos t ON t.id = photo_top(m.photo_id)
WHERE m.run_id = @run_id
  AND m.sidecar -> 'favorited' = 'true'::jsonb
  AND t.purged_at IS NULL
ON CONFLICT DO NOTHING;
