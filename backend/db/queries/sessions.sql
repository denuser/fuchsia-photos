-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, activity_id) VALUES (@token_hash, @user_id, @expires_at, sqlc.narg(activity_id));

-- name: GetSessionUser :one
SELECT sqlc.embed(u), s.activity_id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash AND s.expires_at > now() AND NOT u.disabled;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = @token_hash;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = @user_id;

-- name: DeleteUserSessionsExcept :exec
DELETE FROM sessions WHERE user_id = @user_id AND token_hash <> @token_hash;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
