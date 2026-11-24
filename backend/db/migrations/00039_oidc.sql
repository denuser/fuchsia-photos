-- +goose Up

-- An account created by a sign-in through a provider has no password: its hash is empty,
-- which CheckPassword never accepts.
ALTER TABLE users ADD COLUMN email text NOT NULL DEFAULT '';

-- Who a user is at a provider. The key is the issuer and the subject, never the email.
CREATE TABLE user_identities (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    issuer     text NOT NULL,
    subject    text NOT NULL,
    email      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);

CREATE INDEX user_identities_user_id_idx ON user_identities (user_id);

-- How people may sign in, edited on the administration page. The table holds exactly one row.
-- create_users lets a first sign-in through a provider create an account; allowed narrows it
-- to the addresses and domains listed, and with no list anyone the provider admits is created.
-- password_admins_only closes the password form to everyone but administrators.
CREATE TABLE sign_in_settings (
    id                   smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    create_users         boolean NOT NULL DEFAULT false,
    allowed              text[] NOT NULL DEFAULT '{}',
    password_admins_only boolean NOT NULL DEFAULT false
);

INSERT INTO sign_in_settings DEFAULT VALUES;

-- +goose Down

DROP TABLE sign_in_settings;
DROP TABLE user_identities;
ALTER TABLE users DROP COLUMN email;
