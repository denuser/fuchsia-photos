-- name: RecordActivity :exec
INSERT INTO activity.events (
    action, outcome, http_status, account_id, session_id, browser, os,
    media_type, size_bytes, media_id, generation
) VALUES (
    @action, @outcome, @http_status, sqlc.narg(account_id), sqlc.narg(session_id), @browser, @os,
    sqlc.narg(media_type), sqlc.narg(size_bytes), sqlc.narg(media_id),
    CASE WHEN sqlc.narg(media_id)::bigint IS NOT NULL THEN (SELECT generation FROM activity.reset_state) END
);

-- name: ListActivityEvents :many
SELECT id, occurred_at, action, outcome, http_status, account_id, session_id, browser, os,
       media_type, size_bytes, media_id
FROM activity.events
WHERE id < @before::bigint
ORDER BY id DESC
LIMIT @row_limit::int;

-- name: SummarizeActivity :many
SELECT to_char(occurred_at AT TIME ZONE 'Europe/Berlin', 'YYYY-MM-DD')::text AS day,
       count(*) FILTER (WHERE action = 'login' AND outcome = 'succeeded') AS logins,
       count(*) FILTER (WHERE action = 'login' AND outcome = 'rejected') AS rejected_logins,
       count(*) FILTER (WHERE action = 'login' AND outcome = 'invalid_request') AS invalid_logins,
       count(*) FILTER (WHERE action = 'login' AND outcome = 'error') AS login_errors,
       count(*) FILTER (WHERE action = 'upload' AND outcome = 'accepted') AS new_files,
       count(*) FILTER (WHERE action = 'upload' AND outcome = 'duplicate') AS duplicates,
       count(*) FILTER (WHERE action = 'upload' AND outcome = 'restored') AS restored,
       count(*) FILTER (WHERE action = 'upload' AND outcome = 'rejected') AS rejected_uploads,
       count(*) FILTER (WHERE action = 'upload' AND http_status >= 500) AS upload_errors,
       count(DISTINCT session_id) FILTER (
           WHERE action = 'upload' AND outcome IN ('accepted', 'restored')
       ) AS sessions_with_uploads
FROM activity.events
WHERE occurred_at >= now() - interval '30 days'
GROUP BY 1
ORDER BY 1 DESC;
