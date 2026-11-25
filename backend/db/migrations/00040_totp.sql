-- +goose Up

-- Two-factor sign-in with a time-based code. The secret is encrypted by the application under
-- TOTP_KEY, and sits here before totp_enabled is true while the user sets it up. totp_last_step is
-- the last time step a code was accepted for, so that a code works once.
ALTER TABLE users
    ADD COLUMN totp_secret    bytea,
    ADD COLUMN totp_enabled   boolean NOT NULL DEFAULT false,
    ADD COLUMN totp_last_step bigint NOT NULL DEFAULT 0;

-- One-time codes for when the authenticator is lost. Only their hashes are kept.
CREATE TABLE totp_recovery_codes (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id   bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash bytea NOT NULL,
    used_at   timestamptz,
    UNIQUE (user_id, code_hash)
);

-- +goose Down

DROP TABLE totp_recovery_codes;
ALTER TABLE users DROP COLUMN totp_last_step, DROP COLUMN totp_enabled, DROP COLUMN totp_secret;
