-- +goose Up

-- Passwords for WebDAV clients, one per device. Each is a random token the
-- server generates and shows once, so only its SHA-256 is stored: Basic Auth
-- sends it with every request, too often for argon2id.
CREATE TABLE app_passwords (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE INDEX app_passwords_user_idx ON app_passwords (user_id);

-- The folder tree a user's WebDAV clients see. A file is a record of an upload
-- that points to the photo it became, so that sync clients find what they have
-- already sent. Deleting an entry leaves the photo in the library. Paths start
-- with a slash, and the root folder "/" is implied. parent is the path of the
-- folder that holds the entry.
CREATE TABLE webdav_entries (
    user_id     bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    path        text NOT NULL,
    parent      text NOT NULL,
    is_dir      boolean NOT NULL,
    size        bigint NOT NULL DEFAULT 0,
    modified_at timestamptz NOT NULL DEFAULT now(),
    photo_id    bigint REFERENCES photos (id) ON DELETE SET NULL,
    PRIMARY KEY (user_id, path)
);

CREATE INDEX webdav_entries_parent_idx ON webdav_entries (user_id, parent);

-- +goose Down

DROP TABLE webdav_entries;
DROP TABLE app_passwords;
