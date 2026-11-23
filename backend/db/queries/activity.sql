-- name: RecordActivity :exec
INSERT INTO activity.events (
    action, outcome, http_status, account_id, session_id, browser, os,
    media_type, size_bytes, media_id, generation
) VALUES (
    @action, @outcome, @http_status, sqlc.narg(account_id), sqlc.narg(session_id), @browser, @os,
    sqlc.narg(media_type), sqlc.narg(size_bytes), sqlc.narg(media_id),
    CASE WHEN sqlc.narg(media_id)::bigint IS NOT NULL THEN (SELECT generation FROM activity.reset_state) END
);
