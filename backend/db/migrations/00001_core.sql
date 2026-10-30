-- +goose Up
CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    login         text NOT NULL,
    password_hash text NOT NULL,
    role          text NOT NULL CHECK (role IN ('admin', 'user')),
    disabled      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_login_key ON users (lower(login));

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE photos (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id   bigint NOT NULL REFERENCES users (id),
    sha256     bytea NOT NULL CHECK (octet_length(sha256) = 32),
    mime       text NOT NULL,
    size       bigint NOT NULL CHECK (size >= 0),
    width      integer,
    height     integer,
    taken_at   timestamptz,
    exif       jsonb,
    status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed')),
    error      text,
    visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'all')),
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (owner_id, sha256)
);

CREATE INDEX photos_owner_taken_at_idx ON photos (owner_id, taken_at DESC, id DESC);

-- +goose Down
DROP TABLE photos;
DROP TABLE sessions;
DROP TABLE users;
