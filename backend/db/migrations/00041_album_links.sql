-- +goose Up

-- Links that open an album without an account. The token is the secret in the address, so it is kept
-- as it is: the owner sees the link again later. A link with a password asks for it before it shows
-- anything, and one with an expiry stops working at that time.
CREATE TABLE album_links (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    album_id      bigint NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    token         text NOT NULL UNIQUE,
    password_hash text NOT NULL DEFAULT '',
    expires_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX album_links_album_id_idx ON album_links (album_id);

-- +goose Down

DROP TABLE album_links;
