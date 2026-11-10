-- +goose Up

-- Folders of a user's WebDAV tree linked to the user's albums. A photo uploaded
-- into a linked folder, or into a folder inside it, is added to the album of the
-- nearest linked folder. A link names a path, so it may be made before a sync
-- client creates the folder, and it stays when the folder is deleted or moved.
CREATE TABLE webdav_albums (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    path       text NOT NULL,
    album_id   bigint NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, path)
);

CREATE INDEX webdav_albums_album_idx ON webdav_albums (album_id);

-- +goose Down

DROP TABLE webdav_albums;
