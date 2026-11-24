-- name: GetUserByIdentity :one
SELECT sqlc.embed(u) FROM user_identities i JOIN users u ON u.id = i.user_id
WHERE i.issuer = @issuer AND i.subject = @subject;

-- name: CreateIdentity :exec
INSERT INTO user_identities (user_id, issuer, subject, email) VALUES (@user_id, @issuer, @subject, @email);

-- name: SetIdentityEmail :exec
UPDATE user_identities SET email = @email WHERE issuer = @issuer AND subject = @subject;

-- name: ListUserIdentities :many
SELECT * FROM user_identities WHERE user_id = @user_id ORDER BY created_at;

-- name: DeleteUserIdentity :execrows
DELETE FROM user_identities WHERE id = @id AND user_id = @user_id;

-- name: GetSignInSettings :one
SELECT * FROM sign_in_settings;

-- name: UpdateSignInSettings :one
UPDATE sign_in_settings
SET create_users = @create_users, allowed = @allowed, password_admins_only = @password_admins_only
RETURNING *;

-- name: CreateIdentityUser :one
INSERT INTO users (login, password_hash, role, email) VALUES (@login, '', 'user', @email) RETURNING *;
