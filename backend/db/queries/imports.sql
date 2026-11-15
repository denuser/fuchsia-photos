-- name: CreateImportRun :one
INSERT INTO import_runs (owner_id, started_by, folder, kind)
VALUES (@owner_id, @started_by, @folder, @kind)
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
WHERE id = @id AND status IN ('listing', 'importing');

-- name: StopImportRun :execrows
UPDATE import_runs
SET status = 'stopped', finished_at = now()
WHERE id = @id AND status IN ('listing', 'importing');

-- A part of a Takeout export that was read to its end. Its entries are counted
-- one by one as they are recorded.
-- name: FinishImportPart :exec
DELETE FROM import_files WHERE run_id = @run_id AND path = @path;

-- The paths of the entries already recorded from one part or, with an empty
-- source, from unpacked folders.
-- name: ListTakeoutEntryPaths :many
SELECT export, path FROM takeout_entries WHERE run_id = @run_id AND source = @source;

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
