-- name: StartTOTPSetup :execrows
-- Keeps a secret for the user to confirm with a code; it replaces an earlier unconfirmed one.
UPDATE users SET totp_secret = @secret, totp_last_step = 0 WHERE id = @id AND NOT totp_enabled;

-- name: EnableTOTP :execrows
UPDATE users SET totp_enabled = true, totp_last_step = @step
WHERE id = @id AND NOT totp_enabled AND totp_secret IS NOT NULL;

-- name: ClaimTOTPStep :execrows
-- Accepts a time step once: a second claim of the same step, or of an earlier one, changes nothing.
UPDATE users SET totp_last_step = @step WHERE id = @id AND totp_last_step < @step;

-- name: ClearTOTP :exec
UPDATE users SET totp_secret = NULL, totp_enabled = false, totp_last_step = 0 WHERE id = @id;

-- name: DeleteRecoveryCodes :exec
DELETE FROM totp_recovery_codes WHERE user_id = @user_id;

-- name: InsertRecoveryCodes :exec
INSERT INTO totp_recovery_codes (user_id, code_hash)
SELECT @user_id, unnest(@hashes::bytea[]);

-- name: UseRecoveryCode :execrows
UPDATE totp_recovery_codes SET used_at = now()
WHERE user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: CountRecoveryCodes :one
SELECT count(*) FROM totp_recovery_codes WHERE user_id = @user_id AND used_at IS NULL;

-- name: ListTOTPUsers :many
SELECT id, login, totp_secret FROM users WHERE totp_enabled;
