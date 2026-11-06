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

-- name: SetUserSharedIncludesAlbums :one
UPDATE users SET shared_includes_albums = @shared_includes_albums WHERE id = @id RETURNING *;

-- The logins any user can choose from when sharing. Disabled users cannot log in, so they are left out.
-- name: ListActiveUserLogins :many
SELECT id, login FROM users WHERE NOT disabled ORDER BY lower(login);

-- name: SetUserViewerShowsOriginals :one
UPDATE users SET viewer_shows_originals = @viewer_shows_originals WHERE id = @id RETURNING *;

-- name: SetUserViewerHiddenFields :one
UPDATE users SET viewer_hidden_fields = @viewer_hidden_fields WHERE id = @id RETURNING *;

-- name: SetUserNavigationHiddenOnPhones :one
UPDATE users SET navigation_hidden_on_phones = @navigation_hidden_on_phones WHERE id = @id RETURNING *;

-- name: SetUserGridSquares :one
UPDATE users SET grid_squares = @grid_squares WHERE id = @id RETURNING *;

-- name: SetUserTimelineIncludesShared :one
UPDATE users SET timeline_includes_shared = @timeline_includes_shared WHERE id = @id RETURNING *;
