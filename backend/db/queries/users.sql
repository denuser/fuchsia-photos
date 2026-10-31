-- name: CreateUser :one
INSERT INTO users (login, password_hash, role) VALUES (@login, @password_hash, @role) RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: GetUserByLogin :one
SELECT * FROM users WHERE lower(login) = lower(@login);

-- name: ListUsers :many
SELECT * FROM users ORDER BY lower(login);

-- name: SetUserRole :exec
UPDATE users SET role = @role WHERE id = @id;

-- name: SetUserDisabled :exec
UPDATE users SET disabled = @disabled WHERE id = @id;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = @password_hash WHERE id = @id;
